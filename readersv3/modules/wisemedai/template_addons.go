package wisemedai

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type templateAddonConfirmation struct {
	PlanVersion  string `json:"plan_version"`
	MonthlyMinor int64  `json:"monthly_minor"`
	RequestID    string `json:"request_id"`
}

// Caller holds the user's billing lock. Each template generates one statement per UTC month.
func syncTemplateAddons(ctx context.Context, tx pgx.Tx, user string, now time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO wai_billing_invoices
		(id,user_id,kind,currency,amount_minor,issued_at,due_at,reference,template_addon,addon_month)
		SELECT gen_random_uuid(), a.user_id, 'template_addon', a.currency, a.monthly_minor,
			greatest(a.started_at,m.month), greatest(a.started_at,m.month)+interval '120 hours',
			'WAI-'||gen_random_uuid(), a.template_id, m.month
		FROM wai_template_addons a
		CROSS JOIN LATERAL generate_series(
			date_trunc('month',a.started_at AT TIME ZONE 'UTC'),
			$2::text::timestamp, interval '1 month'
		) s(month)
		CROSS JOIN LATERAL (SELECT s.month AT TIME ZONE 'UTC' AS month) m
		WHERE a.user_id::text=$1 AND (a.ends_at IS NULL OR m.month<a.ends_at)
		AND NOT EXISTS (SELECT 1 FROM wai_billing_invoices i WHERE i.template_addon=a.template_id AND i.addon_month=m.month)
		ON CONFLICT (template_addon,addon_month) DO NOTHING`, user, monthStart(now).Format("2006-01-02"))
	return err
}

func (m *Module) cancelTemplateAddon(w http.ResponseWriter, r *http.Request, p principal, id string) {
	if r.Method != http.MethodDelete {
		fail(w, 405, "method_not_allowed")
		return
	}
	ctx := r.Context()
	tx, err := m.db.Begin(ctx)
	if err != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	if err = lockBillingUser(ctx, tx, p.User); err != nil {
		fail(w, 503, "database_error")
		return
	}
	now := time.Now().UTC()
	end := monthStart(now).AddDate(0, 1, 0)
	err = tx.QueryRow(ctx, `UPDATE wai_template_addons a SET ends_at=coalesce(a.ends_at,$1)
		FROM wai_templates t WHERE t.id=a.template_id AND t.tenant_id=$2
		AND a.template_id::text=$3 AND a.user_id=$4 RETURNING a.ends_at`, end, p.Tenant, id, p.User).Scan(&end)
	if err == pgx.ErrNoRows {
		fail(w, 404, "template_addon_not_found")
		return
	}
	if err == nil {
		err = syncTemplateAddons(ctx, tx, p.User, now)
	}
	if err == nil {
		err = audit(ctx, tx, p, "billing.template_addon.cancel", id)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 200, map[string]any{"ends_at": end})
}
