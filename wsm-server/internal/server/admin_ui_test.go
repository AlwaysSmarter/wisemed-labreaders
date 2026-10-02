package server

import (
	"github.com/go-chi/chi/v5"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedAdminAssets(t *testing.T) {
	r := chi.NewRouter()
	registerAdminUI(r)
	for _, path := range []string{"/admin/", "/admin/app.js", "/admin/styles.css"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 || rec.Body.Len() == 0 {
			t.Fatalf("%s: %d", path, rec.Code)
		}
		if rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Fatalf("missing security headers: %s", path)
		}
	}
	for _, path := range []string{"/admin/app.test.cjs", "/admin/api/devices", "/admin/../../server.go"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 404 {
			t.Fatalf("unexpected asset exposed %s: %d", path, rec.Code)
		}
	}
}
