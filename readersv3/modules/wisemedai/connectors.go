package wisemedai

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

func validateConnector(v map[string]any, hosts []string) error {
	target, _ := v["url"].(string)
	u, e := url.Parse(target)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return errors.New("https_connector_url_required")
	}
	allowed := false
	for _, h := range hosts {
		if strings.EqualFold(u.Host, h) {
			allowed = true
		}
	}
	if !allowed {
		return errors.New("connector_host_not_allowlisted")
	}
	if token, ok := v["token"].(string); !ok || strings.TrimSpace(token) == "" {
		return errors.New("connector_token_required")
	}
	return nil
}
func (m *Module) deliver(w http.ResponseWriter, r *http.Request, p principal, s Session) {
	var in struct {
		ConnectorID string `json:"connector_id"`
		Revision    int    `json:"revision"`
		Confirmed   bool   `json:"confirmed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirmed || s.Status != "approved" || in.Revision != s.Revision {
		fail(w, 409, "approved_revision_and_confirmation_required")
		return
	}
	var rid string
	e := m.db.QueryRow(r.Context(), `SELECT id FROM wai_clinical_results WHERE session_id=$1 AND tenant_id=$2 AND revision=$3`, s.ID, p.Tenant, s.Revision).Scan(&rid)
	if e != nil {
		fail(w, 404, "approved_result_not_found")
		return
	}
	tx, e := m.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(r.Context())
	id := uuid.NewString()
	_, e = tx.Exec(r.Context(), `INSERT INTO wai_outbox_delivery(id,tenant_id,result_id,connector_id)VALUES($1,$2,$3,$4) ON CONFLICT(result_id,connector_id)DO NOTHING`, id, p.Tenant, rid, in.ConnectorID)
	if e == nil {
		e = audit(r.Context(), tx, p, "result.delivery_requested", s.ID)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		fail(w, 400, "invalid_connector")
		return
	}
	var status string
	if e = m.db.QueryRow(r.Context(), `SELECT id,status FROM wai_outbox_delivery WHERE result_id=$1 AND connector_id::text=$2 AND tenant_id=$3`, rid, in.ConnectorID, p.Tenant).Scan(&id, &status); e != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 202, map[string]any{"ok": true, "delivery_id": id, "status": status})
}
func (m *Module) dispatchOutbox(ctx context.Context) {
	tx, e := m.db.Begin(ctx)
	if e != nil {
		return
	}
	defer tx.Rollback(ctx)
	var id, tenant, sid string
	var res, config []byte
	var attempts int
	e = tx.QueryRow(ctx, `SELECT o.id,o.tenant_id,r.session_id,r.payload,c.payload,o.attempts FROM wai_outbox_delivery o JOIN wai_clinical_results r ON r.id=o.result_id AND r.tenant_id=o.tenant_id JOIN wai_integration_configs c ON c.id=o.connector_id AND c.tenant_id=o.tenant_id WHERE o.status='pending' AND o.next_attempt_at<=now() ORDER BY o.created_at FOR UPDATE OF o SKIP LOCKED LIMIT 1`).Scan(&id, &tenant, &sid, &res, &config, &attempts)
	if e != nil {
		return
	}
	var connectorID string
	_ = tx.QueryRow(ctx, `SELECT connector_id FROM wai_outbox_delivery WHERE id=$1`, id).Scan(&connectorID)
	var payload map[string]any
	var result Result
	code := "invalid_configuration"
	success := false
	if m.box.open(config, tenant+":"+connectorID, &payload) == nil && validateConnector(payload, m.cfg.AllowedHosts) == nil && m.box.open(res, tenant+":"+sid, &result) == nil {
		raw, mappingErr := m.connectorBody(ctx, tenant, payload, result)
		if mappingErr != nil {
			raw = nil
		}
		req, e := http.NewRequestWithContext(ctx, "POST", payload["url"].(string), bytes.NewReader(raw))
		if e == nil && mappingErr == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+payload["token"].(string))
			req.Header.Set("Idempotency-Key", id)
			resp, e := m.client.Do(req)
			if e == nil {
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
				resp.Body.Close()
				success = resp.StatusCode >= 200 && resp.StatusCode < 300
				code = "connector_http_error"
			} else {
				code = "connector_transport_error"
			}
		}
	}
	attempts++
	status := "pending"
	if success {
		status = "delivered"
		code = ""
	} else if attempts >= 5 {
		status = "failed"
	}
	delay := time.Duration(1<<attempts) * time.Minute
	_, e = tx.Exec(ctx, `UPDATE wai_outbox_delivery SET status=$1,attempts=$2,next_attempt_at=$3,last_error=$4 WHERE id=$5`, status, attempts, time.Now().Add(delay), code, id)
	if e == nil {
		_ = tx.Commit(ctx)
	}
}
