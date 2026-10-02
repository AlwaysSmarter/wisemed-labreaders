package siui

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"wisemed-labreaders/readersv3/shared/apibridge"
)

func insuredSOAP(raw string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(raw))
	return `<s:Envelope xmlns:s="` + soapNS + `"><s:Body><getInsuredResponse xmlns="` + insuredNS + `"><getInsuredReturn>` + b.String() + `</getInsuredReturn></getInsuredResponse></s:Body></s:Envelope>`
}
func providerSOAP(address string) []byte {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(address))
	return []byte(`<s:Envelope xmlns:s="` + soapNS + `"><s:Body><getProviderInfoResponse xmlns="` + insuredNS + `"><getProviderInfoReturn>` + b.String() + `</getProviderInfoReturn><getProviderInfoReturn>150</getProviderInfoReturn></getProviderInfoResponse></s:Body></s:Envelope>`)
}
func TestPassiveAPIsAuthenticateBeforeReadOnlyCalls(t *testing.T) {
	ocsp, insured, provider, download := 0, 0, 0, 0
	const cnp = "0000000000000"
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, secret, ok := r.BasicAuth()
		if !ok || user != "TEST_CAS" || secret != "test-licence" {
			t.Error("wrong credentials")
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/OCSP/validator" {
			ocsp++
			if r.Method != "GET" {
				t.Error("OCSP method")
			}
			w.Header().Set("OSCP_RESPONSE", fmt.Sprintf("session-%d", ocsp))
			return
		}
		if r.Header.Get("OSCP_RESPONSE") != fmt.Sprintf("session-%d", ocsp) || ocsp == 0 {
			t.Error("missing fresh OCSP session")
		}
		switch r.URL.Path {
		case "/svapntws/services/SiuiInsuredWS":
			insured++
			data, _ := io.ReadAll(r.Body)
			n, err := parseXML(data)
			if err != nil {
				t.Fatal(err)
			}
			op := n.child("Body").child("getInsured")
			if op.XMLName.Space != insuredNS || op.child("pid").Text != cnp || op.child("requestDate").Text != "2026-10-03T00:00:00" {
				t.Error("wrong insured SOAP")
			}
			io.WriteString(w, insuredSOAP(localInsuredXML))
		case "/svapntws/services/SiuiWS":
			provider++
			data, _ := io.ReadAll(r.Body)
			n, _ := parseXML(data)
			op := n.child("Body").child("getProviderInfo")
			if op.XMLName.Space != insuredNS || op.child("partnerCategory").Text != "PARA" || op.child("uic").Text != "TEST" || op.child("start").XMLName.Space != insuredNS {
				t.Error("wrong document/literal provider SOAP")
			}
			w.Write(providerSOAP("/personalization/file.zip"))
		case "/personalization/file.zip":
			download++
			if r.Method != "GET" {
				t.Error("download method")
			}
			var b bytes.Buffer
			z := zip.NewWriter(&b)
			f, _ := z.Create("provider.xml")
			io.WriteString(f, `<provider xmlns="`+piasNS+`"/>`)
			z.Close()
			w.Write(b.Bytes())
		default:
			t.Errorf("unexpected endpoint (must never validate): %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer s.Close()
	cfg := configured(t, s.URL)
	rt := &testRuntime{mux: http.NewServeMux(), path: filepath.Join(t.TempDir(), "config.yaml"), guard: testGuard{}, settings: map[string]interface{}{"username": cfg.Username, "licence": cfg.Licence, "certificate_thumbprint": cfg.Thumbprint, "base_url": s.URL}}
	m := &Module{factory: func(settings) backend { return &simulatedBackend{http: s.Client()} }}
	if err := m.Init(rt); err != nil {
		t.Fatal(err)
	}
	m.nativeSupported = true
	principal := apibridge.Principal{TenantID: "test", Subject: "operator", Admin: true}
	invoke := func(path, body string) apibridge.Response {
		res, err := apibridge.Invoke(context.Background(), rt.mux, principal, apibridge.Request{Method: "POST", Path: "/api/siui/" + path, Body: json.RawMessage(body)})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := invoke("ocsp-test", `{}`)
	if res.Status != 200 || ocsp != 1 || insured+provider+download != 0 {
		t.Fatalf("OCSP was not isolated: %+v", res)
	}
	encoded, _ := json.Marshal(res)
	if strings.Contains(string(encoded), "session-1") || strings.Contains(string(encoded), "test-licence") {
		t.Fatal("secret exposed")
	}
	res = invoke("insured", `{"mode":"cnas","cnp":"0000000000000","date":"2026-10-03"}`)
	if res.Status != 200 || ocsp != 2 || insured != 1 {
		t.Fatalf("insured: %+v", res)
	}
	res = invoke("personalization", `{"start":"2026-10-01","stop":"2026-10-03"}`)
	if res.Status != 200 || ocsp != 3 || provider != 1 || download != 1 {
		t.Fatalf("personalization: %+v", res)
	}
	principal.Admin = false
	if res = invoke("personalization", `{"start":"2026-10-01","stop":"2026-10-03"}`); res.Status != 403 {
		t.Fatal("non-admin personalization allowed")
	}
	if res = invoke("insured", `{"mode":"cnas","cnp":"bad","date":"2026-10-03"}`); res.Status != 400 {
		t.Fatal("bad CNP accepted")
	}
	if ocsp != 3 {
		t.Fatal("invalid or forbidden request contacted CNAS")
	}
}
func TestLocalTestNeverCreatesTransport(t *testing.T) {
	rt := &testRuntime{mux: http.NewServeMux(), path: filepath.Join(t.TempDir(), "config.yaml"), guard: testGuard{}, settings: map[string]interface{}{}}
	m := &Module{factory: func(settings) backend { t.Fatal("simulation attempted CNAS connection"); return nil }}
	if err := m.Init(rt); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/siui/insured", strings.NewReader(`{"mode":"local"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "test-local-session")
	res := httptest.NewRecorder()
	rt.mux.ServeHTTP(res, req)
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"simulated":true`) || !strings.Contains(res.Body.String(), `"cnas_contacted":false`) {
		t.Fatalf("local: %s", res.Body.String())
	}
}
func TestInsuredStatesAndErrors(t *testing.T) {
	for _, state := range []int{-1, 0, 1, 2, 3} {
		raw := fmt.Sprintf(`<insuredResponse xmlns="%s"><insured pid="0000000000000" state="%d"/></insuredResponse>`, piasNS, state)
		if err := validateSchema(context.Background(), "GetInsuredResponse.xsd", []byte(raw)); err != nil {
			t.Fatal(err)
		}
		r, err := parseInsured(raw, "0000000000000")
		if err != nil {
			t.Fatal(err)
		}
		if (state == 1 || state == 2) != (r.Insured != nil) {
			t.Fatal("unknown/error interpreted as uninsured")
		}
		if r.Insured != nil && *r.Insured != (state == 1) {
			t.Fatal("wrong insured state")
		}
		if _, err := parseInsured(raw, "1111111111111"); err == nil {
			t.Fatal("wrong patient accepted")
		}
	}
	if _, err := decodeOperationSOAP([]byte(insuredSOAP(localInsuredXML)), "validateReport", validateNS); err == nil {
		t.Fatal("mixed SOAP operations")
	}
}
func TestPersonalizationRejectsForeignURLAndUnsafeArchive(t *testing.T) {
	for _, address := range []string{"https://evil.example/file", "//evil.example/file", "http://www.siui.ro/file", "https://user@www.siui.ro/file"} {
		if _, err := providerDownloadURL(providerSOAP(address), "https://www.siui.ro"); err == nil {
			t.Fatal("foreign download accepted")
		}
	}
	if _, err := providerDownloadURL(providerSOAP("/file.zip"), "https://www.siui.ro"); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{[]byte("not XML"), []byte(`<html/>`), []byte(`<!DOCTYPE x><provider/>`), bytes.Repeat([]byte("x"), maxData+1)} {
		if _, _, err := checkPersonalizationFile(data); err == nil {
			t.Fatal("unsafe file accepted")
		}
	}
	data := []byte(`<provider xmlns="` + piasNS + `"/>`)
	filename, _, err := checkPersonalizationFile(data)
	if err != nil || filename != "personalizare-PARA.xml" {
		t.Fatal(err)
	}
	// The WSM-compatible payload encodes bytes without changing their content.
	decoded, err := base64.StdEncoding.DecodeString(base64.StdEncoding.EncodeToString(data))
	if err != nil || !bytes.Equal(decoded, data) {
		t.Fatal("download bytes changed")
	}
}

func TestOCSPFailureStopsEveryPassiveOperation(t *testing.T) {
	for _, withToken := range []bool{false, true} {
		calls := 0
		s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.URL.Path != "/OCSP/validator" {
				t.Error("operation proceeded without OCSP authentication")
			}
			if withToken {
				w.Header().Set("OSCP_RESPONSE", "ignored-token")
				w.WriteHeader(401)
			}
		}))
		c := client{cfg: configured(t, s.URL), backend: &simulatedBackend{http: s.Client()}}
		if _, err := c.getInsured(context.Background(), "0000000000000", "2026-10-03"); err == nil {
			t.Fatal("unauthenticated insured request")
		}
		if _, err := c.personalization(context.Background(), "2026-10-01", "2026-10-03"); err == nil {
			t.Fatal("unauthenticated personalization")
		}
		if calls != 2 {
			t.Fatal("unexpected calls or automatic retries")
		}
		s.Close()
	}
}
