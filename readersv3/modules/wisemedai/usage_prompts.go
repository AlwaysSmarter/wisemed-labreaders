package wisemedai

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type promptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type promptOutput struct {
	Content string `json:"content"`
	Refusal string `json:"refusal,omitempty"`
	Finish  string `json:"finish_reason,omitempty"`
}

func promptScope(tenant, id string) string {
	return tenant + ":" + id + ":usage_prompt"
}

func (m *Module) billingUsagePrompt(w http.ResponseWriter, r *http.Request, p principal) {
	w.Header().Set("Cache-Control", "no-store")
	if !p.InstanceAdmin {
		fail(w, 403, "super_admin_required")
		return
	}
	if r.Method != http.MethodGet {
		fail(w, 405, "method_not_allowed")
		return
	}
	id, err := uuid.Parse(r.URL.Query().Get("id"))
	if err != nil {
		fail(w, 400, "invalid_usage_id")
		return
	}
	ctx := r.Context()
	tx, err := m.db.Begin(ctx)
	if err != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)

	var tenant string
	var encrypted, encryptedOutput []byte
	err = tx.QueryRow(ctx, `SELECT tenant_id, encrypted_prompt, encrypted_output FROM wai_usage_records
		WHERE id=$1 AND encrypted_prompt IS NOT NULL
		AND created_at >= now()-make_interval(days=>$2)`, id.String(), m.cfg.RetentionDays).Scan(&tenant, &encrypted, &encryptedOutput)
	if errors.Is(err, pgx.ErrNoRows) {
		respond(w, 200, map[string]any{"messages": []promptMessage{}})
		return
	}
	if err != nil {
		fail(w, 503, "database_error")
		return
	}
	var messages []promptMessage
	if err = m.box.open(encrypted, promptScope(tenant, id.String()), &messages); err != nil {
		fail(w, 500, "prompt_unavailable")
		return
	}
	var output *promptOutput
	if encryptedOutput != nil {
		if err = m.box.open(encryptedOutput, promptScope(tenant, id.String())+":output", &output); err != nil {
			fail(w, 500, "prompt_unavailable")
			return
		}
	}
	p.Tenant = tenant
	if err = audit(ctx, tx, p, "billing.usage.prompt.view", id.String()); err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 200, map[string]any{"messages": messages, "output": output})
}
