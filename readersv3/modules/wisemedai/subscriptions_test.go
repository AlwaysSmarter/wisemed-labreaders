package wisemedai

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestCommercialRounding(t *testing.T) {
	for _, v := range []struct{ tokens, included, rate, want int64 }{{100, 100, 100, 0}, {101, 100, 100, 1}, {1000100, 100, 1999, 1999}, {1000101, 100, 1999, 2000}, {2000000000000, 0, 1000000000000, 2000000000000000000}} {
		got, e := overageMinor(v.tokens, v.included, v.rate)
		if e != nil || got != v.want {
			t.Fatal(v, got, e)
		}
	}
	if _, e := overageMinor(9000000000000000000, 0, 1000000000000); e == nil {
		t.Fatal("overflow accepted")
	}
}
func TestPostgresSubscriptionsGraceCreditAndPayments(t *testing.T) {
	m, rt := integrationModule(t)
	ctx := context.Background()
	password, _ := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	_, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(password)})
	super := out["token"].(string)
	uid, token := createBillingUser(t, m, super, "subscriber@example.invalid", "doctor")
	plans := billingList(t, m, super, "billing/plans")
	raw, _ := json.Marshal(plans[1])
	var plan ServicePlan
	json.Unmarshal(raw, &plan)
	plan.Active = true
	plan.MonthlyPriceMinor = 2000
	plan.IncludedTokens = 100
	plan.ExtraMillionMinor = 1000000
	plan.CreditLimitMinor = 1000
	plan.ExtrasEnabled = true
	code, out := request(t, m, super, "POST", "billing/plans", plan)
	if code != 201 {
		t.Fatal(code, out)
	}
	version := out["version_id"].(string)
	code, out = request(t, m, token, "POST", "billing/subscriptions", map[string]any{"plan_version": version, "confirmed": true})
	if code != 201 || out["payment_collected"] != false {
		t.Fatal(code, out)
	}
	code, _ = request(t, m, token, "POST", "billing/subscriptions", map[string]any{"plan_version": version, "confirmed": true})
	if code != 409 {
		t.Fatal(code)
	}
	code, _ = request(t, m, super, "POST", "billing/assignments", map[string]any{"user_id": uid, "plan_version": version, "confirmed": true})
	if code != 409 {
		t.Fatal("subscription terms overwritten", code)
	}
	invoices := billingList(t, m, token, "billing/invoices")
	if len(invoices) != 1 || invoices[0]["amount_minor"] != float64(2000) {
		t.Fatal(invoices)
	}
	feeID := invoices[0]["id"].(string)
	state, e := m.subscriptionState(ctx, uid)
	if e != nil || state.Suspended || state.OutstandingMinor != 2000 {
		t.Fatal(state, e)
	}
	// Exact five-day boundary, with deterministic time and no sleep.
	var started time.Time
	m.db.QueryRow(ctx, `SELECT started_at FROM wai_subscriptions WHERE user_id=$1`, uid).Scan(&started)
	tx, _ := m.db.Begin(ctx)
	lockBillingUser(ctx, tx, uid)
	state, e = syncSubscription(ctx, tx, uid, started.Add(5*24*time.Hour-time.Microsecond))
	if e != nil || state.Suspended {
		t.Fatal(state, e)
	}
	state, e = syncSubscription(ctx, tx, uid, started.Add(5*24*time.Hour))
	if e != nil || !state.Suspended || state.Reason != "subscription_payment_overdue" {
		t.Fatal(state, e)
	}
	tx.Rollback(ctx)
	state, e = m.subscriptionState(ctx, uid)
	if e != nil {
		t.Fatal(e)
	}
	// Complete actual usage exhausts exactly 10 USD of commercial credit.
	var tenant string
	m.db.QueryRow(ctx, `SELECT tenant_id::text FROM wai_memberships WHERE user_id=$1`, uid).Scan(&tenant)
	_, e = m.db.Exec(ctx, `INSERT INTO wai_usage_records(id,tenant_id,user_id,provider,model,input_tokens,output_tokens,cost_usd,status,reserved_tokens,plan_version,billing_period_id,operation)VALUES($1,$2,$3,'openai','synthetic',1000,100,0,'completed',0,$4,$5,'subscription_test')`, uuid.NewString(), tenant, uid, version, state.PeriodID)
	if e != nil {
		t.Fatal(e)
	}
	code, out = request(t, m, token, "GET", "dashboard", nil)
	if code != 402 || out["error"] != "credit_limit_exhausted" {
		t.Fatal(code, out)
	}
	code, out = request(t, m, token, "GET", "billing/account", nil)
	if code != 200 || !out["subscription"].(map[string]any)["suspended"].(bool) {
		t.Fatal(code, out)
	}
	code, _ = request(t, m, token, "POST", "billing/settle", map[string]any{"confirmed": true})
	if code != 200 {
		t.Fatal(code)
	}
	// Settlement is idempotent by its accrued delta.
	request(t, m, token, "POST", "billing/settle", map[string]any{"confirmed": true})
	invoices = billingList(t, m, token, "billing/invoices")
	if len(invoices) != 2 {
		t.Fatal(invoices)
	}
	var usageID string
	for _, i := range invoices {
		if i["kind"] == "usage" {
			usageID = i["id"].(string)
			if i["amount_minor"] != float64(1000) {
				t.Fatal(i)
			}
		}
	}
	payment := map[string]any{"invoice_id": usageID, "amount_minor": 1000, "reference": "synthetic-usage-payment", "confirmed": true}
	code, _ = request(t, m, token, "POST", "billing/payments", payment)
	if code != 403 {
		t.Fatal("user self-certified payment", code)
	}
	code, out = request(t, m, super, "POST", "billing/payments", payment)
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = request(t, m, super, "POST", "billing/payments", payment)
	if code != 200 || out["duplicate"] != true {
		t.Fatal(code, out)
	}
	state, e = m.subscriptionState(ctx, uid)
	if e != nil || state.Suspended || state.CreditUsedMinor != 0 {
		t.Fatal(state, e)
	}
	// Remaining monthly fee becomes overdue; partial payment does not reactivate.
	_, e = m.db.Exec(ctx, `UPDATE wai_billing_invoices SET due_at=now()-interval '1 second' WHERE id=$1`, feeID)
	if e != nil {
		t.Fatal(e)
	}
	code, out = request(t, m, token, "GET", "dashboard", nil)
	if code != 402 || out["error"] != "subscription_payment_overdue" {
		t.Fatal(code, out)
	}
	for n, amount := range []int{500, 1500} {
		code, out = request(t, m, super, "POST", "billing/payments", map[string]any{"invoice_id": feeID, "amount_minor": amount, "reference": []string{"synthetic-fee-part", "synthetic-fee-final"}[n], "confirmed": true})
		if code != 200 {
			t.Fatal(code, out)
		}
		state, e = m.subscriptionState(ctx, uid)
		if e != nil || state.Suspended != (n == 0) {
			t.Fatal(state, e)
		}
	}
	code, out = request(t, m, token, "GET", "dashboard", nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	// Concurrent conservative reservations consume available commercial credit.
	conn, e := pgx.ConnectConfig(ctx, m.db.Config().ConnConfig.Copy())
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close(ctx)
	p := principal{User: uid, Tenant: tenant, Role: "doctor"}
	startGate := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-startGate
			c, e := pgx.ConnectConfig(ctx, m.db.Config().ConnConfig.Copy())
			if e != nil {
				results <- e
				return
			}
			defer c.Close(ctx)
			results <- reserveTokens(ctx, c, p, uuid.NewString(), "synthetic", 600, 0, aiUsageContext{Operation: "subscription_reserve"})
		}()
	}
	close(startGate)
	accepted, denied := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			accepted++
		} else if err.Error() == "credit_reservation_exceeds_available" {
			denied++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 1 || denied != 1 {
		t.Fatal("concurrent credit overspend", accepted, denied)
	}
	e = reserveTokens(ctx, conn, p, uuid.NewString(), "synthetic", 500, 0, aiUsageContext{Operation: "subscription_reserve"})
	if e == nil || e.Error() != "credit_reservation_exceeds_available" {
		t.Fatal(e)
	}
	// Renewal on next calendar month is repeatable and preserves debt/reservations.
	next := monthStart(time.Now()).AddDate(0, 1, 0)
	tx, _ = m.db.Begin(ctx)
	defer tx.Rollback(ctx)
	lockBillingUser(ctx, tx, uid)
	state, e = syncSubscription(ctx, tx, uid, next)
	if e != nil || state.ReservedCreditMinor != 600 {
		t.Fatal(state, e)
	}
	state, e = syncSubscription(ctx, tx, uid, next)
	if e != nil {
		t.Fatal(e)
	}
	var count int
	tx.QueryRow(ctx, `SELECT count(*) FROM wai_billing_invoices WHERE user_id=$1 AND kind='subscription'`, uid).Scan(&count)
	if count != 2 {
		t.Fatal("duplicate renewal", count)
	}
}
