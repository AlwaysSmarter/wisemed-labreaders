package siui

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

const testXML = `<request xmlns="http://www.cnas.ro/siui/2.0" providerCode="TEST" providerName="Test" insuranceHouse="CAS-TEST" contractNo="TEST" contractType="PARA" reportDate="2026-09-29" reportType="PARA"><laboratoryService AppID="test-service-1" stencilNo="TEST" reportedService="TEST" medSrvPack="TEST" quantity="1" personType="ASIGURAT" serviceDate="2026-09-29T10:00:00+03:00"/></request>`
const testResult = `<response xmlns="http://www.cnas.ro/siui/2.0" validationDate="2026-09-29T10:01:00+03:00" state="1"><laboratoryService AppID="test-service-1" state="1"/></response>`

func soapResponse(raw string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(raw))
	return `<s:Envelope xmlns:s="` + soapNS + `"><s:Body><m:validateReportResponse xmlns:m="` + validateNS + `"><validateReportReturn>` + b.String() + `</validateReportReturn></m:validateReportResponse></s:Body></s:Envelope>`
}

type simulatedBackend struct {
	http       *http.Client
	calls      atomic.Int32
	failPost   bool
	skipSchema bool
}

func (b *simulatedBackend) Do(ctx context.Context, method, url string, h http.Header, body []byte) (int, http.Header, []byte, error) {
	if method == "POST" {
		b.calls.Add(1)
		if b.failPost {
			return 0, nil, nil, errors.New("simulated disconnect")
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, url, strings.NewReader(string(body)))
	if e != nil {
		return 0, nil, nil, e
	}
	req.Header = h.Clone()
	r, e := b.http.Do(req)
	if e != nil {
		return 0, nil, nil, e
	}
	defer r.Body.Close()
	data, e := io.ReadAll(r.Body)
	return r.StatusCode, r.Header, data, e
}
func (b *simulatedBackend) Validate(ctx context.Context, schema string, data []byte) error {
	if b.skipSchema {
		return nil
	}
	return validateSchema(ctx, schema, data)
}
func upstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, secret, ok := r.BasicAuth()
		if !ok || user != "TEST_CAS" || secret != "test-licence" {
			t.Error("missing Basic auth")
		}
		switch r.URL.Path {
		case "/OCSP/validator":
			if r.URL.Query().Get("username") != "TEST_CAS" {
				t.Error("username")
			}
			w.Header().Set("OSCP_RESPONSE", "test-session")
		case "/svapntws/services/SiuiValidateWS":
			if r.Header.Get("OSCP_RESPONSE") != "test-session" || r.Header.Get("SessionID") != "test-session" {
				t.Error("session missing")
			}
			if r.Header.Get("SOAPAction") != `""` {
				t.Error("SOAPAction")
			}
			b, _ := io.ReadAll(r.Body)
			n, e := parseXML(b)
			if e != nil {
				t.Error(e)
				return
			}
			op := n.child("Body").child("validateReport")
			if op.XMLName.Space != validateNS || op.child("reportXml").Text != testXML || op.child("reportType").Text != "PARA" || op.child("requestType").Text != "RQ_PARA_SRV" {
				t.Error("wrong SOAP operation or data")
			}
			w.Header().Set("Content-Type", "text/xml")
			io.WriteString(w, soapResponse(testResult))
		default:
			t.Error("unexpected endpoint")
			w.WriteHeader(404)
		}
	}))
}
func configured(t *testing.T, base string) settings {
	t.Helper()
	d := t.TempDir()
	return settings{Username: "TEST_CAS", Licence: "test-licence", Thumbprint: strings.Repeat("A", 40), Store: "CurrentUser", BaseURL: base, Database: filepath.Join(d, "jobs.db")}
}
func TestValidateTLSFlowAndSchemas(t *testing.T) {
	s := upstream(t)
	defer s.Close()
	b := &simulatedBackend{http: s.Client()}
	c := client{cfg: configured(t, s.URL), backend: b}
	result, sent, e := c.validate(context.Background(), testXML)
	if e != nil || !sent || !result.Validated || len(result.Services) != 1 {
		t.Fatalf("result=%+v sent=%v err=%v", result, sent, e)
	}
	if b.calls.Load() != 1 {
		t.Fatal("unexpected retry")
	}
}
func TestMalformedXMLNeverCallsCNAS(t *testing.T) {
	b := &simulatedBackend{}
	c := client{backend: b}
	for _, raw := range []string{`<!DOCTYPE x [<!ENTITY e SYSTEM "file:///etc/passwd">]><x/>`, `<request/>`, strings.Replace(testXML, `quantity="1"`, `quantity="invalid"`, 1), strings.Replace(testXML, `stencilNo="TEST"`, "", 1)} {
		_, sent, e := c.validate(context.Background(), raw)
		if e == nil || sent {
			t.Fatalf("invalid input sent: %v", e)
		}
	}
}
func TestSOAPAndBusinessErrors(t *testing.T) {
	fault := `<s:Envelope xmlns:s="` + soapNS + `"><s:Body><s:Fault><faultstring>sensitive fault</faultstring></s:Fault></s:Body></s:Envelope>`
	if _, e := decodeSOAP([]byte(fault)); e == nil || strings.Contains(e.Error(), "sensitive") {
		t.Fatal("fault not safely handled")
	}
	partial := strings.Replace(testResult, `state="1"`, `state="2"`, 1)
	result, e := parseResult(partial, []string{"test-service-1"})
	if e != nil || result.Validated {
		t.Fatal("partial result marked valid")
	}
	result, e = parseResult(testResult, []string{"test-service-1", "missing"})
	if e != nil || result.Validated {
		t.Fatal("missing result marked valid")
	}
	if _, e = parseResult(testResult, []string{"another"}); e == nil {
		t.Fatal("foreign AppID accepted")
	}
	raw := `<s:Envelope xmlns:s="` + soapNS + `"><s:Body><m:validateReportResponse xmlns:m="` + validateNS + `"><validateReportReturn href="#id0"/></m:validateReportResponse><multiRef id="id0">&lt;response/&gt;</multiRef></s:Body></s:Envelope>`
	if got, e := decodeSOAP([]byte(raw)); e != nil || got != "<response/>" {
		t.Fatalf("Axis reference: %q %v", got, e)
	}
	if _, e := decodeSOAP([]byte(strings.Replace(raw, "#id0", "https://evil.test", 1))); e == nil {
		t.Fatal("external reference allowed")
	}
}
func TestNoRetryAfterAmbiguousWrite(t *testing.T) {
	s := upstream(t)
	defer s.Close()
	b := &simulatedBackend{http: s.Client(), failPost: true}
	c := client{cfg: configured(t, s.URL), backend: b}
	_, sent, e := c.validate(context.Background(), testXML)
	if e == nil || !sent || b.calls.Load() != 1 {
		t.Fatal("write ambiguity/retry handling")
	}
}
func TestJobIdempotencyAndRestart(t *testing.T) {
	cfg := configured(t, "https://example.test")
	db, e := openJobs(cfg.Database)
	if e != nil {
		t.Fatal(e)
	}
	j, new, e := insertJob(db, "a", "op-1", testXML)
	if e != nil || !new {
		t.Fatal(e)
	}
	again, new, e := insertJob(db, "a", "op-1", testXML)
	if e != nil || new || again.ID != j.ID {
		t.Fatal("duplicate reservation")
	}
	if _, _, e = insertJob(db, "a", "op-1", testXML+" "); e == nil {
		t.Fatal("conflict accepted")
	}
	if _, e = readJob(db, "b", j.ID); e == nil {
		t.Fatal("owner isolation")
	}
	db.Close()
	db, e = openJobs(cfg.Database)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	j, e = readJob(db, "a", j.ID)
	if e != nil || j.Status != "unknown" {
		t.Fatalf("restart replay protection: %+v %v", j, e)
	}
}

