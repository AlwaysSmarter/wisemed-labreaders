package appupdateserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReleaseJobDuplicateAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &Module{releaseJob: &releaseJob{ID: 123, ApplicationID: 27, Status: "running", cancel: cancel}}
	call := func(method, query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		m.handleReleaseJob(w, httptest.NewRequest(method, "/?"+query, strings.NewReader("{}")), 27)
		return w
	}
	if w := call(http.MethodPost, ""); w.Code != http.StatusConflict {
		t.Fatalf("duplicate: %d", w.Code)
	}
	if w := call(http.MethodGet, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"123"`) {
		t.Fatalf("status: %s", w.Body.String())
	}
	if w := call(http.MethodDelete, "id=122"); w.Code != http.StatusConflict {
		t.Fatal("stale cancellation accepted")
	}
	if ctx.Err() != nil {
		t.Fatal("stale request cancelled job")
	}
	if w := call(http.MethodDelete, "id=123"); w.Code != http.StatusAccepted {
		t.Fatalf("cancel: %d", w.Code)
	}
	if ctx.Err() == nil || m.releaseJob.Status != "cancelling" {
		t.Fatal("job not cancelled")
	}
	if err := m.beginReleasePublication(ctx); err == nil {
		t.Fatal("cancelled job allowed to publish")
	}
	if w := call(http.MethodPost, ""); w.Code != http.StatusConflict {
		t.Fatal("lock released before worker exit")
	}
}

func TestReleasePublicationCannotBeCancelled(t *testing.T) {
	m := &Module{releaseJob: &releaseJob{ID: 123, Status: "running", cancel: func() { t.Fatal("publication cancelled") }}}
	if err := m.beginReleasePublication(context.Background()); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	m.handleReleaseJob(w, httptest.NewRequest(http.MethodDelete, "/?id=123", nil), 27)
	if w.Code != http.StatusConflict {
		t.Fatalf("cancel publication: %d", w.Code)
	}
}

func TestReleaseProgressBounded(t *testing.T) {
	m := &Module{releaseJob: &releaseJob{Status: "running"}}
	for i := 0; i < 300; i++ {
		m.releaseProgress("build output")
	}
	if len(m.releaseJob.Logs) != 200 || m.releaseJob.UpdatedAt.IsZero() {
		t.Fatal("invalid progress")
	}
}
