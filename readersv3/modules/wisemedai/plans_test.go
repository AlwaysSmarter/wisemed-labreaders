package wisemedai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func billingList(t *testing.T, m *Module, token, path string) []map[string]any {
	t.Helper()
	r := httptest.NewRequest("GET", "http://localhost/v1/"+path, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	m.serve(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out []map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(e)
	}
	return out
}
func createBillingUser(t *testing.T, m *Module, admin, email, role string) (string, string) {
	t.Helper()
	code, out := request(t, m, admin, "POST", "users", map[string]any{"name": "Synthetic billing user", "email": email, "password": "synthetic-password-123", "role": role})
	if code != 200 {
		t.Fatal(code, out)
	}
	id := out["id"].(string)
	code, out = request(t, m, "", "POST", "auth/login", map[string]any{"email": email, "password": "synthetic-password-123"})
	if code != 200 {
		t.Fatal(code, out)
	}
	return id, out["token"].(string)
}
func selectTestSpecialties(t *testing.T, m *Module, token string, specialties ...string) {
	t.Helper()
	code, out := request(t, m, token, "PUT", "billing/specialties", map[string]any{"specialties": specialties})
	if code != 200 {
		t.Fatal(code, out)
	}
}
func TestPostgresPackagesRolesAndEntitlements(t *testing.T) {
	m, rt := integrationModule(t)
	ctx := context.Background()
	password, _ := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	code, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(password)})
	if code != 200 || out["profile"].(map[string]any)["user_type"] != float64(-1) {
		t.Fatal(code, out)
	}
	super := out["token"].(string)
	if plans := billingList(t, m, super, "billing/plans"); len(plans) != 3 {
		t.Fatal("missing starter tiers", plans)
	}
	uid, token := createBillingUser(t, m, super, "customer@example.invalid", "doctor")
	_, admin := createBillingUser(t, m, super, "orgadmin-billing@example.invalid", "admin")
	code, out = request(t, m, token, "GET", "billing/account", nil)
	if code != 200 || out["plan"].(map[string]any)["code"] != "free" || out["user_type"] != float64(1) || out["online_payments_enabled"] != false || out["credit_spending_enabled"] != false {
		t.Fatal(code, out)
	}
	if plans := billingList(t, m, token, "billing/plans"); len(plans) != 1 {
		t.Fatal("draft plans exposed for sale", plans)
	}
	for _, actor := range []string{token, admin} {
		code, _ = request(t, m, actor, "POST", "billing/plans", map[string]any{})
		if code != 403 {
			t.Fatal("non-super can publish prices", code)
		}
		code, _ = request(t, m, actor, "GET", "billing/users", nil)
		if code != 403 {
			t.Fatal("non-super can enumerate platform users", code)
		}
	}
	code, _ = request(t, m, token, "PUT", "billing/specialties", map[string]any{"specialties": []string{"Neurologie", "Cardiologie"}})
	if code != 402 {
		t.Fatal("specialty quota not enforced", code)
	}
	code, out = request(t, m, token, "PUT", "billing/specialties", map[string]any{"specialties": []string{"Neurologie"}})
	if code != 200 {
		t.Fatal(code, out)
	}
	var createdID, createdVersion string
	for i := 0; i < 4; i++ {
		code, out = request(t, m, token, "POST", "templates", map[string]any{"name": "Synthetic custom template", "specialty": "Neurologie", "html": "<p>{{test}}</p>", "fields": []any{map[string]any{"key": "test", "label": "Test", "type": "string"}}})
		if i < 3 {
			if code != 201 {
				t.Fatal(i, code, out)
			}
			createdID = out["id"].(string)
			createdVersion = out["version_id"].(string)
		} else if code != 402 {
			t.Fatal("custom template quota not enforced", code, out)
		}
	}
	var count int
	_ = m.db.QueryRow(ctx, `SELECT count(*) FROM wai_templates WHERE owner_id=$1`, uid).Scan(&count)
	if count != 3 {
		t.Fatal("template quota count", count)
	}
	otherID, other := createBillingUser(t, m, super, "second-customer@example.invalid", "doctor")
	code, _ = request(t, m, other, "POST", "sessions", map[string]any{"template_version": createdVersion, "patient_ref": "SYNTHETIC"})
	if code != 403 {
		t.Fatal("another doctor used private template", code)
	}
	code, out = request(t, m, token, "POST", "templates", map[string]any{"id": createdID, "name": "Updated own template", "specialty": "Neurologie", "html": "<p>{{test}}</p>", "fields": []any{map[string]any{"key": "test", "label": "Test", "type": "string"}}})
	if code != 201 {
		t.Fatal("editing consumed another template", code, out)
	}
	createdVersion = out["version_id"].(string)
	pro, e := planFromRow(m.db.QueryRow(ctx, planSelect+` WHERE p.code='pro' AND v.id=p.current_version`))
	if e != nil {
		t.Fatal(e)
	}
	pro.Active = true
	pro.MonthlyPriceMinor = 1999
	pro.MaxSpecialties = 2
	pro.MaxTemplates = 5
	pro.MaxFields = 2
	pro.IncludedTokens = 1000
	pro.ExtrasEnabled = true
	pro.ExtraSpecialtyMinor = 500
	pro.ExtraTemplateMinor = 100
	pro.ExtraMillionMinor = 999
	pro.CreditLimitMinor = 2000
	code, out = request(t, m, super, "POST", "billing/plans", pro)
	if code != 201 {
		t.Fatal(code, out)
	}
	_ = json.Unmarshal([]byte(toJSON(out)), &pro)
	oldVersion := pro.VersionID
	code, out = request(t, m, super, "POST", "billing/assignments", map[string]any{"user_id": uid, "plan_version": pro.VersionID, "confirmed": true, "note": "Synthetic administrative grant"})
	if code != 200 || out["payment_collected"] != false {
		t.Fatal(code, out)
	}
	code, out = request(t, m, token, "PUT", "billing/specialties", map[string]any{"specialties": []string{"Neurologie", "Cardiologie"}})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, _ = request(t, m, token, "POST", "templates", map[string]any{"name": "Too complex", "specialty": "Cardiologie", "html": "<p>{{one}}</p>", "fields": []any{map[string]any{"key": "one", "label": "One", "type": "string"}, map[string]any{"key": "two", "label": "Two", "type": "string"}, map[string]any{"key": "three", "label": "Three", "type": "string"}}})
	if code != 402 {
		t.Fatal("template complexity quota", code)
	}
	pro.MonthlyPriceMinor = 2999
	code, out = request(t, m, super, "POST", "billing/plans", pro)
	if code != 201 {
		t.Fatal(code, out)
	}
	code, out = request(t, m, token, "GET", "billing/account", nil)
	if code != 200 || out["plan"].(map[string]any)["version_id"] != oldVersion || out["plan"].(map[string]any)["monthly_price_minor"] != float64(1999) || out["credit_limit_minor"] != float64(2000) {
		t.Fatal("existing commercial terms changed", code, out)
	}
	code, _ = request(t, m, super, "POST", "billing/plans", pro)
	if code != 409 {
		t.Fatal("stale plan overwrite accepted", code)
	}
	users := billingList(t, m, super, "billing/users")
	found := false
	for _, u := range users {
		if u["id"] == uid {
			found = true
			if u["plan"].(map[string]any)["version"] != float64(2) {
				t.Fatal("assigned plan version absent", u)
			}
		}
	}
	if !found {
		t.Fatal("platform user missing")
	}
	code, out = request(t, m, super, "PUT", "billing/users/role", map[string]any{"user_id": otherID, "user_type": -1, "disabled": false, "confirmed": true})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = request(t, m, other, "GET", "profile", nil)
	if code != 200 || out["user_type"] != float64(-1) {
		t.Fatal("super admin role mapping", code, out)
	}
	code, _ = request(t, m, admin, "PUT", "billing/users/role", map[string]any{"user_id": uid, "user_type": -1, "confirmed": true})
	if code != 403 {
		t.Fatal("organization admin escalated", code)
	}
	code, out = request(t, m, token, "POST", "sessions", map[string]any{"template_version": createdVersion, "patient_ref": "SYNTHETIC"})
	if code != 201 {
		t.Fatal(code, out)
	}
	sid := out["id"].(string)
	// Independent reservations enforce a shared per-user token pool, including concurrent requests.
	conn, e := pgx.ConnectConfig(ctx, m.db.Config().ConnConfig.Copy())
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close(ctx)
	p := principal{User: uid, Tenant: outTenant(t, m, uid), Role: "doctor"}
	scope := aiUsageContext{Session: sid, Template: createdVersion, Operation: "transcript_extract"}
	usageID := uuid.NewString()
	if e = reserveTokens(ctx, conn, p, usageID, "synthetic-model", 600, 0.01, scope); e != nil {
		t.Fatal(e)
	}
	if e = reserveTokens(ctx, conn, p, uuid.NewString(), "synthetic-model", 500, 0.01, scope); e == nil || e.Error() != "ai_token_allowance_exceeded" {
		t.Fatal("credit incorrectly enabled spending", e)
	}
	_, e = conn.Exec(ctx, `UPDATE wai_usage_records SET status='completed',input_tokens=100,output_tokens=50 WHERE id=$1`, usageID)
	if e != nil {
		t.Fatal(e)
	}
	if e = reserveTokens(ctx, conn, p, uuid.NewString(), "synthetic-model", 800, 0.01, scope); e != nil {
		t.Fatal(e)
	}
	code, out = request(t, m, token, "GET", "billing/account", nil)
	if code != 200 || out["total_tokens"] != float64(150) || out["reserved_tokens"] != float64(800) {
		t.Fatal("actual vs reserved tokens", code, out)
	}
	ledger := billingList(t, m, token, "billing/usage")
	if len(ledger) != 2 || ledger[0]["session_id"] != sid || ledger[0]["template_version"] != createdVersion || ledger[0]["operation"] != "transcript_extract" {
		t.Fatal("missing per-process attribution", ledger)
	}
}
func outTenant(t *testing.T, m *Module, user string) string {
	t.Helper()
	var tenant string
	if e := m.db.QueryRow(context.Background(), `SELECT tenant_id FROM wai_memberships WHERE user_id=$1 LIMIT 1`, user).Scan(&tenant); e != nil {
		t.Fatal(e)
	}
	return tenant
}
func TestPostgresConcurrentTokenReservations(t *testing.T) {
	m, rt := integrationModule(t)
	raw, _ := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	_, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(raw)})
	super := out["token"].(string)
	uid, _ := createBillingUser(t, m, super, "concurrent@example.invalid", "doctor")
	p := principal{User: uid, Tenant: outTenant(t, m, uid), Role: "doctor"}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			c, e := pgx.ConnectConfig(ctx, m.db.Config().ConnConfig.Copy())
			if e != nil {
				results <- e
				return
			}
			defer c.Close(ctx)
			results <- reserveTokens(ctx, c, p, uuid.NewString(), "synthetic", 30000, 0.01, aiUsageContext{Operation: "test"})
		}()
	}
	wg.Wait()
	close(results)
	success, denied := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if strings.Contains(e.Error(), "ai_token_allowance_exceeded") {
			denied++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || denied != 1 {
		t.Fatal("concurrent quota race", success, denied)
	}
}

