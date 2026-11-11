package wisemedai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPostgresOpenAIMockUsageAndValidation(t *testing.T) {
	m, _ := integrationModule(t)
	ctx := context.Background()
	var p principal
	if e := m.db.QueryRow(ctx, `SELECT u.id,m.tenant_id,m.role,u.email,u.name FROM wai_users u JOIN wai_memberships m ON m.user_id=u.id LIMIT 1`).Scan(&p.User, &p.Tenant, &p.Role, &p.Email, &p.Name); e != nil {
		t.Fatal(e)
	}
	m.cfg.Provider = "openai"
	m.cfg.OpenAIKeyEnv = "WAI_TEST_MOCK_KEY"
	t.Setenv(m.cfg.OpenAIKeyEnv, "synthetic-not-a-real-key")
	calls := 0
	var sentMessages []promptMessage
	m.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://api.openai.com/v1/chat/completions" {
			t.Fatal("unexpected endpoint")
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("invalid request")
		}
		wireMessages, _ := json.Marshal(body["messages"])
		if err := json.Unmarshal(wireMessages, &sentMessages); err != nil {
			t.Fatal(err)
		}
		if body["store"] != false || body["response_format"].(map[string]any)["type"] != "json_schema" {
			t.Fatal("structured outputs configuration")
		}
		reply := map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": `{"facts":[{"field":"ficat","value":"omogen","state":"present","chunk_id":"c1","quote":"ficat: omogen"}]}`}}}, "usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 50}}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(toJSON(reply))), Header: http.Header{}}, nil
	})}
	tx, e := m.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	chunks := []Chunk{{ID: "c1", Revision: 1, Sequence: 1, Text: "ficat: omogen"}}
	facts, e := m.extract(ctx, tx, p, testTemplate(), chunks)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = buildResult(testTemplate(), chunks, facts, "s", 1); e != nil {
		t.Fatal(e)
	}
	// Simulate a later validation failure/rollback: billed usage must remain.
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	var count int
	var cost float64
	if e = m.db.QueryRow(ctx, `SELECT count(*),sum(cost_usd) FROM wai_usage_records WHERE tenant_id=$1`, p.Tenant).Scan(&count, &cost); e != nil || count != 1 || cost <= 0 {
		t.Fatal("usage erased by rollback", count, cost, e)
	}
	var usageID string
	if e = m.db.QueryRow(ctx, `SELECT id FROM wai_usage_records WHERE tenant_id=$1`, p.Tenant).Scan(&usageID); e != nil {
		t.Fatal(e)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/billing/usage/prompt?id="+usageID, nil)
	denied := httptest.NewRecorder()
	m.billing(denied, request, p, "billing/usage/prompt")
	if denied.Code != http.StatusForbidden {
		t.Fatalf("non-admin processing detail access returned %d", denied.Code)
	}
	admin := p
	admin.InstanceAdmin = true
	view := httptest.NewRecorder()
	m.billing(view, request, admin, "billing/usage/prompt")
	var captured struct {
		Messages []promptMessage
		Output   *promptOutput
	}
	if view.Code != http.StatusOK || json.Unmarshal(view.Body.Bytes(), &captured) != nil || !reflect.DeepEqual(captured.Messages, sentMessages) {
		t.Fatal("admin prompt differs from the messages sent to the provider")
	}
	if captured.Output == nil || captured.Output.Finish != "stop" || !strings.Contains(captured.Output.Content, "ficat: omogen") {
		t.Fatal("admin details omitted model output")
	}
	if e = m.db.QueryRow(ctx, `SELECT count(*) FROM wai_audit_events WHERE action='billing.usage.prompt.view' AND entity_id=$1`, usageID).Scan(&count); e != nil || count != 1 {
		t.Fatal("processing detail access was not audited", e)
	}
	m.cfg.MonthlyBudget = cost / 2
	tx, e = m.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = m.extract(ctx, tx, p, testTemplate(), chunks); e == nil || e.Error() != "monthly_budget_exceeded" || calls != 1 {
		t.Fatal("budget did not prevent API call", e, calls)
	}
}

func TestPostgresUnknownOpenAICostRemainsReserved(t *testing.T) {
	m, _ := integrationModule(t)
	ctx := context.Background()
	var p principal
	if e := m.db.QueryRow(ctx, `SELECT user_id,tenant_id FROM wai_memberships LIMIT 1`).Scan(&p.User, &p.Tenant); e != nil {
		t.Fatal(e)
	}
	m.cfg.Provider = "openai"
	m.cfg.OpenAIKeyEnv = "WAI_TEST_UNCERTAIN_KEY"
	t.Setenv(m.cfg.OpenAIKeyEnv, "synthetic-key")
	m.client = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("synthetic connection interrupted")
	})}
	tx, e := m.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = m.extract(ctx, tx, p, testTemplate(), nil); e == nil {
		t.Fatal("expected transport failure")
	}
	var status string
	var cost float64
	if e = m.db.QueryRow(ctx, `SELECT status,cost_usd FROM wai_usage_records WHERE tenant_id=$1`, p.Tenant).Scan(&status, &cost); e != nil || status != "uncertain" || cost <= 0 {
		t.Fatal("uncertain cost erased", status, cost, e)
	}
}
