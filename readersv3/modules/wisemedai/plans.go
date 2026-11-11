package wisemedai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ServicePlan struct {
	ID                  string `json:"id"`
	Code                string `json:"code"`
	VersionID           string `json:"version_id"`
	Version             int    `json:"version"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	Currency            string `json:"currency"`
	MonthlyPriceMinor   int64  `json:"monthly_price_minor"`
	MaxSpecialties      int    `json:"max_specialties"`
	MaxTemplates        int    `json:"max_templates"`
	MaxFields           int    `json:"max_fields_per_template"`
	IncludedTokens      int64  `json:"included_ai_tokens"`
	ExtraSpecialtyMinor int64  `json:"extra_specialty_price_minor"`
	ExtraTemplateMinor  int64  `json:"extra_template_price_minor"`
	ExtraMillionMinor   int64  `json:"extra_million_tokens_price_minor"`
	CreditLimitMinor    int64  `json:"credit_limit_minor"`
	ExtrasEnabled       bool   `json:"extras_enabled"`
	Active              bool   `json:"active"`
}

const planSelect = `SELECT jsonb_build_object('id',p.id,'code',p.code,'version_id',v.id,'version',v.version,'name',v.name,'description',v.description,'currency',v.currency,'monthly_price_minor',v.monthly_price_minor,'max_specialties',v.max_specialties,'max_templates',v.max_templates,'max_fields_per_template',v.max_fields_per_template,'included_ai_tokens',v.included_ai_tokens,'extra_specialty_price_minor',v.extra_specialty_price_minor,'extra_template_price_minor',v.extra_template_price_minor,'extra_million_tokens_price_minor',v.extra_million_tokens_price_minor,'credit_limit_minor',v.credit_limit_minor,'extras_enabled',v.extras_enabled,'active',v.active) FROM wai_billing_plan_versions v JOIN wai_billing_plans p ON p.id=v.plan_id `

var planCode = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,39}$`)

