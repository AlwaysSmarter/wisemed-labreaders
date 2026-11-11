package wisemedai

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func userType(super bool, role string) int {
	if super {
		return -1
	}
	if role == "admin" {
		return 0
	}
	return 1
}
func billingFailure(w http.ResponseWriter, e error) {
	status := 402
	switch e.Error() {
	case "template_not_owned":
		status = 403
	case "template_addon_quote_changed", "template_addon_already_added":
		status = 409
	case "unknown_specialty":
		status = 400
	case "specialty_limit_exceeded_select_allowed_specialties", "specialty_limit_reached", "specialty_not_selected", "template_complexity_limit_reached", "template_limit_reached", "template_addon_expired", "template_addon_payment_overdue", "ai_token_allowance_exceeded", "subscription_payment_overdue", "credit_limit_exhausted", "credit_reservation_exceeds_available":
	default:
		if e == pgx.ErrNoRows {
			fail(w, 404, "template_not_found")
		} else {
			fail(w, 503, "billing_unavailable")
		}
		return
	}
	respond(w, status, map[string]any{"error": e.Error(), "account_url": "/v1/billing/account"})
}
func requireActiveSpecialty(ctx context.Context, tx pgx.Tx, user string, specialties []string, maxSpecialties, requiredMatches int) error {
	var active, count int
	err := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE specialty=ANY($2)),count(*) FROM wai_user_specialties WHERE user_id=$1`, user, specialties).Scan(&active, &count)
	if err != nil {
		return err
	}
	if active < requiredMatches {
		return errors.New("specialty_not_selected")
	}
	if count > maxSpecialties {
		return errors.New("specialty_limit_exceeded_select_allowed_specialties")
	}
	return nil
}
func templateUse(ctx context.Context, tx pgx.Tx, p principal, t Template) error {
	if err := lockBillingUser(ctx, tx, p.User); err != nil {
		return err
	}
	if err := templateUseActive(ctx, tx, p, t); err != nil || p.InstanceAdmin {
		return err
	}
	var expired, overdue bool
	err := tx.QueryRow(ctx, `SELECT
		EXISTS(SELECT 1 FROM wai_template_addons WHERE template_id=$1 AND ends_at<=now()),
		EXISTS(SELECT 1 FROM wai_billing_invoices i WHERE i.template_addon=$1 AND i.due_at<=now()
		  AND i.amount_minor>coalesce((SELECT sum(amount_minor) FROM wai_billing_payments WHERE invoice_id=i.id),0))`, t.ID).Scan(&expired, &overdue)
	if err != nil {
		return err
	}
	if expired {
		return errors.New("template_addon_expired")
	}
	if overdue {
		return errors.New("template_addon_payment_overdue")
	}
	return nil
}
func templatePublication(ctx context.Context, tx pgx.Tx, p principal, t *Template, confirmation *templateAddonConfirmation, addon *ServicePlan) error {
	if e := lockBillingUser(ctx, tx, p.User); e != nil {
		return e
	}
	if confirmation != nil {
		var alreadyAdded bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wai_template_addons WHERE user_id=$1 AND request_id=$2)`, p.User, confirmation.RequestID).Scan(&alreadyAdded); e != nil {
			return e
		}
		if alreadyAdded {
			return errors.New("template_addon_already_added")
		}
	}
	var libraryVersion string
	if t.ID != "" {
		var owner, currentVersion *string
		if e := tx.QueryRow(ctx, `SELECT owner_id,current_version::text FROM wai_templates WHERE id::text=$1 AND tenant_id=$2`, t.ID, p.Tenant).Scan(&owner, &currentVersion); e != nil {
			return e
		}
		if owner != nil && *owner != p.User && !p.InstanceAdmin {
			return errors.New("template_not_owned")
		}
		if owner == nil && !p.InstanceAdmin {
			if currentVersion == nil {
				return pgx.ErrNoRows
			}
			libraryVersion = *currentVersion
			t.ID = ""
		}
	}
	if p.InstanceAdmin {
		return nil
	}
	plan, e := currentPlan(ctx, tx, p.User)
	if e != nil {
		return e
	}
	if libraryVersion != "" {
		source, err := loadTemplate(ctx, tx, p.Tenant, libraryVersion)
		if err != nil {
			return err
		}
		if err = requireActiveSpecialty(ctx, tx, p.User, source.allowedSpecialties(), plan.MaxSpecialties, 1); err != nil {
			return err
		}
	}
	if e = requireActiveSpecialty(ctx, tx, p.User, t.allowedSpecialties(), plan.MaxSpecialties, len(t.allowedSpecialties())); e != nil {
		return e
	}
	if len(t.Fields) > plan.MaxFields {
		return errors.New("template_complexity_limit_reached")
	}
	if t.ID == "" {
		var count int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM wai_templates t WHERE owner_id=$1 AND NOT EXISTS (SELECT 1 FROM wai_template_addons a WHERE a.template_id=t.id)`, p.User).Scan(&count); e != nil {
			return e
		}
		if count >= plan.MaxTemplates {
			if confirmation == nil || !plan.ExtrasEnabled || plan.ExtraTemplateMinor <= 0 {
				return errors.New("template_limit_reached")
			}
			if confirmation.PlanVersion != plan.VersionID || confirmation.MonthlyMinor != plan.ExtraTemplateMinor {
				return errors.New("template_addon_quote_changed")
			}
			*addon = plan
		}
	}
	return nil
}
func (m *Module) billingSpecialties(w http.ResponseWriter, r *http.Request, p principal) {
	if r.Method != "PUT" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		Specialties []string `json:"specialties"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Specialties) > 1000 {
		fail(w, 400, "too_many_specialties")
		return
	}
	ctx := r.Context()
	tx, e := m.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	if e = lockBillingUser(ctx, tx, p.User); e != nil {
		fail(w, 503, "database_error")
		return
	}
	plan, e := currentPlan(ctx, tx, p.User)
	if e != nil {
		fail(w, 503, "package_unavailable")
		return
	}
	unique := map[string]bool{}
	for _, s := range in.Specialties {
		s = strings.TrimSpace(s)
		if s == "" {
			fail(w, 400, "invalid_specialty")
			return
		}
		unique[s] = true
	}
	if !p.InstanceAdmin && len(unique) > plan.MaxSpecialties {
		billingFailure(w, errors.New("specialty_limit_reached"))
		return
	}
	for s := range unique {
		var exists bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wai_specialties WHERE tenant_id=$1 AND name=$2)`, p.Tenant, s).Scan(&exists); e != nil || !exists {
			fail(w, 400, "unknown_specialty")
			return
		}
	}
	_, e = tx.Exec(ctx, `DELETE FROM wai_user_specialties WHERE user_id=$1`, p.User)
	for s := range unique {
		if e != nil {
			break
		}
		_, e = tx.Exec(ctx, `INSERT INTO wai_user_specialties(user_id,specialty)VALUES($1,$2)`, p.User, s)
	}
	if e == nil {
		e = audit(ctx, tx, p, "billing.specialties.update", p.User)
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

type usageContextKey struct{}
type aiUsageContext struct {
	Session, Template, Operation, Generation string
	encryptedPrompt                          []byte
}

func withAIUsage(ctx context.Context, session, template, operation string) context.Context {
	return context.WithValue(ctx, usageContextKey{}, aiUsageContext{Session: session, Template: template, Operation: operation})
}
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Accounting uses an independent transaction so aborted clinical changes cannot erase provider usage.
func reserveTokens(ctx context.Context, conn *pgx.Conn, p principal, usageID, model string, tokens int64, cost float64, scope aiUsageContext) (e error) {
	tx, e := conn.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = lockBillingUser(ctx, tx, p.User); e != nil {
		return e
	}
	plan, e := currentPlan(ctx, tx, p.User)
	if e != nil {
		return e
	}
	if scope.Template != "" {
		t, e := loadTemplate(ctx, tx, p.Tenant, scope.Template)
		if e != nil {
			return e
		}
		if e = templateUseActive(ctx, tx, p, t); e != nil {
			return e
		}
	}
	var used int64
	e = tx.QueryRow(ctx, `SELECT coalesce(sum(CASE WHEN status='completed' THEN input_tokens::bigint+output_tokens WHEN status IN ('reserved','uncertain') THEN reserved_tokens ELSE 0 END),0) FROM wai_usage_records WHERE user_id=$1 AND created_at>=date_trunc('month',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'`, p.User).Scan(&used)
	if e != nil {
		return e
	}
	subscription, e := syncSubscription(ctx, tx, p.User, time.Now().UTC())
	if e != nil {
		return e
	}
	if !p.InstanceAdmin {
		if subscription != nil && subscription.Suspended {
			return errors.New(subscription.Reason)
		}
		if subscription != nil {
			actual, reserved, e := periodUsage(ctx, tx, subscription.PeriodID)
			if e != nil {
				return e
			}
			if actual+reserved+tokens > plan.IncludedTokens {
				if !plan.ExtrasEnabled || plan.ExtraMillionMinor <= 0 {
					return errors.New("ai_token_allowance_exceeded")
				}
				current, e := overageMinor(actual+reserved, plan.IncludedTokens, plan.ExtraMillionMinor)
				if e != nil {
					return e
				}
				projected, e := overageMinor(actual+reserved+tokens, plan.IncludedTokens, plan.ExtraMillionMinor)
				if e != nil {
					return e
				}
				if subscription.CreditUsedMinor+subscription.ReservedCreditMinor+projected-current > subscription.CreditLimitMinor {
					return errors.New("credit_reservation_exceeds_available")
				}
			}
		} else if tokens > plan.IncludedTokens || used > plan.IncludedTokens-tokens {
			return errors.New("ai_token_allowance_exceeded")
		}
	}
	var period any
	if subscription != nil {
		period = subscription.PeriodID
	}
	_, e = tx.Exec(ctx, `INSERT INTO wai_usage_records(id,tenant_id,user_id,provider,model,input_tokens,output_tokens,cost_usd,status,reserved_tokens,plan_version,session_id,template_version,operation,billing_period_id,encrypted_prompt)VALUES($1,$2,$3,'openai',$4,0,0,$5,'reserved',$6,$7,$8,$9,$10,$11,$12)`, usageID, p.Tenant, p.User, model, cost, tokens, plan.VersionID, nullString(scope.Session), nullString(scope.Template), scope.Operation, period, scope.encryptedPrompt)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func templateUseActive(ctx context.Context, tx pgx.Tx, p principal, t Template) error {
	var owner *string
	if e := tx.QueryRow(ctx, `SELECT owner_id FROM wai_templates WHERE id::text=$1 AND tenant_id=$2`, t.ID, p.Tenant).Scan(&owner); e != nil {
		return e
	}
	if owner != nil && *owner != p.User && !p.InstanceAdmin {
		return errors.New("template_not_owned")
	}
	if p.InstanceAdmin {
		return nil
	}
	plan, e := currentPlan(ctx, tx, p.User)
	if e != nil {
		return e
	}
	return requireActiveSpecialty(ctx, tx, p.User, t.allowedSpecialties(), plan.MaxSpecialties, 1)
}

func isCommercialError(e error) bool {
	switch e.Error() {
	case "ai_token_allowance_exceeded", "credit_limit_exhausted", "credit_reservation_exceeds_available", "subscription_payment_overdue":
		return true
	}
	return false
}
