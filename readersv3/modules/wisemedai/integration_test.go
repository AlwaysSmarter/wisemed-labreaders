package wisemedai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

type testRuntime struct {
	dir      string
	settings map[string]any
	mux      *http.ServeMux
	services map[string]any
}

func (t *testRuntime) ConfigPath() string                           { return filepath.Join(t.dir, "config.yaml") }
func (t *testRuntime) ConfigDir() string                            { return t.dir }
func (t *testRuntime) ReaderID() string                             { return "wisemed-ai-test" }
func (t *testRuntime) Logf(string, ...interface{})                  {}
func (t *testRuntime) ModuleSettings(string) map[string]interface{} { return t.settings }
func (t *testRuntime) ResolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(t.dir, p)
}
func (t *testRuntime) AddMenu(...module.MenuEntry)             {}
func (t *testRuntime) Handle(p string, h http.Handler)         { t.mux.Handle(p, h) }
func (t *testRuntime) Mux() *http.ServeMux                     { return t.mux }
func (t *testRuntime) RegisterService(n string, s interface{}) { t.services[n] = s }
func (t *testRuntime) Service(n string) (interface{}, bool)    { v, ok := t.services[n]; return v, ok }
func integrationModule(t *testing.T) (*Module, *testRuntime) {
	t.Helper()
	dsn := os.Getenv("WAI_TEST_DATABASE_URL")
	if dsn == "" {
		if path := os.Getenv("WAI_TEST_CONFIG"); path != "" {
			raw, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			var cfg map[string]any
			if yaml.Unmarshal(raw, &cfg) != nil {
				t.Fatal("invalid test config")
			}
			dsn, _ = cfg["modules"].(map[string]any)["wisemed-ai"].(map[string]any)["database_url"].(string)
		}
	}
	if dsn == "" {
		t.Skip("set WAI_TEST_DATABASE_URL or WAI_TEST_CONFIG for isolated PostgreSQL integration")
	}
	ctx := context.Background()
	admin, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal("test PostgreSQL unavailable")
	}
	schema := "wai_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, e = admin.Exec(ctx, `CREATE SCHEMA `+schema); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`); _ = admin.Close(ctx) })
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal("invalid test DSN")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	rt := &testRuntime{dir: t.TempDir(), mux: http.NewServeMux(), services: map[string]any{}, settings: map[string]any{}}
	box, e := loadBox(rt.ResolvePath("key"))
	if e != nil {
		t.Fatal(e)
	}
	s, _ := readSettings(nil)
	m := &Module{rt: rt, db: pool, box: box, cfg: s, attempts: map[string]loginAttempt{}, client: &http.Client{Timeout: time.Second}}
	if e = m.migrate(ctx); e != nil {
		t.Fatal(e)
	}
	if e = m.migrate(ctx); e != nil {
		t.Fatal("repeat migration", e)
	}
	if e = m.bootstrap(ctx); e != nil {
		t.Fatal(e)
	}
	if e = m.ensureCatalog(ctx); e != nil {
		t.Fatal(e)
	}
	if e = m.ensureCatalog(ctx); e != nil {
		t.Fatal("repeat catalog seed", e)
	}
	return m, rt
}
func request(t *testing.T, m *Module, token, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var raw string
	if body != nil {
		raw = toJSON(body)
	}
	r := httptest.NewRequest(method, "http://localhost/v1/"+path, strings.NewReader(raw))
	r.RemoteAddr = "127.0.0.1:12345"
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	m.serve(w, r)
	var out map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatalf("%s %s returned %d %s", method, path, w.Code, w.Body.String())
	}
	return w.Code, out
}
func TestPostgresClinicalFlowAndIsolation(t *testing.T) {
	m, rt := integrationModule(t)
	ctx := context.Background()
	b, _ := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	code, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(b)})
	if code != 200 {
		t.Fatal(code, out)
	}
	token := out["token"].(string)
	profile := out["profile"].(map[string]any)
	tenant := profile["tenant_id"].(string)
	var version string
	if e := m.db.QueryRow(ctx, `SELECT current_version FROM wai_templates WHERE tenant_id=$1 AND name='Ecografie abdominală'`, tenant).Scan(&version); e != nil {
		t.Fatal(e)
	}
	code, out = request(t, m, token, "POST", "sessions", map[string]any{"template_version": version, "patient_ref": "SYNTHETIC-ONLY"})
	if code != 201 {
		t.Fatal(code, out)
	}
	id := out["id"].(string)
	timestamp := time.Now().UTC()
	chunkID := uuid.NewString()
	chunk := map[string]any{"id": chunkID, "sequence": 2, "revision": 1, "is_final": true, "timestamp": timestamp, "text": "splina: descrisă normală"}
	for i := 0; i < 2; i++ {
		code, out = request(t, m, token, "POST", "sessions/"+id+"/transcript-chunks", chunk)
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	if out["duplicate"] != true {
		t.Fatal("duplicate was not idempotent", out)
	}
	code, out = request(t, m, token, "POST", "sessions/"+id+"/transcript-chunks", map[string]any{"id": uuid.NewString(), "sequence": 1, "revision": 1, "is_final": true, "timestamp": timestamp, "text": "ficat: omogen\nficat_mm: 140"})
	if code != 200 {
		t.Fatal(code, out)
	}
	chunk["revision"] = 2
	chunk["text"] = "splina: neexaminat"
	code, out = request(t, m, token, "POST", "sessions/"+id+"/transcript-chunks", chunk)
	if code != 200 {
		t.Fatal(code, out)
	}
	chunk["revision"] = 1
	code, out = request(t, m, token, "POST", "sessions/"+id+"/transcript-chunks", chunk)
	if code != 200 || out["accepted"] != false {
		t.Fatal("stale revision", code, out)
	}
	code, out = request(t, m, token, "POST", "sessions/"+id+"/approve", map[string]any{"revision": 3, "confirmed": true})
	if code != 409 {
		t.Fatal("approval before finalize accepted", code, out)
	}
	code, out = request(t, m, token, "POST", "sessions/"+id+"/finalize", nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	revision := int(out["revision"].(float64))
	code, out = request(t, m, token, "POST", "sessions/"+id+"/reopen", map[string]any{"revision": revision - 1})
	if code != 409 {
		t.Fatal("stale reopen accepted", code, out)
	}
	code, out = request(t, m, token, "POST", "sessions/"+id+"/reopen", map[string]any{"revision": revision})
	if code != 200 || out["status"] != "recording" || out["revision"] != float64(revision+1) {
		t.Fatal("reopen failed", code, out)
	}
	chunk["revision"] = 3
	code, out = request(t, m, token, "POST", "sessions/"+id+"/audio-transcript-chunks", chunk)
	if code != 200 || out["accepted"] != true {
		t.Fatal("transcript update after reopen failed", code, out)
	}
	code, out = request(t, m, token, "POST", "sessions/"+id+"/finalize", nil)
	if code != 200 {
		t.Fatal("finalize after reopen failed", code, out)
	}
	revision = int(out["revision"].(float64))
	code, out = request(t, m, token, "POST", "sessions/"+id+"/approve", map[string]any{"revision": revision - 1, "confirmed": true})
	if code != 409 {
		t.Fatal("stale approval accepted", code, out)
	}
	code, out = request(t, m, token, "POST", "sessions/"+id+"/approve", map[string]any{"revision": revision, "confirmed": true})
	if code != 200 || out["status"] != "approved" {
		t.Fatal(code, out)
	}
	var count int
	_ = m.db.QueryRow(ctx, `SELECT count(*) FROM wai_clinical_results WHERE session_id=$1`, id).Scan(&count)
	if count != 1 {
		t.Fatal(count)
	}
	code, out = request(t, m, token, "POST", "sessions/"+id+"/approve", map[string]any{"revision": revision, "confirmed": true})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = request(t, m, token, "POST", "sessions/"+id+"/reopen", map[string]any{"revision": revision})
	if code != 200 || out["status"] != "recording" || out["revision"] != float64(revision+1) {
		t.Fatal("approved result cannot create an editable revision", code, out)
	}
	var archivedRevision int
	if e := m.db.QueryRow(ctx, `SELECT revision FROM wai_clinical_results WHERE session_id=$1`, id).Scan(&archivedRevision); e != nil || archivedRevision != revision {
		t.Fatal("archived approval was changed", archivedRevision, e)
	}
	code, out = request(t, m, token, "POST", "sessions/"+id+"/pairing", nil)
	if code != 201 {
		t.Fatal(code, out)
	}
	pairToken := out["token"].(string)
	for i, want := range []int{200, 410} {
		code, out = request(t, m, token, "POST", "pairing/redeem", map[string]any{"token": pairToken})
		if code != want {
			t.Fatal(i, code, out)
		}
	}
	var raw []byte
	_ = m.db.QueryRow(ctx, `SELECT context FROM wai_sessions WHERE id=$1`, id).Scan(&raw)
	if strings.Contains(string(raw), "SYNTHETIC") {
		t.Fatal("context not encrypted")
	}
	// A valid same-tenant colleague cannot access another doctor's session.
	code, out = request(t, m, token, "POST", "users", map[string]any{"name": "Synthetic colleague", "email": "other@example.invalid", "password": "synthetic-password-123", "role": "doctor"})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = request(t, m, "", "POST", "auth/login", map[string]any{"email": "other@example.invalid", "password": "synthetic-password-123"})
	if code != 200 {
		t.Fatal(code, out)
	}
	other := out["token"].(string)
	code, _ = request(t, m, other, "GET", "sessions/"+id, nil)
	if code != 404 {
		t.Fatal("cross-user access", code)
	}
	code, _ = request(t, m, other, "GET", "settings", nil)
	if code != 403 {
		t.Fatal("doctor settings access", code)
	}
	// WSM identity is only trusted from the attested bridge and explicit tenant mapping.
	r := httptest.NewRequest("GET", "http://localhost/v1/sessions/"+id, nil)
	r.Header.Set("X-Tenant-ID", tenant)
	r.Header.Set("X-User-ID", profile["user_id"].(string))
	w := httptest.NewRecorder()
	m.serve(w, r)
	if w.Code != 401 {
		t.Fatal("forged identity accepted", w.Code)
	}
	rt.mux.Handle("/api/wisemed-ai/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := r.Clone(r.Context())
		u := *r.URL
		c.URL = &u
		c.URL.Path = strings.TrimPrefix(r.URL.Path, "/api/wisemed-ai")
		m.serve(w, c)
	}))
	m.cfg.WSMIdentity = true
	_, e := m.db.Exec(ctx, `UPDATE wai_memberships SET external_subject='test-subject' WHERE tenant_id=$1 AND user_id=$2`, tenant, profile["user_id"])
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		tenant string
		want   int
	}{{tenant, 200}, {uuid.NewString(), 401}} {
		resp, e := apibridge.Invoke(ctx, rt.mux, apibridge.Principal{TenantID: tc.tenant, Subject: "test-subject"}, apibridge.Request{Method: "GET", Path: "/api/wisemed-ai/v1/sessions/" + id})
		if e != nil || resp.Status != tc.want {
			t.Fatal("WSM isolation", resp, e)
		}
	}
	code, out = request(t, m, token, "DELETE", "sessions/"+id, nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	_ = m.db.QueryRow(ctx, `SELECT count(*) FROM wai_clinical_results WHERE session_id=$1`, id).Scan(&count)
	if count != 0 {
		t.Fatal("cascade retention", count)
	}
	var unexpected int
	e = m.db.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name NOT LIKE 'wai_%'`).Scan(&unexpected)
	if e != nil || unexpected != 0 {
		t.Fatal("table prefix", unexpected, e)
	}
	_ = fmt.Sprint(tenant)
}
