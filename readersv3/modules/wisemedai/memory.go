package wisemedai

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
)

func vectorLiteral(values []float64) (string, error) {
	if len(values) != 1536 {
		return "", fmt.Errorf("embedding_requires_1536_dimensions")
	}
	parts := make([]string, len(values))
	norm := 0.0
	for i, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e6 {
			return "", fmt.Errorf("invalid_embedding")
		}
		norm += v * v
		parts[i] = strconv.FormatFloat(v, 'g', -1, 64)
	}
	if norm == 0 {
		return "", fmt.Errorf("zero_embedding_not_allowed")
	}
	return "[" + strings.Join(parts, ",") + "]", nil
}
func (m *Module) memorySearch(w http.ResponseWriter, r *http.Request, p principal) {
	if r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		Query     string    `json:"query"`
		Embedding []float64 `json:"embedding"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Query) > 500 {
		fail(w, 400, "query_too_long")
		return
	}
	query := `SELECT id,name,payload,0::float8 FROM wai_memory_examples WHERE tenant_id=$1 AND user_id=$2 ORDER BY created_at DESC LIMIT 200`
	args := []any{p.Tenant, p.User}
	if len(in.Embedding) > 0 {
		vector, e := vectorLiteral(in.Embedding)
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		query = `SELECT id,name,payload,1-(embedding <=> $3::vector) FROM wai_memory_examples WHERE tenant_id=$1 AND user_id=$2 AND embedding IS NOT NULL ORDER BY embedding <=> $3::vector LIMIT 10`
		args = append(args, vector)
	}
	rows, e := m.db.Query(r.Context(), query, args...)
	if e != nil {
		fail(w, 503, "semantic_memory_unavailable")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	needle := strings.ToLower(strings.TrimSpace(in.Query))
	for rows.Next() {
		var id, name string
		var raw []byte
		var score float64
		if rows.Scan(&id, &name, &raw, &score) != nil {
			fail(w, 500, "database_error")
			return
		}
		var payload map[string]any
		if m.box.open(raw, p.Tenant+":"+id, &payload) != nil {
			fail(w, 500, "decryption_failed")
			return
		}
		if len(in.Embedding) == 0 && needle != "" && !strings.Contains(strings.ToLower(name+" "+fmt.Sprint(payload["text"])), needle) {
			continue
		}
		out = append(out, map[string]any{"id": id, "name": name, "payload": redact(payload), "score": score})
		if len(out) == 10 {
			break
		}
	}
	respond(w, 200, map[string]any{"items": out, "mode": map[bool]string{true: "vector", false: "text"}[len(in.Embedding) > 0], "scope": "own_approved_memory_examples"})
}
func (m *Module) memoryEmbedding(w http.ResponseWriter, r *http.Request, p principal, id string) {
	if r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		Embedding []float64 `json:"embedding"`
		Confirmed bool      `json:"confirmed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirmed {
		fail(w, 400, "explicit_confirmation_required")
		return
	}
	vector, e := vectorLiteral(in.Embedding)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	tx, e := m.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(r.Context())
	tag, e := tx.Exec(r.Context(), `UPDATE wai_memory_examples SET embedding=$1::vector WHERE id::text=$2 AND tenant_id=$3 AND user_id=$4`, vector, id, p.Tenant, p.User)
	if e != nil {
		fail(w, 503, "pgvector_not_installed")
		return
	}
	if tag.RowsAffected() != 1 {
		fail(w, 404, "memory_example_not_found")
		return
	}
	if e = audit(r.Context(), tx, p, "memory.embedding", id); e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
