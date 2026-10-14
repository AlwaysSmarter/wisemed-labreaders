package siui

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOCSPDebugShowsActualConfigWithoutCredentials(t *testing.T) {
	m, rt := serviceTestModule(t, &operationBackend{t: t})
	m.cfg.Username = "7991962_CAS-DJ"
	m.cfg.BaseURL = "https://www.siui.ro:444"
	m.cfg.Licence = "private-licence"
	req := httptest.NewRequest("GET", "/api/siui/general/ocsp", nil)
	req.Header.Set("Authorization", "test-local-session")
	w := httptest.NewRecorder()
	rt.mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var response struct {
		Debug map[string]any `json:"debug"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	d := response.Debug
	if d["username"] != "7991962_CAS-DJ" || d["port"] != "444" || d["url"] != "https://www.siui.ro:444/OCSP/validator?username=7991962_CAS-DJ" || d["providerCategory_sent_to_ocsp"] != false {
		t.Fatal(d)
	}
	if strings.Contains(w.Body.String(), "private-licence") || strings.Contains(w.Body.String(), "Authorization") {
		t.Fatal("secret in debug")
	}
	req = httptest.NewRequest("GET", "/api/siui/general/ocsp", nil)
	w = httptest.NewRecorder()
	rt.mux.ServeHTTP(w, req)
	if w.Code == 200 {
		t.Fatal("unauthenticated debug")
	}
}
