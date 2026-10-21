package siui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDashboardScopeQueueAndHourly(t *testing.T) {
	db, e := openJobs(filepath.Join(t.TempDir(), "calls.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, e = db.Exec(`INSERT INTO siui_calls(id,owner,operation_id,operation,digest,status,created_at) VALUES
 ('a','local:a','a','getCatalogues','d','completed','2026-10-21T09:00:00Z'),
 ('b','local:b','b','getCatalogues','d','failed','2026-10-21T10:00:00Z'),
 ('q','local:a','q','getInsured','d','queued','2026-10-20T09:00:00Z');`)
	if e != nil {
		t.Fatal(e)
	}
	ready := make(chan struct{})
	close(ready)
	m := &Module{db: db, ready: ready, calls: make(chan serviceWork, 20), queue: make(chan work, 20)}
	for _, admin := range []bool{false, true} {
		w := httptest.NewRecorder()
		m.dashboardHTTP(w, httptest.NewRequest("GET", "/?from=2026-10-21T00:00:00Z&to=2026-10-22T00:00:00Z", nil), "local:a", admin)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var got struct {
			Counts map[string]int   `json:"counts"`
			Hourly []map[string]any `json:"hourly"`
			Queue  []operationEntry `json:"queue"`
		}
		if e = json.Unmarshal(w.Body.Bytes(), &got); e != nil {
			t.Fatal(e)
		}
		if got.Counts["completed"] != 1 || got.Counts["queued"] != 0 || len(got.Queue) != 1 {
			t.Fatal(got)
		}
		if admin && got.Counts["failed"] != 1 || !admin && got.Counts["failed"] != 0 {
			t.Fatal(got)
		}
		if got.Hourly[0]["at"] != "2026-10-21T09:00:00Z" {
			t.Fatal(got.Hourly)
		}
	}
	w := httptest.NewRecorder()
	m.dashboardHTTP(w, httptest.NewRequest("GET", "/?from=2025-01-01T00:00:00Z&to=2026-01-01T00:00:00Z", nil), "local:a", true)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
func TestOCSPDashboardStateNeverWaitsOrLeaksToken(t *testing.T) {
	cfg := settings{BaseURL: "https://example.org"}
	s := ocspSession{key: ocspConfigKey(cfg), token: "SECRET", acquired: time.Now(), lastAttempt: time.Now()}
	if s.dashboardState(cfg)["state"] != "cached" {
		t.Fatal("expected active session")
	}
	b, _ := json.Marshal(s.dashboardState(cfg))
	if strings.Contains(string(b), "SECRET") {
		t.Fatal("secret leaked")
	}
	s.acquired = time.Now().Add(-10 * time.Minute)
	if s.dashboardState(cfg)["state"] != "expired" {
		t.Fatal("expired session is not an outage")
	}
	s.lastFailed = true
	if s.dashboardState(cfg)["state"] != "failed" {
		t.Fatal("failure missing")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dashboardState(cfg)["state"] != "authenticating" {
		t.Fatal("must not wait for token mutex")
	}
}
func TestEndpointProbeIsUnauthenticatedHEAD(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "HEAD" || r.Header.Get("Authorization") != "" || r.URL.RawQuery != "" {
			t.Error("probe sent credentials or unsafe method")
		}
		w.WriteHeader(401)
	}))
	defer srv.Close()
	result := probeEndpoint(context.Background(), srv.Client(), endpointCheck{URL: srv.URL})
	if !result.Reachable || result.HTTPStatus != 401 || result.CheckedAt == "" {
		t.Fatal(result)
	}
	// A strict client must not accept the untrusted test certificate.
	result = probeEndpoint(context.Background(), &http.Client{Timeout: time.Second}, endpointCheck{URL: srv.URL})
	if result.Reachable || result.Error == "" {
		t.Fatal(result)
	}
}