func TestPostgresDoctorAIReservationWithinClinicalTransaction(t *testing.T) {
	m, rt := integrationModule(t)
	ctx := context.Background()
	password, _ := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	_, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(password)})
	super := out["token"].(string)
	uid, token := createBillingUser(t, m, super, "process@example.invalid", "doctor")
	selectTestSpecialties(t, m, token, "Neurologie")
	code, out := request(t, m, token, "POST", "templates", map[string]any{"name": "Synthetic accounting", "ai_context": map[string]any{"instructions": "SYNTHETIC_TEMPLATE_CONTEXT_RULE", "terminology": "", "examples": ""}, "specialty": "Neurologie", "html": "<p>{{test}}</p>", "fields": []any{map[string]any{"key": "test", "label": "Test", "type": "string"}}})
	if code != 201 {
		t.Fatal(code, out)
	}
	version := out["version_id"].(string)
	code, out = request(t, m, token, "POST", "sessions", map[string]any{"template_version": version, "patient_ref": "SYNTHETIC-ONLY"})
	if code != 201 {
		t.Fatal(code, out)
	}
	session := out["id"].(string)
	chunkID := uuid.NewString()
	m.cfg.Provider = "openai"
	m.cfg.OpenAIKeyEnv = "WAI_BILLING_SYNTHETIC_KEY"
	t.Setenv(m.cfg.OpenAIKeyEnv, "synthetic-not-a-key")
	calls := 0
	m.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), "SYNTHETIC_TEMPLATE_CONTEXT_RULE") {
			t.Fatal("persistent context missing from provider input")
		}
		calls++
		content := toJSON(map[string]any{"facts": []any{map[string]any{"field": "test", "value": "synthetic", "state": "present", "chunk_id": chunkID, "quote": "test: synthetic"}}})
		reply := toJSON(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}}, "usage": map[string]any{"prompt_tokens": 123, "completion_tokens": 45, "prompt_tokens_details": map[string]any{"cached_tokens": 64}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(reply))}, nil
	})}
	chunk := map[string]any{"id": chunkID, "sequence": 1, "revision": 1, "is_final": true, "timestamp": time.Now().UTC(), "text": "test: synthetic"}
	for i := 0; i < 2; i++ {
		code, out = request(t, m, token, "POST", "sessions/"+session+"/transcript-chunks", chunk)
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	if calls != 1 {
		t.Fatal("duplicate was billed again", calls)
	}
	var cached, originalBytes, sentBytes int64
	if e := m.db.QueryRow(ctx, `SELECT cached_input_tokens,original_context_bytes,sent_context_bytes FROM wai_usage_records WHERE user_id=$1`, uid).Scan(&cached, &originalBytes, &sentBytes); e != nil || cached != 64 || originalBytes <= sentBytes {
		t.Fatal("context usage metrics", cached, originalBytes, sentBytes, e)
	}
	var input, output int
	var sid, vid, operation string
	e := m.db.QueryRow(ctx, `SELECT input_tokens,output_tokens,session_id,template_version,operation FROM wai_usage_records WHERE user_id=$1`, uid).Scan(&input, &output, &sid, &vid, &operation)
	if e != nil || input != 123 || output != 45 || sid != session || vid != version || operation != "transcript_extract" {
		t.Fatal("usage attribution", input, output, sid, vid, operation, e)
	}
}

