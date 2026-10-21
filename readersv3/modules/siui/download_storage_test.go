package siui

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wisemed-labreaders/readersv3/shared/history"
)

func TestDownloadStorageMigrationAndSafeCleanup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "jobs.db")
	db, e := openJobs(dbPath)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ready := make(chan struct{})
	close(ready)
	m := &Module{db: db, ready: ready, cfg: settings{Database: dbPath}}
	root := filepath.Join(dir, "siui-downloads")
	os.MkdirAll(root, 0700)
	for _, id := range []string{"archived", "pending", "callback", "mismatch"} {
		path := filepath.Join(root, id)
		os.WriteFile(path, []byte("zip"), 0600)
		a, e := hashHistoryFile(path, "download")
		if e != nil {
			t.Fatal(e)
		}
		if id == "mismatch" {
			a.SHA256 = strings.Repeat("0", 64)
		}
		manifest, _ := json.Marshal(history.Record{Artifacts: []history.Artifact{a}})
		db.Exec(`INSERT INTO siui_calls(id,owner,operation_id,operation,digest,status,created_at) VALUES(?,'user',?,'getCatalogues','d','completed','2026-10-21T10:00:00Z')`, id, id)
		state := "archived"
		if id == "pending" {
			state = "pending"
		}
		db.Exec(`INSERT INTO siui_history_outbox(call_id,status,manifest) VALUES(?,?,?)`, id, state, string(manifest))
	}
	// A failed callback can be retried and must keep its local input.
	_, e = db.Exec(`INSERT INTO siui_wm_callbacks(call_id,spec,parameters,destination,path,status) VALUES('callback','{}','{}','x','/x','failed')`)
	if e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	m.downloadStorageHTTP(w, httptest.NewRequest("GET", "/", nil), false)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	m.downloadStorageHTTP(w, httptest.NewRequest("GET", "/", nil), true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	path := datedDownloadPath(dbPath, "archived", time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC))
	if existingDownloadPath(dbPath, "archived") != path {
		t.Fatal("migration failed")
	}
	w = httptest.NewRecorder()
	m.downloadStorageHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"confirm":true}`)), true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("archived copy retained", e)
	}
	for _, id := range []string{"pending", "callback", "mismatch"} {
		if _, e = os.Stat(existingDownloadPath(dbPath, id)); e != nil {
			t.Fatal("protected file removed", id, e)
		}
	}
	var got struct {
		Storage storageTotals `json:"storage"`
	}
	json.Unmarshal(w.Body.Bytes(), &got)
	if got.Storage.RemovedFiles != 1 || len(got.Storage.Errors) != 1 {
		t.Fatal(w.Body.String())
	}
}
