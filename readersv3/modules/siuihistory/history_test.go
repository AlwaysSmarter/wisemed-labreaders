package siuihistory

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/shared/apibridge"
	"wisemed-labreaders/readersv3/shared/history"
)

type testRuntime struct {
	dir string
	mux *http.ServeMux
}

func (r *testRuntime) ConfigPath() string                           { return filepath.Join(r.dir, "config.yaml") }
func (r *testRuntime) ConfigDir() string                            { return r.dir }
func (r *testRuntime) ReaderID() string                             { return "history" }
func (r *testRuntime) Logf(string, ...interface{})                  {}
func (r *testRuntime) ModuleSettings(string) map[string]interface{} { return nil }
func (r *testRuntime) ResolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(r.dir, p)
}
func (r *testRuntime) AddMenu(...module.MenuEntry)         {}
func (r *testRuntime) Handle(p string, h http.Handler)     { r.mux.Handle(p, h) }
func (r *testRuntime) Mux() *http.ServeMux                 { return r.mux }
func (r *testRuntime) RegisterService(string, interface{}) {}
func (r *testRuntime) Service(string) (interface{}, bool)  { return testGuard{}, true }

type testGuard struct{}

func (testGuard) RequireSession(h http.Handler) http.Handler { return h }
func (testGuard) SessionIdentity(r *http.Request) (string, bool, bool) {
	if p, ok := apibridge.PrincipalFrom(r.Context()); ok {
		return p.Subject, p.Admin, true
	}
	return "admin", true, true
}
func setup(t *testing.T) *Module {
	t.Helper()
	m := New().(*Module)
	rt := &testRuntime{t.TempDir(), http.NewServeMux()}
	if e := m.Init(rt); e != nil {
		t.Fatal(e)
	}
	m.cfg.Tenant = "tenant"
	return m
}
func request(m *Module, method, path, token string, b []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	m.authenticate(w, r)
	return w
}
func TestLocalStorageAndProbe(t *testing.T) {
	m := setup(t)
	w := request(m, "POST", "/api/history/test-storage", "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"deleted":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	files, _ := filepath.Glob(filepath.Join(m.rt.ConfigDir(), "history/_tests/*"))
	if len(files) != 0 {
		t.Fatal("test file not removed")
	}
	s := localStore(t.TempDir())
	if e := s.Put(context.Background(), "../escape", strings.NewReader("x"), 1); e == nil {
		t.Fatal("traversal accepted")
	}
	if e := s.Put(context.Background(), "day/file", strings.NewReader("x"), 2); e == nil {
		t.Fatal("short write accepted")
	}
}
func TestS3ProbeDeleteDeniedStillReportsWrite(t *testing.T) {
	var put, deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
			t.Error("unsigned S3 request")
		}
		switch r.Method {
		case "PUT":
			put = true
			io.Copy(io.Discard, r.Body)
			w.Header().Set("ETag", `"test"`)
		case "DELETE":
			deleted = true
			w.WriteHeader(403)
			fmt.Fprint(w, `<Error><Code>AccessDenied</Code><Message>delete denied</Message></Error>`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()
	m := setup(t)
	m.cfg.Storage = "s3"
	m.cfg.Endpoint = strings.TrimPrefix(srv.URL, "http://")
	m.cfg.Region = "us-east-1"
	m.cfg.Bucket = "history-test"
	m.cfg.S3Insecure = true
	m.cfg.AccessKey = "test-access"
	m.cfg.SecretKey = "test-secret"
	w := request(m, "POST", "/api/history/test-storage", "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"write":true`) || !strings.Contains(w.Body.String(), `"deleted":false`) || !put || !deleted {
		t.Fatal(w.Code, w.Body.String(), put, deleted)
	}
}
func TestMySQLArchiveResumeIsolationAndWSM(t *testing.T) {
	port := os.Getenv("HISTORY_TEST_MYSQL_PORT")
	if port == "" {
		t.Skip("set HISTORY_TEST_MYSQL_PORT for disposable MySQL integration")
	}
	m := setup(t)
	m.cfg.MySQLPort = port
	m.cfg.MySQLUser = "root"
	m.cfg.MySQLDatabase = "history_test"
	defer func() {
		if m.db != nil {
			m.db.Close()
		}
	}()
	token := "test-key"
	h := sha256.Sum256([]byte(token))
	m.cfg.Keys = []bridgeKey{{"tenant", "bridge", hex.EncodeToString(h[:])}}
	id := fmt.Sprintf("test-%d", time.Now().UnixNano())
	data := bytes.Repeat([]byte("abc"), 100000)
	h = sha256.Sum256(data)
	rec := history.Record{CallID: id, OperationID: id, Operation: "getRefund", CreatedAt: "2026-10-18T10:00:00Z", CompletedAt: "2026-10-18T10:00:03Z", Status: "completed", Artifacts: []history.Artifact{{Name: "download", Size: int64(len(data)), SHA256: hex.EncodeToString(h[:])}}}
	manifest, _ := json.Marshal(rec)
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("expected %d got %d: %s", status, w.Code, w.Body.String())
		}
	}
	check(request(m, "POST", "/api/history/calls", token, manifest), 200)
	path := "/api/history/calls/" + id
	check(request(m, "POST", path+"/commit", token, nil), 409)
	check(request(m, "PUT", path+"/files/download?offset=0", token, data[:history.ChunkSize]), 200)
	check(request(m, "PUT", path+"/files/download?offset=0", token, data[:1]), 409)
	// Close and reopen MySQL connection to model process restart; staging survives.
	m.db.Close()
	m.db = nil
	w := request(m, "POST", "/api/history/calls", token, manifest)
	check(w, 200)
	var progress history.Progress
	json.Unmarshal(w.Body.Bytes(), &progress)
	if progress.Offsets["download"] != history.ChunkSize {
		t.Fatal(progress)
	}
	check(request(m, "PUT", path+fmt.Sprintf("/files/download?offset=%d", history.ChunkSize), token, data[history.ChunkSize:]), 200)
	check(request(m, "POST", path+"/commit", token, nil), 200)
	check(request(m, "POST", path+"/commit", token, nil), 200)
	m.cfg.BasePath = "./another-location"
	w = request(m, "GET", path+"/files/download?offset=0", token, nil)
	check(w, 200)
	if !bytes.Equal(w.Body.Bytes(), data[:history.ChunkSize]) {
		t.Fatal("download mismatch")
	}
	w = request(m, "GET", path+fmt.Sprintf("/files/download?offset=%d", history.ChunkSize), token, nil)
	check(w, 200)
	if !bytes.Equal(w.Body.Bytes(), data[history.ChunkSize:]) {
		t.Fatal("tail mismatch")
	}
	rec.Operation = "changed"
	changed, _ := json.Marshal(rec)
	check(request(m, "POST", "/api/history/calls", token, changed), 409)
	check(request(m, "GET", path, "wrong-key", nil), 401)
	other := sha256.Sum256([]byte("other"))
	m.cfg.Keys = append(m.cfg.Keys, bridgeKey{"other-tenant", "bridge", hex.EncodeToString(other[:])})
	check(request(m, "GET", path, "other", nil), 404)
	invoke := func(tenant string, admin bool) apibridge.Response {
		t.Helper()
		res, e := apibridge.Invoke(context.Background(), http.HandlerFunc(m.authenticate), apibridge.Principal{Subject: "admin", TenantID: tenant, Admin: admin}, apibridge.Request{Method: "GET", Path: path + "?bridge=bridge"})
		if e != nil {
			t.Fatal(e)
		}
		return res
	}
	if r := invoke("tenant", true); r.Status != 200 {
		t.Fatal(r)
	}
	if r := invoke("other-tenant", true); r.Status != 403 {
		t.Fatal(r)
	}
	if r := invoke("tenant", false); r.Status != 403 {
		t.Fatal(r)
	}
}

func TestInsuredDetailsKeepAllCategoriesAndStates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record-gzip")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	gz := gzip.NewWriter(f)
	xml := `<insuredResponse xmlns="http://www.cnas.ro/siui/2.0"><insured pid="test-pid" cid="12345678901234567890" state="1"><categories><category code="3" validFrom="2017-04-04"/><category code="4"/></categories><states><state code="SAL"/><state code="OTHER" validTo="2030-01-01"/></states></insured></insuredResponse>`
	json.NewEncoder(gz).Encode(map[string]any{"result": map[string]any{"values": []string{xml}}})
	gz.Close()
	f.Close()
	details, e := parseInsuredArtifact(path)
	if e != nil {
		t.Fatal(e)
	}
	if details["state"] != 1 || details["cid"] != "12345678901234567890" || len(details["categories"].([]map[string]any)) != 2 || len(details["states"].([]map[string]any)) != 2 {
		t.Fatal(details)
	}
}
