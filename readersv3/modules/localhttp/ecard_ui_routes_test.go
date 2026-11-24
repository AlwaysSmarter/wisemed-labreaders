package localhttp

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestECardUIRefreshRoutes(t *testing.T) {
	m := &Module{rt: &wssSecretRuntime{services: map[string]interface{}{
		"analyzer-config": map[string]interface{}{"protocol": "ecard"},
	}}}
	for _, path := range []string{"/", "/settings", "/settings/reader", "/settings/wss", "/settings/ecard", "/debug", "/debug/ecard", "/debug/wsm"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			m.handleIndex(w, httptest.NewRequest("GET", path, nil))
			if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("refresh returned %d %s", w.Code, w.Header().Get("Content-Type"))
			}
		})
	}
	for _, path := range []string{"/missing", "/api/missing", "/debug/missing"} {
		w := httptest.NewRecorder()
		m.handleIndex(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatalf("unknown route %s returned %d", path, w.Code)
		}
	}
}
