package wisemedai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	xhtml "golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
)

const speechSchema = "wisemed.speech-adaptation.v1"
const speechNotice = "speech-community-2026-11-05-v1"
const activeSpeechBudget = 2048

func normalizeSpeech(s string) string {
	return strings.Join(strings.Fields(norm.NFC.String(s)), " ")
}
func speechKey(s string) string { return strings.ToLower(normalizeSpeech(s)) }

// Numeric values, addresses, handles, identifiers and proper-name-like terms do
// not become shared dictionary entries. Feedback itself stays encrypted.
func safeSpeechTerm(s string) bool {
	if len(s) > 160 || utf8.RuneCountInString(s) < 3 || len(strings.Fields(s)) > 6 {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && r != ' ' && r != '-' {
			return false
		}
	}
	for _, word := range strings.Fields(s) {
		if word != "Doppler" && len(word) > 1 && unicode.IsUpper([]rune(word)[0]) && strings.ToUpper(word) != word {
			return false
		}
	}
	for _, w := range []string{"pacient", "nume", "prenume", "adresa", "adresă", "telefon", "cnp", "email", "născut", "nascut", "strada"} {
		if strings.Contains(speechKey(s), w) {
			return false
		}
	}
	return true
}
func medicalSpeechTerm(s string) bool {
	if !safeSpeechTerm(s) {
		return false
	}
	for _, stem := range []string{"ecogen", "ecogra", "fibro", "vascular", "doppler", "hepato", "colecist", "pancrea", "renal", "rinichi", "tiroid", "nodul", "parench", "parench", "hipo", "hiper", "calcif", "adenop", "splen", "ventric", "miocard", "endometr", "prostat", "ovar", "vezic", "cortical", "medular", "litiaz", "dilata", "omogen", "neomogen"} {
		if strings.Contains(speechKey(s), stem) {
			return true
		}
	}
	return false
}

type speechTerm struct {
	Text         string  `json:"text"`
	Status       string  `json:"status"`
	Source       string  `json:"source"`
	Observations int64   `json:"observations"`
	First        string  `json:"first_observed"`
	Provenance   *string `json:"provenance"`
	Key          string  `json:"key"`
}
type speechTiming struct {
	Text  string `json:"text"`
	Start int    `json:"start_ms"`
	End   *int   `json:"end_ms,omitempty"`
	Kind  string `json:"kind"`
}
type speechSegment struct {
	ID                string          `json:"id"`
	ChunkID           string          `json:"chunk_id"`
	Raw               string          `json:"raw_text"`
	Corrected         []string        `json:"corrections"`
	Transcript        string          `json:"verified_transcript,omitempty"`
	Verified          bool            `json:"transcript_verified"`
	Runtime           string          `json:"runtime"`
	RuntimeVersion    string          `json:"runtime_version"`
	Model             string          `json:"model"`
	ModelVersion      string          `json:"model_version"`
	Mode              string          `json:"mode"`
	VocabularyVersion int64           `json:"vocabulary_version"`
	GrammarVersion    string          `json:"grammar_version"`
	StartMS           int             `json:"start_ms"`
	EndMS             int             `json:"end_ms"`
	CaptureStarted    string          `json:"capture_started"`
	ConsentGeneration string          `json:"consent_generation,omitempty"`
	Timings           []speechTiming  `json:"timings,omitempty"`
	Terms             []string        `json:"terms,omitempty"`
	Capabilities      json.RawMessage `json:"capabilities,omitempty"`
	GrammarIRHash     string          `json:"grammar_ir_hash,omitempty"`
}
type speechFeedbackInput struct {
	ID       string              `json:"event_id"`
	Version  string              `json:"template_version"`
	Session  string              `json:"session_id"`
	Revision int                 `json:"accepted_revision"`
	Section  string              `json:"section_id"`
	Final    string              `json:"accepted_section_text"`
	Segments []speechSegment     `json:"segments"`
	Changes  []map[string]string `json:"changes,omitempty"`
}

