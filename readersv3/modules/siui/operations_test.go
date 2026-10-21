package siui

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

type operationBackend struct {
	t        *testing.T
	post     atomic.Int32
	fail     bool
	check    func(string, []byte)
	download []byte
}

func (b *operationBackend) Validate(context.Context, string, []byte) error { return nil }
func (b *operationBackend) Do(_ context.Context, method, address string, h http.Header, data []byte) (int, http.Header, []byte, error) {
	if strings.Contains(address, "/OCSP/") {
		return 200, http.Header{"Oscp_response": []string{"test-ocsp-session"}}, nil, nil
	}
	if h.Get("SessionID") != "test-ocsp-session" {
		b.t.Error("missing fresh OCSP session")
	}
	if method == "GET" {
		return 200, http.Header{}, b.download, nil
	}
	b.post.Add(1)
	if b.check != nil {
		b.check(address, data)
	}
	if b.fail {
		return 0, nil, nil, errors.New("connection lost")
	}
	root, err := parseXML(data)
	if err != nil {
		b.t.Fatal(err)
	}
	request := root.child("Body").Children[0]
	body := `<s:Envelope xmlns:s="` + soapNS + `"><s:Body><m:` + request.XMLName.Local + `Response xmlns:m="` + request.XMLName.Space + `"><` + request.XMLName.Local + `Return>0</` + request.XMLName.Local + `Return></m:` + request.XMLName.Local + `Response></s:Body></s:Envelope>`
	return 200, http.Header{}, []byte(body), nil
}
func sampleParameters(op Operation) map[string]string {
	args := map[string]string{}
	for _, p := range op.Parameters {
		v := "test & <value>"
		switch p.Type {
		case "dateTime":
			v = "2026-10-05T00:00:00+03:00"
		case "boolean":
			v = "false"
		case "int":
			v = "0"
		}
		if strings.HasSuffix(strings.ToLower(p.Name), "xml") {
			v = testXML
		}
		args[p.Name] = v
	}
	if op.Name == "validateReport" {
		args["reportType"] = "PARA"
		args["requestType"] = "RQ_PARA_SRV"
	}
	return args
}
func TestEveryRequestedSOAPOperation(t *testing.T) {
	if len(operations()) != 40 {
		t.Fatal("expected 40 SOAP operations")
	}
	for _, op := range operations() {
		t.Run(op.Name, func(t *testing.T) {
			args := sampleParameters(op)
			b := &operationBackend{t: t}
			b.check = func(address string, data []byte) {
				expected := "https://www.siui.ro"
				if op.Service == "EPrescriptionWS" {
					expected = "https://sipe.siui.ro"
				}
				if address != expected+"/svapntws/services/"+op.Service {
					t.Errorf("wrong endpoint %s", address)
				}
				root, e := parseXML(data)
				if e != nil {
					t.Fatal(e)
				}
				r := root.child("Body").Children[0]
				if r.XMLName.Space != op.Namespace || r.XMLName.Local != op.Name || len(r.Children) != len(op.Parameters) {
					t.Fatal("invalid operation wrapper")
				}
				for i, p := range op.Parameters {
					n := r.Children[i]
					if n.XMLName.Local != p.Name || n.Text != args[p.Name] {
						t.Fatal("parameter order or escaping mismatch", p.Name)
					}
					if op.Style == "rpc" && n.attr("type") != "xsd:"+p.Type {
						t.Fatal("missing typed RPC parameter")
					}
					if op.Style == "document" && n.XMLName.Space != op.Namespace {
						t.Fatal("literal parameter namespace mismatch")
					}
				}
			}
			c := client{cfg: settings{Username: "123_CAS-DJ", Licence: "test", BaseURL: "https://www.siui.ro", PEBaseURL: "https://sipe.siui.ro"}, backend: b}
			values, submitted, e := c.invokeOperation(context.Background(), op, args)
			if e != nil || len(values) != 1 || values[0] != "0" || submitted != op.Mutating {
				t.Fatalf("result=%v submitted=%v err=%v", values, submitted, e)
			}
		})
	}
	for _, name := range []string{"validateEnlisted", "validatePharmacyDrugs"} {
		for _, op := range operations() {
			if op.Name == name {
				t.Fatal("excluded operation exposed")
			}
		}
	}
}
func TestSOAPReferencesFaultsAndClinicalSelection(t *testing.T) {
	op, _ := findOperation("general", "getCatalogues")
	data := `<s:Envelope xmlns:s="` + soapNS + `"><s:Body><m:getCataloguesResponse xmlns:m="` + op.Namespace + `"><getCataloguesReturn href="#a"/></m:getCataloguesResponse><multiRef id="a"><item>/files/catalog.zip</item><item>123</item></multiRef></s:Body></s:Envelope>`
	values, e := operationValues(op, []byte(data))
	if e != nil || len(values) != 2 || values[1] != "123" {
		t.Fatal(values, e)
	}
	for _, bad := range []string{strings.Replace(data, `href="#a"`, `href="https://other/"`, 1), strings.Replace(data, `<item>/files/catalog.zip</item>`, `<item href="#a"/>`, 1)} {
		if _, e = operationValues(op, []byte(bad)); e == nil {
			t.Fatal("unsafe reference accepted")
		}
	}
	op, _ = findOperation("general", "validateReport")
	args := sampleParameters(op)
	args["reportType"] = "CLIN"
	args["requestType"] = "RQ_CLIN_SRV"
	args["reportXml"] = strings.Replace(testXML, `reportType="PARA"`, `reportType="CLIN"`, 1)
	if e = validateParameters(op, args); e != nil || operationSchema(op, args) != "ClinicSpecialityServicesValidateRequest.xsd" {
		t.Fatal(e)
	}
	args["requestType"] = "RQ_PARA_SRV"
	if validateParameters(op, args) == nil {
		t.Fatal("mismatched report accepted")
	}
}
func serviceTestModule(t *testing.T, b backend) (*Module, *testRuntime) {
	t.Helper()
	dir := t.TempDir()
	rt := &testRuntime{mux: http.NewServeMux(), guard: testGuard{}, path: filepath.Join(dir, "config.yaml"), settings: map[string]interface{}{"username": "123_CAS-DJ", "licence": "test", "certificate_thumbprint": strings.Repeat("A", 40), "database": filepath.Join(dir, "jobs.db")}}
	if e := os.WriteFile(rt.path, []byte("modules:\n  siui: {}\n"), 0600); e != nil {
		t.Fatal(e)
	}
	m := &Module{nativeSupported: true, factory: func(settings) backend { return b }}
	if e := m.Init(rt); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()
	select {
	case <-m.ready:
	case err := <-done:
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		if e := <-done; e != nil {
			t.Error(e)
		}
	})
	return m, rt
}
func TestJWTAndWSMServiceLifecycle(t *testing.T) {
	b := &operationBackend{t: t, fail: true}
	m, rt := serviceTestModule(t, b)
	call := func(method, path, body, auth string, secure bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", auth)
		if secure {
			r.TLS = &tls.ConnectionState{}
		}
		w := httptest.NewRecorder()
		rt.mux.ServeHTTP(w, r)
		return w
	}
	res := call("POST", "/api/siui/auth/token", "{}", "test-local-session", true)
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	var issued struct {
		Token string `json:"access_token"`
	}
	json.Unmarshal(res.Body.Bytes(), &issued)
	auth := "Bearer " + issued.Token
	if res = call("GET", "/api/siui/catalog", "", auth, true); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	for _, tc := range []struct {
		auth   string
		secure bool
	}{{auth, false}, {auth + "bad", true}, {"", true}} {
		if r := call("GET", "/api/siui/catalog", "", tc.auth, tc.secure); r.Code != 401 {
			t.Fatal("unauthorized call allowed", r.Code)
		}
	}
	now := time.Now().Unix()
	expired := m.signJWT(apiClaims{"operator", "siui-bridge", "siui-api", now - 1, now - 100, true, ""})
	if _, e := m.verifyJWT(expired); e == nil {
		t.Fatal("expired JWT accepted")
	}
	wrong := m.signJWT(apiClaims{"operator", "siui-bridge", "another-api", now + 100, now, true, ""})
	if _, e := m.verifyJWT(wrong); e == nil {
		t.Fatal("wrong audience accepted")
	}
	if r := call("POST", "/api/siui/auth/token", "{}", auth, true); r.Code != 403 {
		t.Fatal("JWT self-renewal allowed")
	}
	op, _ := findOperation("general", "sendReport")
	payload, _ := json.Marshal(map[string]any{"operation_id": "send-1", "parameters": sampleParameters(op)})
	res = call("POST", "/api/siui/general/sendReport", string(payload), auth, true)
	if res.Code != 202 {
		t.Fatal(res.Code, res.Body.String())
	}
	var reply struct {
		Call serviceCall `json:"call"`
	}
	json.Unmarshal(res.Body.Bytes(), &reply)
	id := reply.Call.ID
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		res = call("GET", "/api/siui/calls/"+id, "", auth, true)
		json.Unmarshal(res.Body.Bytes(), &reply)
		if reply.Call.Status == "unknown" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if reply.Call.Status != "unknown" {
		t.Fatalf("write failure must be unknown: %+v", reply.Call)
	}
	res = call("POST", "/api/siui/general/sendReport", string(payload), auth, true)
	if res.Code != 200 || b.post.Load() != 1 {
		t.Fatal("duplicate write submitted")
	}
	p := apibridge.Principal{Subject: "operator", TenantID: "clinic", ConnectionID: "wsm", Admin: false}
	result, e := apibridge.Invoke(context.Background(), rt.mux, p, apibridge.Request{Method: "POST", Path: "/api/siui/general/sendReport", Body: payload})
	if e != nil || result.Status != 403 {
		t.Fatal("WSM mutation permission", result, e)
	}
	result, e = apibridge.Invoke(context.Background(), rt.mux, p, apibridge.Request{Method: "GET", Path: "/api/siui/calls/" + id})
	if e != nil || result.Status != 404 {
		t.Fatal("owner isolation", result, e)
	}
	result, e = apibridge.Invoke(context.Background(), rt.mux, p, apibridge.Request{Method: "GET", Path: "/api/siui/catalog"})
	if e != nil || result.Status != 200 {
		t.Fatal("WSM catalog", result, e)
	}
	p.Admin = true
	result, e = apibridge.Invoke(context.Background(), rt.mux, p, apibridge.Request{Method: "POST", Path: "/api/siui/general/ocsp", Body: json.RawMessage(`{}`)})
	encoded, _ := json.Marshal(result.Body)
	if e != nil || result.Status != 200 || !strings.Contains(string(encoded), "test-ocsp-session") {
		t.Fatal("OCSP token not exposed to authorized admin", result, e)
	}
}
func TestCertificateProfilesAndCAS(t *testing.T) {
	if len(insuranceHouses()) != 45 {
		t.Fatal("CAS catalogue incomplete")
	}
	for _, cas := range []string{"CAS-DJ", "AOPSNAJ", "MLPTL"} {
		u, e := cnasUsername("7991962", cas)
		if e != nil || u != "7991962_"+cas {
			t.Fatal(u, e)
		}
	}
	if _, e := cnasUsername("7991962", "CAS-FAKE"); e == nil {
		t.Fatal("invalid CAS accepted")
	}
	s := settings{Store: "CurrentUser", Thumbprint: "A", BaseURL: "https://www.siui.ro", PEBaseURL: "https://sipe.siui.ro"}
	s.saveActiveProfile()
	s.Thumbprint = "B"
	s.BaseURL = "https://www.siui.ro:444"
	s.saveActiveProfile()
	if len(s.Profiles) != 2 || s.Profiles[0].BaseURL == s.Profiles[1].BaseURL {
		t.Fatal("profiles not isolated")
	}
	b, _ := json.Marshal(s.Profiles)
	var copy []certificateProfile
	if json.Unmarshal(b, &copy) != nil || len(copy) != 2 {
		t.Fatal("profiles serialization")
	}
	for _, bad := range []string{"http://www.siui.ro/file", "https://evil.test/file", "https://www.siui.ro@evil.test/file"} {
		if _, e := downloadAddress(bad, "https://www.siui.ro"); e == nil {
			t.Fatal("credential forwarding allowed", bad)
		}
	}
}

