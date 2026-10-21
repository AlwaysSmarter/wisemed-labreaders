package siuidata

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"
	"wisemed-labreaders/readersv3/core/module"
)

type testRuntime struct {
	path     string
	settings map[string]interface{}
	mux      *http.ServeMux
	t        testing.TB
	bridge   bridgeClient
}

func (r *testRuntime) ConfigPath() string              { return r.path }
func (r *testRuntime) ConfigDir() string               { return filepath.Dir(r.path) }
func (r *testRuntime) ReaderID() string                { return "test" }
func (r *testRuntime) Logf(f string, a ...interface{}) { r.t.Logf(f, a...) }
func (r *testRuntime) ModuleSettings(s string) map[string]interface{} {
	if s == "siui-data" {
		return r.settings
	}
	return nil
}
func (r *testRuntime) ResolvePath(s string) string {
	if filepath.IsAbs(s) {
		return s
	}
	return filepath.Join(filepath.Dir(r.path), s)
}
func (r *testRuntime) AddMenu(...module.MenuEntry)         {}
func (r *testRuntime) Handle(s string, h http.Handler)     { r.mux.Handle(s, h) }
func (r *testRuntime) Mux() *http.ServeMux                 { return r.mux }
func (r *testRuntime) RegisterService(string, interface{}) {}
func (r *testRuntime) Service(s string) (interface{}, bool) {
	if s == "wisemed-ws-client" && r.bridge != nil {
		return r.bridge, true
	}
	if s == "local-http-control" {
		return fakeGuard{true, true}, true
	}
	return nil, false
}

