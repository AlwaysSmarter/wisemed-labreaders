package wisemedai

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (m *Module) sessionPresentation(w http.ResponseWriter, r *http.Request, p principal, tx pgx.Tx, s Session) {
	if s.Status != "recording" && s.Status != "review" {
		fail(w, 409, "session_locked")
		return
	}
	var in struct {
		Revision int               `json:"revision"`
		Detail   string            `json:"detail"`
		Sections map[string]string `json:"sections"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Revision != s.Revision {
		fail(w, 409, "explicit_current_revision_required")
		return
	}
	ctx := r.Context()
	t, e := loadTemplate(ctx, tx, p.Tenant, s.Version)
	if e != nil {
		fail(w, 503, "template_unavailable")
		return
	}
	options := PresentationOptions{Detail: in.Detail, Sections: in.Sections}
	if e = validatePresentation(t, options); e != nil {
		fail(w, 400, e.Error())
		return
	}
	if s.Context == nil {
		s.Context = map[string]any{}
	}
	raw, _ := json.Marshal(options)
	var stored map[string]any
	json.Unmarshal(raw, &stored)
	s.Context["presentation"] = stored
	encrypted, e := m.box.seal(s.Context, p.Tenant+":"+s.ID)
	if e != nil {
		fail(w, 503, "encryption_error")
		return
	}
	if _, e = tx.Exec(ctx, `UPDATE wai_sessions SET context=$1 WHERE id=$2 AND tenant_id=$3`, encrypted, s.ID, p.Tenant); e != nil {
		fail(w, 503, "database_error")
		return
	}
	facts := []Fact{}
	if s.Result != nil {
		facts = s.Result.Evidence
	}
	result, e := buildSessionResult(applyContext(t, s.Context, p), s, facts, "")
	if e == nil {
		result.Status = s.Status
		e = m.saveResult(ctx, tx, p, &s, result)
	}
	if e == nil {
		e = audit(ctx, tx, p, "session.presentation", s.ID)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 503, "presentation_update_failed")
		return
	}
	m.notify(ctx, s, "progress")
	respond(w, 200, s)
}
