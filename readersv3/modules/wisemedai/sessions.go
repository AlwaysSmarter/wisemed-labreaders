package wisemedai

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Session struct {
	Template *Template      `json:"template,omitempty"`
	ID       string         `json:"id"`
	Version  string         `json:"template_version"`
	Status   string         `json:"status"`
	Revision int            `json:"revision"`
	Context  map[string]any `json:"context"`
	Chunks   []Chunk        `json:"chunks"`
	Result   *Result        `json:"result"`
	Created  time.Time      `json:"created_at"`
}

func (m *Module) loadSession(ctx context.Context, tx pgx.Tx, p principal, id string) (s Session, e error) {
	var raw, res []byte
	e = tx.QueryRow(ctx, `SELECT id,template_version,status,revision,context,result,created_at FROM wai_sessions WHERE id::text=$1 AND tenant_id=$2 AND user_id=$3 FOR UPDATE`, id, p.Tenant, p.User).Scan(&s.ID, &s.Version, &s.Status, &s.Revision, &raw, &res, &s.Created)
	if e != nil {
		return
	}
	if e = m.box.open(raw, p.Tenant+":"+s.ID, &s.Context); e != nil {
		return
	}
	if len(res) > 0 {
		var result Result
		if e = m.box.open(res, p.Tenant+":"+s.ID, &result); e != nil {
			return
		}
		s.Result = &result
	}
	s.Chunks = []Chunk{}
	rows, e := tx.Query(ctx, `SELECT id,sequence,revision,is_final,timestamp,text,target_field FROM wai_session_chunks WHERE session_id=$1 AND tenant_id=$2 ORDER BY sequence`, s.ID, p.Tenant)
	if e != nil {
		return s, e
	}
	defer rows.Close()
	for rows.Next() {
		var c Chunk
		var text []byte
		if e = rows.Scan(&c.ID, &c.Sequence, &c.Revision, &c.Final, &c.Timestamp, &text, &c.TargetField); e != nil {
			return
		}
		if e = m.box.open(text, p.Tenant+":"+s.ID+":"+c.ID, &c.Text); e != nil {
			return
		}
		s.Chunks = append(s.Chunks, c)
	}
	e = rows.Err()
	rows.Close()
	if e == nil {
		var t Template
		t, e = loadTemplate(ctx, tx, p.Tenant, s.Version)
		if e == nil {
			s.Template = &t
		}
	}
	return
}
func (m *Module) saveResult(ctx context.Context, tx pgx.Tx, p principal, s *Session, result Result) error {
	b, e := m.box.seal(result, p.Tenant+":"+s.ID)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE wai_sessions SET result=$1,revision=$2,status=$3,updated_at=now() WHERE id=$4 AND tenant_id=$5`, b, result.Revision, result.Status, s.ID, p.Tenant); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO wai_session_revisions(id,tenant_id,session_id,revision,payload)VALUES($1,$2,$3,$4,$5)`, uuid.NewString(), p.Tenant, s.ID, result.Revision, b); e != nil {
		return e
	}
	facts, e := m.box.seal(result.Evidence, p.Tenant+":"+s.ID)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO wai_extracted_facts(id,tenant_id,session_id,revision,payload)VALUES($1,$2,$3,$4,$5)`, uuid.NewString(), p.Tenant, s.ID, result.Revision, facts); e != nil {
		return e
	}
	s.Result = &result
	s.Revision = result.Revision
	s.Status = result.Status
	return nil
}
func (m *Module) sessions(w http.ResponseWriter, r *http.Request, p principal, suffix string) {
	ctx := r.Context()
	path := strings.Split(strings.Trim(suffix, "/"), "/")
	if suffix == "" || suffix == "/" {
		if r.Method == "GET" {
			rows, e := m.db.Query(ctx, `SELECT id,template_version,status,revision,created_at FROM wai_sessions WHERE tenant_id=$1 AND user_id=$2 ORDER BY created_at DESC LIMIT 200`, p.Tenant, p.User)
			if e != nil {
				fail(w, 503, "database_error")
				return
			}
			defer rows.Close()
			out := []Session{}
			for rows.Next() {
				var s Session
				if rows.Scan(&s.ID, &s.Version, &s.Status, &s.Revision, &s.Created) != nil {
					fail(w, 500, "database_error")
					return
				}
				out = append(out, s)
			}
			respond(w, 200, out)
			return
		}
		if r.Method != "POST" {
			fail(w, 405, "method_not_allowed")
			return
		}
		m.createSession(w, r, p)
		return
	}
	id := path[0]
	if _, e := uuid.Parse(id); e != nil {
		fail(w, 404, "session_not_found")
		return
	}
	tx, e := m.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	s, e := m.loadSession(ctx, tx, p, id)
	if e != nil {
		fail(w, 404, "session_not_found")
		return
	}
	action := ""
	if len(path) > 1 {
		action = path[1]
	}
	if len(path) > 2 {
		fail(w, 404, "not_found")
		return
	}
	if r.Method == "DELETE" && action == "" {
		if _, e = tx.Exec(ctx, `DELETE FROM wai_sessions WHERE id=$1 AND tenant_id=$2`, s.ID, p.Tenant); e == nil {
			e = audit(ctx, tx, p, "session.delete", s.ID)
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		if e != nil {
			fail(w, 503, "database_error")
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method == "GET" {
		switch action {
		case "":
			_ = tx.Commit(ctx)
			respond(w, 200, s)
		case "result":
			if s.Result == nil {
				fail(w, 409, "result_unavailable")
				return
			}
			_ = tx.Commit(ctx)
			respond(w, 200, s.Result)
		case "events":
			_ = tx.Commit(ctx)
			m.eventStream(w, r, p, s.ID)
		default:
			fail(w, 404, "not_found")
		}
		return
	}
	if r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	switch action {
	case "field-edit", "field-options":
		m.sessionField(w, r, p, tx, s, action == "field-options")
	case "presentation":
		m.sessionPresentation(w, r, p, tx, s)
	case "deliver":
		_ = tx.Commit(ctx)
		m.deliver(w, r, p, s)
	case "briefing":
		_ = tx.Commit(ctx)
		respond(w, 200, map[string]any{"patient_ref": s.Context["patient_ref"], "message": "Nu există istoric HIS autorizat atașat. Informarea este disponibilă exclusiv la cerere; nu se deduce istoric din memorie.", "source": "standalone"})
	case "audio-transcription":
		if s.Status != "recording" {
			fail(w, 409, "session_locked")
			return
		}
		var in struct {
			Audio []byte `json:"audio"`
		}
		if !decode(w, r, &in) {
			return
		}
		seconds, valid := dictationAudioSeconds(in.Audio)
		if !valid {
			fail(w, 400, "invalid_audio")
			return
		}
		t, e := loadTemplate(ctx, tx, p.Tenant, s.Version)
		if e != nil {
			fail(w, 500, "template_unavailable")
			return
		}
		text, e := m.transcribe(ctx, tx, p, s, t, in.Audio, seconds)
		if e != nil {
			if isCommercialError(e) {
				billingFailure(w, e)
			} else if e.Error() == "audio_transcription_not_configured" {
				fail(w, 503, "audio_transcription_not_configured")
			} else {
				fail(w, 502, safeExtractionError(e))
			}
			return
		}
		respond(w, 200, map[string]string{"text": text})
	case "transcript-chunks", "audio-transcript-chunks":
		if s.Status != "recording" {
			fail(w, 409, "session_locked")
			return
		}
		var in Chunk
		if !decode(w, r, &in) {
			return
		}
		if _, e = uuid.Parse(in.ID); e != nil || in.Sequence < 1 || in.Sequence > 10000 || in.Revision < 1 || len(in.Text) > 16000 || strings.TrimSpace(in.Text) == "" || in.Timestamp.IsZero() {
			fail(w, 400, "invalid_chunk")
			return
		}
		in.Timestamp = in.Timestamp.UTC().Truncate(time.Microsecond)
		existing := -1
		total := len(in.Text)
		for i, c := range s.Chunks {
			if c.ID == in.ID {
				existing = i
			}
			if c.Sequence == in.Sequence && c.ID != in.ID {
				fail(w, 409, "sequence_conflict")
				return
			}
			if c.ID != in.ID {
				total += len(c.Text)
			}
		}
		if total > 100000 || len(s.Chunks) >= 500 && existing < 0 {
			fail(w, 413, "session_limit")
			return
		}
		if existing >= 0 {
			old := s.Chunks[existing]
			if in.Revision < old.Revision {
				respond(w, 200, map[string]any{"accepted": false, "reason": "stale_revision", "result": s.Result})
				return
			}
			if in.Revision == old.Revision {
				if in.TargetField != old.TargetField || in.Sequence != old.Sequence || in.Final != old.Final || in.Text != old.Text || !in.Timestamp.Equal(old.Timestamp) {
					fail(w, 409, "chunk_revision_conflict")
					return
				}
				respond(w, 200, map[string]any{"accepted": true, "duplicate": true, "result": s.Result})
				return
			}
			if in.Sequence != old.Sequence {
				fail(w, 409, "sequence_immutable")
				return
			}
			s.Chunks[existing] = in
		} else {
			s.Chunks = append(s.Chunks, in)
		}
		sort.Slice(s.Chunks, func(i, j int) bool { return s.Chunks[i].Sequence < s.Chunks[j].Sequence })
		encrypted, e := m.box.seal(in.Text, p.Tenant+":"+s.ID+":"+in.ID)
		if e != nil {
			fail(w, 500, "encryption_error")
			return
		}
		raw, _ := json.Marshal(in)
		if _, e = tx.Exec(ctx, `INSERT INTO wai_session_chunks(id,tenant_id,session_id,sequence,revision,is_final,timestamp,text,digest,target_field)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(session_id,id)DO UPDATE SET revision=excluded.revision,is_final=excluded.is_final,timestamp=excluded.timestamp,text=excluded.text,digest=excluded.digest,target_field=excluded.target_field`, in.ID, p.Tenant, s.ID, in.Sequence, in.Revision, in.Final, in.Timestamp, encrypted, digest(string(raw)), in.TargetField); e != nil {
			fail(w, 409, "chunk_conflict")
			return
		}
		t, e := loadTemplate(ctx, tx, p.Tenant, s.Version)
		if e != nil {
			fail(w, 500, "template_unavailable")
			return
		}
		if e = templateUseActive(ctx, tx, p, t); e != nil {
			billingFailure(w, e)
			return
		}
		if in.TargetField != "" {
			if _, ok := fieldTemplate(t, in.TargetField); !ok {
				fail(w, 400, "unknown_field")
				return
			}
			if overrides, ok := s.Context["field_overrides"].(map[string]any); ok {
				delete(overrides, in.TargetField)
				if e = m.saveSessionContext(ctx, tx, p, s); e != nil {
					fail(w, 503, "database_error")
					return
				}
			}
		}
		facts, e := m.extractChunk(withAIUsage(ctx, s.ID, s.Version, "transcript_extract"), tx, p, t, in)
		if e != nil {
			if isCommercialError(e) {
				billingFailure(w, e)
			} else {
				fail(w, 502, safeExtractionError(e))
			}
			return
		}
		result, e := buildSessionResult(applyContext(t, s.Context, p), s, facts, in.TargetField)
		if e != nil {
			fail(w, 422, e.Error())
			return
		}
		if e = m.saveResult(ctx, tx, p, &s, result); e == nil {
			e = audit(ctx, tx, p, "session.chunk", s.ID)
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		if e != nil {
			fail(w, 503, "database_error")
			return
		}
		m.notify(ctx, s, "progress")
		respond(w, 200, map[string]any{"accepted": true, "patch_revision": s.Revision, "result": s.Result})
	case "finalize":
		if s.Status != "recording" {
			respond(w, 200, s.Result)
			return
		}
		if len(s.Chunks) == 0 {
			fail(w, 409, "transcript_required")
			return
		}
		for _, c := range s.Chunks {
			if !c.Final {
				fail(w, 409, "final_chunks_required")
				return
			}
		}
		var result Result
		if s.Result != nil {
			result = *s.Result
			result.Revision++
		} else {
			t, err := loadTemplate(ctx, tx, p.Tenant, s.Version)
			if err != nil {
				fail(w, 500, "template_unavailable")
				return
			}
			if err = templateUseActive(ctx, tx, p, t); err != nil {
				billingFailure(w, err)
				return
			}
			facts := []Fact{}
			for _, chunk := range s.Chunks {
				var extracted []Fact
				extracted, err = m.extractChunk(withAIUsage(ctx, s.ID, s.Version, "transcript_extract"), tx, p, t, chunk)
				if err != nil {
					break
				}
				facts = append(facts, extracted...)
			}
			if err != nil {
				if isCommercialError(err) {
					billingFailure(w, err)
				} else {
					fail(w, 502, safeExtractionError(err))
				}
				return
			}
			result, err = buildSessionResult(applyContext(t, s.Context, p), s, facts, "")
			if err != nil {
				fail(w, 422, err.Error())
				return
			}
		}
		result.Status = "review"
		if e = m.saveResult(ctx, tx, p, &s, result); e == nil {
			e = audit(ctx, tx, p, "session.finalize", s.ID)
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		if e != nil {
			fail(w, 503, "database_error")
			return
		}
		respond(w, 200, s.Result)
	case "reopen":
		var in struct {
			Revision int `json:"revision"`
		}
		if !decode(w, r, &in) {
			return
		}
		if (s.Status != "review" && s.Status != "approved") || s.Result == nil || in.Revision != s.Revision {
			fail(w, 409, "current_review_required")
			return
		}
		result := *s.Result
		result.Status = "recording"
		result.Revision++
		if e = m.saveResult(ctx, tx, p, &s, result); e == nil {
			e = audit(ctx, tx, p, "session.reopen", s.ID)
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		if e != nil {
			fail(w, 503, "database_error")
			return
		}
		m.notify(ctx, s, "progress")
		respond(w, 200, s.Result)
	case "approve":
		var in struct {
			Revision  int  `json:"revision"`
			Confirmed bool `json:"confirmed"`
		}
		if !decode(w, r, &in) {
			return
		}
		if !in.Confirmed || in.Revision != s.Revision || s.Result == nil {
			fail(w, 409, "explicit_current_revision_required")
			return
		}
		if s.Status == "approved" {
			respond(w, 200, s.Result)
			return
		}
		if s.Status != "review" {
			fail(w, 409, "finalize_required")
			return
		}
		if len(s.Result.ValidationErrors) > 0 {
			fail(w, 422, "field_validation_failed")
			return
		}
		if len(s.Result.Missing) > 0 {
			fail(w, 422, "required_fields_missing")
			return
		}
		result := *s.Result
		result.Status = "approved"
		b, e := m.box.seal(result, p.Tenant+":"+s.ID)
		if e != nil {
			fail(w, 500, "encryption_error")
			return
		}
		rid := uuid.NewString()
		if _, e = tx.Exec(ctx, `INSERT INTO wai_clinical_results(id,tenant_id,session_id,revision,payload)VALUES($1,$2,$3,$4,$5)`, rid, p.Tenant, s.ID, s.Revision, b); e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO wai_approvals(id,tenant_id,session_id,user_id,revision)VALUES($1,$2,$3,$4,$5)`, uuid.NewString(), p.Tenant, s.ID, p.User, s.Revision)
		}
		if e == nil {
			_, e = tx.Exec(ctx, `UPDATE wai_sessions SET status='approved',result=$1,updated_at=now() WHERE id=$2 AND tenant_id=$3`, b, s.ID, p.Tenant)
		}
		if e == nil {
			e = audit(ctx, tx, p, "result.approve", s.ID)
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		if e != nil {
			fail(w, 503, "database_error")
			return
		}
		m.notify(ctx, Session{ID: s.ID, Revision: s.Revision, Status: "approved"}, "result")
		respond(w, 200, result)
	case "pairing":
		token := randomToken()
		_, e = tx.Exec(ctx, `INSERT INTO wai_pairing_tokens(token_hash,tenant_id,user_id,session_id,expires_at)VALUES($1,$2,$3,$4,now()+interval '2 minutes')`, digest(token), p.Tenant, p.User, s.ID)
		if e == nil {
			e = audit(ctx, tx, p, "session.pairing", s.ID)
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		if e != nil {
			fail(w, 503, "database_error")
			return
		}
		respond(w, 201, map[string]any{"token": token, "qr_png": pairingQR(token), "expires_in": 120, "session_id": s.ID})
	default:
		fail(w, 404, "not_found")
	}
}
func safeExtractionError(e error) string {
	for _, s := range []string{"openai_key_not_configured", "monthly_budget_exceeded", "openai_refused_or_truncated", "invalid_extraction_schema", "ai_token_allowance_exceeded"} {
		if e.Error() == s {
			return s
		}
	}
	return "extraction_failed"
}
func (m *Module) createSession(w http.ResponseWriter, r *http.Request, p principal) {
	var in struct {
		Version     string `json:"template_version"`
		PatientRef  string `json:"patient_ref"`
		WorkContext string `json:"work_context,omitempty"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.WorkContext != "" && in.WorkContext != "ambulatoriu" && in.WorkContext != "internare" && in.WorkContext != "ambele" {
		fail(w, 400, "invalid_work_context")
		return
	}
	if len(in.PatientRef) > 128 {
		fail(w, 400, "invalid_patient_reference")
		return
	}
	tx, e := m.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(r.Context())
	template, e := loadTemplate(r.Context(), tx, p.Tenant, in.Version)
	if e != nil {
		fail(w, 404, "template_not_found")
		return
	}

	id := uuid.NewString()
	key := r.Header.Get("Idempotency-Key")
	if len(key) > 128 {
		fail(w, 400, "invalid_idempotency_key")
		return
	}
	if key != "" {
		if _, e = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtext($1))`, p.Tenant+":"+p.User+":"+key); e != nil {
			fail(w, 503, "database_error")
			return
		}
		id = uuid.NewSHA1(uuid.NameSpaceURL, []byte(p.Tenant+":"+p.User+":"+key)).String()
		var exists bool
		_ = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM wai_sessions WHERE id=$1 AND tenant_id=$2 AND user_id=$3)`, id, p.Tenant, p.User).Scan(&exists)
		if exists {
			s, e := m.loadSession(r.Context(), tx, p, id)
			if e != nil {
				fail(w, 503, "database_error")
				return
			}
			previousContext, _ := s.Context["work_context"].(string)
			if s.Version != in.Version || s.Context["patient_ref"] != in.PatientRef || previousContext != in.WorkContext {
				fail(w, 409, "idempotency_conflict")
				return
			}
			respond(w, 200, s)
			return
		}
	}
	if e = templateUse(r.Context(), tx, p, template); e != nil {
		billingFailure(w, e)
		return
	}
	context := map[string]any{"work_context": in.WorkContext, "patient_ref": in.PatientRef, "source": "standalone", "authorized_by": p.User}
	raw, e := m.box.seal(context, p.Tenant+":"+id)
	if e != nil {
		fail(w, 500, "encryption_error")
		return
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO wai_sessions(id,tenant_id,user_id,template_version,context)VALUES($1,$2,$3,$4,$5)`, id, p.Tenant, p.User, in.Version, raw)
	if e == nil {
		e = audit(r.Context(), tx, p, "session.create", id)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		fail(w, 409, "session_conflict")
		return
	}
	m.notify(r.Context(), Session{ID: id, Status: "recording"}, "start")
	respond(w, 201, Session{Template: &template, ID: id, Version: in.Version, Status: "recording", Context: context, Chunks: []Chunk{}, Created: time.Now()})
}
func (m *Module) eventStream(w http.ResponseWriter, r *http.Request, p principal, id string) {
	if _, ok := w.(http.Flusher); !ok {
		fail(w, 400, "sse_not_supported_on_transport")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	last := r.Header.Get("Last-Event-ID")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var rev int
		var status string
		e := m.db.QueryRow(r.Context(), `SELECT revision,status FROM wai_sessions WHERE id=$1 AND tenant_id=$2 AND user_id=$3`, id, p.Tenant, p.User).Scan(&rev, &status)
		if e != nil {
			return
		}
		eventID := id + ":" + strconv.Itoa(rev) + ":" + status
		if eventID != last {
			b, _ := json.Marshal(map[string]any{"session_id": id, "revision": rev, "status": status, "result_url": "/v1/sessions/" + id + "/result"})
			_, _ = w.Write([]byte("id: " + eventID + "\nevent: progress\ndata: " + string(b) + "\n\n"))
			w.(http.Flusher).Flush()
			last = eventID
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
func (m *Module) redeem(w http.ResponseWriter, r *http.Request, p principal) {
	if r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &in) {
		return
	}
	var id string
	e := m.db.QueryRow(r.Context(), `DELETE FROM wai_pairing_tokens WHERE token_hash=$1 AND tenant_id=$2 AND user_id=$3 AND expires_at>now() RETURNING session_id`, digest(in.Token), p.Tenant, p.User).Scan(&id)
	if e != nil {
		fail(w, 410, "pairing_invalid_or_expired")
		return
	}
	respond(w, 200, map[string]any{"session_id": id})
}