func testPostgresSettings(t testing.TB) settings {
	t.Helper()
	dsn := os.Getenv("SIUI_DATA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set SIUI_DATA_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	name := "siui_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, e = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); e != nil {
		admin.Close(ctx)
		t.Fatal(e)
	}
	t.Cleanup(func() {
		admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		admin.Close(ctx)
	})
	c := defaultSettings()
	p := admin.Config()
	c.PGHost = p.Host
	c.PGPort = int(p.Port)
	c.PGUser = p.User
	c.PGPassword = p.Password
	c.PGDatabase = name
	c.PGSSLMode = "disable"
	return c
}
func newTestModule(t testing.TB, c settings) (*Module, *httptest.Server) {
	t.Helper()
	b, _ := json.Marshal(c)
	var values map[string]interface{}
	json.Unmarshal(b, &values)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw, _ := yaml.Marshal(map[string]any{"analyzer": map[string]any{"protocol": "siui-data"}, "modules": map[string]any{"siui-data": values}})
	os.WriteFile(path, raw, 0600)
	rt := &testRuntime{path: path, settings: values, mux: http.NewServeMux(), t: t}
	m := &Module{}
	if e := m.Init(rt); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()
	s := httptest.NewTLSServer(rt.mux)
	t.Cleanup(func() {
		s.Close()
		cancel()
		if e := <-done; e != nil {
			t.Error(e)
		}
	})
	return m, s
}
func zipXML(t testing.TB, name, xml string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, e := z.Create(name)
	if e != nil {
		t.Fatal(e)
	}
	io.WriteString(f, xml)
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func apiCall(t testing.TB, s *httptest.Server, method, path, contentType string, body io.Reader) (int, []byte) {
	t.Helper()
	r, _ := http.NewRequest(method, s.URL+path, body)
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	response, e := s.Client().Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, data
}
func upload(t testing.TB, s *httptest.Server, typ string, body io.Reader) string {
	t.Helper()
	status, data := apiCall(t, s, "POST", "/upload?type="+typ, "application/zip", body)
	if status != 202 {
		t.Fatalf("upload %d: %s", status, data)
	}
	var result map[string]any
	decodeData(t, data, &result)
	return result["importId"].(string)
}
func awaitImport(t testing.TB, s *httptest.Server, id, wanted string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		status, data := apiCall(t, s, "GET", prefix+"imports/"+id, "", nil)
		if status != 200 {
			t.Fatalf("job %d %s", status, data)
		}
		var job map[string]any
		decodeData(t, data, &job)
		if job["status"] == "ready" || job["status"] == "failed" || job["status"] == "unchanged" {
			if job["status"] != wanted {
				t.Fatalf("job %+v", job)
			}
			return job
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("import timeout")
	return nil
}

const paraFixture = `<Catalogues xmlns="http://www.cnas.ro/siui/2.0"><Specialities><Speciality code="CARD" name="Diferit PARA" validFrom="2020-01-01"/></Specialities><MedSrvPacks><MedSrvPack code="BAZA" name="Pachet PARA" validFrom="2020-01-01"/></MedSrvPacks><LaboratoriesServices><LaboratoriesService code="001" name="Hemoleucogramă" highLevel="true" recByPC="false" formula="0" inClinic="1" serviceType="1" validFrom="2020-01-01"/></LaboratoriesServices><LabSrvMedPacks><LabSrvMedPack service="001" medSrvPack="BAZA" validFrom="2020-01-01"/></LabSrvMedPacks></Catalogues>`

func TestPostgresPipeline(t *testing.T) {
	fixtureData := currentFixture()
	settings := testPostgresSettings(t)
	m, s := newTestModule(t, settings)
	clin := zipXML(t, "clin.xml", fixtureData)
	para := zipXML(t, "para.xml", paraFixture)
	first := upload(t, s, "CLIN", bytes.NewReader(clin))
	awaitImport(t, s, first, "ready")
	second := upload(t, s, "PARA", bytes.NewReader(para))
	awaitImport(t, s, second, "ready")
	for _, path := range []string{"services/001?type=CLIN", "services/001?type=PARA", "services?type=CLIN&q=consultatie&specialty=CARD&isExam=true&package=BAZA", "services?type=PARA&highLevel=true&serviceType=1", "catalogues?type=CLIN", "records/Speciality?type=PARA"} {
		status, data := apiCall(t, s, "GET", prefix+path, "", nil)
		if status != 200 {
			t.Fatalf("%s: %d %s", path, status, data)
		}
		if strings.Contains(path, "services?") && !bytes.Contains(data, []byte(`"code": "001"`)) && !bytes.Contains(data, []byte(`"code":"001"`)) {
			t.Fatalf("search %s: %s", path, data)
		}
	}
	_, data := apiCall(t, s, "GET", prefix+"services/001?type=PARA", "", nil)
	if !bytes.Contains(data, []byte("Pachet PARA")) || bytes.Contains(data, []byte("Cardiologie")) {
		t.Fatal("cross-type contamination")
	}
	db, e := m.database(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	block, e := db.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = block.Exec(context.Background(), `SELECT * FROM siui_data_active WHERE catalogue_type='CLIN' FOR UPDATE`); e != nil {
		t.Fatal(e)
	}
	defer block.Rollback(context.Background())
	replacement := zipXML(t, "clin.xml", strings.ReplaceAll(fixtureData, `"001"`, `"002"`))
	third := upload(t, s, "CLIN", bytes.NewReader(replacement))
	// A second process/pool may start while a large import holds write locks.
	// It skips DDL on an initialized schema and must still serve the old data.
	_, replica := newTestModule(t, settings)
	if code, data := apiCall(t, replica, "GET", prefix+"services/001", "", nil); code != 200 {
		t.Fatalf("cold replica blocked by import: %d %s", code, data)
	}
	// Even while the replacement is waiting to commit, every query sees the old catalogue.
	for range 8 {
		status, data := apiCall(t, s, "GET", prefix+"services/001?type=CLIN", "", nil)
		if status != 200 || !bytes.Contains(data, []byte("Consulta")) {
			t.Fatal("reader unavailable during import")
		}
	}
	status, _ := apiCall(t, s, "POST", "/upload?type=CLIN", "application/zip", bytes.NewReader(clin))
	if status != 409 {
		t.Fatalf("parallel same-type import accepted: %d", status)
	}
	block.Commit(context.Background())
	awaitImport(t, s, third, "ready")
	if status, _ = apiCall(t, s, "GET", prefix+"services/001?type=CLIN", "", nil); status != 404 {
		t.Fatal("old service retained")
	}
	if status, _ = apiCall(t, s, "GET", prefix+"services/001?type=PARA", "", nil); status != 200 {
		t.Fatal("PARA changed during CLIN import")
	}
	for _, bad := range []string{fixtureData[:len(fixtureData)-8], strings.Replace(fixtureData, `specialty="CARD"`, `specialty="MISSING"`, 1), strings.Replace(fixtureData, `</ClinicServices>`, `<ClinicService code="001" name="duplicate" isExam="true" opRoom="false" isConnectedService="false" validFrom="2020-01-01"/></ClinicServices>`, 1), paraFixture} {
		id := upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "bad.xml", bad)))
		awaitImport(t, s, id, "failed")
		if status, _ = apiCall(t, s, "GET", prefix+"services/002", "", nil); status != 200 {
			t.Fatal("failed import replaced catalogue")
		}
	}
	if status, _ = apiCall(t, s, "POST", "/upload?type=OTHER", "application/zip", bytes.NewReader(clin)); status != 400 {
		t.Fatal("unknown type accepted")
	}
	if status, _ = apiCall(t, s, "POST", "/upload?type=CLIN", "application/zip", bytes.NewReader(zipXML(t, "../escape.xml", fixtureData))); status != 400 {
		t.Fatal("ZIP traversal accepted")
	}
	if status, _ = apiCall(t, s, "GET", prefix+"services?isExam=wrong", "", nil); status != 400 {
		t.Fatal("invalid filter accepted")
	}
	if status, _ = apiCall(t, s, "DELETE", prefix+"imports/"+third, "", nil); status != 409 {
		t.Fatal("active version deleted")
	}
	if status, _ = apiCall(t, s, "DELETE", prefix+"imports/"+first, "", nil); status != 200 {
		t.Fatal("cannot prune old version")
	}
}

