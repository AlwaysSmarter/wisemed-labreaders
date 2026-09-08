package signingpad

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type authRuntime struct{ *testRuntime }

func (r *authRuntime) Service(name string) (interface{}, bool) {
	if name == "local-http-control" {
		return testSessionGuard{}, true
	}
	return nil, false
}

type testSessionGuard struct{}

func (testSessionGuard) HasSession(r *http.Request) bool {
	return r.Header.Get("X-Fixture-Session") == "yes"
}
func (g testSessionGuard) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.HasSession(r) {
			http.Error(w, "authentication required", 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func TestSettingsAndDemoRequireLogin(t *testing.T) {
	rt := &authRuntime{&testRuntime{dir: t.TempDir(), mux: http.NewServeMux(), settings: map[string]interface{}{"shared_http": true}}}
	if err := os.WriteFile(rt.ConfigPath(), []byte("modules: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m := &Module{}
	if err := m.Init(rt); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/esignature/settings", "/api/esignature/jobs", "/api/esignature/stats/daily", "/ws/demo"} {
		w := httptest.NewRecorder()
		rt.mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 {
			t.Errorf("%s returned %d without login", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	rt.mux.ServeHTTP(w, httptest.NewRequest("GET", "/esignature", nil))
	if w.Code != 303 || w.Header().Get("Location") != "/" {
		t.Fatal("legacy page did not redirect to login", w.Code)
	}
	request := httptest.NewRequest("PUT", "/api/esignature/settings", strings.NewReader(`{"manufacturer":"signotec","pad_type":"omega","model":"Evolis Sig200","session_timeout_seconds":240,"device_index":1}`))
	request.Header.Set("X-Fixture-Session", "yes")
	w = httptest.NewRecorder()
	rt.mux.ServeHTTP(w, request)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if m.padSettingsSnapshot().DeviceIndex != 1 || m.sessionTimeout().Seconds() != 240 {
		t.Fatal("saved settings not applied")
	}
	persisted, _ := os.ReadFile(rt.ConfigPath())
	if !strings.Contains(string(persisted), "session_timeout_seconds: 240") {
		t.Fatal(string(persisted))
	}
}
