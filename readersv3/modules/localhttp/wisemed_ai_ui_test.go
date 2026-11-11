package localhttp

import (
	"net/http/httptest"
	"testing"
)

func TestWiseMEDAIHomeAndLegacyReaderRoutes(t *testing.T) {
	for _, protocol := range []string{"wisemed-ai", "astm"} {
		m := &Module{rt: &wssSecretRuntime{services: map[string]interface{}{
			"analyzer-config": map[string]interface{}{"protocol": protocol},
		}}}
		for _, path := range []string{"/", "/orders", "/qc", "/daily-details", "/settings/qc", "/settings/analytes", "/settings/reader", "/settings/wss", "/debug/wsm"} {
			t.Run(protocol+path, func(t *testing.T) {
				w := httptest.NewRecorder()
				m.handleIndex(w, httptest.NewRequest("GET", path, nil))
				want := ""
				if protocol == "wisemed-ai" {
					switch path {
					case "/", "/orders", "/qc", "/daily-details":
						want = "/ai/"
					case "/settings/qc", "/settings/analytes":
						want = "/settings/reader"
					}
				}
				if want != "" {
					if w.Code != 307 || w.Header().Get("Location") != want {
						t.Fatalf("got %d %s, want redirect to %s", w.Code, w.Header().Get("Location"), want)
					}
				} else if w.Code != 200 {
					t.Fatalf("shared console route changed: %d", w.Code)
				}
			})
		}
		if m.loginOnlyUtility() != (protocol == "wisemed-ai") {
			t.Fatal("wrong utility authentication mode")
		}
	}
}
