package siui

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"wisemed-labreaders/readersv3/shared/history"
)

func TestHistoryOutageRestartAndResumption(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "jobs.db")
	db, e := openJobs(dbPath)
	if e != nil {
		t.Fatal(e)
	}
	m := &Module{db: db, cfg: settings{Database: dbPath}, rt: &testRuntime{path: filepath.Join(dir, "config.yaml")}, history: &historySync{database: dbPath}}
	defer func() { m.db.Close() }()
	created := "2026-10-18T10:00:01Z"
	_, e = db.Exec(`INSERT INTO siui_calls(id,owner,operation_id,operation,digest,status,result,created_at) VALUES('call','user','op','getInsured','digest','completed',?,?)`, `{"insured":{"pid":"test","state":1},"values":["original XML"]}`, created)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`INSERT INTO siui_history_outbox(call_id,parameters,started_at,completed_at) VALUES('call','{"pid":"test"}',?,?)`, created, "2026-10-18T10:00:03Z")
	if e != nil {
		t.Fatal(e)
	}
	var outage atomic.Bool
	outage.Store(true)
	var offset int64
	var uploads int
	var manifest history.Record
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if outage.Load() {
			http.Error(w, "offline", 503)
			return
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing credential")
		}
		switch {
		case r.URL.Path == "/api/history/identity":
			json.NewEncoder(w).Encode(map[string]any{"tenant": "tenant", "bridge": "bridge"})
		case r.URL.Path == "/api/history/calls":
			json.NewDecoder(r.Body).Decode(&manifest)
			json.NewEncoder(w).Encode(history.Progress{Success: true, Status: "pending", Offsets: map[string]int64{"record-gzip": offset}})
		case r.Method == "PUT":
			uploads++
			if uploads == 1 {
				http.Error(w, "interrupted", 503)
				return
			}
			buf := make([]byte, history.ChunkSize)
			n, _ := r.Body.Read(buf)
			offset += int64(n)
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case strings.HasSuffix(r.URL.Path, "/commit"):
			json.NewEncoder(w).Encode(history.Progress{Success: true, Status: "archived"})
		default:
			t.Error(r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	// Trust only the test server CA through the production client configuration.
	certPath := filepath.Join(dir, "ca.pem")
	writeTestCertificate(t, certPath, srv.Certificate().Raw)
	m.history.cfg = historySettings{Enabled: true, URL: srv.URL, Token: "token", CAFile: certPath, RetrySeconds: 5}
	m.syncHistory(context.Background())
	var status string
	var attempts int
	db.QueryRow(`SELECT status,attempts FROM siui_history_outbox`).Scan(&status, &attempts)
	if status != "pending" || attempts != 1 {
		t.Fatal(status, attempts)
	}
	outage.Store(false)
	db.Exec(`UPDATE siui_history_outbox SET next_attempt=''`)
	m.syncHistory(context.Background())
	if uploads != 1 {
		t.Fatal("upload not attempted")
	}
	db.Close()
	m.db, e = openJobs(dbPath)
	if e != nil {
		t.Fatal(e)
	}
	m.db.Exec(`UPDATE siui_history_outbox SET next_attempt=''`)
	m.syncHistory(context.Background())
	m.db.QueryRow(`SELECT status FROM siui_history_outbox`).Scan(&status)
	if status != "archived" {
		var msg string
		m.db.QueryRow(`SELECT error FROM siui_history_outbox`).Scan(&msg)
		t.Fatal(status, msg)
	}
	if manifest.CreatedAt != created || manifest.CompletedAt != "2026-10-18T10:00:03Z" {
		t.Fatal(manifest)
	}
	if _, e = os.Stat(filepath.Join(dir, "siui-history-outbox", "call")); !os.IsNotExist(e) {
		t.Fatal("acknowledged staging not cleaned")
	}
	var retained string
	if e = m.db.QueryRow(`SELECT parameters FROM siui_history_outbox WHERE call_id='call'`).Scan(&retained); e != nil || !strings.Contains(retained, "test") {
		t.Fatal("request lost after archive", e, retained)
	}
	before := uploads
	m.syncHistory(context.Background())
	if uploads != before {
		t.Fatal("archived call replayed")
	}
}
func TestHistoryLegacyRequestPersisted(t *testing.T) {
	db, e := openJobs(filepath.Join(t.TempDir(), "jobs.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	job, _, e := insertJob(db, "owner", "operation", testXML)
	if e != nil {
		t.Fatal(e)
	}
	var params string
	if e = db.QueryRow(`SELECT parameters FROM siui_history_outbox WHERE call_id=?`, job.ID).Scan(&params); e != nil || !strings.Contains(params, "laboratoryService") {
		t.Fatal(e, params)
	}
}

func writeTestCertificate(t *testing.T, path string, der []byte) {
	t.Helper()
	if e := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); e != nil {
		t.Fatal(e)
	}
}

func TestHistoryRequiresExplicitArchiveConfirmation(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/commit") {
			w.Write([]byte(`{"success":true}`))
			return
		}
		w.Write([]byte(`{"success":true,"status":"pending"}`))
	}))
	defer srv.Close()
	m := &Module{}
	e := m.uploadHistory(context.Background(), srv.Client(), historySettings{URL: srv.URL}, history.Record{CallID: "call"})
	if e == nil {
		t.Fatal("unconfirmed archive accepted")
	}
}

func TestHistoryArchivesSentXMLAsArtifact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.db")
	db, e := openJobs(path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	job, _, e := insertJob(db, "owner", "operation", testXML)
	if e != nil {
		t.Fatal(e)
	}
	db.Exec(`UPDATE siui_jobs SET status='completed' WHERE id=?`, job.ID)
	var params string
	db.QueryRow(`SELECT parameters FROM siui_history_outbox WHERE call_id=?`, job.ID).Scan(&params)
	m := &Module{db: db, history: &historySync{database: path}}
	rec, e := m.prepareHistory(context.Background(), job.ID, params, "", "2026-10-21T10:00:00Z", "2026-10-21T10:00:01Z")
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, a := range rec.Artifacts {
		if a.Name == "request-xml" {
			found = true
			b, e := os.ReadFile(filepath.Join(dir, "siui-history-outbox", job.ID, a.Name))
			if e != nil || string(b) != testXML {
				t.Fatal(e, string(b))
			}
		}
	}
	if !found {
		t.Fatal(rec)
	}
}
