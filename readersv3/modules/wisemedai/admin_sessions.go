package wisemedai

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type adminSessionOwner struct {
	ID           string `json:"user_id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Tenant       string `json:"organization_id"`
	Organization string `json:"organization"`
}

type adminSessionSummary struct {
	ID       string            `json:"id"`
	Template string            `json:"template_name"`
	Status   string            `json:"status"`
	Revision int               `json:"revision"`
	Created  time.Time         `json:"created_at"`
	Result   bool              `json:"has_result"`
	Owner    adminSessionOwner `json:"owner"`
}

// Administrative reads are separate from the doctor's mutation endpoints.
// Installation admins see the instance; organization admins retain tenant scope.
func (m *Module) adminSessions(w http.ResponseWriter, r *http.Request, p principal, suffix string) {
	if !p.InstanceAdmin && p.Role != "admin" {
		fail(w, 403, "admin_required")
		return
	}
	if r.Method != "GET" {
		fail(w, 405, "method_not_allowed")
		return
	}
	id := strings.Trim(suffix, "/")
	if id != "" {
		m.adminSession(w, r, p, id)
		return
	}
	user, status := r.URL.Query().Get("user_id"), r.URL.Query().Get("status")
	if user != "" {
		if _, e := uuid.Parse(user); e != nil {
			fail(w, 422, "invalid_user_id")
			return
		}
	}
	if status != "" && status != "recording" && status != "review" && status != "approved" {
		fail(w, 422, "invalid_session_status")
		return
	}
	offset := 0
	if value := r.URL.Query().Get("offset"); value != "" {
		var e error
		offset, e = strconv.Atoi(value)
		if e != nil || offset < 0 || offset > 1000000 {
			fail(w, 422, "invalid_offset")
			return
		}
	}
	const limit = 50
	const where = ` WHERE ($1::boolean OR s.tenant_id=$2::uuid) AND ($3::text='' OR s.user_id::text=$3) AND ($4::text='' OR s.status=$4)`
	ctx := r.Context()
	var total int
	if e := m.db.QueryRow(ctx, `SELECT count(*) FROM wai_sessions s`+where, p.InstanceAdmin, p.Tenant, user, status).Scan(&total); e != nil {
		fail(w, 503, "database_error")
		return
	}
	rows, e := m.db.Query(ctx, `SELECT s.id,coalesce(v.document->>'name','Sesiune clinică'),s.status,s.revision,s.created_at,s.result IS NOT NULL,u.id,u.name,u.email,o.id,o.name FROM wai_sessions s JOIN wai_users u ON u.id=s.user_id JOIN wai_organizations o ON o.id=s.tenant_id JOIN wai_template_versions v ON v.id=s.template_version AND v.tenant_id=s.tenant_id`+where+` ORDER BY s.created_at DESC,s.id DESC LIMIT $5 OFFSET $6`, p.InstanceAdmin, p.Tenant, user, status, limit, offset)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	items := []adminSessionSummary{}
	for rows.Next() {
		var s adminSessionSummary
		if e = rows.Scan(&s.ID, &s.Template, &s.Status, &s.Revision, &s.Created, &s.Result, &s.Owner.ID, &s.Owner.Name, &s.Owner.Email, &s.Owner.Tenant, &s.Owner.Organization); e != nil {
			break
		}
		items = append(items, s)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	users := []map[string]string{}
	rows, e = m.db.Query(ctx, `SELECT DISTINCT u.id,u.name,u.email FROM wai_users u JOIN wai_memberships m ON m.user_id=u.id WHERE ($1::boolean OR m.tenant_id=$2::uuid) ORDER BY u.email LIMIT 1000`, p.InstanceAdmin, p.Tenant)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	for rows.Next() {
		var id, name, email string
		if e = rows.Scan(&id, &name, &email); e != nil {
			break
		}
		users = append(users, map[string]string{"id": id, "name": name, "email": email})
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 200, map[string]any{"items": items, "users": users, "total": total, "offset": offset, "limit": limit, "read_only": true, "instance_scope": p.InstanceAdmin})
}

func (m *Module) adminSession(w http.ResponseWriter, r *http.Request, p principal, id string) {
	if _, e := uuid.Parse(id); e != nil {
		fail(w, 404, "session_not_found")
		return
	}
	ctx := r.Context()
	var owner adminSessionOwner
	e := m.db.QueryRow(ctx, `SELECT u.id,u.name,u.email,o.id,o.name FROM wai_sessions s JOIN wai_users u ON u.id=s.user_id JOIN wai_organizations o ON o.id=s.tenant_id WHERE s.id=$1 AND ($2::boolean OR s.tenant_id=$3::uuid)`, id, p.InstanceAdmin, p.Tenant).Scan(&owner.ID, &owner.Name, &owner.Email, &owner.Tenant, &owner.Organization)
	if e != nil {
		fail(w, 404, "session_not_found")
		return
	}
	tx, e := m.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	target := principal{User: owner.ID, Tenant: owner.Tenant}
	session, e := m.loadSession(ctx, tx, target, id)
	if e != nil {
		fail(w, 404, "session_not_found")
		return
	}
	if e = audit(ctx, tx, p, "session.admin.read", session.ID); e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 503, "audit_failed")
		return
	}
	respond(w, 200, map[string]any{"session": session, "owner": owner, "read_only": true})
}