func TestDownloadChunksAndScope(t *testing.T) {
	payload := []byte(strings.Repeat("x", downloadChunk+19))
	b := &operationBackend{t: t, download: payload}
	m, rt := serviceTestModule(t, b)
	c := client{cfg: m.cfg, backend: b}
	file, err := m.downloadServiceFile(context.Background(), &c, "download-test", []string{"/files/nomenclature.zip", "262163"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(map[string]any{"file": file})
	_, err = m.db.Exec(`INSERT INTO siui_calls(id,owner,operation_id,operation,digest,status,result,created_at) VALUES('download-test','local:operator','dl','getCatalogues','hash','completed',?,'now')`, string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	var combined []byte
	for _, offset := range []string{"0", "262144"} {
		req := httptest.NewRequest("GET", "/api/siui/calls/download-test/file?offset="+offset, nil)
		req.Header.Set("Authorization", "test-local-session")
		w := httptest.NewRecorder()
		rt.mux.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var part struct {
			Data string `json:"data_base64"`
		}
		json.Unmarshal(w.Body.Bytes(), &part)
		data, err := base64.StdEncoding.DecodeString(part.Data)
		if err != nil {
			t.Fatal(err)
		}
		combined = append(combined, data...)
	}
	if !bytes.Equal(combined, payload) {
		t.Fatal("download chunk reconstruction failed")
	}
	if _, err = m.downloadServiceFile(context.Background(), &c, "bad", []string{"https://evil.test/secret", "12"}); err == nil {
		t.Fatal("external download accepted")
	}
}
func TestProfilesPersistThroughSettings(t *testing.T) {
	m, rt := serviceTestModule(t, &operationBackend{t: t})
	body := `{"fiscal_code":"7991962","cas_code":"CAS-DJ","base_url":"https://www.siui.ro:444","pe_base_url":"https://sipe.siui.ro","einvoice_base_url":"https://www.siui.ro:443","certificate_store":"CurrentUser","certificate_thumbprint":"` + strings.Repeat("A", 40) + `","allow_invalid_server_certificate_date":true,"allow_revoked_server_certificate":true}`
	req := httptest.NewRequest("PUT", "/api/siui/settings", strings.NewReader(body))
	req.Header.Set("Authorization", "test-local-session")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	rt.mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if m.cfg.Username != "7991962_CAS-DJ" || len(m.cfg.Profiles) != 1 || m.cfg.Profiles[0].BaseURL != "https://www.siui.ro:444" {
		t.Fatal("settings not applied")
	}
	saved, err := config.Load(rt.path)
	if err != nil {
		t.Fatal(err)
	}
	copyRT := *rt
	copyRT.mux = http.NewServeMux()
	copyRT.settings = saved.ModuleSettings("siui")
	restarted := &Module{}
	if err = restarted.Init(&copyRT); err != nil {
		t.Fatal(err)
	}
	if restarted.cfg.EInvoiceBaseURL != "https://www.siui.ro:443" || restarted.cfg.Profiles[0].EInvoiceBaseURL != "https://www.siui.ro:443" {
		t.Fatal("eFactura origin not persisted independently")
	}
	if restarted.cfg.Username != m.cfg.Username || len(restarted.cfg.Profiles) != 1 || !restarted.cfg.Profiles[0].AllowInvalidServerCertificateDate || !restarted.cfg.AllowRevokedServerCertificate || !restarted.cfg.Profiles[0].AllowRevokedServerCertificate {
		t.Fatal("profile restart mismatch")
	}
}
func TestCatalogMatchesBundledWSDL(t *testing.T) {
	for _, op := range operations() {
		b, err := assets.ReadFile("assets/wsdl/" + op.Service + ".wsdl")
		if err != nil {
			t.Fatal(err)
		}
		root, err := parseXML(b)
		if err != nil {
			t.Fatal(err)
		}
		if root.attr("targetNamespace") != op.Namespace {
			t.Fatal("namespace drift", op.Name)
		}
		var params []node
		if op.Style == "rpc" {
			for _, m := range root.Children {
				if m.XMLName.Local == "message" && m.attr("name") == op.Name+"Request" {
					params = m.Children
				}
			}
		} else {
			for _, schema := range root.child("types").Children {
				for _, el := range schema.Children {
					if el.attr("name") == op.Name {
						params = el.child("complexType").child("sequence").Children
					}
				}
			}
		}
		if len(params) != len(op.Parameters) {
			t.Fatal("WSDL parameter count drift", op.Name)
		}
		for i, p := range params {
			_, typ, _ := strings.Cut(p.attr("type"), ":")
			if p.attr("name") != op.Parameters[i].Name || typ != op.Parameters[i].Type {
				t.Fatal("WSDL parameter drift", op.Name)
			}
		}
	}
}

func TestWSMIssuedJWTRetainsTenantIsolation(t *testing.T) {
	_, rt := serviceTestModule(t, &operationBackend{t: t})
	p := apibridge.Principal{Subject: "operator", TenantID: "tenant-a", ConnectionID: "test", Admin: false}
	res, err := apibridge.Invoke(context.Background(), rt.mux, p, apibridge.Request{Method: "POST", Path: "/api/siui/auth/token", Body: json.RawMessage(`{}`)})
	if err != nil || res.Status != 200 {
		t.Fatal(res, err)
	}
	data, _ := json.Marshal(res.Body)
	var response struct {
		Token string `json:"access_token"`
	}
	json.Unmarshal(data, &response)
	req := httptest.NewRequest("POST", "/api/siui/general/ocsp", strings.NewReader(`{}`))
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+response.Token)
	w := httptest.NewRecorder()
	rt.mux.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("JWT escalated WSM role", w.Code)
	}
	parts := strings.Split(response.Token, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims apiClaims
	json.Unmarshal(raw, &claims)
	if claims.Owner != `wsm:"tenant-a":"operator"` {
		t.Fatal("lost tenant ownership")
	}
}
func TestServiceRestartNeverReplaysPendingCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.db")
	db, err := openJobs(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO siui_calls(id,owner,operation_id,operation,digest,status,created_at) VALUES('id','owner','op','sendReport','hash','running','now')`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = openJobs(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	call, err := readServiceCall(db, "owner", "id")
	if err != nil || call.Status != "unknown" {
		t.Fatal(call, err)
	}
}
