package siuihistory

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"wisemed-labreaders/readersv3/shared/history"
)

// Use an isolated MySQL instance; the test creates and drops its own database.
func TestMySQLHistoryBrowseAndDownload(t *testing.T) {
	dsn := os.Getenv("HISTORY_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set HISTORY_TEST_MYSQL_DSN for MySQL integration")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := fmt.Sprintf("history_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP DATABASE " + name)
	t.Chdir(t.TempDir())
	m := setup(t)
	m.rt.(*testRuntime).dir = "deployments"
	m.cfg.MySQLHost = "127.0.0.1"
	m.cfg.MySQLPort = strings.Split(cfg.Addr, ":")[1]
	m.cfg.MySQLUser = cfg.User
	m.cfg.MySQLPassword = cfg.Passwd
	m.cfg.MySQLDatabase = name
	db, err := m.database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	payload := bytes.Repeat([]byte("archive"), 100000)
	rec := history.Record{CallID: "call-01", OperationID: "op-01", Operation: "getCatalogues", Owner: "operator", Status: "completed", CreatedAt: "2026-10-21T10:00:00+03:00", CompletedAt: "2026-10-21T07:01:00Z", Artifacts: []history.Artifact{{Name: "download", Size: int64(len(payload))}}}
	store, err := m.store()
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Put(context.Background(), objectKey("tenant", "bridge-01", rec, "download"), bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatal(err)
	}
	insert := func(rec history.Record) {
		t.Helper()
		raw, _ := json.Marshal(rec)
		_, err := db.Exec(`INSERT INTO siui_history_calls(tenant,bridge,call_id,operation_id,operation,created_at,received_at,status,digest,record) VALUES('tenant','bridge-01',?,?,?,?,?,'archived','digest',?)`, rec.CallID, rec.OperationID, rec.Operation, rec.CreatedAt, rec.CompletedAt, string(raw))
		if err != nil {
			t.Fatal(err)
		}
	}
	insert(rec)
	if err = m.saveLocation(context.Background(), db, "tenant", "bridge-01", rec.CallID); err != nil {
		t.Fatal(err)
	}
	for i, at := range []string{"2026-10-21T07:00:00.001Z", "2026-10-21T06:59:59.999999999Z", "2026-10-21T07:00:01Z"} {
		r := rec
		r.CallID = fmt.Sprintf("other-%d", i)
		r.CreatedAt = at
		insert(r)
	}
	w := request(m, "GET", "/api/history/calls?from=2026-10-21T07:00:00Z&to=2026-10-21T07:00:00Z", "", nil)
	var result struct {
		Calls []struct {
			Bridge, Owner string
			Artifacts     []history.Artifact
		}
		HasMore bool `json:"has_more"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || len(result.Calls) != 1 || result.Calls[0].Bridge != "bridge-01" || result.Calls[0].Owner != "operator" || len(result.Calls[0].Artifacts) != 1 {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	w = request(m, "GET", "/api/history/calls?from=2026-10-21T07:00:00Z&to=2026-10-21T07:00:00.001Z", "", nil)
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Calls) != 2 {
		t.Fatal(w.Body.String(), err)
	}
	for i := 0; i < 101; i++ {
		r := rec
		r.CallID = fmt.Sprintf("page-%d", i)
		insert(r)
	}
	w = request(m, "GET", "/api/history/calls", "", nil)
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Calls) != 100 || !result.HasMore {
		t.Fatal(w.Body.String(), err)
	}
	w = request(m, "GET", "/api/history/calls?offset=100", "", nil)
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Calls) != 5 || result.HasMore {
		t.Fatal(w.Body.String(), err)
	}
	// Regression: the real runtime may use deployments/config.yaml, with a
	// relative config directory. Saved locations must not be resolved twice.
	var locationRaw string
	if err = db.QueryRow("SELECT location FROM siui_history_storage WHERE call_id=?", rec.CallID).Scan(&locationRaw); err != nil {
		t.Fatal(err)
	}
	var saved location
	if err = json.Unmarshal([]byte(locationRaw), &saved); err != nil || !filepath.IsAbs(saved.BasePath) {
		t.Fatal("new location is not absolute", locationRaw, err)
	}
	m.cfg.BasePath = "./new-storage"
	probe := request(m, "GET", "/api/history/calls/call-01/files/download?bridge=bridge-01&format=json", "", nil)
	if probe.Code != 200 {
		t.Fatal("new absolute location not readable", probe.Code, probe.Body.String())
	}
	saved.BasePath = "deployments/history" // Legacy location, as stored before the fix.
	legacy, _ := json.Marshal(saved)
	if _, err = db.Exec("UPDATE siui_history_storage SET location=? WHERE call_id=?", string(legacy), rec.CallID); err != nil {
		t.Fatal(err)
	}
	var actual []byte
	for offset := 0; offset < len(payload); {
		r := httptest.NewRequest("GET", fmt.Sprintf("/calls/call-01/files/download?bridge=bridge-01&format=json&offset=%d", offset), nil)
		w := httptest.NewRecorder()
		m.calls(w, r, db, "calls/call-01/files/download", "tenant", "", true)
		var chunk struct {
			Data string `json:"data_base64"`
			Next int    `json:"next_offset"`
		}
		if err = json.Unmarshal(w.Body.Bytes(), &chunk); err != nil || w.Code != 200 {
			t.Fatal(w.Code, w.Body.String(), err)
		}
		b, err := base64.StdEncoding.DecodeString(chunk.Data)
		if err != nil || len(b) == 0 || chunk.Next != offset+len(b) {
			t.Fatal("invalid chunk", err)
		}
		actual = append(actual, b...)
		offset = chunk.Next
	}
	if !bytes.Equal(actual, payload) {
		t.Fatal("truncated download")
	}
	// History activity is measured at receipt: a late upload of an older SIUI
	// operation still belongs to today's incoming volume.
	if _, err = db.Exec(`UPDATE siui_history_calls SET created_at='2026-09-01T10:00:00Z',record=JSON_SET(record,'$.created_at','2026-09-01T10:00:00Z') WHERE call_id='page-0'`); err != nil {
		t.Fatal(err)
	}
	// The dashboard combines period results with live/global archive state.
	pending := rec
	pending.CallID = "pending-old"
	pending.CreatedAt = "2026-09-20T10:00:00Z"
	pending.Status = "failed"
	pending.CompletedAt = "2026-09-20T10:01:00Z"
	insert(pending)
	if _, err = db.Exec("UPDATE siui_history_calls SET status='pending' WHERE call_id='pending-old'"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO siui_history_calls SELECT 'other-tenant',bridge,call_id,operation_id,operation,created_at,received_at,status,digest,record FROM siui_history_calls WHERE call_id='call-01'`); err != nil {
		t.Fatal(err)
	}
	m.cfg.Keys = []bridgeKey{{Tenant: "tenant", Bridge: "bridge-01"}, {Tenant: "tenant", Bridge: "bridge-01"}, {Tenant: "tenant", Bridge: "unused-bridge"}, {Tenant: "other-tenant", Bridge: "foreign"}}
	w = request(m, "GET", "/api/history/dashboard?from=2026-10-21T00:00:00Z&to=2026-10-22T00:00:00Z", "", nil)
	var dash struct {
		Totals     archiveTotals
		Counts     map[string]int64
		Bridges    []bridgeLoad
		Queue      []pendingArchive
		Hourly     []archiveBucket
		Authorized int `json:"authorized_bridges"`
		Senders    int `json:"sending_bridges"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &dash); err != nil || w.Code != 200 {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	if dash.Totals.Calls != 106 || dash.Totals.Archived != 105 || dash.Totals.Pending != 1 || dash.Totals.Files != 105 || dash.Totals.PayloadBytes != 105*int64(len(payload)) || dash.Totals.PendingBytes != int64(len(payload)) {
		t.Fatalf("incorrect totals: %+v", dash.Totals)
	}
	if dash.Counts["completed"] != 105 || dash.Counts["failed"] != 0 || dash.Authorized != 2 || dash.Senders != 1 || len(dash.Bridges) != 2 || len(dash.Queue) != 1 || dash.Queue[0].CallID != "pending-old" {
		t.Fatal(w.Body.String())
	}
	var bucketTotal int64
	for _, bucket := range dash.Hourly {
		bucketTotal += bucket.Count
	}
	if bucketTotal != 105 {
		t.Fatal("chart does not match totals", bucketTotal)
	}
	w = request(m, "GET", "/api/history/dashboard?from=2025-01-01T00:00:00Z&to=2025-01-02T00:00:00Z", "", nil)
	if err = json.Unmarshal(w.Body.Bytes(), &dash); err != nil || dash.Counts["completed"] != 0 || dash.Totals.Pending != 1 {
		t.Fatal("global queue changed with reporting period", w.Body.String(), err)
	}

	w = request(m, "GET", "/api/history/calls?call_id=page-0&from=2026-10-21T00:00:00Z&to=2026-10-22T00:00:00Z&time_field=received_at&to_exclusive=1", "", nil)
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Calls) != 1 {
		t.Fatal("receipt-time drilldown excluded late operation", w.Body.String(), err)
	}
	w = request(m, "GET", "/api/history/calls?call_id=page-0&from=2026-10-21T00:00:00Z&to=2026-10-22T00:00:00Z&time_field=created_at&to_exclusive=1", "", nil)
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Calls) != 0 {
		t.Fatal("operation-time filter ignored", w.Body.String(), err)
	}
	w = request(m, "GET", "/api/history/calls?operation_status=failed&status=pending", "", nil)
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Calls) != 1 {
		t.Fatal("result/archive filters failed", w.Body.String(), err)
	}
	w = request(m, "GET", "/api/history/calls?from=2026-10-21T07:00:00Z&to=2026-10-21T07:00:00Z&to_exclusive=1", "", nil)
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Calls) != 0 {
		t.Fatal("exclusive end not respected", w.Body.String(), err)
	}

	// Legacy archives expose the original CNAS filename through details, binary
	// headers and JSON chunks, without changing their stored manifest.
	named := rec
	named.CallID = "legacy-filename"
	zipBytes := []byte{'P', 'K', 3, 4}
	var gzBody bytes.Buffer
	gz := gzip.NewWriter(&gzBody)
	_ = json.NewEncoder(gz).Encode(map[string]any{"result": map[string]any{"values": []string{"/svapntws/download?fileName=7pf6aug8hu06t1k5ff9gnc", "4"}}})
	gz.Close()
	named.Artifacts = []history.Artifact{{Name: "download", Size: 4}, {Name: "record-gzip", Size: int64(gzBody.Len())}}
	insert(named)
	if err = store.Put(context.Background(), objectKey("tenant", "bridge-01", named, "download"), bytes.NewReader(zipBytes), 4); err != nil {
		t.Fatal(err)
	}
	if err = store.Put(context.Background(), objectKey("tenant", "bridge-01", named, "record-gzip"), bytes.NewReader(gzBody.Bytes()), int64(gzBody.Len())); err != nil {
		t.Fatal(err)
	}
	m.cfg.BasePath = "./history"
	if err = m.saveLocation(context.Background(), db, "tenant", "bridge-01", named.CallID); err != nil {
		t.Fatal(err)
	}
	w = request(m, "GET", "/api/history/calls/legacy-filename?bridge=bridge-01", "", nil)
	var detail struct{ Record history.Record }
	if err = json.Unmarshal(w.Body.Bytes(), &detail); err != nil || w.Code != 200 || detail.Record.Artifacts[0].Filename != "7pf6aug8hu06t1k5ff9gnc.zip" {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	w = request(m, "GET", "/api/history/calls/legacy-filename/files/download?bridge=bridge-01", "", nil)
	_, disposition, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
	if err != nil || w.Code != 200 || disposition["filename"] != "7pf6aug8hu06t1k5ff9gnc.zip" || w.Header().Get("Content-Type") != "application/zip" || !bytes.Equal(w.Body.Bytes(), zipBytes) {
		t.Fatal(w.Code, w.Header(), err)
	}
	w = request(m, "GET", "/api/history/calls/legacy-filename/files/download?bridge=bridge-01&format=json", "", nil)
	var chunkMeta struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &chunkMeta); err != nil || chunkMeta.Filename != "7pf6aug8hu06t1k5ff9gnc.zip" || chunkMeta.ContentType != "application/zip" {
		t.Fatal(w.Body.String(), err)
	}
	var persisted string
	if err = db.QueryRow("SELECT record FROM siui_history_calls WHERE call_id='legacy-filename'").Scan(&persisted); err != nil || strings.Contains(persisted, `"filename"`) {
		t.Fatal("legacy manifest was modified", err)
	}

}
