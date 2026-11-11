package wisemedai

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type speechConsent struct {
	Enabled    bool       `json:"enabled"`
	Notice     string     `json:"notice_version"`
	Generation string     `json:"generation"`
	Activated  *time.Time `json:"activated_at"`
	Withdrawn  *time.Time `json:"withdrawn_at"`
}

func readSpeechConsent(ctx context.Context, tx pgx.Tx, p principal, lock bool) (speechConsent, error) {
	v := speechConsent{Notice: speechNotice}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	e := tx.QueryRow(ctx, `SELECT enabled,notice_version,generation,activated_at,withdrawn_at FROM wai_speech_consent WHERE tenant_id=$1 AND user_id=$2`+suffix, p.Tenant, p.User).Scan(&v.Enabled, &v.Notice, &v.Generation, &v.Activated, &v.Withdrawn)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, nil
	}
	return v, e
}
func (m *Module) speechConsent(w http.ResponseWriter, r *http.Request, p principal) {
	ctx := r.Context()
	tx, e := m.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	// Serialize first activation as well as updates and contribution creation.
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, p.Tenant+":"+p.User+":speech-consent"); e != nil {
		fail(w, 503, "database_error")
		return
	}
	v, e := readSpeechConsent(ctx, tx, p, true)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	if r.Method == "GET" {
		respond(w, 200, v)
		return
	}
	if r.Method != "PUT" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		Enabled bool   `json:"enabled"`
		Notice  string `json:"notice_version"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Notice != speechNotice {
		fail(w, 409, "speech_notice_changed")
		return
	}
	generation := v.Generation
	if generation == "" || in.Enabled != v.Enabled || in.Notice != v.Notice {
		generation = uuid.NewString()
	}
	_, e = tx.Exec(ctx, `INSERT INTO wai_speech_consent(tenant_id,user_id,enabled,notice_version,generation,activated_at,withdrawn_at)
 VALUES($1,$2,$3,$4,$5,CASE WHEN $3 THEN now() END,CASE WHEN NOT $3 THEN now() END)
 ON CONFLICT(tenant_id,user_id)DO UPDATE SET enabled=$3,notice_version=$4,generation=$5,
 activated_at=CASE WHEN $3 AND (NOT wai_speech_consent.enabled OR wai_speech_consent.notice_version<>$4) THEN now() ELSE wai_speech_consent.activated_at END,
 withdrawn_at=CASE WHEN NOT $3 THEN now() ELSE wai_speech_consent.withdrawn_at END`, p.Tenant, p.User, in.Enabled, in.Notice, generation)
	if e == nil && v.Enabled != in.Enabled || v.Notice != in.Notice {
		_, e = tx.Exec(ctx, `INSERT INTO wai_speech_consent_events(id,tenant_id,user_id,enabled,notice_version)VALUES($1,$2,$3,$4,$5)`, uuid.NewString(), p.Tenant, p.User, in.Enabled, in.Notice)
	}
	if e == nil && !in.Enabled {
		_, e = tx.Exec(ctx, `UPDATE wai_speech_dataset_entries SET revoked=true WHERE contribution_id IN(SELECT id FROM wai_speech_contributions WHERE tenant_id=$1 AND user_id=$2)`, p.Tenant, p.User)
		if e == nil {
			_, e = tx.Exec(ctx, `DELETE FROM wai_speech_contributions WHERE tenant_id=$1 AND user_id=$2`, p.Tenant, p.User)
		}
	}
	if e == nil {
		e = audit(ctx, tx, p, "speech.consent.update", p.User)
	}
	if e == nil {
		v, e = readSpeechConsent(ctx, tx, p, false)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 503, "consent_update_failed")
		return
	}
	// Orphan ciphertext is removed by the retention sweep. It is immediately
	// unreachable by API and cannot enter an export after withdrawal.
	respond(w, 200, v)
}

type speechContribution struct {
	ID            string        `json:"id"`
	SourceSession string        `json:"source_session"`
	Section       string        `json:"section_id"`
	Template      string        `json:"template_id"`
	Version       string        `json:"template_version"`
	Revision      int           `json:"accepted_revision"`
	Segment       speechSegment `json:"segment"`
	Final         string        `json:"accepted_section_text"`
	Notice        string        `json:"notice_version"`
	Bytes         int           `json:"byte_length"`
	Hash          string        `json:"sha256"`
	DurationMS    int           `json:"duration_ms"`
	Codec         string        `json:"codec"`
	SampleRate    int           `json:"sample_rate"`
	Channels      int           `json:"channels"`
	Language      string        `json:"language"`
}

func (m *Module) speechFileRoot() (string, error) {
	root := m.rt.ResolvePath("community-speech")
	return root, os.MkdirAll(root, 0700)
}
func (m *Module) writeSpeechFile(payload any, scope string) (string, error) {
	root, e := m.speechFileRoot()
	if e != nil {
		return "", e
	}
	b, e := m.box.seal(payload, scope)
	if e != nil {
		return "", e
	}
	name := uuid.NewString() + ".enc"
	path := filepath.Join(root, name)
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		_ = os.Remove(path)
		return "", e
	}
	return name, nil
}
func (m *Module) readSpeechFile(name, scope string) ([]byte, error) {
	if filepath.Base(name) != name || !strings.HasSuffix(name, ".enc") {
		return nil, errors.New("invalid_audio_reference")
	}
	root, e := m.speechFileRoot()
	if e != nil {
		return nil, e
	}
	raw, e := os.ReadFile(filepath.Join(root, name))
	if e != nil {
		return nil, e
	}
	var audio []byte
	e = m.box.open(raw, scope, &audio)
	return audio, e
}
func speechWAV(b []byte) (int, error) {
	if len(b) < 46 || len(b) > 960044 || string(b[:4]) != "RIFF" || string(b[8:16]) != "WAVEfmt " || string(b[36:40]) != "data" {
		return 0, errors.New("invalid_wav")
	}
	u32 := binary.LittleEndian.Uint32
	u16 := binary.LittleEndian.Uint16
	if u32(b[4:8]) != uint32(len(b)-8) || u32(b[16:20]) != 16 || u16(b[20:22]) != 1 || u16(b[22:24]) != 1 || u32(b[24:28]) != 16000 || u32(b[28:32]) != 32000 || u16(b[32:34]) != 2 || u16(b[34:36]) != 16 || u32(b[40:44]) != uint32(len(b)-44) || (len(b)-44)%2 != 0 {
		return 0, errors.New("invalid_wav")
	}
	return (len(b) - 44) * 1000 / 32000, nil
}
func consentForCapture(ctx context.Context, tx pgx.Tx, p principal, s speechSegment) (speechConsent, error) {
	if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, p.Tenant+":"+p.User+":speech-consent"); e != nil {
		return speechConsent{}, e
	}
	consent, e := readSpeechConsent(ctx, tx, p, true)
	if e != nil {
		return consent, e
	}
	started, e := time.Parse(time.RFC3339Nano, s.CaptureStarted)
	if e != nil || !consent.Enabled || consent.Notice != speechNotice || s.ConsentGeneration != consent.Generation || consent.Activated == nil || started.Before(*consent.Activated) || started.After(time.Now().Add(time.Minute)) {
		return consent, errors.New("speech_consent_required")
	}
	return consent, nil
}
func (m *Module) communitySpeech(w http.ResponseWriter, r *http.Request, p principal, parts []string) {
	ctx := r.Context()
	tx, e := m.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, p.Tenant+":"+p.User+":speech-consent"); e != nil {
		fail(w, 503, "database_error")
		return
	}
	if _, e = readSpeechConsent(ctx, tx, p, true); e != nil {
		fail(w, 503, "database_error")
		return
	}
	// community/speech-contributions[/id[/audio|finalize|review]]
	if len(parts) == 2 && r.Method == "POST" {
		m.createSpeechContribution(w, r, p, tx)
		return
	}
	if len(parts) == 2 && r.Method == "GET" {
		admin := p.Role == "admin" || p.InstanceAdmin
		rows, e := tx.Query(ctx, `SELECT id,section_id,template_id,template_version,state,uploaded,byte_length,created_at FROM wai_speech_contributions WHERE tenant_id=$1 AND (user_id=$2 OR $3) ORDER BY created_at DESC LIMIT 200`, p.Tenant, p.User, admin)
		if e != nil {
			fail(w, 503, "database_error")
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, section, t, v, state string
			var uploaded, size int
			var at time.Time
			if rows.Scan(&id, &section, &t, &v, &state, &uploaded, &size, &at) != nil {
				fail(w, 503, "database_error")
				return
			}
			out = append(out, map[string]any{"id": id, "section_id": section, "template_id": t, "template_version": v, "state": state, "uploaded": uploaded, "byte_length": size, "created_at": at})
		}
		if rows.Err() != nil {
			fail(w, 503, "database_error")
			return
		}
		respond(w, 200, out)
		return
	}
	if len(parts) == 3 && parts[2] == "export" && r.Method == "POST" {
		m.exportSpeechDataset(w, r, p, tx)
		return
	}
	if len(parts) < 3 || len(parts) > 4 {
		fail(w, 404, "not_found")
		return
	}
	id, err := uuid.Parse(parts[2])
	if err != nil {
		fail(w, 404, "contribution_not_found")
		return
	}
	var owner, state, hash string
	var metadata []byte
	var uploaded, size int
	var rawPath, sanitizedPath *string
	e = tx.QueryRow(ctx, `SELECT user_id,state,audio_hash,metadata,uploaded,byte_length,raw_path,sanitized_path FROM wai_speech_contributions WHERE id=$1 AND tenant_id=$2 AND expires_at>now() FOR UPDATE`, id.String(), p.Tenant).Scan(&owner, &state, &hash, &metadata, &uploaded, &size, &rawPath, &sanitizedPath)
	admin := p.Role == "admin" || p.InstanceAdmin
	if e != nil || owner != p.User && !admin {
		fail(w, 404, "contribution_not_found")
		return
	}
	var item speechContribution
	if e = m.box.open(metadata, p.Tenant+":"+id.String()+":contribution", &item); e != nil {
		fail(w, 503, "contribution_unavailable")
		return
	}
	if len(parts) == 3 && r.Method == "DELETE" {
		if _, e = tx.Exec(ctx, `UPDATE wai_speech_dataset_entries SET revoked=true WHERE contribution_id=$1`, id.String()); e == nil {
			_, e = tx.Exec(ctx, `DELETE FROM wai_speech_contributions WHERE id=$1`, id.String())
		}
		if e == nil {
			e = audit(ctx, tx, p, "speech.contribution.delete", id.String())
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
	if len(parts) == 3 && r.Method == "GET" {
		if e = audit(ctx, tx, p, "speech.contribution.view", id.String()); e == nil {
			e = tx.Commit(ctx)
		}
		if e != nil {
			fail(w, 503, "database_error")
			return
		}
		respond(w, 200, map[string]any{"metadata": item, "state": state, "uploaded": uploaded})
		return
	}
	if len(parts) != 4 {
		fail(w, 405, "method_not_allowed")
		return
	}
	if parts[3] == "audio" && r.Method == "GET" {
		path := rawPath
		scope := p.Tenant + ":" + id.String() + ":audio"
		if r.URL.Query().Get("sanitized") == "1" {
			path = sanitizedPath
			scope += ":sanitized"
		}
		if path == nil {
			fail(w, 404, "audio_unavailable")
			return
		}
		audio, e := m.readSpeechFile(*path, scope)
		if e != nil {
			fail(w, 503, "audio_unavailable")
			return
		}
		if e = audit(ctx, tx, p, "speech.audio.view", id.String()); e == nil {
			e = tx.Commit(ctx)
		}
		if e != nil {
			fail(w, 503, "database_error")
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(200)
		_, _ = w.Write(audio)
		return
	}
	if parts[3] == "review" && r.Method == "POST" {
		if !admin {
			fail(w, 403, "admin_required")
			return
		}
		m.reviewSpeechContribution(w, r, p, tx, item, state, rawPath)
		return
	}
	if owner != p.User {
		fail(w, 403, "contribution_owner_required")
		return
	}
	if _, e = consentForCapture(ctx, tx, p, item.Segment); e != nil {
		fail(w, 409, "speech_consent_required")
		return
	}
	if parts[3] == "audio" && r.Method == "PUT" {
		if state != "uploading" {
			fail(w, 409, "upload_closed")
			return
		}
		offset, e := strconv.Atoi(r.URL.Query().Get("offset"))
		if e != nil || offset < 0 {
			fail(w, 400, "invalid_offset")
			return
		}
		b, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 256*1024))
		if e != nil || len(b) == 0 || offset+len(b) > size {
			fail(w, 400, "invalid_audio_part")
			return
		}
		partHash := digest(string(b))
		if offset < uploaded {
			var existing string
			e = tx.QueryRow(ctx, `SELECT digest FROM wai_speech_upload_parts WHERE contribution_id=$1 AND offset_bytes=$2`, id.String(), offset).Scan(&existing)
			if e != nil || existing != partHash {
				fail(w, 409, "audio_part_conflict")
				return
			}
			respond(w, 200, map[string]int{"uploaded": uploaded})
			return
		}
		if offset != uploaded {
			fail(w, 409, "unexpected_offset")
			return
		}
		path, e := m.writeSpeechFile(b, p.Tenant+":"+id.String()+":part:"+strconv.Itoa(offset))
		if e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO wai_speech_upload_parts(contribution_id,offset_bytes,byte_length,digest,path)VALUES($1,$2,$3,$4,$5)`, id.String(), offset, len(b), partHash, path)
		}
		if e == nil {
			_, e = tx.Exec(ctx, `UPDATE wai_speech_contributions SET uploaded=uploaded+$2,updated_at=now() WHERE id=$1`, id.String(), len(b))
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		if e != nil {
			fail(w, 503, "upload_failed")
			return
		}
		respond(w, 200, map[string]int{"uploaded": uploaded + len(b)})
		return
	}
	if parts[3] != "finalize" || r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	if state != "uploading" {
		respond(w, 200, map[string]string{"state": state})
		return
	}
	if size != uploaded {
		fail(w, 409, "upload_incomplete")
		return
	}
	rows, e := tx.Query(ctx, `SELECT offset_bytes,path FROM wai_speech_upload_parts WHERE contribution_id=$1 ORDER BY offset_bytes`, id.String())
	if e != nil {
		fail(w, 503, "upload_failed")
		return
	}
	paths := []struct {
		Offset int
		Path   string
	}{}
	for rows.Next() {
		var v struct {
			Offset int
			Path   string
		}
		if e = rows.Scan(&v.Offset, &v.Path); e != nil {
			break
		}
		paths = append(paths, v)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	audio := []byte{}
	for _, part := range paths {
		if e != nil {
			break
		}
		var b []byte
		b, e = m.readSpeechFile(part.Path, p.Tenant+":"+id.String()+":part:"+strconv.Itoa(part.Offset))
		if len(audio) != part.Offset {
			e = errors.New("invalid_offset")
		}
		audio = append(audio, b...)
	}
	duration, valid := speechWAV(audio)
	if e != nil || valid != nil || len(audio) != size || digest(string(audio)) != hash || duration != item.DurationMS {
		fail(w, 422, "invalid_audio_integrity")
		return
	}
	path, e := m.writeSpeechFile(audio, p.Tenant+":"+id.String()+":audio")
	if e == nil {
		_, e = tx.Exec(ctx, `UPDATE wai_speech_contributions SET raw_path=$2,state='pending_deidentification',updated_at=now() WHERE id=$1`, id.String(), path)
	}
	if e == nil {
		_, e = tx.Exec(ctx, `DELETE FROM wai_speech_upload_parts WHERE contribution_id=$1`, id.String())
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 503, "finalize_failed")
		return
	}
	respond(w, 200, map[string]string{"state": "pending_deidentification"})
}
func (m *Module) createSpeechContribution(w http.ResponseWriter, r *http.Request, p principal, tx pgx.Tx) {
	var in speechContribution
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	if _, e := uuid.Parse(in.ID); e != nil || in.Notice != speechNotice || in.Language != "ro" || in.Codec != "pcm_s16le_wav" || in.SampleRate != 16000 || in.Channels != 1 || in.Bytes < 46 || in.Bytes > 960044 || len(in.Final) > 32000 || len(in.Segment.Raw) > 16000 || len(in.Segment.Corrected) > 30 || len(in.Segment.Transcript) > 16000 || len(in.Hash) != 64 || in.Segment.ID == "" || len(in.Segment.ID) > 64 || in.DurationMS <= 0 || in.DurationMS > 30000 {
		fail(w, 400, "invalid_contribution")
		return
	}
	if _, e := hex.DecodeString(in.Hash); e != nil {
		fail(w, 400, "invalid_audio_hash")
		return
	}
	if _, e := consentForCapture(ctx, tx, p, in.Segment); e != nil {
		fail(w, 409, "speech_consent_required")
		return
	}
	t, e := speechTemplate(ctx, tx, p, in.Template, in.Version)
	if e != nil || !sectionKnown(t, in.Section) {
		fail(w, 404, "section_not_found")
		return
	}
	var approved bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wai_approvals a JOIN wai_sessions s ON s.id=a.session_id AND s.tenant_id=a.tenant_id WHERE a.tenant_id=$1 AND a.session_id::text=$2 AND a.user_id=$3 AND a.revision=$4 AND s.template_version=$5)`, p.Tenant, in.SourceSession, p.User, in.Revision, in.Version).Scan(&approved)
	if e != nil || !approved {
		fail(w, 409, "accepted_revision_required")
		return
	}
	var feedbackID string
	var feedbackRaw []byte
	if e = tx.QueryRow(ctx, `SELECT id,payload FROM wai_speech_feedback WHERE tenant_id=$1 AND session_id=$2 AND accepted_revision=$3 AND section_id=$4`, p.Tenant, in.SourceSession, in.Revision, in.Section).Scan(&feedbackID, &feedbackRaw); e != nil {
		fail(w, 409, "source_feedback_required")
		return
	}
	var feedback speechFeedbackInput
	if m.box.open(feedbackRaw, p.Tenant+":"+feedbackID+":speech-feedback", &feedback) != nil {
		fail(w, 503, "feedback_unavailable")
		return
	}
	in.Final = feedback.Final
	matched := false
	offered, _ := json.Marshal(in.Segment)
	for _, segment := range feedback.Segments {
		original, _ := json.Marshal(segment)
		if segment.ID == in.Segment.ID && bytes.Equal(original, offered) {
			matched = true
		}
	}
	if !matched {
		fail(w, 409, "source_segment_required")
		return
	}
	raw, _ := json.Marshal(in)
	hash := digest(string(raw))
	var existing, oldHash string
	var uploaded int
	e = tx.QueryRow(ctx, `SELECT id,digest,uploaded FROM wai_speech_contributions WHERE tenant_id=$1 AND user_id=$2 AND source_session=$3 AND source_segment=$4`, p.Tenant, p.User, in.SourceSession, in.Segment.ID).Scan(&existing, &oldHash, &uploaded)
	if e == nil {
		if oldHash != hash {
			fail(w, 409, "contribution_conflict")
			return
		}
		respond(w, 200, map[string]any{"id": existing, "uploaded": uploaded})
		return
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		fail(w, 503, "database_error")
		return
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM wai_speech_contributions WHERE tenant_id=$1 AND user_id=$2 AND created_at>now()-interval '1 day'`, p.Tenant, p.User).Scan(&count); e != nil || count >= 100 {
		fail(w, 429, "speech_daily_limit")
		return
	}
	encrypted, e := m.box.seal(in, p.Tenant+":"+in.ID+":contribution")
	if e == nil {
		_, e = tx.Exec(ctx, `INSERT INTO wai_speech_contributions(id,tenant_id,user_id,source_session,source_segment,template_id,template_version,section_id,consent_generation,metadata,digest,audio_hash,byte_length)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, in.ID, p.Tenant, p.User, in.SourceSession, in.Segment.ID, in.Template, in.Version, in.Section, in.Segment.ConsentGeneration, encrypted, hash, in.Hash, in.Bytes)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 503, "contribution_failed")
		return
	}
	respond(w, 201, map[string]any{"id": in.ID, "uploaded": 0})
}

type speechSpan struct {
	Start int    `json:"start_ms"`
	End   int    `json:"end_ms"`
	Text  string `json:"text"`
}
type speechReview struct {
	Decision      string       `json:"decision"`
	Transcript    string       `json:"verified_transcript"`
	Verified      bool         `json:"transcript_verified"`
	Listened      bool         `json:"audio_reviewed_in_full"`
	Deidentified  bool         `json:"deidentification_verified"`
	Alignment     []speechSpan `json:"alignment"`
	Redactions    []speechSpan `json:"redactions"`
	SpeakerKey    string       `json:"speaker_key"`
	SingleSpeaker bool         `json:"single_speaker_verified"`
}

var speechPII = regexp.MustCompile(`(?i)([a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}|\b[0-9]{5,}\b|\b(cnp|telefon|email|nume|prenume|adresa|adresă|născut|nascut)\b)`)
var speechName = regexp.MustCompile(`\b[A-ZĂÂÎȘȚ][a-zăâîșț]+\s+[A-ZĂÂÎȘȚ][a-zăâîșț]+\b`)

func sanitizedSpeech(audio []byte, in speechReview) ([]byte, string, error) {
	duration, e := speechWAV(audio)
	if e != nil {
		return nil, "", e
	}
	if !in.Verified || !in.Listened || !in.Deidentified || len(in.Alignment) == 0 || len(in.Alignment) > 500 || len(in.Transcript) > 16000 || len(in.Redactions) > 100 {
		return nil, "", errors.New("verified_alignment_required")
	}
	last := 0
	full := []string{}
	clean := []string{}
	for _, span := range in.Alignment {
		if span.Start < last || span.End <= span.Start || span.End > duration || strings.TrimSpace(span.Text) == "" || len(span.Text) > 300 {
			return nil, "", errors.New("invalid_alignment")
		}
		last = span.End
		full = append(full, span.Text)
		redacted := false
		for _, r := range in.Redactions {
			if r.Start < 0 || r.End <= r.Start || r.End > duration {
				return nil, "", errors.New("invalid_redaction")
			}
			if span.Start < r.End && r.Start < span.End {
				redacted = true
			}
		}
		if !redacted {
			clean = append(clean, span.Text)
		}
	}
	if normalizeSpeech(strings.Join(full, " ")) != normalizeSpeech(in.Transcript) {
		return nil, "", errors.New("transcript_alignment_mismatch")
	}
	text := normalizeSpeech(strings.Join(clean, " "))
	if text == "" || speechPII.MatchString(text) || speechName.MatchString(text) {
		return nil, "", errors.New("identifying_transcript_requires_redaction")
	}
	out := bytes.Clone(audio)
	clear(out[44:])
	for _, span := range in.Alignment {
		masked := false
		for _, redaction := range in.Redactions {
			if span.Start < redaction.End && redaction.Start < span.End {
				masked = true
			}
		}
		if !masked {
			copy(out[44+span.Start*32:44+span.End*32], audio[44+span.Start*32:44+span.End*32])
		}
	}
	for _, r := range in.Redactions {
		start, end := 44+r.Start*32, 44+r.End*32
		if start < 44 || end > len(out) || end <= start {
			return nil, "", errors.New("invalid_redaction")
		}
		clear(out[start:end])
	}
	return out, text, nil
}
func (m *Module) reviewSpeechContribution(w http.ResponseWriter, r *http.Request, p principal, tx pgx.Tx, item speechContribution, state string, path *string) {
	var in speechReview
	if !decode(w, r, &in) {
		return
	}
	if path == nil || state == "uploading" {
		fail(w, 409, "audio_required")
		return
	}
	if in.Decision != "needs_review" && in.Decision != "rejected" && in.Decision != "approved_for_dataset" {
		fail(w, 400, "invalid_review")
		return
	}
	ctx := r.Context()
	var sanitizedPath any
	var transcript, speaker []byte
	var e error
	if in.Decision == "approved_for_dataset" {
		if !in.SingleSpeaker || !regexp.MustCompile("^[a-z0-9_-]{8,64}$").MatchString(in.SpeakerKey) {
			fail(w, 422, "verified_pseudonymous_speaker_required")
			return
		}
		speaker, e = m.box.seal(in.SpeakerKey, p.Tenant+":"+item.ID+":speaker")
		if e != nil {
			fail(w, 503, "speaker_encryption_failed")
			return
		}

		audio, e := m.readSpeechFile(*path, p.Tenant+":"+item.ID+":audio")
		if e != nil {
			fail(w, 503, "audio_unavailable")
			return
		}
		sanitized, text, e := sanitizedSpeech(audio, in)
		if e != nil {
			fail(w, 422, e.Error())
			return
		}
		// Raw/final report texts are never exported. Only this independently verified,
		// aligned and redacted transcript can become an audio training target.
		file, e := m.writeSpeechFile(sanitized, p.Tenant+":"+item.ID+":audio:sanitized")
		if e == nil {
			transcript, e = m.box.seal(text, p.Tenant+":"+item.ID+":target")
		}
		if e != nil {
			fail(w, 503, "sanitization_failed")
			return
		}
		sanitizedPath = file
	}
	review, e := m.box.seal(in, p.Tenant+":"+item.ID+":review")
	if e == nil {
		_, e = tx.Exec(ctx, `UPDATE wai_speech_contributions SET state=$2,sanitized_path=$3,sanitized_transcript=$4,reviewer_id=$5,review_payload=$6,speaker_key=$7,updated_at=now(),expires_at=CASE WHEN $2='approved_for_dataset' THEN now()+interval '365 days' ELSE expires_at END WHERE id=$1`, item.ID, in.Decision, sanitizedPath, transcript, p.User, review, speaker)
	}
	if e == nil && in.Decision != "approved_for_dataset" {
		_, e = tx.Exec(ctx, `UPDATE wai_speech_dataset_entries SET revoked=true WHERE contribution_id=$1`, item.ID)
	}
	if e == nil {
		e = audit(ctx, tx, p, "speech.contribution.review", item.ID)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 503, "review_failed")
		return
	}
	respond(w, 200, map[string]string{"state": in.Decision})
}
func (m *Module) exportSpeechDataset(w http.ResponseWriter, r *http.Request, p principal, tx pgx.Tx) {
	if p.Role != "admin" && !p.InstanceAdmin {
		fail(w, 403, "admin_required")
		return
	}
	var in struct {
		ID string `json:"dataset_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if _, e := uuid.Parse(in.ID); e != nil {
		fail(w, 400, "invalid_dataset_id")
		return
	}
	ctx := r.Context()
	if _, err := tx.Exec(ctx, `SELECT user_id FROM wai_speech_consent WHERE tenant_id=$1 ORDER BY user_id FOR UPDATE`, p.Tenant); err != nil {
		fail(w, 503, "database_error")
		return
	}
	// Consent rows are locked to prevent an export crossing a withdrawal commit.
	rows, e := tx.Query(ctx, `SELECT c.id,c.speaker_key,c.sanitized_transcript,c.sanitized_path FROM wai_speech_contributions c JOIN wai_speech_consent s ON s.tenant_id=c.tenant_id AND s.user_id=c.user_id AND s.generation=c.consent_generation WHERE c.tenant_id=$1 AND c.state='approved_for_dataset' AND c.speaker_key IS NOT NULL AND s.enabled AND c.expires_at>now() ORDER BY c.id LIMIT 500 FOR UPDATE OF c,s`, p.Tenant)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	type entry struct {
		ID, Path string
		Speaker  []byte
		Raw      []byte
	}
	items := []entry{}
	for rows.Next() {
		var v entry
		if e = rows.Scan(&v.ID, &v.Speaker, &v.Raw, &v.Path); e != nil {
			break
		}
		items = append(items, v)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	root, er := m.speechFileRoot()
	if e == nil {
		e = er
	}
	key, er := secretFile(filepath.Join(root, "speaker.key"), 32)
	if e == nil {
		e = er
	}
	out := []map[string]any{}
	for _, v := range items {
		if e != nil {
			break
		}
		var target string
		e = m.box.open(v.Raw, p.Tenant+":"+v.ID+":target", &target)
		if e != nil {
			break
		}
		var speakerKey string
		if e = m.box.open(v.Speaker, p.Tenant+":"+v.ID+":speaker", &speakerKey); e != nil {
			break
		}
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(p.Tenant + ":" + speakerKey))
		speaker := hex.EncodeToString(mac.Sum(nil))
		audio, er := m.readSpeechFile(v.Path, p.Tenant+":"+v.ID+":audio:sanitized")
		if er != nil {
			e = er
			break
		}
		out = append(out, map[string]any{"contribution_id": v.ID, "speaker_id": speaker, "split_key": speaker, "language": "ro", "transcript": target, "audio_url": "/v1/community/speech-contributions/" + v.ID + "/audio?sanitized=1", "sha256": digest(string(audio)), "target_status": "human_verified_aligned", "deidentification_status": "human_reviewed"})
		_, e = tx.Exec(ctx, `INSERT INTO wai_speech_dataset_entries(dataset_id,contribution_id,exported_by)VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, in.ID, v.ID, p.User)
	}
	if e == nil {
		e = audit(ctx, tx, p, "speech.dataset.export", in.ID)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 503, "export_failed")
		return
	}
	respond(w, 200, map[string]any{"schema": "wisemed.speech-dataset.v1", "dataset_id": in.ID, "split_policy": "group_by_speaker_id", "items": out})
}
func (m *Module) retainSpeech(ctx context.Context) {
	_, _ = m.db.Exec(ctx, `DELETE FROM wai_speech_feedback WHERE created_at<now()-make_interval(days=>least($1,30))`, m.cfg.RetentionDays)
	// Inaccessible ciphertext from interrupted transactions/deletions is reaped
	// after a one-hour grace period. A newly written file cannot be swept while its
	// transaction is committing.
	_, _ = m.db.Exec(ctx, `UPDATE wai_speech_dataset_entries SET revoked=true WHERE contribution_id IN(SELECT id FROM wai_speech_contributions WHERE expires_at<now())`)
	_, _ = m.db.Exec(ctx, `DELETE FROM wai_speech_contributions WHERE expires_at<now()`)
	sensitive, err := m.db.Query(ctx, `SELECT id,tenant_id,metadata FROM wai_speech_contributions WHERE created_at<now()-interval '7 days' AND raw_path IS NOT NULL AND state='approved_for_dataset'`)
	if err == nil {
		type expired struct {
			id, tenant string
			raw        []byte
		}
		items := []expired{}
		for sensitive.Next() {
			var v expired
			if sensitive.Scan(&v.id, &v.tenant, &v.raw) != nil {
				items = nil
				break
			}
			items = append(items, v)
		}
		if sensitive.Err() != nil {
			items = nil
		}
		sensitive.Close()
		for _, v := range items {
			var item speechContribution
			if m.box.open(v.raw, v.tenant+":"+v.id+":contribution", &item) != nil {
				continue
			}
			item.Final = ""
			item.Segment.Raw = ""
			item.Segment.Corrected = nil
			item.Segment.Transcript = ""
			encrypted, err := m.box.seal(item, v.tenant+":"+v.id+":contribution")
			if err == nil {
				_, _ = m.db.Exec(ctx, `UPDATE wai_speech_contributions SET raw_path=NULL,metadata=$2,review_payload=NULL WHERE id=$1`, v.id, encrypted)
			}
		}
	}
	// Approved records keep only sanitized targets after sensitive retention.
	root, e := m.speechFileRoot()
	if e != nil {
		return
	}
	rows, e := m.db.Query(ctx, `SELECT raw_path FROM wai_speech_contributions WHERE raw_path IS NOT NULL UNION SELECT sanitized_path FROM wai_speech_contributions WHERE sanitized_path IS NOT NULL UNION SELECT path FROM wai_speech_upload_parts`)
	if e != nil {
		return
	}
	keep := map[string]bool{}
	for rows.Next() {
		var s string
		if rows.Scan(&s) != nil {
			rows.Close()
			return
		}
		keep[s] = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return
	}
	files, e := os.ReadDir(root)
	if e != nil {
		return
	}
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".enc") || keep[f.Name()] {
			continue
		}
		info, e := f.Info()
		if e == nil && info.ModTime().Before(time.Now().Add(-time.Hour)) {
			_ = os.Remove(filepath.Join(root, f.Name()))
		}
	}
}