func TestRealCatalogues(t *testing.T) {
	dir := os.Getenv("SIUI_DATA_REAL_CATALOGUES")
	if dir == "" {
		t.Skip("set SIUI_DATA_REAL_CATALOGUES to test actual large CNAS ZIP files")
	}
	_, s := newTestModule(t, testPostgresSettings(t))
	for _, source := range []struct{ typ, name string }{{"CLIN", "NomenclatoareClinice_20261001.xml.zip"}, {"PARA", "NomenclatoareParaclinice_20261001.xml.zip"}} {
		f, e := os.Open(filepath.Join(dir, source.name))
		if e != nil {
			t.Fatal(e)
		}
		start := time.Now()
		id := upload(t, s, source.typ, f)
		f.Close()
		job := awaitImport(t, s, id, "ready")
		t.Logf("%s: %v records / %v services in %s", source.typ, job["records"], job["services"], time.Since(start))
		status, data := apiCall(t, s, "GET", prefix+"services?type="+source.typ+"&limit=10", "", nil)
		if status != 200 {
			t.Fatalf("search %d %s", status, data)
		}
	}
	status, data := apiCall(t, s, "GET", prefix+"records/Drug?type=CLIN&limit=5", "", nil)
	if status != 200 || !bytes.Contains(data, []byte(`"Drug"`)) {
		t.Fatalf("drug preservation: %d %s", status, data)
	}
	// Report latency for real queries without claiming a production capacity.
	start := time.Now()
	for range 50 {
		status, _ = apiCall(t, s, "GET", prefix+"services?type=CLIN&q=consultatie&limit=20", "", nil)
		if status != 200 {
			t.Fatal(status)
		}
	}
	t.Logf("50 HTTPS searches: %s", time.Since(start))
}

func TestSettingsSecrets(t *testing.T) {
	c := defaultSettings()
	c.PGPassword = "private-test-password"
	m, s := newTestModule(t, c)
	status, data := apiCall(t, s, "GET", prefix+"settings", "", nil)
	if status != 200 || bytes.Contains(data, []byte(c.PGPassword)) {
		t.Fatal("settings exposed password")
	}
	status, data = apiCall(t, s, "PUT", prefix+"settings", "application/json", strings.NewReader(`{"postgres_host":"db.example.test","postgres_password":""}`))
	if status != 200 {
		t.Fatalf("settings %d %s", status, data)
	}
	raw, _ := os.ReadFile(m.rt.ConfigPath())
	if !bytes.Contains(raw, []byte(c.PGPassword)) {
		t.Fatal("blank password erased secret")
	}
	info, _ := os.Stat(m.rt.ConfigPath())
	if info.Mode().Perm() != 0600 {
		t.Fatal("config permissions")
	}
	status, data = apiCall(t, s, "GET", prefix+"settings", "", nil)
	if status != 200 || bytes.Contains(data, []byte(c.PGPassword)) {
		t.Fatal("settings exposed saved password")
	}
}

// Optional local visual QA harness. The permissive session exists only inside
// this test; production always uses the shared WiseMED authentication guard.
func TestManualPreview(t *testing.T) {
	ready := os.Getenv("SIUI_DATA_PREVIEW_READY_FILE")
	if ready == "" {
		t.Skip("manual UI preview disabled")
	}
	m, tlsServer := newTestModule(t, testPostgresSettings(t))
	for _, entry := range []struct{ typ, body string }{{"CLIN", medicinesXML()}, {"PARA", paraFixture}, {"PHM", `<Catalogues xmlns="http://www.cnas.ro/siui/2.0">` + drugFixture + `</Catalogues>`}} {
		body := strings.Replace(entry.body, `<Catalogues xmlns=`, `<Catalogues issueDate="2026-10-01T00:00:00" xmlns=`, 1)
		id := upload(t, tlsServer, entry.typ, bytes.NewReader(zipXML(t, "catalogue.xml", body)))
		awaitImport(t, tlsServer, id, "ready")
	}
	s := httptest.NewServer(m.rt.Mux())
	defer s.Close()
	if e := os.WriteFile(ready, []byte(s.URL), 0600); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(15 * time.Minute)
	for time.Now().Before(deadline) {
		if _, e := os.Stat(ready + ".stop"); e == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func currentFixture() string {
	loc, _ := time.LoadLocation("Europe/Bucharest")
	today := time.Now().In(loc)
	return strings.NewReplacer("2026-10-21", today.Format("2006-01-02"), "2026-10-20", today.AddDate(0, 0, -1).Format("2006-01-02"), "2026-10-22", today.AddDate(0, 0, 1).Format("2006-01-02")).Replace(fixture)
}

func decodeData(t testing.TB, body []byte, dest any) {
	t.Helper()
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if e := json.Unmarshal(body, &envelope); e != nil || !envelope.Success || len(envelope.Data) == 0 {
		t.Fatalf("invalid success envelope: %s", body)
	}
	if e := json.Unmarshal(envelope.Data, dest); e != nil {
		t.Fatal(e)
	}
}
