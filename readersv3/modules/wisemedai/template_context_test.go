package wisemedai

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestPostgresTemplateContextEncryptedPinnedAndScoped(t *testing.T) {
	m, rt := integrationModule(t)
	ctx := context.Background()
	password, _ := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	_, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(password)})
	super := out["token"].(string)
	uid, token := createBillingUser(t, m, super, "context@example.invalid", "doctor")
	_, other := createBillingUser(t, m, super, "context-other@example.invalid", "doctor")
	selectTestSpecialties(t, m, token, "Neurologie")
	selectTestSpecialties(t, m, other, "Neurologie")
	payload := map[string]any{"name": "Synthetic persistent context", "specialty": "Neurologie", "html": "<p>{{test}}</p>", "fields": []any{map[string]any{"key": "test", "label": "Test", "type": "string"}}, "ai_context": map[string]any{"instructions": "SYNTHETIC_PRIVATE_REFERENCE_OLD", "terminology": "TA = tensiune arterială", "examples": "Fără date reale de pacient."}}
	code, out := request(t, m, token, "POST", "templates", payload)
	if code != 201 {
		t.Fatal(code, out)
	}
	first := out["version_id"].(string)
	payload["id"] = out["id"]
	var raw, doc []byte
	var tenant string
	if e := m.db.QueryRow(ctx, `SELECT c.payload,v.document,c.tenant_id::text FROM wai_template_contexts c JOIN wai_template_versions v ON v.id=c.template_version WHERE c.template_version=$1`, first).Scan(&raw, &doc, &tenant); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "SYNTHETIC_PRIVATE_REFERENCE_OLD") || strings.Contains(string(doc), "SYNTHETIC_PRIVATE_REFERENCE_OLD") {
		t.Fatal("context stored in plaintext")
	}
	payload["ai_context"] = map[string]any{"instructions": "SYNTHETIC_PRIVATE_REFERENCE_NEW", "terminology": "", "examples": ""}
	code, out = request(t, m, token, "POST", "templates", payload)
	if code != 201 {
		t.Fatal(code, out)
	}
	second := out["version_id"].(string)
	tx, _ := m.db.Begin(ctx)
	defer tx.Rollback(ctx)
	old, e := m.templateContext(ctx, tx, tenant, first)
	if e != nil || old.Instructions != "SYNTHETIC_PRIVATE_REFERENCE_OLD" {
		t.Fatal(old, e)
	}
	next, e := m.templateContext(ctx, tx, tenant, second)
	if e != nil || next.Instructions != "SYNTHETIC_PRIVATE_REFERENCE_NEW" {
		t.Fatal(next, e)
	}
	// API listing respects the original template ownership filter.
	items := billingList(t, m, other, "templates")
	for _, item := range items {
		if item["id"] == payload["id"] {
			t.Fatal("private context/template leaked")
		}
	}
	own := billingList(t, m, token, "templates/"+payload["id"].(string))
	if len(own) != 1 || own[0]["ai_context"].(map[string]any)["instructions"] != "SYNTHETIC_PRIVATE_REFERENCE_NEW" {
		t.Fatal(own)
	}
	payload["ai_context"] = map[string]any{"instructions": strings.Repeat("x", 2001), "terminology": "", "examples": ""}
	code, _ = request(t, m, token, "POST", "templates", payload)
	if code != 400 {
		t.Fatal("oversized context accepted", code)
	}
	var count int
	m.db.QueryRow(ctx, `SELECT count(*) FROM wai_templates WHERE owner_id=$1`, uid).Scan(&count)
	if count != 1 {
		t.Fatal("context version duplicated template", count)
	}
}
