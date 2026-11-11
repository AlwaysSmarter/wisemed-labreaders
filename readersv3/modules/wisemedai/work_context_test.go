package wisemedai

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPostgresWorkContextIdempotency(t *testing.T) {
	m, rt := integrationModule(t)
	pw, _ := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	_, login := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(pw)})
	token := login["token"].(string)
	templates := billingList(t, m, token, "templates")
	version := templates[0]["version_id"].(string)
	makeRequest := func(mode string) (int, map[string]any) {
		r := httptest.NewRequest("POST", "http://localhost/v1/sessions", strings.NewReader(toJSON(map[string]any{"template_version": version, "patient_ref": "SYNTHETIC", "work_context": mode})))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Idempotency-Key", "work-context-synthetic")
		w := httptest.NewRecorder()
		m.serve(w, r)
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	code, first := makeRequest("internare")
	if code != 201 || first["context"].(map[string]any)["work_context"] != "internare" {
		t.Fatal(code, first)
	}
	code, next := makeRequest("internare")
	if code != 200 || next["id"] != first["id"] {
		t.Fatal(code, next)
	}
	code, _ = makeRequest("ambulatoriu")
	if code != 409 {
		t.Fatal("changed work context accepted under same key", code)
	}
	code, _ = makeRequest("unsupported")
	if code != 400 {
		t.Fatal(code)
	}
}
