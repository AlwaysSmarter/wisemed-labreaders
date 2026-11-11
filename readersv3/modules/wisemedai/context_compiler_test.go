package wisemedai

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCompiledContextPreservesClinicalInformation(t *testing.T) {
	m := &Module{}
	template := testTemplate()
	template.Sections = nil // Small fixed benchmark; section hierarchy is tested separately.
	chunks := []Chunk{{ID: "c1", Revision: 2, Sequence: 3, Timestamp: time.Now(), Text: "Fără durere; nu s-a efectuat ecografie. Nu ignora negația."}}
	compiled := m.compileContext(template, chunks)
	raw, _ := json.Marshal(compiled)
	if len(raw) >= compiled.OriginalBytes {
		t.Fatal("context did not shrink", len(raw), compiled.OriginalBytes)
	}
	if !strings.Contains(string(raw), chunks[0].Text) || !strings.Contains(string(raw), "c1") {
		t.Fatal("clinical text changed")
	}
	var fields []compactField
	json.Unmarshal(compiled.Fields, &fields)
	if len(fields) != len(template.Fields) {
		t.Fatal("fields omitted")
	}
	for i, f := range fields {
		if f.Key != template.Fields[i].Key || f.Label != template.Fields[i].Label || f.Type != template.Fields[i].Type {
			t.Fatal("field meaning changed")
		}
	}
	before := string(compiled.Fields)
	template.Fields[0].Label = "Changed definition"
	if string(m.compileContext(template, chunks).Fields) == before {
		t.Fatal("stale compiled cache")
	}
	t.Logf("Synthetic context: %d -> %d bytes (all clinical text/fields preserved)", compiled.OriginalBytes, len(raw))
}
func TestPostgresLocalGenerationAttributionAndPrivacy(t *testing.T) {
	m, rt := integrationModule(t)
	ctx := context.Background()
	password, _ := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	_, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(password)})
	super := out["token"].(string)
	uid, token := createBillingUser(t, m, super, "compiler@example.invalid", "doctor")
	_, other := createBillingUser(t, m, super, "compiler-other@example.invalid", "doctor")
	selectTestSpecialties(t, m, token, "Neurologie")
	selectTestSpecialties(t, m, other, "Neurologie")
	// Local compilation succeeds with the real provider selected and no provider key/call.
	m.cfg.Provider = "openai"
	m.cfg.OpenAIKeyEnv = "WAI_TEST_UNUSED_COMPILER_KEY"
	t.Setenv(m.cfg.OpenAIKeyEnv, "")
	code, out := request(t, m, token, "POST", "templates/proposals", map[string]any{"report": "Reflexe: diminuate\nDurere: absentă"})
	if code != 200 || out["provider"] != "local" {
		t.Fatal(code, out)
	}
	generation := out["generation_id"].(string)
	payload := map[string]any{"name": "Synthetic compiled template", "specialty": "Neurologie", "html": "<p>{{reflexe}}</p>", "fields": []any{map[string]any{"key": "reflexe", "label": "Reflexe", "type": "string"}}, "generation_id": generation}
	code, out = request(t, m, other, "POST", "templates", payload)
	if code != 400 {
		t.Fatal("foreign generation accepted", code, out)
	}
	code, out = request(t, m, token, "POST", "templates", payload)
	if code != 201 {
		t.Fatal(code, out)
	}
	version := out["version_id"].(string)
	code, out = request(t, m, token, "GET", "billing/statistics?period=all", nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	items := out["items"].([]any)
	if len(items) != 1 {
		t.Fatal(items)
	}
	stat := items[0].(map[string]any)
	if stat["template_version"] != version || stat["total_tokens"] != float64(0) || stat["calls"] != float64(1) {
		t.Fatal(stat)
	}
	code, _ = request(t, m, other, "GET", "billing/statistics?user_id="+uid, nil)
	if code != 403 {
		t.Fatal("statistics leaked", code)
	}
	var tenant string
	m.db.QueryRow(ctx, `SELECT tenant_id::text FROM wai_memberships WHERE user_id=$1`, uid).Scan(&tenant)
	tx, _ := m.db.Begin(ctx)
	defer tx.Rollback(ctx)
	defs, e := retrieveDefinitions(ctx, tx, principal{Tenant: tenant, User: uid}, "Reflexe diminuate")
	if e != nil || len(defs) == 0 {
		t.Fatal(defs, e)
	}
}
func TestLocalGenerationRejectsNarrativeAndIdentifiers(t *testing.T) {
	for _, s := range []string{"Pacient: Ion Popescu", "CNP: 1234567890123", "Pacientul afirmă: durere abdominală de ieri.\nExamen încă neefectuat", "Durere fără etichete"} {
		if _, ok := localFieldProposals(s); ok {
			t.Fatal("unsafe local classification", s)
		}
	}
}