type testRuntime struct {
	mux      *http.ServeMux
	path     string
	settings map[string]interface{}
	guard    sessionGuard
}

func (r *testRuntime) ConfigPath() string                           { return r.path }
func (r *testRuntime) ConfigDir() string                            { return filepath.Dir(r.path) }
func (r *testRuntime) ReaderID() string                             { return "siui-bridge" }
func (r *testRuntime) Logf(string, ...interface{})                  {}
func (r *testRuntime) ModuleSettings(string) map[string]interface{} { return r.settings }
func (r *testRuntime) ResolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(r.ConfigDir(), p)
}
func (r *testRuntime) AddMenu(...module.MenuEntry)         {}
func (r *testRuntime) Handle(p string, h http.Handler)     { r.mux.Handle(p, h) }
func (r *testRuntime) Mux() *http.ServeMux                 { return r.mux }
func (r *testRuntime) RegisterService(string, interface{}) {}
func (r *testRuntime) Service(string) (interface{}, bool)  { return r.guard, true }

type testGuard struct{}

func (g testGuard) SessionIdentity(r *http.Request) (string, bool, bool) {
	if p, ok := apibridge.PrincipalFrom(r.Context()); ok {
		return p.Subject, p.Admin, true
	}
	if r.Header.Get("Authorization") == "test-local-session" {
		return "operator", true, true
	}
	return "", false, false
}
func (g testGuard) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := g.SessionIdentity(r); !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func TestWSMAdapterAndHTTPUseSameHandler(t *testing.T) {
	s := upstream(t)
	defer s.Close()
	cfg := configured(t, s.URL)
	b := &simulatedBackend{http: s.Client()}
	rt := &testRuntime{mux: http.NewServeMux(), path: filepath.Join(filepath.Dir(cfg.Database), "config.yaml"), guard: testGuard{}, settings: map[string]interface{}{"username": cfg.Username, "base_url": s.URL, "certificate_thumbprint": cfg.Thumbprint, "licence": cfg.Licence, "database": cfg.Database}}
	m := &Module{factory: func(settings) backend { return b }}
	if e := m.Init(rt); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Start(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	<-m.ready
	local := httptest.NewRecorder()
	rt.mux.ServeHTTP(local, httptest.NewRequest("GET", "/api/siui/status", nil))
	if local.Code != 401 {
		t.Fatal("anonymous HTTP allowed")
	}
	req := httptest.NewRequest("GET", "/api/siui/status", nil)
	req.Header.Set("Authorization", "test-local-session")
	local = httptest.NewRecorder()
	rt.mux.ServeHTTP(local, req)
	if local.Code != 200 {
		t.Fatal("local HTTP failed")
	}
	p := apibridge.Principal{Subject: "operator", TenantID: "clinic-a", ConnectionID: "browser-1"}
	payload, _ := json.Marshal(map[string]string{"operation_id": "test-72h-1", "xml": testXML})
	invoke := func(principal apibridge.Principal, method, path string, body json.RawMessage) apibridge.Response {
		t.Helper()
		r, e := apibridge.Invoke(context.Background(), rt.mux, principal, apibridge.Request{Method: method, Path: path, Body: body})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	response := invoke(p, "POST", "/api/siui/validations", payload)
	if response.Status != 202 {
		t.Fatalf("submit: %+v", response)
	}
	body, _ := json.Marshal(response.Body)
	var decoded struct {
		Job Job `json:"job"`
	}
	json.Unmarshal(body, &decoded)
	id := decoded.Job.ID
	if id == "" {
		t.Fatal("missing job")
	}
	duplicate := invoke(p, "POST", "/api/siui/validations", payload)
	if duplicate.Status != 200 {
		t.Fatal("duplicate failed")
	}
	for deadline := time.Now().Add(10 * time.Second); ; {
		r := invoke(p, "GET", "/api/siui/validations/"+id, nil)
		body, _ = json.Marshal(r.Body)
		json.Unmarshal(body, &decoded)
		if decoded.Job.Status == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job not complete: %+v", decoded.Job)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !decoded.Job.Result.Validated || b.calls.Load() != 1 {
		t.Fatal("validation result or dedupe failed")
	}
	foreign := p
	foreign.TenantID = "clinic-b"
	if r := invoke(foreign, "GET", "/api/siui/validations/"+id, nil); r.Status != 404 {
		t.Fatal("cross-tenant job exposed")
	}
	if r := invoke(p, "GET", "/api/siui/certificates", nil); r.Status != 403 {
		t.Fatal("non-admin certificate access")
	}
}

func TestLicenceSettingsWithoutTokenMaskedAndPreserved(t *testing.T) {
	dir := t.TempDir()
	rt := &testRuntime{mux: http.NewServeMux(), path: filepath.Join(dir, "config.yaml"), guard: testGuard{}, settings: map[string]interface{}{}}
	if err := os.WriteFile(rt.path, []byte("modules:\n  siui:\n    base_url: https://www.siui.ro\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m := &Module{}
	if err := m.Init(rt); err != nil {
		t.Fatal(err)
	}
	call := func(method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/siui/settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "test-local-session")
		res := httptest.NewRecorder()
		rt.mux.ServeHTTP(res, req)
		return res
	}
	payload := `{"username":"TEST_CAS","base_url":"https://www.siui.ro","certificate_store":"CurrentUser","certificate_thumbprint":"","licence":"secret-test-licence"}`
	if res := call("PUT", payload); res.Code != 200 {
		t.Fatalf("save without token: %s", res.Body.String())
	}
	res := call("GET", "")
	if res.Code != 200 || strings.Contains(res.Body.String(), "secret-test-licence") || !strings.Contains(res.Body.String(), `"licence_configured":true`) || !strings.Contains(res.Body.String(), `"licence_hint":"sec…nce"`) {
		t.Fatalf("licence leaked or status missing: %s", res.Body.String())
	}
	if res := call("PUT", strings.ReplaceAll(payload, "secret-test-licence", "")); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	if m.cfg.Licence != "secret-test-licence" {
		t.Fatal("blank field erased saved licence")
	}
	data, err := os.ReadFile(rt.path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "licence: secret-test-licence") || strings.Contains(string(data), "licence_file") {
		t.Fatal("licence not persisted in config")
	}
	h, err := (&client{cfg: m.cfg}).credentials()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header = h
	_, password, ok := req.BasicAuth()
	if !ok || password != "secret-test-licence" {
		t.Fatal("credentials did not use saved setting")
	}
	restarted := &Module{}
	rt2 := *rt
	rt2.mux = http.NewServeMux()
	rt2.settings = map[string]interface{}{"licence": "secret-test-licence"}
	if err := restarted.Init(&rt2); err != nil || restarted.cfg.Licence != "secret-test-licence" {
		t.Fatal("licence not loaded on restart", err)
	}
}

func TestLicenceHint(t *testing.T) {
	for input, want := range map[string]string{"": "", "ABC123456XYZ": "ABC…XYZ", "123456": "••••••", "abc": "••••••", "ĂBC12345ȘȚZ": "ĂBC…ȘȚZ"} {
		if got := licenceHint(input); got != want {
			t.Errorf("unexpected hint: %q", got)
		}
	}
}

func TestServerCertificateDateExceptionSettings(t *testing.T) {
	dir := t.TempDir()
	rt := &testRuntime{mux: http.NewServeMux(), path: filepath.Join(dir, "config.yaml"), guard: testGuard{}, settings: map[string]interface{}{}}
	if err := os.WriteFile(rt.path, []byte("modules:\n  siui:\n    base_url: https://www.siui.ro\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m := &Module{}
	if err := m.Init(rt); err != nil {
		t.Fatal(err)
	}
	if m.cfg.AllowInvalidServerCertificateDate {
		t.Fatal("exception enabled by default")
	}
	call := func(method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/siui/settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "test-local-session")
		res := httptest.NewRecorder()
		rt.mux.ServeHTTP(res, req)
		return res
	}
	base := `{"username":"TEST_CAS","base_url":"https://www.siui.ro","certificate_store":"CurrentUser"`
	for _, tc := range []struct {
		field string
		want  bool
	}{
		{`,"allow_invalid_server_certificate_date":true`, true},
		{"", true}, // Older clients preserve an explicit operator choice.
		{`,"allow_invalid_server_certificate_date":false`, false},
	} {
		if res := call("PUT", base+tc.field+"}"); res.Code != 200 {
			t.Fatal(res.Body.String())
		}
		if m.cfg.AllowInvalidServerCertificateDate != tc.want {
			t.Fatal("live configuration mismatch")
		}
		var body struct {
			Settings struct {
				Allow bool `json:"allow_invalid_server_certificate_date"`
			} `json:"settings"`
		}
		if err := json.Unmarshal(call("GET", "").Body.Bytes(), &body); err != nil || body.Settings.Allow != tc.want {
			t.Fatal("GET mismatch", err)
		}
		saved, err := config.Load(rt.path)
		if err != nil {
			t.Fatal(err)
		}
		restarted := &Module{}
		rt2 := *rt
		rt2.mux = http.NewServeMux()
		rt2.settings = saved.ModuleSettings("siui")
		if err := restarted.Init(&rt2); err != nil {
			t.Fatal(err)
		}
		if restarted.cfg.AllowInvalidServerCertificateDate != tc.want {
			t.Fatal("persisted configuration mismatch")
		}
	}
}
