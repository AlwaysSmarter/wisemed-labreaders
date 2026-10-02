package localhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/modules/ws"
)

type wssSecretRuntime struct {
	module.Runtime
	services map[string]interface{}
	secret   string
}

func (r *wssSecretRuntime) ReaderID() string { return "secret-test-reader" }
func (r *wssSecretRuntime) ModuleSettings(string) map[string]interface{} {
	return map[string]interface{}{"device_secret": r.secret}
}
func (r *wssSecretRuntime) RegisterService(name string, value interface{}) { r.services[name] = value }
func (r *wssSecretRuntime) Service(name string) (interface{}, bool) {
	value, ok := r.services[name]
	return value, ok
}

func TestWSSStatusReportsConfiguredSecretWithoutReturningIt(t *testing.T) {
	secret := strings.Repeat("secret", 8)
	rt := &wssSecretRuntime{services: map[string]interface{}{}, secret: secret}
	client := ws.New()
	if err := client.Init(rt); err != nil {
		t.Fatal(err)
	}
	m := &Module{rt: rt, sessions: map[string]session{"admin": {Username: "admin", ExpiresAt: time.Now().Add(time.Hour)}}}
	req := httptest.NewRequest(http.MethodGet, "/api/wss/status", nil)
	req.AddCookie(&http.Cookie{Name: legacySessionCookieName, Value: "admin"})
	w := httptest.NewRecorder()
	m.handleWSS(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var response struct {
		Settings map[string]interface{} `json:"settings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Settings["device_secret_configured"] != true {
		t.Fatal("configured flag missing")
	}
	if _, found := response.Settings["device_secret"]; found || strings.Contains(w.Body.String(), secret) {
		t.Fatal("status response leaked key")
	}
}
