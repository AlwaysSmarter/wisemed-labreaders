package wisemedai

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestPostgresInstancePermissionsAndTransientProposals(t *testing.T) {
	m, rt := integrationModule(t)
	b, _ := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	code, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(b)})
	if code != 200 {
		t.Fatal(code, out)
	}
	token := out["token"].(string)
	// The public provider metadata never returns secrets.
	code, out = request(t, m, "", "GET", "auth/providers", nil)
	if code != 200 || out["standalone"] != true {
		t.Fatal(code, out)
	}
	code, out = request(t, m, token, "POST", "templates/proposals", map[string]any{"report": "Ficat: descriere sintetică\nSplină: neexaminat"})
	if code != 200 || out["requires_acceptance"] != true || out["persisted"] != false || len(out["fields"].([]any)) != 2 {
		t.Fatal(code, out)
	}
	var count int
	_ = m.db.QueryRow(context.Background(), `SELECT count(*) FROM wai_templates`).Scan(&count)
	if count != 52 {
		t.Fatal("proposals persisted without acceptance", count)
	}
	code, out = request(t, m, token, "POST", "users", map[string]any{"name": "Tenant administrator", "email": "orgadmin@example.invalid", "password": "synthetic-password-123", "role": "admin"})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = request(t, m, "", "POST", "auth/login", map[string]any{"email": "orgadmin@example.invalid", "password": "synthetic-password-123"})
	if code != 200 {
		t.Fatal(code, out)
	}
	other := out["token"].(string)
	for _, path := range []string{"settings", "runtime-settings"} {
		code, out = request(t, m, other, "GET", path, nil)
		if code != 403 {
			t.Fatal("tenant admin escalated to installation admin", path, code, out)
		}
	}
	code, out = request(t, m, token, "POST", "users", map[string]any{"name": "Bad escalation", "email": "bad@example.invalid", "password": "synthetic-password-123", "role": "admin", "instance_admin": true})
	if code != 400 {
		t.Fatal("mass assignment", code, out)
	}
	// Synthetic deployment has a secret which public runtime settings must mask.
	raw := `reader:
  id: wisemed-ai
analyzer:
  comm_type: utility
  protocol: wisemed-ai
modules:
  wisemed-api:
    cfg_wisemed_key: synthetic-private-secret
  wisemed-ai:
    database_url: postgres://synthetic:private@localhost/wai
  app-updates:
    enabled: true
    app_id: wisemed-ai
    channel: stable
    base_url: http://127.0.0.1:19090
    auto_download: true
`
	if e := os.WriteFile(rt.ConfigPath(), []byte(raw), 0600); e != nil {
		t.Fatal(e)
	}
	code, out = request(t, m, token, "GET", "runtime-settings", nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	if strings.Contains(toJSON(out), "synthetic-private-secret") || strings.Contains(toJSON(out), "postgres://") {
		t.Fatal("runtime secret disclosed")
	}
	code, out = request(t, m, token, "PUT", "runtime-settings", map[string]any{"sections": map[string]any{"app-updates": map[string]any{"enabled": "not_boolean"}}})
	if code != 400 {
		t.Fatal(code, out)
	}
	code, out = request(t, m, token, "POST", "memory/examples", map[string]any{"name": "Sintetic", "payload": map[string]any{"text": "Preferință de stil sintetică"}, "confirmed": true})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = request(t, m, token, "POST", "memory/search", map[string]any{"query": "stil"})
	if code != 200 || len(out["items"].([]any)) != 1 {
		t.Fatal(code, out)
	}
	code, out = request(t, m, other, "POST", "memory/search", map[string]any{"query": "stil"})
	if code != 200 || len(out["items"].([]any)) != 0 {
		t.Fatal("private memory leaked", code, out)
	}
	// A published successor does not change a session's pinned template version.
	code, out = request(t, m, token, "POST", "templates", map[string]any{"name": "Synthetic versioning", "specialty": "Test", "html": "<p>{{test}}</p>", "fields": []any{map[string]any{"key": "test", "label": "Test", "type": "string"}}})
	if code != 201 {
		t.Fatal(code, out)
	}
	var tpl Template
	_ = json.Unmarshal([]byte(toJSON(out)), &tpl)
	v1 := tpl.VersionID
	code, out = request(t, m, token, "POST", "sessions", map[string]any{"template_version": v1, "patient_ref": "SYNTHETIC"})
	if code != 201 {
		t.Fatal(code, out)
	}
	sid := out["id"].(string)
	tpl.HTML = "<h2>New version</h2><p>{{test}}</p>"
	code, out = request(t, m, token, "POST", "templates", tpl)
	if code != 201 || out["version_id"] == v1 {
		t.Fatal(code, out)
	}
	code, out = request(t, m, token, "GET", "sessions/"+sid, nil)
	if code != 200 || out["template_version"] != v1 {
		t.Fatal("pinned version changed", code, out)
	}
}
