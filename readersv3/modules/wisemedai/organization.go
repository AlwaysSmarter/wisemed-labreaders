package wisemedai

import (
	"net/http"
	"strings"
)

func (m *Module) organization(w http.ResponseWriter, r *http.Request, p principal) {
	if p.Role != "admin" {
		fail(w, 403, "admin_required")
		return
	}
	if r.Method != "PUT" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if len(in.Name) < 2 || len(in.Name) > 180 {
		fail(w, 400, "invalid_organization_name")
		return
	}
	tx, e := m.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(r.Context())
	_, e = tx.Exec(r.Context(), `UPDATE wai_organizations SET name=$1 WHERE id=$2`, in.Name, p.Tenant)
	if e == nil {
		e = audit(r.Context(), tx, p, "organization.update", p.Tenant)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 200, map[string]any{"id": p.Tenant, "name": in.Name})
}
