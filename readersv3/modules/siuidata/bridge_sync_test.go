package siuidata

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"wisemed-labreaders/readersv3/modules/ws"
)

type fakeBridge struct {
	mu         sync.Mutex
	data       []byte
	filename   string
	badOffset  bool
	starts     []string
	calls      int
	operations []string
}

func (f *fakeBridge) downloadFilename() string {
	if f.filename != "" {
		return f.filename
	}
	return "catalogue.zip"
}
func (f *fakeBridge) Connected() bool { return true }
func (f *fakeBridge) Exchange(ctx context.Context, e ws.Envelope) (map[string]interface{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e.Type == "list_connections" {
		return map[string]interface{}{"connections": []any{map[string]any{"id": "bridge-one", "client_type": "siui-bridge", "reader_id": "test-bridge"}, map[string]any{"id": "other", "client_type": "reader"}}}, nil
	}
	if e.Target == nil || e.Target.ConnectionID != "bridge-one" {
		return nil, fmt.Errorf("wrong target")
	}
	args := e.Payload["args"].(map[string]any)
	path := args["path"].(string)
	wrap := func(status int, body map[string]any) map[string]interface{} {
		return map[string]interface{}{"kind": "api.response", "status": status, "body": body}
	}
	if path == "/api/siui/general/getCatalogues" || path == "/api/siui/general/getPhysicianCatalogues" {
		f.operations = append(f.operations, path)
		f.calls++
		parameters := args["body"].(map[string]any)["parameters"].(map[string]string)
		if path == "/api/siui/general/getPhysicianCatalogues" && len(parameters) != 0 {
			return nil, fmt.Errorf("getPhysicianCatalogues must receive empty parameters")
		}
		f.starts = append(f.starts, parameters["start"])
		return wrap(202, map[string]any{"call": map[string]any{"id": "call-one", "status": "queued"}}), nil
	}
	if path == "/api/siui/calls/call-one" {
		return wrap(200, map[string]any{"call": map[string]any{"id": "call-one", "status": "completed", "result": map[string]any{"file": map[string]any{"size": len(f.data), "filename": f.downloadFilename()}}}}), nil
	}
	if strings.HasPrefix(path, "/api/siui/calls/call-one/file?") {
		u, _ := url.Parse(path)
		offset, _ := strconv.ParseInt(u.Query().Get("offset"), 10, 64)
		end := offset + 1024
		if end > int64(len(f.data)) {
			end = int64(len(f.data))
		}
		reported := offset
		if f.badOffset {
			reported++
		}
		return wrap(200, map[string]any{"offset": reported, "next_offset": end, "eof": end == int64(len(f.data)), "data_base64": base64.StdEncoding.EncodeToString(f.data[offset:end])}), nil
	}
	return nil, fmt.Errorf("unexpected request %s", path)
}

func TestWSMDownloadAndNewerOnlyActivation(t *testing.T) {
	m, server := newTestModule(t, testPostgresSettings(t))
	dated := func(day, code string) string {
		return strings.Replace(strings.ReplaceAll(currentFixture(), `"001"`, `"`+code+`"`), `<Catalogues xmlns=`, `<Catalogues issueDate="`+day+`T06:00:00" xmlns=`, 1)
	}
	first := upload(t, server, "CLIN", bytes.NewReader(zipXML(t, "clin.xml", dated("2026-10-01", "001"))))
	awaitImport(t, server, first, "ready")
	bridge := &fakeBridge{data: zipXML(t, "clin.xml", dated("2026-10-02", "002"))}
	m.rt.(*testRuntime).bridge = bridge
	syncJob := func() string {
		t.Helper()
		status, data := apiCall(t, server, "POST", prefix+"bridge/sync", "application/json", strings.NewReader(`{"partnerCategory":"CLIN"}`))
		if status != 202 {
			t.Fatalf("sync %d %s", status, data)
		}
		var job map[string]any
		decodeData(t, data, &job)
		return job["importId"].(string)
	}
	job := syncJob()
	awaitImport(t, server, job, "ready")
	status, _ := apiCall(t, server, "GET", prefix+"services/002", "", nil)
	if status != 200 {
		t.Fatal("WSM download was not activated")
	}
	bridge.mu.Lock()
	if len(bridge.starts) != 1 || bridge.starts[0] != catalogueCheckStart(time.Now().In(m.loc), nil) {
		t.Fatal("first check did not request from January 1 of previous year")
	}
	bridge.mu.Unlock()
	job = syncJob()
	awaitImport(t, server, job, "unchanged")
	bridge.mu.Lock()
	bridge.data = zipXML(t, "clin.xml", dated("2026-09-30", "003"))
	bridge.mu.Unlock()
	job = syncJob()
	awaitImport(t, server, job, "unchanged")
	if status, _ = apiCall(t, server, "GET", prefix+"services/002", "", nil); status != 200 {
		t.Fatal("older WSM source replaced catalogue")
	}
	bridge.mu.Lock()
	bridge.badOffset = true
	bridge.mu.Unlock()
	job = syncJob()
	awaitImport(t, server, job, "failed")
	if status, _ = apiCall(t, server, "GET", prefix+"services/002", "", nil); status != 200 {
		t.Fatal("bad chunk replaced catalogue")
	}
	status, _ = apiCall(t, server, "POST", prefix+"bridge/sync", "application/json", strings.NewReader(`{"partnerCategory":"CLIN","connection_id":"other"}`))
	if status != 400 {
		t.Fatal("non-bridge target accepted")
	}
	bridge.mu.Lock()
	if bridge.calls != 4 {
		t.Fatal("unexpected automatic CNAS resubmission")
	}
	bridge.mu.Unlock()
}

// Reproduces CNAS returning its latest PARA version for a start in the past.
func TestWSMHistoricalStartUsesReturnedCatalogueDate(t *testing.T) {
	m, server := newTestModule(t, testPostgresSettings(t))
	dated := func(day string) string {
		return strings.Replace(paraFixture, "<Catalogues", `<Catalogues issueDate="`+day+`T06:00:00"`, 1)
	}
	first := upload(t, server, "PARA", bytes.NewReader(zipXML(t, "para.xml", dated("2026-01-21"))))
	awaitImport(t, server, first, "ready")
	db, e := m.database(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(context.Background(), `UPDATE siui_data_imports SET source_kind='wsm',created_at='2026-01-21T18:21:07Z' WHERE id=$1`, first); e != nil {
		t.Fatal(e)
	}
	// A failed newer attempt must not become the next request watermark.
	if _, e = db.Exec(context.Background(), `INSERT INTO siui_data_imports(id,catalogue_type,status,as_of,source_kind,created_at) VALUES('00000000-0000-0000-0000-000000000001','PARA','failed','2026-01-22','wsm','2026-01-22T18:21:07Z')`); e != nil {
		t.Fatal(e)
	}
	bridge := &fakeBridge{data: zipXML(t, "para.xml", dated("2026-06-02")), filename: "NomenclatoareParaclinice_20260602.xml.zip"}
	m.rt.(*testRuntime).bridge = bridge
	syncJob := func() string {
		t.Helper()
		status, data := apiCall(t, server, "POST", prefix+"bridge/sync", "application/json", strings.NewReader(`{"partnerCategory":"PARA"}`))
		if status != 202 {
			t.Fatalf("sync %d %s", status, data)
		}
		var job map[string]any
		decodeData(t, data, &job)
		return job["importId"].(string)
	}
	job := awaitImport(t, server, syncJob(), "ready")
	if job["sourceDate"] != "2026-06-02" {
		t.Fatalf("request start was mistaken for source date: %+v", job)
	}
	bridge.mu.Lock()
	if len(bridge.starts) != 1 || bridge.starts[0] != "2026-01-21T18:21:07Z" {
		t.Error("unexpected update watermark")
	}
	bridge.mu.Unlock()
	var checked time.Time
	if e = db.QueryRow(context.Background(), `SELECT created_at FROM siui_data_imports WHERE id=$1`, job["importId"]).Scan(&checked); e != nil {
		t.Fatal(e)
	}
	unchanged := awaitImport(t, server, syncJob(), "unchanged")
	// The displayed page now contains only a failed attempt. The successful
	// check must still be available from the full history.
	if _, e = db.Exec(context.Background(), `UPDATE siui_data_imports SET created_at=now()+interval '1 day' WHERE id='00000000-0000-0000-0000-000000000001'`); e != nil {
		t.Fatal(e)
	}
	status, data := apiCall(t, server, "GET", prefix+"versions?partnerCategory=PARA&limit=1", "", nil)
	if status != 200 {
		t.Fatalf("versions %d %s", status, data)
	}
	var history struct {
		LastCheckedAt map[string]time.Time `json:"lastCheckedAt"`
	}
	if e = json.Unmarshal(data, &history); e != nil {
		t.Fatal(e)
	}
	var expected time.Time
	if e = db.QueryRow(context.Background(), `SELECT created_at FROM siui_data_imports WHERE id=$1`, unchanged["importId"]).Scan(&expected); e != nil {
		t.Fatal(e)
	}
	if !history.LastCheckedAt["PARA"].Equal(expected) || !history.LastCheckedAt["CLIN"].IsZero() {
		t.Fatalf("incorrect per-category last check: %s", data)
	}
	bridge.mu.Lock()
	if len(bridge.starts) != 2 || bridge.starts[1] != checked.UTC().Format(time.RFC3339) {
		t.Error("next request did not use last successful check time")
	}
	bridge.mu.Unlock()
}

func TestCatalogueCheckStart(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.FixedZone("RO", 3*3600))
	if got := catalogueCheckStart(now, nil); got != "2025-01-01T00:00:00Z" {
		t.Fatal(got)
	}
	last := time.Date(2026, 1, 21, 20, 21, 7, 0, time.FixedZone("RO", 2*3600))
	if got := catalogueCheckStart(now, &last); got != "2026-01-21T18:21:07Z" {
		t.Fatal(got)
	}
}
