package wisemedai

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
)

type compactField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Unit        string   `json:"unit,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	SemanticTag string   `json:"semantic_tag,omitempty"`
	Repeatable  bool     `json:"repeatable,omitempty"`
	Section     string   `json:"section,omitempty"`
}
type compactChunk struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}
type compactSection struct {
	SpeechTerms []string `json:"speech_terms,omitempty"`
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Parent      string   `json:"parent,omitempty"`
}
type compiledInput struct {
	Sections        []compactSection `json:"sections,omitempty"`
	TemplateContext *TemplateContext `json:"template_context,omitempty"`
	// Definitions precede changing transcript content, preserving a stable prefix.
	Fields        json.RawMessage `json:"fields"`
	Chunks        []compactChunk  `json:"chunks"`
	OriginalBytes int             `json:"-"`
}

func (m *Module) compileContext(t Template, chunks []Chunk) compiledInput {
	originalInput := map[string]any{"fields": t.Fields, "chunks": chunks}
	if len(t.Sections) > 0 {
		originalInput["sections"] = t.Sections
	}
	if t.AIContext != nil {
		originalInput["template_context"] = t.AIContext
	}
	original, _ := json.Marshal(originalInput)
	source, _ := json.Marshal(t.Fields)
	key := sha256.Sum256(source)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.compiled == nil {
		m.compiled = map[[32]byte]json.RawMessage{}
	}
	fields, ok := m.compiled[key]
	if !ok {
		out := make([]compactField, 0, len(t.Fields))
		for _, f := range t.Fields {
			out = append(out, compactField{f.Key, f.Label, f.Type, f.Unit, f.Aliases, f.SemanticTag, f.Repeatable, f.Section})
		}
		fields, _ = json.Marshal(out)
		if len(m.compiled) >= 256 {
			m.compiled = map[[32]byte]json.RawMessage{}
		}
		m.compiled[key] = fields
	}
	out := compiledInput{TemplateContext: t.AIContext, Fields: fields, Chunks: []compactChunk{}, OriginalBytes: len(original)}
	for _, s := range t.Sections {
		out.Sections = append(out.Sections, compactSection{ID: s.ID, Label: s.Label, Parent: s.Parent, SpeechTerms: s.SpeechTerms})
	}
	for _, c := range chunks {
		out.Chunks = append(out.Chunks, compactChunk{c.ID, c.Text})
	}
	return out
}
func searchWords(s string) map[string]bool {
	s = strings.NewReplacer("ă", "a", "â", "a", "î", "i", "ș", "s", "ş", "s", "ț", "t", "ţ", "t").Replace(strings.ToLower(s))
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(w) >= 3 {
			out[w] = true
		}
	}
	return out
}

// Retrieval uses definitions only from the user's authorized library, never other doctors' reports.
func retrieveDefinitions(ctx context.Context, tx pgx.Tx, p principal, report string) ([]compactField, error) {
	rows, e := tx.Query(ctx, `SELECT f.definition FROM wai_template_fields f JOIN wai_templates t ON t.current_version=f.version_id AND t.tenant_id=f.tenant_id WHERE f.tenant_id=$1 AND (t.owner_id IS NULL OR t.owner_id=$2) ORDER BY t.id,f.field_key`, p.Tenant, p.User)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	query := searchWords(report)
	out := []compactField{}
	seen := map[string]bool{}
	for rows.Next() {
		var raw []byte
		var f Field
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &f); e != nil {
			return nil, e
		}
		match := false
		for w := range searchWords(f.Label + " " + strings.Join(f.Aliases, " ")) {
			if query[w] {
				match = true
				break
			}
		}
		if match && !seen[f.Key] {
			out = append(out, compactField{f.Key, f.Label, f.Type, f.Unit, f.Aliases, f.SemanticTag, f.Repeatable, f.Section})
			seen[f.Key] = true
		}
		if len(out) == 12 {
			break
		}
	}
	return out, rows.Err()
}
func localFieldProposals(report string) ([]fieldProposal, bool) {
	out := []fieldProposal{}
	seen := map[string]bool{}
	for _, line := range strings.Split(report, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, false
		}
		label := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		// Reject narrative sentences/identifiers; such input needs the existing AI review path.
		key := slugField(label)
		if label == "" || value == "" || len(label) > 80 || len(strings.Fields(label)) > 8 || strings.ContainsAny(label, ".!?0123456789") || key == "patient_ref" || key == "doctor_name" {
			return nil, false
		}
		for _, word := range []string{"pacient", "cnp", "nume", "prenume", "adresa", "telefon", "email", "patient", "name", "address"} {
			if searchWords(label)[word] {
				return nil, false
			}
		}
		if !seen[key] {
			out = append(out, fieldProposal{key, label, "string", "", line})
			seen[key] = true
		}
		if len(out) > 100 {
			return nil, false
		}
	}
	return out, len(out) > 0
}
