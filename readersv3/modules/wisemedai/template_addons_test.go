package wisemedai

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPostgresPaidTemplateConfirmationAndRenewal(t *testing.T) {
	m, rt := integrationModule(t)
	ctx := context.Background()
	password, err := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	if err != nil {
		t.Fatal(err)
	}
	code, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(password)})
	if code != 200 {
		t.Fatal(code)
	}
	user, token := createBillingUser(t, m, out["token"].(string), "addons@example.invalid", "doctor")
	selectTestSpecialties(t, m, token, "Neurologie")
	var plan string
	err = m.db.QueryRow(ctx, `UPDATE wai_billing_plan_versions SET max_templates=0,extras_enabled=true,extra_template_price_minor=750
		WHERE id=(SELECT current_version FROM wai_billing_plans WHERE code='free') RETURNING id`).Scan(&plan)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"name": "Șablon suplimentar de test", "specialty": "Neurologie",
		"html": "<p>{{examen}}</p>", "fields": []Field{{Key: "examen", Label: "Examen", Type: "string"}},
	}
	code, _ = request(t, m, token, "POST", "templates", payload)
	if code != 402 {
		t.Fatal("publication without consent", code)
	}
	confirmation := &templateAddonConfirmation{PlanVersion: plan, MonthlyMinor: 500, RequestID: uuid.NewString()}
	payload["addon_confirmation"] = confirmation
	code, _ = request(t, m, token, "POST", "templates", payload)
	if code != 409 {
		t.Fatal("stale quote accepted", code)
	}
	confirmation.MonthlyMinor = 750
	code, out = request(t, m, token, "POST", "templates", payload)
	if code != 201 || out["addon_monthly_minor"] != float64(750) {
		t.Fatal("paid publication", code, out)
	}
	id := out["id"].(string)
	code, _ = request(t, m, token, "POST", "templates", payload)
	if code != 409 {
		t.Fatal("duplicate confirmation accepted", code)
	}
	invoices := billingList(t, m, token, "billing/invoices")
	if len(invoices) != 1 || invoices[0]["kind"] != "template_addon" || invoices[0]["amount_minor"] != float64(750) {
		t.Fatal("first monthly charge", invoices)
	}
	next := monthStart(time.Now().UTC()).AddDate(0, 1, 0)
	checkRenewal := func(want int) {
		t.Helper()
		tx, err := m.db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err := lockBillingUser(ctx, tx, user); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := syncTemplateAddons(ctx, tx, user, next); err != nil {
				t.Fatal(err)
			}
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM wai_billing_invoices WHERE template_addon=$1`, id).Scan(&count); err != nil || count != want {
			t.Fatal("monthly charges", count, err)
		}
	}
	checkRenewal(2)
	code, _ = request(t, m, token, "DELETE", "billing/template-addons/"+id, nil)
	if code != 200 {
		t.Fatal("cancel renewal", code)
	}
	checkRenewal(1)
}