func speechTemplate(ctx context.Context, tx pgx.Tx, p principal, id, version string) (Template, error) {
	var raw []byte
	var t Template
	var owner *string
	e := tx.QueryRow(ctx, `SELECT v.document,t.owner_id::text FROM wai_templates t JOIN wai_template_versions v ON v.template_id=t.id AND v.tenant_id=t.tenant_id WHERE t.id::text=$4 AND (v.id::text=$5 OR ($5='' AND v.id=t.current_version)) AND `+visibleTemplateSQL, p.Tenant, p.User, p.InstanceAdmin, id, version).Scan(&raw, &owner)
	if e == nil {
		e = json.Unmarshal(raw, &t)
	}
	if owner != nil {
		t.OwnerID = *owner
	}
	return t, e
}
func sectionKnown(t Template, id string) bool {
	for _, s := range t.Sections {
		if s.ID == id {
			return true
		}
	}
	return false
}
func speechLock(ctx context.Context, tx pgx.Tx, p principal, id string) error {
	_, e := tx.Exec(ctx, `INSERT INTO wai_speech_vocabularies(tenant_id,template_id)VALUES($1,$2)ON CONFLICT DO NOTHING`, p.Tenant, id)
	if e == nil {
		_, e = tx.Exec(ctx, `SELECT version FROM wai_speech_vocabularies WHERE tenant_id=$1 AND template_id=$2 FOR UPDATE`, p.Tenant, id)
	}
	return e
}
func (m *Module) speechTerms(ctx context.Context, tx pgx.Tx, p principal, t Template) ([]speechTerm, map[string][]speechTerm, error) {
	rows, e := tx.Query(ctx, `SELECT section_id,term_key,term,status,source,observations,first_observed::text,provenance::text FROM wai_speech_terms WHERE tenant_id=$1 AND template_id=$2 ORDER BY section_id,observations DESC,term_key`, p.Tenant, t.ID)
	if e != nil {
		return nil, nil, e
	}
	defer rows.Close()
	all := []speechTerm{}
	by := map[string][]speechTerm{}
	for rows.Next() {
		var s string
		var b []byte
		var v speechTerm
		if e = rows.Scan(&s, &v.Key, &b, &v.Status, &v.Source, &v.Observations, &v.First, &v.Provenance); e != nil {
			return nil, nil, e
		}
		if e = m.box.open(b, p.Tenant+":"+t.ID+":"+s+":"+v.Key, &v.Text); e != nil {
			return nil, nil, e
		}
		if sectionKnown(t, s) {
			by[s] = append(by[s], v)
			all = append(all, v)
		}
	}
	if e = rows.Err(); e != nil {
		return nil, nil, e
	}
	for _, section := range t.Sections {
		by[section.ID] = mergeTemplateSpeechTerms(section.SpeechTerms, by[section.ID])
	}
	all = nil
	for _, section := range t.Sections {
		all = append(all, by[section.ID]...)
	}
	return all, by, nil
}
func grammarIR(t Template, s Section) map[string]any {
	fields := []map[string]any{}
	for _, f := range t.Fields {
		if f.Section != s.ID {
			continue
		}
		if f.Type == "boolean" || f.Type == "number" {
			v := map[string]any{"field": f.Key, "kind": f.Type, "unit": f.Unit, "minimum": f.Min, "maximum": f.Max}
			if f.Type == "boolean" {
				v["alternatives"] = []string{"da", "nu", "prezent", "absent", "nedocumentat"}
			}
			fields = append(fields, v)
		}
	}
	return map[string]any{"schema": "wisemed.speech-grammar-ir.v1", "section_id": s.ID, "narrative_unrestricted": true, "fields": fields, "runtime_artifacts": []any{}, "runtime_grammar_supported": false}
}
func (m *Module) speechAdaptation(w http.ResponseWriter, r *http.Request, p principal, parts []string) {
	// templates/{id}/speech-adaptation or templates/{id}/sections/{section}/speech-{terms,feedback}
	if len(parts) < 3 {
		fail(w, 404, "not_found")
		return
	}
	ctx := r.Context()
	tx, e := m.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	t, e := speechTemplate(ctx, tx, p, parts[1], r.URL.Query().Get("version_id"))
	if e != nil {
		fail(w, 404, "template_not_found")
		return
	}
	if len(parts) == 3 && parts[2] == "speech-adaptation" && r.Method == "GET" {
		if r.URL.Query().Get("manage") == "1" && p.Role != "admin" && !p.InstanceAdmin && t.OwnerID != p.User {
			fail(w, 403, "template_editor_required")
			return
		}
		_, by, e := m.speechTerms(ctx, tx, p, t)
		if e != nil {
			fail(w, 503, "adaptation_unavailable")
			return
		}
		var version int64 = 1
		e = tx.QueryRow(ctx, `SELECT version FROM wai_speech_vocabularies WHERE tenant_id=$1 AND template_id=$2`, p.Tenant, t.ID).Scan(&version)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			fail(w, 503, "adaptation_unavailable")
			return
		}
		sections := []map[string]any{}
		for _, s := range t.Sections {
			terms := activeSpeechTerms(by[s.ID])
			grammar := grammarIR(t, s)
			b, _ := json.Marshal(grammar)
			if r.URL.Query().Get("manage") != "1" {
				by[s.ID] = nil
			}
			sections = append(sections, map[string]any{"id": s.ID, "label": s.Label, "terms": terms, "entries": by[s.ID], "grammar": grammar, "grammar_hash": digest(string(b))})
		}
		pack := map[string]any{"schema": speechSchema, "tenant_id": p.Tenant, "template_id": t.ID, "template_name": t.Name, "template_version": t.VersionID, "vocabulary_version": version, "active_budget_bytes": activeSpeechBudget, "sections": sections}
		raw, _ := json.Marshal(pack)
		hash := digest(string(raw))
		pack["canonical_json"] = string(raw)
		pack["sha256"] = hash
		w.Header().Set("ETag", `"`+hash+`"`)
		w.Header().Set("Cache-Control", "private, no-cache")
		if r.Header.Get("If-None-Match") == `"`+hash+`"` {
			w.WriteHeader(304)
			return
		}
		respond(w, 200, pack)
		return
	}
	if len(parts) != 5 || parts[2] != "sections" || !sectionKnown(t, parts[3]) {
		fail(w, 404, "section_not_found")
		return
	}
	if parts[4] == "speech-feedback" && r.Method == "POST" {
		m.receiveSpeechFeedback(w, r, p, tx, t, parts[3])
		return
	}
	if parts[4] != "speech-terms" || r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	if p.Role != "admin" && t.OwnerID != p.User && !p.InstanceAdmin {
		fail(w, 403, "template_editor_required")
		return
	}
	var in struct {
		Text   string `json:"text"`
		Status string `json:"status"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Text = normalizeSpeech(in.Text)
	if !safeSpeechTerm(in.Text) || (in.Status != "approved" && in.Status != "candidate" && in.Status != "rejected") {
		fail(w, 400, "invalid_or_identifying_term")
		return
	}
	if e = speechLock(ctx, tx, p, t.ID); e == nil {
		e = m.upsertSpeechTerm(ctx, tx, p, t, parts[3], in.Text, in.Status, "manual", "")
	}
	if e == nil {
		_, e = tx.Exec(ctx, `UPDATE wai_speech_vocabularies SET version=version+1 WHERE tenant_id=$1 AND template_id=$2`, p.Tenant, t.ID)
	}
	if e == nil {
		e = audit(ctx, tx, p, "speech.term.update", t.ID+":"+parts[3])
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 503, "speech_update_failed")
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
func (m *Module) upsertSpeechTerm(ctx context.Context, tx pgx.Tx, p principal, t Template, section, text, status, source, provenance string) error {
	key := digest(speechKey(text))
	event := provenance
	if event == "" {
		event = uuid.NewString()
	}
	var observed bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wai_speech_term_observations WHERE tenant_id=$1 AND template_id=$2 AND section_id=$3 AND term_key=$4 AND event_id=$5)`, p.Tenant, t.ID, section, key, event).Scan(&observed); e != nil {
		return e
	}
	if observed {
		return nil
	}
	raw, e := m.box.seal(text, p.Tenant+":"+t.ID+":"+section+":"+key)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO wai_speech_terms(tenant_id,template_id,section_id,term_key,term,status,source,provenance,author_id)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(tenant_id,template_id,section_id,term_key)DO UPDATE SET
 observations=wai_speech_terms.observations+1,last_observed=now(),
 status=CASE WHEN EXCLUDED.source='manual' THEN EXCLUDED.status ELSE wai_speech_terms.status END`, p.Tenant, t.ID, section, key, raw, status, source, nullString(provenance), p.User)
	if e == nil {
		_, e = tx.Exec(ctx, `INSERT INTO wai_speech_term_observations(tenant_id,template_id,section_id,term_key,event_id,source)VALUES($1,$2,$3,$4,$5,$6)`, p.Tenant, t.ID, section, key, event, source)
	}
	return e
}

// A bounded token diff. Only a single local substitution can be a terminology
// observation. Insertions, negation/value changes, multi-span rewrites and unknown
// words stay in encrypted feedback, outside the shared dictionary.
func speechCorrection(raw, corrected string) (string, string) {
	strip := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsPunct(r) && r != '-' {
				return -1
			}
			return r
		}, s)
	}
	cleanRaw, cleanCorrected := strip(raw), strip(corrected)
	if speechKey(cleanRaw) == speechKey(cleanCorrected) {
		return "", "formatting"
	}
	a, b := strings.Fields(speechKey(cleanRaw)), strings.Fields(speechKey(cleanCorrected))
	if len(a) > 512 || len(b) > 512 {
		return "", "ambiguous"
	}
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	j, k := len(a), len(b)
	for j > i && k > i && a[j-1] == b[k-1] {
		j--
		k--
	}
	if i == j && i == k {
		return "", "formatting"
	}
	if j == i || k == i {
		return "", "addition_or_removal"
	}
	from, to := strings.Join(a[i:j], " "), strings.Join(b[i:k], " ")
	for _, s := range []string{from, to} {
		for _, r := range s {
			if unicode.IsDigit(r) {
				return "", "measurement"
			}
		}
	}
	for _, w := range []string{"nu", "fără", "fara", "absent", "prezent", "drept", "stâng", "stang", "bilateral", "cm", "mm"} {
		if strings.Contains(" "+from+" ", " "+w+" ") || strings.Contains(" "+to+" ", " "+w+" ") {
			return "", "clinical_change"
		}
	}
	if j-i > 3 || k-i > 3 || !medicalSpeechTerm(to) || !safeSpeechTerm(from) {
		return "", "ambiguous"
	}
	// Require substantial letter overlap to avoid learning a new finding as STT.
	nearby := strings.Join(a[max(0, i-4):min(len(a), j+2)], " ") + " " + strings.Join(b[max(0, i-4):min(len(b), k+2)], " ")
	for _, marker := range []string{"pacient", "nume", "prenume", "strada", "telefon", "cnp", "născut", "nascut", "adresa", "adresă"} {
		if strings.Contains(nearby, marker) {
			return "", "identifying_context"
		}
	}
	x, y := []rune(strings.ReplaceAll(from, " ", "")), []rune(strings.ReplaceAll(to, " ", ""))
	if len(x) < 4 || len(y) < 4 {
		return "", "ambiguous"
	}
	distance := make([]int, len(y)+1)
	for j := range distance {
		distance[j] = j
	}
	for i, a := range x {
		diagonal := distance[0]
		distance[0] = i + 1
		for j, b := range y {
			old := distance[j+1]
			cost := 1
			if a == b {
				cost = 0
			}
			distance[j+1] = min(distance[j+1]+1, distance[j]+1, diagonal+cost)
			diagonal = old
		}
	}
	if distance[len(y)]*3 > max(len(x), len(y)) {
		return "", "ambiguous"
	}
	display := strings.Join(strings.Fields(normalizeSpeech(cleanCorrected))[i:k], " ")
	if !safeSpeechTerm(display) {
		return "", "identifying_or_ambiguous"
	}
	return display, "terminology_candidate"
}
func (m *Module) receiveSpeechFeedback(w http.ResponseWriter, r *http.Request, p principal, tx pgx.Tx, t Template, section string) {
	var in speechFeedbackInput
	if !decode(w, r, &in) {
		return
	}
	if _, e := uuid.Parse(in.ID); e != nil || in.Version != t.VersionID || in.Section != section || len(in.Final) > 32000 || len(in.Segments) == 0 || len(in.Segments) > 100 {
		fail(w, 400, "invalid_speech_feedback")
		return
	}
	ctx := r.Context()
	var accepted bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wai_approvals a JOIN wai_sessions s ON s.id=a.session_id AND s.tenant_id=a.tenant_id WHERE a.tenant_id=$1 AND a.session_id::text=$2 AND a.revision=$3 AND a.user_id=$4 AND s.template_version=$5)`, p.Tenant, in.Session, in.Revision, p.User, t.VersionID).Scan(&accepted)
	if e != nil || !accepted {
		fail(w, 409, "accepted_revision_required")
		return
	}
	ids := map[string]bool{}
	for _, s := range in.Segments {
		if s.ID == "" || ids[s.ID] || len(s.Raw) > 16000 || len(s.Corrected) > 30 || len(s.Model) > 150 || len(s.Runtime) > 100 || s.EndMS < s.StartMS || s.StartMS < 0 {
			fail(w, 400, "invalid_speech_segment")
			return
		}
		if _, err := uuid.Parse(s.ID); err != nil {
			fail(w, 400, "invalid_speech_segment")
			return
		}
		var known bool
		if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wai_session_chunks WHERE tenant_id=$1 AND session_id=$2 AND id::text=$3)`, p.Tenant, in.Session, s.ChunkID).Scan(&known) != nil || !known {
			fail(w, 409, "source_chunk_required")
			return
		}
		if len(s.Timings) > 2048 || len(s.Terms) > 64 || len(s.Capabilities) > 16000 {
			fail(w, 400, "invalid_adaptation_metadata")
			return
		}
		bytes := 0
		for _, term := range s.Terms {
			bytes += len(term) + 1
			if !safeSpeechTerm(term) {
				fail(w, 400, "invalid_context_term")
				return
			}
		}
		if bytes > activeSpeechBudget {
			fail(w, 400, "invalid_context_budget")
			return
		}
		for _, timing := range s.Timings {
			if len(timing.Text) > 500 || timing.Start < 0 || timing.Start > 30000 || (timing.End != nil && (*timing.End < timing.Start || *timing.End > 30000)) {
				fail(w, 400, "invalid_asr_timing")
				return
			}
		}
		ids[s.ID] = true
		for _, v := range s.Corrected {
			if len(v) > 16000 {
				fail(w, 400, "invalid_speech_segment")
				return
			}
		}
		if s.Verified && strings.TrimSpace(s.Transcript) == "" {
			fail(w, 400, "verified_transcript_required")
			return
		}
	}
	if e = speechLock(ctx, tx, p, t.ID); e != nil {
		fail(w, 503, "database_error")
		return
	}
	encoded, _ := json.Marshal(in)
	hash := digest(string(encoded))
	acceptedText, e := acceptedSpeechSection(ctx, tx, m, p, in.Session, section)
	if e != nil {
		fail(w, 503, "accepted_report_unavailable")
		return
	}
	in.Final = acceptedText
	in.Changes = nil
	for _, segment := range in.Segments {
		correction := ""
		if len(segment.Corrected) > 0 {
			correction = segment.Corrected[len(segment.Corrected)-1]
		}
		if segment.Verified {
			correction = segment.Transcript
		}
		kind := "no_verified_audio_target"
		if correction != "" {
			_, kind = speechCorrection(segment.Raw, correction)
		}
		in.Changes = append(in.Changes, map[string]string{"segment_id": segment.ID, "kind": kind})
	}
	var existing, oldHash string
	e = tx.QueryRow(ctx, `SELECT id,digest FROM wai_speech_feedback WHERE tenant_id=$1 AND session_id=$2 AND accepted_revision=$3 AND section_id=$4`, p.Tenant, in.Session, in.Revision, section).Scan(&existing, &oldHash)
	if e == nil {
		if hash != oldHash {
			fail(w, 409, "feedback_revision_already_recorded")
			return
		}
		respond(w, 200, map[string]any{"event_id": existing, "duplicate": true})
		return
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		fail(w, 503, "database_error")
		return
	}
	raw, e := m.box.seal(in, p.Tenant+":"+in.ID+":speech-feedback")
	if e == nil {
		_, e = tx.Exec(ctx, `INSERT INTO wai_speech_feedback(id,tenant_id,user_id,template_id,template_version,section_id,session_id,accepted_revision,digest,payload)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, in.ID, p.Tenant, p.User, t.ID, t.VersionID, section, in.Session, in.Revision, hash, raw)
	}
	count := 0
	observed := map[string]bool{}
	for _, s := range in.Segments {
		corrected := ""
		if len(s.Corrected) > 0 {
			corrected = s.Corrected[len(s.Corrected)-1]
		}
		if s.Verified {
			corrected = s.Transcript
		}
		if corrected == "" {
			continue
		}
		term, _ := speechCorrection(s.Raw, corrected)
		if term == "" || observed[speechKey(term)] {
			continue
		}
		observed[speechKey(term)] = true
		status := "candidate"
		// Only an already approved medical term can be confirmed automatically by a
		// correction. Novel terms require a reviewer and never create replacement rules.
		var approved bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wai_speech_terms WHERE tenant_id=$1 AND template_id=$2 AND section_id=$3 AND term_key=$4 AND status='approved')`, p.Tenant, t.ID, section, digest(speechKey(term))).Scan(&approved)
		if approved || s.Verified {
			status = "approved"
		}
		if e == nil {
			e = m.upsertSpeechTerm(ctx, tx, p, t, section, term, status, "correction", in.ID)
			count++
		}
	}
	if e == nil && count > 0 {
		_, e = tx.Exec(ctx, `UPDATE wai_speech_vocabularies SET version=version+1 WHERE tenant_id=$1 AND template_id=$2`, p.Tenant, t.ID)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 503, "feedback_unavailable")
		return
	}
	respond(w, 200, map[string]any{"event_id": in.ID, "terms_observed": count})
}

func publishSpeechGrammar(ctx context.Context, tx pgx.Tx, tenant string, t Template) error {
	for _, section := range t.Sections {
		ir := grammarIR(t, section)
		raw, e := json.Marshal(ir)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO wai_speech_grammars(tenant_id,template_version,section_id,schema_version,ir,sha256)VALUES($1,$2,$3,'wisemed.speech-grammar-ir.v1',$4,$5)`, tenant, t.VersionID, section.ID, raw, digest(string(raw))); e != nil {
			return e
		}
	}
	return nil
}

func acceptedSpeechSection(ctx context.Context, tx pgx.Tx, m *Module, p principal, session, section string) (string, error) {
	var raw []byte
	if e := tx.QueryRow(ctx, `SELECT result FROM wai_sessions WHERE tenant_id=$1 AND id::text=$2 AND user_id=$3`, p.Tenant, session, p.User).Scan(&raw); e != nil {
		return "", e
	}
	if len(raw) == 0 {
		return "", nil
	}
	var result Result
	if e := m.box.open(raw, p.Tenant+":"+session, &result); e != nil {
		return "", e
	}
	root, e := xhtml.Parse(strings.NewReader(result.HTML))
	if e != nil {
		return "", e
	}
	var out strings.Builder
	var walk func(*xhtml.Node, bool)
	walk = func(n *xhtml.Node, inside bool) {
		for _, a := range n.Attr {
			if a.Key == "data-section" && a.Val == section {
				inside = true
			}
		}
		if inside && n.Type == xhtml.TextNode {
			out.WriteString(n.Data)
			out.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inside)
		}
	}
	walk(root, false)
	return normalizeSpeech(out.String()), nil
}
