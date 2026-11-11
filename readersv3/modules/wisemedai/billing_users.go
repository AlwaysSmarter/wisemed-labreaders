package wisemedai

import (
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func (m *Module) billingUserRole(w http.ResponseWriter, r *http.Request, p principal) {
	if !p.InstanceAdmin {
		fail(w, 403, "super_admin_required")
		return
	}
	if r.Method != "PUT" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		User      string `json:"user_id"`
		UserType  int    `json:"user_type"`
		Disabled  bool   `json:"disabled"`
		Confirmed bool   `json:"confirmed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirmed || in.User == p.User || in.UserType < -1 || in.UserType > 1 {
		fail(w, 400, "explicit_other_user_role_change_required")
		return
	}
	ctx := r.Context()
	tx, e := m.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7741921108)`); e != nil {
		fail(w, 503, "database_error")
		return
	}
	var actorStillSuper bool
	if e = tx.QueryRow(ctx, `SELECT instance_admin AND NOT disabled FROM wai_users WHERE id=$1`, p.User).Scan(&actorStillSuper); e != nil || !actorStillSuper {
		fail(w, 403, "super_admin_required")
		return
	}
	tag, e := tx.Exec(ctx, `UPDATE wai_users SET instance_admin=$1,disabled=$2 WHERE id::text=$3`, in.UserType == -1, in.Disabled, in.User)
	if e != nil || tag.RowsAffected() != 1 {
		fail(w, 404, "user_not_found")
		return
	}
	role := "doctor"
	if in.UserType <= 0 {
		role = "admin"
	}
	_, e = tx.Exec(ctx, `UPDATE wai_memberships SET role=$1 WHERE user_id::text=$2`, role, in.User)
	if e == nil {
		e = audit(ctx, tx, p, "billing.user.role", in.User)
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
func (m *Module) billingUserPassword(w http.ResponseWriter, r *http.Request, p principal) {
	if !p.InstanceAdmin {
		fail(w, 403, "super_admin_required")
		return
	}
	if r.Method != http.MethodPut {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		User     string `json:"user_id"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.User == p.User {
		fail(w, 400, "use_profile_to_change_own_password")
		return
	}
	if len(in.Password) < 12 || len(in.Password) > 72 {
		fail(w, 400, "password_requires_12_to_72_bytes")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		fail(w, 400, "invalid_password")
		return
	}
	ctx := r.Context()
	tx, err := m.db.Begin(ctx)
	if err != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7741921108)`); err != nil {
		fail(w, 503, "database_error")
		return
	}
	var actorStillSuper bool
	if err = tx.QueryRow(ctx, `SELECT instance_admin AND NOT disabled FROM wai_users WHERE id=$1`, p.User).Scan(&actorStillSuper); err != nil || !actorStillSuper {
		fail(w, 403, "super_admin_required")
		return
	}
	tag, err := tx.Exec(ctx, `UPDATE wai_users SET password_hash=$1 WHERE id::text=$2`, string(hash), in.User)
	if err == nil && tag.RowsAffected() == 0 {
		fail(w, 404, "user_not_found")
		return
	}
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM wai_auth_sessions WHERE user_id::text=$1`, in.User)
	}
	if err == nil {
		err = audit(ctx, tx, p, "billing.user.password", in.User)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}

func (m *Module) billingUsage(w http.ResponseWriter, r *http.Request, p principal) {
	if r.Method != "GET" {
		fail(w, 405, "method_not_allowed")
		return
	}
	user := p.User
	if target := r.URL.Query().Get("user_id"); target != "" && target != p.User {
		if !p.InstanceAdmin {
			fail(w, 403, "super_admin_required")
			return
		}
		user = target
	}
	rows, e := m.db.Query(r.Context(), `SELECT id,operation,model,input_tokens,output_tokens,reserved_tokens,status,session_id::text,template_version::text,plan_version::text,cost_usd,created_at FROM wai_usage_records WHERE user_id::text=$1 ORDER BY created_at DESC LIMIT 200`, user)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, operation, model, status string
		var session, template, plan *string
		var input, output, reserved int64
		var cost float64
		var at time.Time
		if rows.Scan(&id, &operation, &model, &input, &output, &reserved, &status, &session, &template, &plan, &cost, &at) != nil {
			fail(w, 500, "database_error")
			return
		}
		row := map[string]any{"id": id, "operation": operation, "model": model, "input_tokens": input, "output_tokens": output, "total_tokens": input + output, "reserved_tokens": reserved, "status": status, "session_id": session, "template_version": template, "plan_version": plan, "created_at": at}
		if p.InstanceAdmin {
			row["provider_cost_usd"] = cost
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 200, out)
}