func TestPostgresBillingUserPassword(t *testing.T) {
	m, rt := integrationModule(t)
	password, err := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	if err != nil {
		t.Fatal(err)
	}
	code, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(password)})
	if code != 200 {
		t.Fatal("bootstrap login", code)
	}
	super := out["token"].(string)
	_, admin := createBillingUser(t, m, super, "admin@example.invalid", "admin")
	email := "doctor@example.invalid"
	id, session := createBillingUser(t, m, super, email, "doctor")
	input := map[string]any{"user_id": id, "password": "replacement-password-123"}

	code, _ = request(t, m, admin, "PUT", "billing/users/password", input)
	if code != 403 {
		t.Fatal("organization admin could reset global password", code)
	}
	code, _ = request(t, m, super, "PUT", "billing/users/password", input)
	if code != 200 {
		t.Fatal("super admin password reset", code)
	}
	code, _ = request(t, m, session, "GET", "profile", nil)
	if code != 401 {
		t.Fatal("previous session still active", code)
	}
	code, _ = request(t, m, "", "POST", "auth/login", map[string]any{"email": email, "password": "synthetic-password-123"})
	if code != 401 {
		t.Fatal("previous password still works", code)
	}
	code, _ = request(t, m, "", "POST", "auth/login", map[string]any{"email": email, "password": input["password"]})
	if code != 200 {
		t.Fatal("new password did not work", code)
	}
}
