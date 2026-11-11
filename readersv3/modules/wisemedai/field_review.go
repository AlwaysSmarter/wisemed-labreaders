package wisemedai

import (
	"context"
	"encoding/json"
	"html"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

type FieldIssue struct {
	Field       string   `json:"field"`
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	RawValue    string   `json:"raw_value,omitempty"`
	Quote       string   `json:"quote,omitempty"`
	ChunkID     string   `json:"chunk_id,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

func stringOverride(v any) string { s, _ := v.(string); return s }
func ambiguousLeadingZero(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) > 1 && s[0] == '0' && s[1] >= '0' && s[1] <= '9'
}
func fieldHasIssue(r Result, key string) bool {
	for _, issue := range r.FieldIssues {
		if issue.Field == key {
			return true
		}
	}
	return false
}
func fieldMarkup(key string, data map[string]any) string {
	value := "—"
	if v, ok := data[key]; ok {
		value = displayValue(v)
	}
	return `<span data-review-field="` + html.EscapeString(key) + `">` + html.EscapeString(value) + `</span>`
}

// Keep accepted fields across incremental extraction. A new occurrence replaces only
// that field. Explicit clinician edits remain authoritative until that field is redictated.
func buildSessionResult(t Template, s Session, incoming []Fact, target string) (Result, error) {
	replacement := map[string]bool{}
	for _, f := range incoming {
		replacement[f.Field] = true
	}
	if target != "" {
		replacement[target] = true
	}
	chunks := map[string]Chunk{}
	for _, c := range s.Chunks {
		chunks[c.ID] = c
	}
	facts := []Fact{}
	if s.Result != nil {
		for _, f := range s.Result.Evidence {
			c, ok := chunks[f.ChunkID]
			if !replacement[f.Field] && (f.UserEdited || ok && (f.ChunkRevision == 0 || f.ChunkRevision == c.Revision)) {
				facts = append(facts, f)
			}
		}
	}
	facts = append(facts, incoming...)
	// User edits live in encrypted session context, never in model output.
	overrides, _ := s.Context["field_overrides"].(map[string]any)
	for _, field := range t.Fields {
		raw, exists := overrides[field.Key]
		if !exists {
			continue
		}
		kept := facts[:0]
		for _, f := range facts {
			if f.Field != field.Key {
				kept = append(kept, f)
			}
		}
		facts = kept
		facts = append(facts, Fact{Field: field.Key, Value: stringOverride(raw), State: "present", UserEdited: true})
	}
	return buildResult(t, s.Chunks, facts, s.ID, s.Revision+1)
}
func fieldTemplate(t Template, key string) (Template, bool) {
	for _, f := range t.Fields {
		if f.Key == key {
			t.Fields = []Field{f}
			sections := []Section{}
			for _, section := range t.Sections {
				if section.ID == f.Section {
					sections = append(sections, section)
				}
			}
			t.Sections = sections
			return t, true
		}
	}
	return t, false
}
func (m *Module) extractChunk(ctx context.Context, tx pgx.Tx, p principal, t Template, c Chunk) ([]Fact, error) {
	if c.TargetField != "" {
		t, _ = fieldTemplate(t, c.TargetField)
		if m.cfg.Provider == "mock" {
			return []Fact{{Field: c.TargetField, Value: c.Text, Quote: c.Text, State: "present", ChunkID: c.ID}}, nil
		}
	}
	facts, err := m.extract(ctx, tx, p, t, []Chunk{c})
	if c.TargetField != "" {
		filtered := []Fact{}
		for _, f := range facts {
			if f.Field == c.TargetField {
				filtered = append(filtered, f)
			}
		}
		facts = filtered
	}
	return facts, err
}
func (m *Module) sessionField(w http.ResponseWriter, r *http.Request, p principal, tx pgx.Tx, s Session, options bool) {
	if s.Status != "recording" && s.Status != "review" {
		fail(w, 409, "session_locked")
		return
	}
	var in struct {
		Revision int    `json:"revision"`
		Field    string `json:"field"`
		Value    string `json:"value"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Revision != s.Revision {
		fail(w, 409, "explicit_current_revision_required")
		return
	}
	t := *s.Template
	scoped, ok := fieldTemplate(t, in.Field)
	if !ok {
		fail(w, 400, "unknown_field")
		return
	}
	if options {
		suggestions := []string{}
		if s.Result != nil {
			for _, issue := range s.Result.FieldIssues {
				if issue.Field == in.Field {
					suggestions = append(suggestions, issue.Suggestions...)
				}
			}
		}
		// Missing evidence is displayed as a dash, never fabricated as a clinical value.
		if len(suggestions) == 0 && s.Result != nil {
			sources := []string{}
			for _, f := range s.Result.Evidence {
				if f.Field == in.Field && f.Quote != "" {
					sources = append(sources, f.Quote)
				}
			}
			if len(sources) > 0 && m.cfg.Provider != "mock" {
				schema := map[string]any{"type": "object", "properties": map[string]any{"suggestions": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"suggestions"}, "additionalProperties": false}
				content, e := m.aiJSON(withAIUsage(r.Context(), s.ID, s.Version, "field_interpretation"), tx, p, "Suggest at most three plausible interpretations for this one clinical field from its untrusted verbatim source. Understand spoken numbers and missing decimal separators (02 may mean 0.2). Preserve units and negation. No new observations. Return an empty array if there is no relevant evidence. These are alternatives for clinician confirmation, never accepted facts.", "field_options", map[string]any{"field": scoped.Fields[0], "source": sources}, schema)
				if e != nil {
					if isCommercialError(e) {
						billingFailure(w, e)
					} else {
						fail(w, 502, safeExtractionError(e))
					}
					return
				}
				var reply struct {
					Suggestions []string `json:"suggestions"`
				}
				if json.Unmarshal([]byte(content), &reply) == nil {
					suggestions = reply.Suggestions
				}
			}
		}
		respond(w, 200, map[string]any{"field": in.Field, "revision": s.Revision, "suggestions": suggestions})
		return
	}
	if len(in.Value) > 16000 || strings.TrimSpace(in.Value) == "" {
		fail(w, 400, "field_value_required")
		return
	}
	if s.Context == nil {
		s.Context = map[string]any{}
	}
	overrides, _ := s.Context["field_overrides"].(map[string]any)
	if overrides == nil {
		overrides = map[string]any{}
	}
	overrides[in.Field] = strings.TrimSpace(in.Value)
	s.Context["field_overrides"] = overrides
	if e := m.saveSessionContext(r.Context(), tx, p, s); e != nil {
		fail(w, 503, "database_error")
		return
	}
	result, e := buildSessionResult(applyContext(t, s.Context, p), s, nil, "")
	if e == nil {
		result.Status = s.Status
		e = m.saveResult(r.Context(), tx, p, &s, result)
	}
	if e == nil {
		e = audit(r.Context(), tx, p, "session.field_edit", s.ID)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	m.notify(r.Context(), s, "progress")
	respond(w, 200, s)
}
func (m *Module) saveSessionContext(ctx context.Context, tx pgx.Tx, p principal, s Session) error {
	raw, e := m.box.seal(s.Context, p.Tenant+":"+s.ID)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE wai_sessions SET context=$1 WHERE id=$2 AND tenant_id=$3`, raw, s.ID, p.Tenant)
	return e
}
