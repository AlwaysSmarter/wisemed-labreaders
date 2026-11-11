package wisemedai

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
)

// Opt-in browser verification uses a disposable PostgreSQL schema, never runtime users/payments.
func TestPostgresBrowserSubscriptionRecovery(t *testing.T) {
	if os.Getenv("WAI_BROWSER_TEST") != "1" {
		t.Skip("set WAI_BROWSER_TEST=1 with installed Chromium/frontend dependencies")
	}
	m, rt := integrationModule(t)
	password, _ := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	_, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(password)})
	super := out["token"].(string)
	uid, token := createBillingUser(t, m, super, "browser-billing@example.invalid", "doctor")
	plans := billingList(t, m, super, "billing/plans")
	plan := plans[1]
	plan["active"] = true
	plan["monthly_price_minor"] = 2000
	plan["extras_enabled"] = true
	plan["extra_million_tokens_price_minor"] = 1000
	plan["credit_limit_minor"] = 1000
	code, out := request(t, m, super, "POST", "billing/plans", plan)
	if code != 201 {
		t.Fatal(code, out)
	}
	code, out = request(t, m, token, "POST", "billing/subscriptions", map[string]any{"plan_version": out["version_id"], "confirmed": true})
	if code != 201 {
		t.Fatal(code, out)
	}
	if _, e := m.db.Exec(t.Context(), `UPDATE wai_billing_invoices SET due_at=now()-interval '1 second' WHERE user_id=$1`, uid); e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/", m.serve)
	sub, _ := fs.Sub(assets, "web/dist")
	mux.Handle("/ai/", http.StripPrefix("/ai/", http.FileServer(http.FS(sub))))
	server := httptest.NewServer(mux)
	defer server.Close()
	cmd := exec.CommandContext(t.Context(), "npm", "run", "test:e2e", "--", "tests/billing.e2e.ts", "tests/subscription.e2e.ts")
	cmd.Dir = "../../apps/wisemed-ai/web"
	cmd.Env = append(os.Environ(), "WAI_BASE_URL="+server.URL, "WAI_PASSWORD_FILE="+rt.ResolvePath(m.cfg.BootstrapPasswordFile), "WAI_SYNTHETIC_BILLING_USER=browser-billing@example.invalid")
	raw, e := cmd.CombinedOutput()
	t.Log(string(raw))
	if e != nil {
		t.Fatal("browser subscription verification failed")
	}
}