func (plan ServicePlan) validate() error {
	if !planCode.MatchString(plan.Code) || len(strings.TrimSpace(plan.Name)) < 2 || len(plan.Name) > 100 || len(plan.Description) > 2000 || plan.MaxSpecialties < 1 || plan.MaxSpecialties > 10000 || plan.MaxTemplates < 0 || plan.MaxTemplates > 1000000 || plan.MaxFields < 1 || plan.MaxFields > 150 || plan.IncludedTokens < 0 || plan.IncludedTokens > 1e12 {
		return errors.New("invalid_plan_limits")
	}
	if plan.Currency != "USD" && plan.Currency != "EUR" && plan.Currency != "RON" {
		return errors.New("invalid_currency")
	}
	for _, price := range []int64{plan.MonthlyPriceMinor, plan.ExtraSpecialtyMinor, plan.ExtraTemplateMinor, plan.ExtraMillionMinor, plan.CreditLimitMinor} {
		if price < 0 || price > 1e12 {
			return errors.New("invalid_plan_price")
		}
	}
	if plan.Code == "free" && (!plan.Active || plan.MonthlyPriceMinor != 0) {
		return errors.New("free_must_remain_active_and_free")
	}
	if plan.Code != "free" && plan.Active && plan.MonthlyPriceMinor <= 0 {
		return errors.New("paid_plan_requires_monthly_price")
	}
	return nil
}
func planFromRow(row pgx.Row) (plan ServicePlan, e error) {
	var raw []byte
	e = row.Scan(&raw)
	if e == nil {
		e = json.Unmarshal(raw, &plan)
	}
	return
}
func currentPlan(ctx context.Context, tx pgx.Tx, user string) (ServicePlan, error) {
	return planFromRow(tx.QueryRow(ctx, planSelect+` WHERE v.id=coalesce((SELECT plan_version FROM wai_plan_assignments WHERE user_id=$1),(SELECT current_version FROM wai_billing_plans WHERE code='free'))`, user))
}
func lockBillingUser(ctx context.Context, tx pgx.Tx, user string) error {
	_, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "wai-billing-user:"+user)
	return e
}
func (m *Module) billing(w http.ResponseWriter, r *http.Request, p principal, path string) {
	switch path {
	case "billing/statistics":
		m.billingStatistics(w, r, p)
	case "billing/status":
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
		state, e := m.subscriptionState(r.Context(), user)
		if e != nil {
			fail(w, 503, "billing_unavailable")
			return
		}
		respond(w, 200, map[string]any{"subscription": state})
	case "billing/subscriptions":
		m.subscriptions(w, r, p)
	case "billing/invoices":
		m.billingInvoices(w, r, p)
	case "billing/payments":
		m.billingPayment(w, r, p)
	case "billing/settle":
		m.settleUsage(w, r, p)
	case "billing/plans":
		m.planCatalog(w, r, p)
	case "billing/account":
		m.billingAccount(w, r, p)
	case "billing/specialties":
		m.billingSpecialties(w, r, p)
	case "billing/users/password":
		m.billingUserPassword(w, r, p)
	case "billing/users/role":
		m.billingUserRole(w, r, p)
	case "billing/usage/prompt":
		m.billingUsagePrompt(w, r, p)
	case "billing/usage":
		m.billingUsage(w, r, p)
	case "billing/users":
		m.billingUsers(w, r, p)
	case "billing/assignments":
		m.assignPlan(w, r, p)
	default:
		fail(w, 404, "not_found")
	}
}
func (m *Module) planCatalog(w http.ResponseWriter, r *http.Request, p principal) {
	ctx := r.Context()
	if r.Method == "GET" {
		rows, e := m.db.Query(ctx, planSelect+` WHERE v.id=p.current_version AND ($1 OR v.active) ORDER BY CASE p.code WHEN 'free' THEN 0 WHEN 'pro' THEN 1 WHEN 'premium' THEN 2 ELSE 3 END,v.monthly_price_minor`, p.InstanceAdmin)
		if e != nil {
			fail(w, 503, "database_error")
			return
		}
		defer rows.Close()
		out := []ServicePlan{}
		for rows.Next() {
			plan, e := planFromRow(rows)
			if e != nil {
				fail(w, 500, "invalid_plan_data")
				return
			}
			out = append(out, plan)
		}
		if rows.Err() != nil {
			fail(w, 503, "database_error")
			return
		}
		respond(w, 200, out)
		return
	}
	if !p.InstanceAdmin {
		fail(w, 403, "super_admin_required")
		return
	}
	if r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var plan ServicePlan
	if !decode(w, r, &plan) {
		return
	}
	plan.Code = strings.ToLower(strings.TrimSpace(plan.Code))
	if e := plan.validate(); e != nil {
		fail(w, 400, e.Error())
		return
	}
	tx, e := m.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7741921107)`); e != nil {
		fail(w, 503, "database_error")
		return
	}
	if plan.ID == "" {
		plan.ID = uuid.NewString()
		if _, e = tx.Exec(ctx, `INSERT INTO wai_billing_plans(id,code)VALUES($1,$2)`, plan.ID, plan.Code); e != nil {
			fail(w, 409, "plan_code_exists")
			return
		}
	}
	var code string
	var currentVersion string
	if e = tx.QueryRow(ctx, `SELECT code,coalesce(current_version::text,'') FROM wai_billing_plans WHERE id::text=$1 FOR UPDATE`, plan.ID).Scan(&code, &currentVersion); e != nil {
		fail(w, 404, "plan_not_found")
		return
	}
	if code != plan.Code {
		fail(w, 400, "plan_code_immutable")
		return
	}
	if currentVersion != "" && plan.VersionID != currentVersion {
		fail(w, 409, "plan_changed_reload_before_saving")
		return
	}
	if e = tx.QueryRow(ctx, `SELECT coalesce(max(version),0)+1 FROM wai_billing_plan_versions WHERE plan_id=$1`, plan.ID).Scan(&plan.Version); e != nil {
		fail(w, 503, "database_error")
		return
	}
	plan.VersionID = uuid.NewString()
	_, e = tx.Exec(ctx, `INSERT INTO wai_billing_plan_versions(id,plan_id,version,name,description,currency,monthly_price_minor,max_specialties,max_templates,max_fields_per_template,included_ai_tokens,extra_specialty_price_minor,extra_template_price_minor,extra_million_tokens_price_minor,credit_limit_minor,extras_enabled,active,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, plan.VersionID, plan.ID, plan.Version, plan.Name, plan.Description, plan.Currency, plan.MonthlyPriceMinor, plan.MaxSpecialties, plan.MaxTemplates, plan.MaxFields, plan.IncludedTokens, plan.ExtraSpecialtyMinor, plan.ExtraTemplateMinor, plan.ExtraMillionMinor, plan.CreditLimitMinor, plan.ExtrasEnabled, plan.Active, p.User)
	if e == nil {
		_, e = tx.Exec(ctx, `UPDATE wai_billing_plans SET current_version=$1 WHERE id=$2`, plan.VersionID, plan.ID)
	}
	if e == nil {
		e = audit(ctx, tx, p, "billing.plan.publish", plan.VersionID)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 201, plan)
}
func (m *Module) assignPlan(w http.ResponseWriter, r *http.Request, p principal) {
	if !p.InstanceAdmin {
		fail(w, 403, "super_admin_required")
		return
	}
	if r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		User        string `json:"user_id"`
		PlanVersion string `json:"plan_version"`
		CreditLimit *int64 `json:"credit_limit_minor"`
		Note        string `json:"note"`
		Confirmed   bool   `json:"confirmed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirmed || len(in.Note) > 500 || (in.CreditLimit != nil && (*in.CreditLimit < 0 || *in.CreditLimit > 1e12)) {
		fail(w, 400, "invalid_assignment")
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
	var subscribed bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wai_subscriptions WHERE user_id::text=$1 AND status='active')`, in.User).Scan(&subscribed); e != nil {
		fail(w, 503, "database_error")
		return
	}
	if subscribed {
		fail(w, 409, "active_subscription_assignment_protected")
		return
	}
	plan, e := planFromRow(tx.QueryRow(ctx, planSelect+` WHERE v.id::text=$1 AND v.id=p.current_version AND v.active`, in.PlanVersion))
	if e != nil {
		fail(w, 400, "active_current_plan_required")
		return
	}
	_, e = tx.Exec(ctx, `INSERT INTO wai_plan_assignments(user_id,plan_version,assigned_by,credit_limit_minor,note)VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id) DO UPDATE SET plan_version=excluded.plan_version,assigned_by=excluded.assigned_by,assigned_at=now(),credit_limit_minor=excluded.credit_limit_minor,note=excluded.note`, in.User, plan.VersionID, p.User, in.CreditLimit, in.Note)
	if e == nil {
		e = audit(ctx, tx, p, "billing.plan.assign", in.User)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 400, "invalid_user_or_plan")
		return
	}
	respond(w, 200, map[string]any{"ok": true, "plan": plan, "activation": "manual", "payment_collected": false})
}
func (m *Module) billingAccount(w http.ResponseWriter, r *http.Request, p principal) {
	if r.Method != "GET" {
		fail(w, 405, "method_not_allowed")
		return
	}
	ctx := r.Context()
	subscription, e := m.subscriptionState(ctx, p.User)
	if e != nil {
		fail(w, 503, "billing_unavailable")
		return
	}
	tx, e := m.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	plan, e := currentPlan(ctx, tx, p.User)
	if e != nil {
		fail(w, 503, "package_unavailable")
		return
	}
	var templates int
	var input, output, reserved int64
	var providerCost float64
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM wai_templates t WHERE owner_id=$1 AND NOT EXISTS (SELECT 1 FROM wai_template_addons a WHERE a.template_id=t.id)`, p.User).Scan(&templates); e != nil {
		fail(w, 503, "database_error")
		return
	}
	var billingPeriod any
	if subscription != nil {
		billingPeriod = subscription.PeriodID
	}
	e = tx.QueryRow(ctx, `SELECT coalesce(sum(input_tokens) FILTER(WHERE status='completed'),0),coalesce(sum(output_tokens) FILTER(WHERE status='completed'),0),coalesce(sum(reserved_tokens) FILTER(WHERE status IN ('reserved','uncertain')),0),coalesce(sum(cost_usd),0) FROM wai_usage_records WHERE user_id=$1 AND created_at>=date_trunc('month',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AND ($2::uuid IS NULL OR billing_period_id=$2)`, p.User, billingPeriod).Scan(&input, &output, &reserved, &providerCost)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	selected := []string{}
	rows, e := tx.Query(ctx, `SELECT specialty FROM wai_user_specialties WHERE user_id=$1 ORDER BY specialty`, p.User)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	for rows.Next() {
		var s string
		if rows.Scan(&s) != nil {
			rows.Close()
			fail(w, 503, "database_error")
			return
		}
		selected = append(selected, s)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	available := []string{}
	rows, e = tx.Query(ctx, `SELECT DISTINCT name FROM wai_specialties WHERE tenant_id=$1 ORDER BY name`, p.Tenant)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	for rows.Next() {
		var s string
		if rows.Scan(&s) != nil {
			rows.Close()
			fail(w, 503, "database_error")
			return
		}
		available = append(available, s)
	}
	rows.Close()
	var addons json.RawMessage
	if e = tx.QueryRow(ctx, `SELECT coalesce(jsonb_agg(jsonb_build_object('template_id',a.template_id,'name',t.name,'monthly_minor',a.monthly_minor,'currency',a.currency,'ends_at',a.ends_at) ORDER BY t.name),'[]')
		FROM wai_template_addons a JOIN wai_templates t ON t.id=a.template_id
		WHERE a.user_id=$1 AND t.tenant_id=$2 AND (a.ends_at IS NULL OR a.ends_at>now())`, p.User, p.Tenant).Scan(&addons); e != nil {
		fail(w, 503, "database_error")
		return
	}
	var credit *int64
	_ = tx.QueryRow(ctx, `SELECT credit_limit_minor FROM wai_plan_assignments WHERE user_id=$1`, p.User).Scan(&credit)
	limit := plan.CreditLimitMinor
	if credit != nil {
		limit = *credit
	}
	respond(w, 200, map[string]any{"plan": plan, "user_type": userType(p.InstanceAdmin, p.Role), "selected_specialties": selected, "available_specialties": available, "templates_used": templates, "template_addons": addons, "input_tokens": input, "output_tokens": output, "total_tokens": input + output, "reserved_tokens": reserved, "provider_cost_usd": providerCost, "credit_limit_minor": limit, "quota_exempt": p.InstanceAdmin, "period": "calendar_month_UTC", "online_payments_enabled": false, "subscription": subscription, "subscriptions_enabled": true, "credit_spending_enabled": subscription != nil, "payment_provider": "euplatesc", "as_of": time.Now().UTC()})
}
func (m *Module) billingUsers(w http.ResponseWriter, r *http.Request, p principal) {
	if !p.InstanceAdmin {
		fail(w, 403, "super_admin_required")
		return
	}
	if r.Method != "GET" {
		fail(w, 405, "method_not_allowed")
		return
	}
	rows, e := m.db.Query(r.Context(), `SELECT u.id,u.email,u.name,u.instance_admin,u.disabled,coalesce(jsonb_agg(jsonb_build_object('tenant_id',m.tenant_id,'organization',o.name,'role',m.role,'disabled',m.disabled)) FILTER(WHERE m.id IS NOT NULL),'[]'),coalesce(a.plan_version,(SELECT current_version FROM wai_billing_plans WHERE code='free')),a.credit_limit_minor,a.assigned_at,(SELECT jsonb_build_object('name',pv.name,'version',pv.version,'plan_id',pv.plan_id,'currency',pv.currency,'monthly_price_minor',pv.monthly_price_minor) FROM wai_billing_plan_versions pv WHERE pv.id=coalesce(a.plan_version,(SELECT current_version FROM wai_billing_plans WHERE code='free'))) FROM wai_users u LEFT JOIN wai_memberships m ON m.user_id=u.id LEFT JOIN wai_organizations o ON o.id=m.tenant_id LEFT JOIN wai_plan_assignments a ON a.user_id=u.id GROUP BY u.id,a.plan_version,a.credit_limit_minor,a.assigned_at ORDER BY u.created_at DESC LIMIT 1000`)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, email, name, plan string
		var super, disabled bool
		var memberships, assignedPlan []byte
		var credit *int64
		var assigned *time.Time
		if rows.Scan(&id, &email, &name, &super, &disabled, &memberships, &plan, &credit, &assigned, &assignedPlan) != nil {
			fail(w, 500, "database_error")
			return
		}
		var ms, planInfo any
		if json.Unmarshal(memberships, &ms) != nil {
			fail(w, 500, "database_error")
			return
		}
		if json.Unmarshal(assignedPlan, &planInfo) != nil {
			fail(w, 500, "database_error")
			return
		}
		out = append(out, map[string]any{"plan": planInfo, "id": id, "email": email, "name": name, "super_admin": super, "disabled": disabled, "memberships": ms, "plan_version": plan, "credit_limit_minor": credit, "assigned_at": assigned})
	}
	if rows.Err() != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 200, out)
}
