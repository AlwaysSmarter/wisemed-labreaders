package wisemedai

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Statements are internal receivables, not statutory/e-Factura documents.
type SubscriptionState struct {
	ReservedCreditMinor int64     `json:"reserved_credit_minor"`
	ID                  string    `json:"id"`
	PeriodID            string    `json:"period_id"`
	PeriodStart         time.Time `json:"period_start"`
	PeriodEnd           time.Time `json:"period_end"`
	Currency            string    `json:"currency"`
	OutstandingMinor    int64     `json:"outstanding_minor"`
	UnbilledMinor       int64     `json:"unbilled_usage_minor"`
	CreditUsedMinor     int64     `json:"credit_used_minor"`
	CreditLimitMinor    int64     `json:"credit_limit_minor"`
	OverdueMinor        int64     `json:"overdue_minor"`
	Suspended           bool      `json:"suspended"`
	Reason              string    `json:"reason"`
	GraceDays           int       `json:"grace_days"`
}

func monthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}
func overageMinor(tokens, included, rate int64) (int64, error) {
	if tokens <= included || rate == 0 {
		return 0, nil
	}
	n := new(big.Int).Mul(big.NewInt(tokens-included), big.NewInt(rate))
	n.Add(n, big.NewInt(999999))
	n.Quo(n, big.NewInt(1000000))
	if !n.IsInt64() {
		return 0, errors.New("commercial_amount_overflow")
	}
	return n.Int64(), nil
}
func effectiveCredit(ctx context.Context, tx pgx.Tx, user string, plan ServicePlan) (int64, error) {
	var override *int64
	e := tx.QueryRow(ctx, `SELECT credit_limit_minor FROM wai_plan_assignments WHERE user_id=$1`, user).Scan(&override)
	if e != nil && e != pgx.ErrNoRows {
		return 0, e
	}
	if override != nil {
		return *override, nil
	}
	return plan.CreditLimitMinor, nil
}
func insertInvoice(ctx context.Context, tx pgx.Tx, user, period, kind, currency string, amount int64, issued time.Time) error {
	if amount <= 0 {
		return nil
	}
	_, e := tx.Exec(ctx, `INSERT INTO wai_billing_invoices(id,user_id,period_id,kind,currency,amount_minor,issued_at,due_at,reference)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, uuid.NewString(), user, period, kind, currency, amount, issued, issued.AddDate(0, 0, 5), "WAI-"+uuid.NewString())
	return e
}
func periodUsage(ctx context.Context, tx pgx.Tx, period string) (actual, reserved int64, e error) {
	e = tx.QueryRow(ctx, `SELECT coalesce(sum(input_tokens::bigint+output_tokens) FILTER(WHERE status='completed'),0),coalesce(sum(reserved_tokens) FILTER(WHERE status IN ('reserved','uncertain')),0) FROM wai_usage_records WHERE billing_period_id=$1`, period).Scan(&actual, &reserved)
	return
}
func unbilledUsage(ctx context.Context, tx pgx.Tx, period string, plan ServicePlan) (int64, error) {
	actual, _, e := periodUsage(ctx, tx, period)
	if e != nil {
		return 0, e
	}
	total, e := overageMinor(actual, plan.IncludedTokens, plan.ExtraMillionMinor)
	if e != nil {
		return 0, e
	}
	var billed int64
	e = tx.QueryRow(ctx, `SELECT coalesce(sum(amount_minor),0) FROM wai_billing_invoices WHERE period_id=$1 AND kind='usage'`, period).Scan(&billed)
	if e != nil {
		return 0, e
	}
	if total < billed {
		return 0, errors.New("usage_reconciliation_conflict")
	}
	return total - billed, nil
}

// Caller holds the global user billing lock. Repeat calls and multiple workers are idempotent.
func syncSubscription(ctx context.Context, tx pgx.Tx, user string, now time.Time) (*SubscriptionState, error) {
	var id string
	var started time.Time
	e := tx.QueryRow(ctx, `SELECT id,started_at FROM wai_subscriptions WHERE user_id=$1 AND status='active'`, user).Scan(&id, &started)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	plan, e := currentPlan(ctx, tx, user)
	if e != nil {
		return nil, e
	}
	state := &SubscriptionState{ID: id, Currency: plan.Currency, GraceDays: 5}
	state.CreditLimitMinor, e = effectiveCredit(ctx, tx, user, plan)
	if e != nil {
		return nil, e
	}
	// The first period starts on activation; all subsequent periods follow calendar UTC.
	first := monthStart(started)
	for at := first; !at.After(monthStart(now)); at = at.AddDate(0, 1, 0) {
		end := at.AddDate(0, 1, 0)
		issue := at
		if started.After(issue) {
			issue = started
		}
		period := uuid.NewString()
		e = tx.QueryRow(ctx, `INSERT INTO wai_billing_periods(id,subscription_id,starts_at,ends_at)VALUES($1,$2,$3,$4) ON CONFLICT(subscription_id,starts_at) DO UPDATE SET starts_at=excluded.starts_at RETURNING id`, period, id, at, end).Scan(&period)
		if e != nil {
			return nil, e
		}
		var exists bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wai_billing_invoices WHERE period_id=$1 AND kind='subscription')`, period).Scan(&exists); e != nil {
			return nil, e
		}
		if !exists {
			if e = insertInvoice(ctx, tx, user, period, "subscription", plan.Currency, plan.MonthlyPriceMinor, issue); e != nil {
				return nil, e
			}
		}
		actual, reserved, e := periodUsage(ctx, tx, period)
		if e != nil {
			return nil, e
		}
		actualCost, e := overageMinor(actual, plan.IncludedTokens, plan.ExtraMillionMinor)
		if e != nil {
			return nil, e
		}
		reservedCost, e := overageMinor(actual+reserved, plan.IncludedTokens, plan.ExtraMillionMinor)
		if e != nil {
			return nil, e
		}
		state.ReservedCreditMinor += reservedCost - actualCost
		delta, e := unbilledUsage(ctx, tx, period, plan)
		if e != nil {
			return nil, e
		}
		if !now.Before(end) {
			if e = insertInvoice(ctx, tx, user, period, "usage", plan.Currency, delta, now); e != nil {
				return nil, e
			}
		} else {
			state.PeriodID = period
			state.PeriodStart = at
			state.PeriodEnd = end
			state.UnbilledMinor = delta
		}
	}
	e = tx.QueryRow(ctx, `SELECT coalesce(sum(i.amount_minor-coalesce(p.paid,0)),0),coalesce(sum(i.amount_minor-coalesce(p.paid,0)) FILTER(WHERE i.due_at<=$2),0),coalesce(sum(i.amount_minor-coalesce(p.paid,0)) FILTER(WHERE i.kind='usage'),0) FROM wai_billing_invoices i LEFT JOIN LATERAL(SELECT sum(amount_minor) paid FROM wai_billing_payments WHERE invoice_id=i.id)p ON true WHERE i.user_id=$1 AND i.period_id IS NOT NULL`, user, now).Scan(&state.OutstandingMinor, &state.OverdueMinor, &state.CreditUsedMinor)
	if e != nil {
		return nil, e
	}
	state.CreditUsedMinor += state.UnbilledMinor
	if state.OverdueMinor > 0 {
		state.Suspended = true
		state.Reason = "subscription_payment_overdue"
	} else if state.CreditUsedMinor > 0 && state.CreditUsedMinor >= state.CreditLimitMinor {
		state.Suspended = true
		state.Reason = "credit_limit_exhausted"
	}
	return state, nil
}
func (m *Module) subscriptionState(ctx context.Context, user string) (*SubscriptionState, error) {
	tx, e := m.db.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if e = lockBillingUser(ctx, tx, user); e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	if e = syncTemplateAddons(ctx, tx, user, now); e != nil {
		return nil, e
	}
	state, e := syncSubscription(ctx, tx, user, now)
	if e != nil {
		return nil, e
	}
	return state, tx.Commit(ctx)
}
func (m *Module) subscriptionWorker(ctx context.Context) {
	rows, e := m.db.Query(ctx, `SELECT user_id::text FROM wai_subscriptions WHERE status='active' UNION SELECT user_id::text FROM wai_template_addons WHERE ends_at IS NULL OR ends_at>now()`)
	if e != nil {
		return
	}
	users := []string{}
	for rows.Next() {
		var u string
		if rows.Scan(&u) == nil {
			users = append(users, u)
		}
	}
	rows.Close()
	for _, u := range users {
		if ctx.Err() != nil {
			return
		}
		if _, e = m.subscriptionState(ctx, u); e != nil {
			m.rt.Logf("wisemed-ai: subscription reconciliation failed")
		}
	}
}
func (m *Module) subscriptions(w http.ResponseWriter, r *http.Request, p principal) {
	if r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		User        string `json:"user_id"`
		PlanVersion string `json:"plan_version"`
		Confirmed   bool   `json:"confirmed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirmed {
		fail(w, 400, "subscription_confirmation_required")
		return
	}
	if in.User == "" {
		in.User = p.User
	}
	if in.User != p.User && !p.InstanceAdmin {
		fail(w, 403, "super_admin_required")
		return
	}
	ctx := r.Context()
	tx, e := m.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	if e = lockBillingUser(ctx, tx, in.User); e != nil {
		fail(w, 503, "database_error")
		return
	}
	plan, e := planFromRow(tx.QueryRow(ctx, planSelect+` WHERE v.id::text=$1 AND v.id=p.current_version AND v.active AND p.code<>'free'`, in.PlanVersion))
	if e != nil {
		fail(w, 400, "active_paid_plan_required")
		return
	}
	var exists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wai_subscriptions WHERE user_id::text=$1)`, in.User).Scan(&exists); e != nil {
		fail(w, 503, "database_error")
		return
	}
	if exists {
		fail(w, 409, "subscription_already_exists")
		return
	}
	_, e = tx.Exec(ctx, `INSERT INTO wai_subscriptions(id,user_id,plan_version,started_at,created_by)VALUES($1,$2,$3,$4,$5)`, uuid.NewString(), in.User, plan.VersionID, time.Now().UTC(), p.User)
	if e == nil {
		_, e = tx.Exec(ctx, `INSERT INTO wai_plan_assignments(user_id,plan_version,assigned_by,note)VALUES($1,$2,$3,'Monthly subscription') ON CONFLICT(user_id) DO UPDATE SET plan_version=excluded.plan_version,assigned_by=excluded.assigned_by,assigned_at=now(),credit_limit_minor=NULL,note=excluded.note`, in.User, plan.VersionID, p.User)
	}
	var state *SubscriptionState
	if e == nil {
		state, e = syncSubscription(ctx, tx, in.User, time.Now().UTC())
	}
	if e == nil {
		e = audit(ctx, tx, p, "billing.subscription.start", in.User)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 503, "subscription_creation_failed")
		return
	}
	respond(w, 201, map[string]any{"subscription": state, "payment_collected": false})
}
func (m *Module) billingInvoices(w http.ResponseWriter, r *http.Request, p principal) {
	if r.Method != "GET" {
		fail(w, 405, "method_not_allowed")
		return
	}
	user := r.URL.Query().Get("user_id")
	if user == "" {
		user = p.User
	}
	if user != p.User && !p.InstanceAdmin {
		fail(w, 403, "super_admin_required")
		return
	}
	if _, e := m.subscriptionState(r.Context(), user); e != nil {
		fail(w, 503, "billing_unavailable")
		return
	}
	rows, e := m.db.Query(r.Context(), `SELECT i.id,i.reference,i.kind,i.currency,i.amount_minor,i.issued_at,i.due_at,coalesce((SELECT sum(amount_minor) FROM wai_billing_payments WHERE invoice_id=i.id),0),coalesce(b.starts_at,i.addon_month),coalesce(b.ends_at,((i.addon_month AT TIME ZONE 'UTC')+interval '1 month') AT TIME ZONE 'UTC'),coalesce(t.name,'') FROM wai_billing_invoices i LEFT JOIN wai_billing_periods b ON b.id=i.period_id LEFT JOIN wai_templates t ON t.id=i.template_addon WHERE i.user_id::text=$1 ORDER BY i.issued_at DESC,i.id LIMIT 200`, user)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, ref, kind, currency, templateName string
		var amount, paid int64
		var issued, due, start, end time.Time
		if e = rows.Scan(&id, &ref, &kind, &currency, &amount, &issued, &due, &paid, &start, &end, &templateName); e != nil {
			fail(w, 503, "database_error")
			return
		}
		out = append(out, map[string]any{"id": id, "reference": ref, "kind": kind, "currency": currency, "amount_minor": amount, "paid_minor": paid, "remaining_minor": amount - paid, "issued_at": issued, "due_at": due, "period_start": start, "period_end": end, "template_name": templateName})
	}
	if rows.Err() != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 200, out)
}

