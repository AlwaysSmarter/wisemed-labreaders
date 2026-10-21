package siui

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"wisemed-labreaders/readersv3/shared/apibridge"
	"wisemed-labreaders/readersv3/shared/history"
)

func TestOperationsTimelineFiltersAndOwnership(t *testing.T) {
	db, err := openJobs(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ready := make(chan struct{})
	close(ready)
	m := &Module{db: db, ready: ready, guard: testGuard{}}
	for _, row := range []struct{ id, owner, at string }{{"mine", "local:operator", "2026-10-21T21:30:00Z"}, {"other", "local:other", "2026-10-21T10:00:00Z"}, {"earlier", "local:operator", "2026-10-20T20:00:00Z"}} {
		_, err = db.Exec(`INSERT INTO siui_calls(id,owner,operation_id,operation,digest,status,created_at) VALUES(?,?,?,'getCatalogues','d','queued',?)`, row.id, row.owner, row.id, row.at)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.Exec(`UPDATE siui_calls SET status='running' WHERE id='mine'; UPDATE siui_calls SET status='completed',result='{"values":["ok"]}' WHERE id='mine'`)
	if err != nil {
		t.Fatal(err)
	}
	list := func(query, owner string, admin bool) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/siui/operations?"+query, nil)
		m.operationsHTTP(w, r, "operations", owner, admin)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	q := "from=2026-10-21T00:00:00%2B03:00&to=2026-10-22T00:00:00%2B03:00"
	// mine is on the next local day, earlier on the previous day; only other matches.
	out := list(q, "local:operator", false)
	if out["total"].(float64) != 0 {
		t.Fatal(out)
	}
	out = list(q, "local:operator", true)
	if out["total"].(float64) != 1 {
		t.Fatal(out)
	}
	out = list("from=2026-10-21T00:00:00Z&to=2026-10-22T00:00:00Z&operation_id=mine", "local:operator", false)
	if out["total"].(float64) != 1 {
		t.Fatal(out)
	}
	w := httptest.NewRecorder()
	m.operationsHTTP(w, httptest.NewRequest("GET", "/", nil), "operations/mine", "local:other", false)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	m.operationsHTTP(w, httptest.NewRequest("GET", "/", nil), "operations/mine", "local:operator", false)
	var details struct {
		Call operationEntry `json:"call"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &details); err != nil {
		t.Fatal(err)
	}
	if len(details.Call.Events) != 3 {
		t.Fatal(w.Body.String())
	}
	for i, s := range []string{"queued", "running", "completed"} {
		if details.Call.Events[i].Status != s || details.Call.Events[i].At == "" {
			t.Fatal(details)
		}
	}
	if err = initializeOperationAudit(db); err != nil {
		t.Fatal(err)
	}
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM siui_operation_events WHERE call_id='mine'`).Scan(&count)
	if count != 3 {
		t.Fatal("migration duplicated events")
	}
	w = httptest.NewRecorder()
	m.operationsHTTP(w, httptest.NewRequest("GET", "/?from=bad&to=bad", nil), "operations", "local:operator", false)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func TestOperationsAttestedWSMSource(t *testing.T) {
	m := &Module{guard: testGuard{}}
	var got operationSource
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = m.operationSource(r); w.WriteHeader(200) })
	_, err := apibridge.Invoke(t.Context(), handler, apibridge.Principal{Subject: "user-17", TenantID: "clinic", ConnectionID: "connection-1", SourceIP: "192.0.2.5"}, apibridge.Request{Method: "GET", Path: "/api/siui/operations"})
	if err != nil || got.Transport != "wsm" || got.Subject != "user-17" || got.IP != "192.0.2.5" || got.Connection != "connection-1" {
		t.Fatal(got, err)
	}
}

func TestOperationHistoryDownloadProxy(t *testing.T) {
	dir := t.TempDir()
	db, err := openJobs(filepath.Join(dir, "calls.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	data := strings.Repeat("x", history.ChunkSize+17)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("credential missing")
		}
		if r.URL.Path == "/api/history/identity" {
			json.NewEncoder(w).Encode(map[string]string{"tenant": "tenant", "bridge": "bridge"})
			return
		}
		if r.URL.Path != "/api/history/calls/call/files/download" {
			t.Error(r.URL.Path)
			http.NotFound(w, r)
			return
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		io.WriteString(w, data[offset:min(len(data), offset+history.ChunkSize)])
	}))
	defer srv.Close()
	ca := filepath.Join(dir, "ca.pem")
	writeTestCertificate(t, ca, srv.Certificate().Raw)
	cfg := historySettings{URL: srv.URL, Token: "test-secret", CAFile: ca}
	record := history.Record{Artifacts: []history.Artifact{{Name: "download", Size: int64(len(data))}}}
	raw, _ := json.Marshal(record)
	destination := history.Digest(struct{ URL, Tenant, Bridge string }{srv.URL, "tenant", "bridge"})
	_, err = db.Exec(`INSERT INTO siui_history_outbox(call_id,manifest,destination) VALUES('call',?,?)`, string(raw), destination)
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{db: db, rt: &testRuntime{}, history: &historySync{cfg: cfg}}
	w := httptest.NewRecorder()
	m.operationHistoryHTTP(w, httptest.NewRequest("GET", "/", nil), operationEntry{ID: "call"}, []string{"history", "download"})
	if w.Code != 200 || w.Body.String() != data {
		t.Fatal(w.Code, w.Body.Len())
	}
	w = httptest.NewRecorder()
	m.operationHistoryHTTP(w, httptest.NewRequest("GET", "/?offset=262144", nil), operationEntry{ID: "call"}, []string{"history", "download"})
	var chunk struct {
		Data string `json:"data_base64"`
		Next int    `json:"next_offset"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &chunk)
	decoded, _ := base64.StdEncoding.DecodeString(chunk.Data)
	if w.Code != 200 || len(decoded) != 17 || chunk.Next != len(data) {
		t.Fatal(w.Code, w.Body.String())
	}
	m.history.cfg.URL = srv.URL + "/changed"
	// Wrong destination identity must not expose any other call's file.
	_, _ = db.Exec(`UPDATE siui_history_outbox SET destination='different' WHERE call_id='call'`)
	m.history.cfg = cfg
	w = httptest.NewRecorder()
	m.operationHistoryHTTP(w, httptest.NewRequest("GET", "/", nil), operationEntry{ID: "call"}, []string{"history", "download"})
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
}

func TestRequestDownloadOwnershipAndChunks(t *testing.T) {
	db, e := openJobs(filepath.Join(t.TempDir(), "jobs.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	job, _, e := insertJob(db, "owner", "op", testXML)
	if e != nil {
		t.Fatal(e)
	}
	ready := make(chan struct{})
	close(ready)
	m := &Module{db: db, ready: ready}
	for _, owner := range []string{"owner", "other"} {
		w := httptest.NewRecorder()
		m.operationsHTTP(w, httptest.NewRequest("GET", "/?name=request-xml&offset=0", nil), "operations/"+job.ID+"/request", owner, false)
		if owner == "other" {
			if w.Code != 404 {
				t.Fatal(w.Code)
			}
			continue
		}
		var response struct {
			Data string `json:"data_base64"`
		}
		if e = json.Unmarshal(w.Body.Bytes(), &response); e != nil {
			t.Fatal(e)
		}
		b, _ := base64.StdEncoding.DecodeString(response.Data)
		if string(b) != testXML {
			t.Fatal(w.Body.String())
		}
	}
}
