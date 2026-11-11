package wisemedai

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

//go:embed browser_speech.html
var browserSpeechHTML string

type browserSpeechGrant struct {
	Hash, Session, Version, Section, Field string
	Base                                   int
	p                                      principal
}

func (m *Module) browserSpeechPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var b [16]byte
	rand.Read(b[:])
	nonce := hex.EncodeToString(b[:])
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+nonce+"'; style-src 'nonce-"+nonce+"'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	io.WriteString(w, strings.ReplaceAll(browserSpeechHTML, "__NONCE__", nonce))
}
func (m *Module) createBrowserSpeech(w http.ResponseWriter, r *http.Request, p principal) {
	if r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	if p.Role == "viewer" {
		fail(w, 403, "doctor_required")
		return
	}
	var in struct {
		Session string `json:"session_id"`
		Version string `json:"template_version"`
		Section string `json:"section_id"`
		Field   string `json:"target_field"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	tx, e := m.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	base := 1
	if in.Session != "" {
		s, e := m.loadSession(ctx, tx, p, in.Session)
		if e != nil {
			fail(w, 404, "session_not_found")
			return
		}
		if s.Status != "recording" {
			fail(w, 409, "session_locked")
			return
		}
		in.Version = s.Version
		for _, c := range s.Chunks {
			base = max(base, c.Sequence+1)
		}
	}
	var id string
	if e = tx.QueryRow(ctx, `SELECT template_id FROM wai_template_versions WHERE id::text=$1 AND tenant_id=$2`, in.Version, p.Tenant).Scan(&id); e != nil {
		fail(w, 404, "template_not_found")
		return
	}
	tpl, e := speechTemplate(ctx, tx, p, id, in.Version)
	if e != nil {
		fail(w, 404, "template_not_found")
		return
	}
	if in.Section != "" && !sectionKnown(tpl, in.Section) {
		fail(w, 422, "unknown_section")
		return
	}
	if in.Field != "" {
		if _, ok := fieldTemplate(tpl, in.Field); !ok {
			fail(w, 422, "unknown_field")
			return
		}
	}
	login := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if login == "" {
		if c, e := r.Cookie("wai_session"); e == nil {
			login = c.Value
		}
	}
	var random [32]byte
	if _, e = rand.Read(random[:]); e != nil {
		fail(w, 503, "grant_failed")
		return
	}
	token := hex.EncodeToString(random[:])
	var session any
	if in.Session != "" {
		session = in.Session
	}
	_, e = tx.Exec(ctx, `INSERT INTO wai_browser_speech(token_hash,login_hash,tenant_id,user_id,session_id,template_version,section_id,target_field,base_sequence)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, digest(token), digest(login), p.Tenant, p.User, session, in.Version, in.Section, in.Field, base)
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 503, "browser_grant_failed")
		return
	}
	respond(w, 200, map[string]any{"token": token, "path": "/ai/voice#" + token, "expires_in": 7200, "locale": "ro-RO", "template_name": tpl.Name, "template_version": tpl.VersionID, "submitted_to_session": in.Session != ""})
}
func (m *Module) browserSpeechBridge(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Browser ")
	if len(token) != 64 {
		fail(w, 401, "browser_grant_required")
		return
	}
	g := browserSpeechGrant{Hash: digest(token)}
	e := m.db.QueryRow(r.Context(), `SELECT coalesce(b.session_id::text,''),b.template_version,b.section_id,b.target_field,b.base_sequence,u.id,b.tenant_id,m.role,u.email,u.name,u.instance_admin FROM wai_browser_speech b JOIN wai_auth_sessions a ON a.token_hash=b.login_hash JOIN wai_users u ON u.id=b.user_id JOIN wai_memberships m ON m.user_id=b.user_id AND m.tenant_id=b.tenant_id WHERE b.token_hash=$1 AND b.expires_at>now() AND a.expires_at>now() AND NOT u.disabled AND NOT m.disabled`, g.Hash).Scan(&g.Session, &g.Version, &g.Section, &g.Field, &g.Base, &g.p.User, &g.p.Tenant, &g.p.Role, &g.p.Email, &g.p.Name, &g.p.InstanceAdmin)
	if e != nil || g.p.Role == "viewer" {
		fail(w, 401, "browser_grant_expired")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == "GET" {
		var partial []byte
		var stop, finished bool
		var captureState string
		if e = m.db.QueryRow(r.Context(), `SELECT partial,stop_requested,finished,CASE WHEN capture_state='listening' AND capture_state_updated_at<now()-interval '8 seconds' THEN 'waiting' ELSE capture_state END FROM wai_browser_speech WHERE token_hash=$1`, g.Hash).Scan(&partial, &stop, &finished, &captureState); e != nil {
			fail(w, 503, "browser_status_failed")
			return
		}
		var text string
		if len(partial) > 0 {
			m.box.open(partial, g.Hash+":partial", &text)
		}
		after, _ := strconv.Atoi(r.URL.Query().Get("after"))
		if after < 0 || after > 500 {
			fail(w, 400, "invalid_browser_cursor")
			return
		}
		rows, e := m.db.Query(r.Context(), `SELECT ordinal,payload FROM wai_browser_speech_events WHERE token_hash=$1 AND ordinal>$2 ORDER BY ordinal LIMIT 500`, g.Hash, after)
		if e != nil {
			fail(w, 503, "browser_events_failed")
			return
		}
		defer rows.Close()
		events := []map[string]any{}
		for rows.Next() {
			var n int
			var raw []byte
			if e = rows.Scan(&n, &raw); e != nil {
				break
			}
			var chunk Chunk
			if m.box.open(raw, g.Hash+":"+stringID(n), &chunk) != nil {
				fail(w, 503, "browser_event_failed")
				return
			}
			events = append(events, map[string]any{"ordinal": n, "chunk": chunk})
		}
		var name string
		m.db.QueryRow(r.Context(), `SELECT document->>'name' FROM wai_template_versions WHERE id=$1 AND tenant_id=$2`, g.Version, g.p.Tenant).Scan(&name)
		respond(w, 200, map[string]any{"partial": text, "events": events, "stop_requested": stop, "finished": finished, "capture_state": captureState, "submitted_to_session": g.Session != "", "template_name": name, "template_version": g.Version, "locale": "ro-RO"})
		return
	}
	if r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		Action    string    `json:"action"`
		State     string    `json:"state"`
		ID        string    `json:"event_id"`
		Ordinal   int       `json:"ordinal"`
		Text      string    `json:"text"`
		Timestamp time.Time `json:"timestamp"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Text) > 16000 {
		fail(w, 413, "text_too_large")
		return
	}
	switch in.Action {
	case "state":
		if in.State != "waiting" && in.State != "listening" && in.State != "restarting" && in.State != "stopping" {
			fail(w, 422, "invalid_browser_capture_state")
			return
		}
		_, e = m.db.Exec(r.Context(), `UPDATE wai_browser_speech SET capture_state=$2,capture_state_updated_at=now() WHERE token_hash=$1 AND NOT finished`, g.Hash, in.State)
		if e != nil {
			fail(w, 503, "browser_status_failed")
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
	case "partial":
		b, e := m.box.seal(in.Text, g.Hash+":partial")
		if e == nil {
			_, e = m.db.Exec(r.Context(), `UPDATE wai_browser_speech SET partial=$2 WHERE token_hash=$1 AND NOT finished`, g.Hash, b)
		}
		if e != nil {
			fail(w, 503, "browser_partial_failed")
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
	case "stop", "finished":
		_, e = m.db.Exec(r.Context(), `UPDATE wai_browser_speech SET stop_requested=true,finished=finished OR $2,capture_state=CASE WHEN $2 THEN 'finished' ELSE 'stopping' END WHERE token_hash=$1`, g.Hash, in.Action == "finished")
		if e != nil {
			fail(w, 503, "browser_stop_failed")
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
	case "segment":
		if _, e = uuid.Parse(in.ID); e != nil || in.Ordinal < 1 || in.Ordinal > 500 || strings.TrimSpace(in.Text) == "" || in.Timestamp.IsZero() {
			fail(w, 422, "invalid_browser_segment")
			return
		}
		c := Chunk{ID: in.ID, Sequence: g.Base + in.Ordinal - 1, Revision: 1, Final: true, Timestamp: in.Timestamp.UTC().Truncate(time.Microsecond), Text: in.Text, TargetField: g.Field}
		raw, _ := json.Marshal(c)
		hash := digest(string(raw))
		var old string
		if m.db.QueryRow(r.Context(), `SELECT digest FROM wai_browser_speech_events WHERE token_hash=$1 AND ordinal=$2`, g.Hash, in.Ordinal).Scan(&old) == nil {
			if old != hash {
				fail(w, 409, "browser_event_conflict")
				return
			}
			respond(w, 200, map[string]bool{"accepted": true})
			return
		}
		if g.Session != "" {
			if !g.p.InstanceAdmin {
				state, e := m.subscriptionState(r.Context(), g.p.User)
				if e != nil {
					fail(w, 503, "billing_unavailable")
					return
				}
				if state != nil && state.Suspended {
					fail(w, 402, state.Reason)
					return
				}
			}
			r2 := r.Clone(r.Context())
			r2.Method = "POST"
			r2.Body = io.NopCloser(bytes.NewReader(raw))
			r2.ContentLength = int64(len(raw))
			capture := &browserResponse{header: make(http.Header)}
			m.sessions(capture, r2, g.p, g.Session+"/transcript-chunks")
			if capture.status >= 400 {
				for k, v := range capture.header {
					w.Header()[k] = v
				}
				w.WriteHeader(capture.status)
				w.Write(capture.body.Bytes())
				return
			}
		}
		sealed, e := m.box.seal(c, g.Hash+":"+stringID(in.Ordinal))
		if e == nil {
			_, e = m.db.Exec(r.Context(), `INSERT INTO wai_browser_speech_events(token_hash,ordinal,event_id,digest,payload)VALUES($1,$2,$3,$4,$5)ON CONFLICT DO NOTHING`, g.Hash, in.Ordinal, in.ID, hash, sealed)
		}
		if e != nil {
			fail(w, 503, "browser_event_failed")
			return
		}
		var stored string
		if e = m.db.QueryRow(r.Context(), `SELECT digest FROM wai_browser_speech_events WHERE token_hash=$1 AND ordinal=$2`, g.Hash, in.Ordinal).Scan(&stored); e != nil || stored != hash {
			fail(w, 409, "browser_event_conflict")
			return
		}
		_, _ = m.db.Exec(r.Context(), `UPDATE wai_browser_speech SET partial=NULL WHERE token_hash=$1`, g.Hash)
		respond(w, 200, map[string]bool{"accepted": true})
	default:
		fail(w, 400, "invalid_browser_action")
	}
}

type browserResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *browserResponse) Header() http.Header { return b.header }
func (b *browserResponse) WriteHeader(s int) {
	if b.status == 0 {
		b.status = s
	}
}
func (b *browserResponse) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = 200
	}
	return b.body.Write(p)
}
func stringID(n int) string { return strconv.Itoa(n) }
