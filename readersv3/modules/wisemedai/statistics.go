package wisemedai

import (
	"net/http"
	"time"
)

func (m *Module) billingStatistics(w http.ResponseWriter, r *http.Request, p principal) {
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
	start := monthStart(time.Now())
	all := r.URL.Query().Get("period") == "all"
	rows, e := m.db.Query(r.Context(), `SELECT u.template_version::text,coalesce(t.name,'Propunere / fără șablon publicat'),u.operation,u.provider,u.model,count(*),coalesce(sum(input_tokens),0),coalesce(sum(output_tokens),0),coalesce(sum(cached_input_tokens),0),coalesce(sum(reserved_tokens) FILTER(WHERE status IN ('reserved','uncertain')),0),coalesce(sum(original_context_bytes),0),coalesce(sum(sent_context_bytes),0),coalesce(sum(latency_ms),0),count(*) FILTER(WHERE status IN ('reserved','uncertain')) FROM wai_usage_records u LEFT JOIN wai_template_versions v ON v.id=u.template_version LEFT JOIN wai_templates t ON t.id=v.template_id AND t.tenant_id=u.tenant_id WHERE u.user_id::text=$1 AND ($2 OR u.created_at>=$3) GROUP BY u.template_version,t.name,u.operation,u.provider,u.model ORDER BY sum(input_tokens::bigint+output_tokens) DESC LIMIT 500`, user, all, start)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var version *string
		var name, op, provider, model string
		var calls, input, output, cached, reserved, original, sent, latency, uncertain int64
		if e = rows.Scan(&version, &name, &op, &provider, &model, &calls, &input, &output, &cached, &reserved, &original, &sent, &latency, &uncertain); e != nil {
			fail(w, 503, "database_error")
			return
		}
		out = append(out, map[string]any{"template_version": version, "template_name": name, "operation": op, "provider": provider, "model": model, "calls": calls, "input_tokens": input, "output_tokens": output, "total_tokens": input + output, "cached_input_tokens": cached, "reserved_tokens": reserved, "original_context_bytes": original, "sent_context_bytes": sent, "latency_ms": latency, "uncertain_calls": uncertain})
	}
	if rows.Err() != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 200, map[string]any{"items": out, "period_start": start, "all_time": all, "limit": 500})
}
