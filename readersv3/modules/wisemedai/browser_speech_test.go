package wisemedai

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBrowserSpeechPageSecurityAndRestart(t *testing.T) {
	m := &Module{}
	w := httptest.NewRecorder()
	m.browserSpeechPage(w, httptest.NewRequest("GET", "https://example.test/voice", nil))
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") || !strings.Contains(w.Body.String(), "recognizer.continuous=true") || !strings.Contains(w.Body.String(), "recognizer.interimResults=true") || !strings.Contains(w.Body.String(), "recognizer.lang='ro-RO'") {
		t.Fatal("invalid browser page", w.Code)
	}
	if strings.Contains(w.Body.String(), "__NONCE__") || w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("unsafe page headers")
	}
}
func TestPostgresBrowserSpeechGrantsScopeAndIdempotency(t *testing.T) {
	m, rt := integrationModule(t)
	ctx := context.Background()
	secret, e := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	if e != nil {
		t.Fatal(e)
	}
	code, login := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(secret)})
	if code != 200 {
		t.Fatal(code, login)
	}
	loginToken := login["token"].(string)
	p := login["profile"].(map[string]any)
	tenant := p["tenant_id"].(string)
	var version string
	if e = m.db.QueryRow(ctx, `SELECT id FROM wai_template_versions WHERE tenant_id=$1 ORDER BY version LIMIT 1`, tenant).Scan(&version); e != nil {
		t.Fatal(e)
	}
	code, grant := request(t, m, loginToken, "POST", "browser-speech", map[string]any{"template_version": version})
	if code != 200 {
		t.Fatal(code, grant)
	}
	token := grant["token"].(string)
	bridge := func(method string, body any, credential string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "http://localhost/v1/browser-speech/bridge", bytes.NewReader(b))
		r.Header.Set("Authorization", "Browser "+credential)
		w := httptest.NewRecorder()
		m.serve(w, r)
		return w
	}
	if w := bridge("GET", nil, strings.Repeat("a", 64)); w.Code != 401 {
		t.Fatal("grant bypass", w.Code)
	}
	if w := bridge("GET", nil, token); !strings.Contains(w.Body.String(), `"capture_state":"waiting"`) {
		t.Fatal("microphone must initially be waiting", w.Body.String())
	}
	if w := bridge("POST", map[string]any{"action": "state", "state": "listening"}, token); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := bridge("GET", nil, token); !strings.Contains(w.Body.String(), `"capture_state":"listening"`) {
		t.Fatal("missing browser audio state", w.Body.String())
	}
	if _, e := m.db.Exec(ctx, `UPDATE wai_browser_speech SET capture_state_updated_at=now()-interval '1 minute' WHERE token_hash=$1`, digest(token)); e != nil {
		t.Fatal(e)
	}
	if w := bridge("GET", nil, token); !strings.Contains(w.Body.String(), `"capture_state":"waiting"`) {
		t.Fatal("stale browser audio must not appear active", w.Body.String())
	}
	if w := bridge("POST", map[string]any{"action": "state", "state": "invalid"}, token); w.Code != 422 {
		t.Fatal("invalid capture state accepted", w.Code)
	}
	if w := bridge("POST", map[string]any{"action": "partial", "text": "vorbire"}, token); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	event := map[string]any{"action": "segment", "event_id": uuid.NewString(), "ordinal": 1, "text": "text sintetic", "timestamp": time.Now().UTC().Format(time.RFC3339Nano)}
	for i := 0; i < 2; i++ {
		if w := bridge("POST", event, token); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	event["text"] = "different"
	if w := bridge("POST", event, token); w.Code != 409 {
		t.Fatal("idempotency conflict accepted", w.Code)
	}
	w := bridge("GET", nil, token)
	var data map[string]any
	json.Unmarshal(w.Body.Bytes(), &data)
	if w.Code != 200 || len(data["events"].([]any)) != 1 || data["locale"] != "ro-RO" || data["submitted_to_session"] != false {
		t.Fatal(w.Code, data)
	}
	var raw []byte
	if e = m.db.QueryRow(ctx, `SELECT payload FROM wai_browser_speech_events LIMIT 1`).Scan(&raw); e != nil || bytes.Contains(raw, []byte("sintetic")) {
		t.Fatal("plaintext event", e)
	}
	if w := bridge("POST", map[string]any{"action": "finished"}, token); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	// Clinical Chrome batches use the existing incremental extraction exactly once.
	code, clinical := request(t, m, loginToken, "POST", "sessions", map[string]any{"template_version": version, "patient_ref": "SYNTHETIC"})
	if code != 201 && code != 200 {
		t.Fatal(code, clinical)
	}
	session := clinical["id"].(string)
	code, cgrant := request(t, m, loginToken, "POST", "browser-speech", map[string]any{"session_id": session, "template_version": version})
	if code != 200 {
		t.Fatal(code, cgrant)
	}
	clinicalToken := cgrant["token"].(string)
	final := map[string]any{"action": "segment", "event_id": uuid.NewString(), "ordinal": 1, "text": "observație sintetică omogenă", "timestamp": time.Now().UTC().Format(time.RFC3339Nano)}
	for i := 0; i < 2; i++ {
		if w := bridge("POST", final, clinicalToken); w.Code != 200 {
			t.Fatal("clinical browser batch", w.Code, w.Body.String())
		}
	}
	code, snapshot := request(t, m, loginToken, "GET", "sessions/"+session, nil)
	if code != 200 || len(snapshot["chunks"].([]any)) != 1 {
		t.Fatal("duplicate clinical extraction", code, snapshot)
	}
	final["session_id"] = uuid.NewString()
	if w := bridge("POST", final, clinicalToken); w.Code != 400 {
		t.Fatal("grant accepted a different session", w.Code)
	}
	// A grant is tied to the app login: logout must revoke Chrome without exposing the app bearer.
	code, _ = request(t, m, loginToken, "POST", "auth/logout", map[string]any{})
	if code != 200 {
		t.Fatal(code)
	}
	if w := bridge("GET", nil, token); w.Code != 401 {
		t.Fatal("revoked grant accepted", w.Code)
	}
}
