package wisemedai

import (
	"context"
	"github.com/google/uuid"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestPostgresApprovedDeliveryRetryAndIdempotency(t *testing.T) {
	m, rt := integrationModule(t)
	ctx := context.Background()
	raw, _ := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	code, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(raw)})
	if code != 200 {
		t.Fatal(code, out)
	}
	token := out["token"].(string)
	p := out["profile"].(map[string]any)
	tenant := p["tenant_id"].(string)
	m.cfg.AllowedHosts = []string{"his.test"}
	code, out = request(t, m, token, "POST", "integrations", map[string]any{"name": "Synthetic authorized receiver", "payload": map[string]any{"url": "https://his.test/import", "token": "synthetic-only"}, "confirmed": true})
	if code != 200 {
		t.Fatal(code, out)
	}
	connector := out["id"].(string)
	var version string
	_ = m.db.QueryRow(ctx, `SELECT current_version FROM wai_templates WHERE name='Ecografie abdominală' AND tenant_id=$1`, tenant).Scan(&version)
	code, out = request(t, m, token, "POST", "sessions", map[string]any{"template_version": version, "patient_ref": "SYNTHETIC-DELIVERY"})
	if code != 201 {
		t.Fatal(code, out)
	}
	sid := out["id"].(string)
	code, _ = request(t, m, token, "POST", "sessions/"+sid+"/deliver", map[string]any{"connector_id": connector, "revision": 1, "confirmed": true})
	if code != 409 {
		t.Fatal("unapproved delivery accepted", code)
	}
	result := Result{SessionID: sid, Status: "approved", Revision: 1, SchemaVersion: "1.0", TemplateVersion: version, Data: map[string]any{"synthetic": true}}
	encrypted, e := m.box.seal(result, tenant+":"+sid)
	if e != nil {
		t.Fatal(e)
	}
	_, e = m.db.Exec(ctx, `UPDATE wai_sessions SET status='approved',revision=1,result=$1 WHERE id=$2`, encrypted, sid)
	if e != nil {
		t.Fatal(e)
	}
	_, e = m.db.Exec(ctx, `INSERT INTO wai_clinical_results(id,tenant_id,session_id,revision,payload)VALUES($1,$2,$3,1,$4)`, uuid.NewString(), tenant, sid, encrypted)
	if e != nil {
		t.Fatal(e)
	}
	var deliveryID string
	for i := 0; i < 2; i++ {
		code, out = request(t, m, token, "POST", "sessions/"+sid+"/deliver", map[string]any{"connector_id": connector, "revision": 1, "confirmed": true})
		if code != 202 {
			t.Fatal(code, out)
		}
		if i == 0 {
			deliveryID = out["delivery_id"].(string)
		} else if deliveryID != out["delivery_id"] {
			t.Fatal("delivery identity changed", out)
		}
	}
	calls := 0
	m.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Idempotency-Key") != deliveryID || r.Header.Get("Authorization") != "Bearer synthetic-only" {
			t.Fatal("missing receiver authorization/idempotency")
		}
		status := 503
		if calls > 1 {
			status = 200
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("synthetic"))}, nil
	})}
	m.dispatchOutbox(ctx)
	var status string
	var attempts int
	if e = m.db.QueryRow(ctx, `SELECT status,attempts FROM wai_outbox_delivery WHERE id=$1`, deliveryID).Scan(&status, &attempts); e != nil || status != "pending" || attempts != 1 {
		t.Fatal(status, attempts, e)
	}
	_, _ = m.db.Exec(ctx, `UPDATE wai_outbox_delivery SET next_attempt_at=now() WHERE id=$1`, deliveryID)
	m.dispatchOutbox(ctx)
	if e = m.db.QueryRow(ctx, `SELECT status,attempts FROM wai_outbox_delivery WHERE id=$1`, deliveryID).Scan(&status, &attempts); e != nil || status != "delivered" || attempts != 2 {
		t.Fatal(status, attempts, e)
	}
	m.dispatchOutbox(ctx)
	if calls != 2 {
		t.Fatal("delivered result resent", calls)
	}
}