// Finalize accrued usage on demand so the current credit balance can be paid before renewal.
func (m *Module) settleUsage(w http.ResponseWriter, r *http.Request, p principal) {
	if r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		User      string `json:"user_id"`
		Confirmed bool   `json:"confirmed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirmed {
		fail(w, 400, "confirmation_required")
		return
	}
	if in.User == "" {
		in.User = p.User
	}
	if in.User != p.User && !p.InstanceAdmin {
		fail(w, 403, "super_admin_required")
		return
	}
	ctx := r.Context()
	tx, e := m.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	if e = lockBillingUser(ctx, tx, in.User); e != nil {
		fail(w, 503, "database_error")
		return
	}
	state, e := syncSubscription(ctx, tx, in.User, time.Now().UTC())
	if e != nil || state == nil {
		fail(w, 400, "subscription_required")
		return
	}
	if e = insertInvoice(ctx, tx, in.User, state.PeriodID, "usage", state.Currency, state.UnbilledMinor, time.Now().UTC()); e == nil {
		e = audit(ctx, tx, p, "billing.usage.settle", in.User)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
func (m *Module) billingPayment(w http.ResponseWriter, r *http.Request, p principal) {
	if !p.InstanceAdmin {
		fail(w, 403, "super_admin_required")
		return
	}
	if r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		Invoice   string `json:"invoice_id"`
		Amount    int64  `json:"amount_minor"`
		Reference string `json:"reference"`
		Confirmed bool   `json:"confirmed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirmed || in.Amount <= 0 || in.Amount > 1e12 || len(in.Reference) < 3 || len(in.Reference) > 200 {
		fail(w, 400, "verified_payment_required")
		return
	}
	ctx := r.Context()
	tx, e := m.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	var user string
	if e = tx.QueryRow(ctx, `SELECT user_id::text FROM wai_billing_invoices WHERE id::text=$1`, in.Invoice).Scan(&user); e != nil {
		fail(w, 404, "invoice_not_found")
		return
	}
	if e = lockBillingUser(ctx, tx, user); e != nil {
		fail(w, 503, "database_error")
		return
	}
	var oldInvoice string
	var oldAmount int64
	e = tx.QueryRow(ctx, `SELECT invoice_id::text,amount_minor FROM wai_billing_payments WHERE reference=$1`, in.Reference).Scan(&oldInvoice, &oldAmount)
	if e == nil {
		if oldInvoice != in.Invoice || oldAmount != in.Amount {
			fail(w, 409, "payment_reference_conflict")
			return
		}
		respond(w, 200, map[string]bool{"ok": true, "duplicate": true})
		return
	}
	if e != pgx.ErrNoRows {
		fail(w, 503, "database_error")
		return
	}
	var remaining int64
	if e = tx.QueryRow(ctx, `SELECT amount_minor-coalesce((SELECT sum(amount_minor) FROM wai_billing_payments WHERE invoice_id=i.id),0) FROM wai_billing_invoices i WHERE id::text=$1`, in.Invoice).Scan(&remaining); e != nil {
		fail(w, 503, "database_error")
		return
	}
	if in.Amount > remaining {
		fail(w, 409, "payment_exceeds_remaining_balance")
		return
	}
	_, e = tx.Exec(ctx, `INSERT INTO wai_billing_payments(id,invoice_id,amount_minor,reference,recorded_by)VALUES($1,$2,$3,$4,$5)`, uuid.NewString(), in.Invoice, in.Amount, in.Reference, p.User)
	if e == nil {
		e = audit(ctx, tx, p, "billing.payment.verified", in.Invoice)
	}
	var state *SubscriptionState
	if e == nil {
		state, e = syncSubscription(ctx, tx, user, time.Now().UTC())
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 409, "payment_not_recorded")
		return
	}
	respond(w, 200, map[string]any{"ok": true, "subscription": state})
}
