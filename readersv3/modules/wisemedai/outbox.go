package wisemedai

import (
	"net/http"
	"time"
)

func (m *Module) outbox(w http.ResponseWriter, r *http.Request, p principal) {
	if p.Role != "admin" {
		fail(w, 403, "admin_required")
		return
	}
	if r.Method != "GET" {
		fail(w, 405, "method_not_allowed")
		return
	}
	rows, e := m.db.Query(r.Context(), `SELECT id,result_id,connector_id,status,attempts,last_error,next_attempt_at FROM wai_outbox_delivery WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 100`, p.Tenant)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, result, connector, status, errcode string
		var attempts int
		var next time.Time
		if rows.Scan(&id, &result, &connector, &status, &attempts, &errcode, &next) != nil {
			fail(w, 500, "database_error")
			return
		}
		out = append(out, map[string]any{"id": id, "result_id": result, "connector_id": connector, "status": status, "attempts": attempts, "error": errcode, "next_attempt_at": next})
	}
	respond(w, 200, out)
}
