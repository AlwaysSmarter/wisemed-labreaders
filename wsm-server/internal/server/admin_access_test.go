package server

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"wisemed-labreaders/serverlast/wsm-server/internal/config"
)

func TestAdminBrowserLifecycleAndOrigins(t *testing.T) {
	s, c := adminFixture(t, `{"user_type":-1,"token":"LOGIN"}`)
	cookie, csrf := loginAdmin(t, s, "a")
	other, otherCSRF := loginAdmin(t, s, "b")
	call := func(method, path, body string, status int) map[string]interface{} {
		t.Helper()
		w := adminCall(s, method, path, body, cookie, csrf)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var out map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	create := call("POST", "/admin/api/browsers", `{"key_id":"web-test","origin":"https://php.example"}`, 201)
	secret := create["secret"].(string)
	if len(secret) < 32 {
		t.Fatal("weak secret")
	}
	key := s.current().Security.Keys["web-test"]
	if !keyHasRole(key, "browser") || !keyHasRole(key, "service") {
		t.Fatal("roles")
	}
	info := call("GET", "/admin/api/tenant", "", 200)
	if strings.Contains(fmt.Sprint(info), c.Tenants["a"].WiseMed.APIKey) {
		t.Fatal("upstream credential exposed")
	}
	listed := call("GET", "/admin/api/browsers", "", 200)
	if strings.Contains(fmt.Sprint(listed), secret) {
		t.Fatal("browser secret exposed")
	}
	if !strings.Contains(fmt.Sprint(listed), maskCredential(secret)) {
		t.Fatal("mask missing")
	}
	if w := adminCall(s, "GET", "/admin/api/browsers", "", other, ""); strings.Contains(w.Body.String(), "web-test") {
		t.Fatal("cross tenant list")
	}
	if w := adminCall(s, "POST", "/admin/api/browsers/key", `{"key_id":"web-test"}`, other, otherCSRF); w.Code != 404 {
		t.Fatal("cross tenant reveal")
	}
	if w := adminCall(s, "DELETE", "/admin/api/browsers/web-test", `{}`, other, otherCSRF); w.Code != 404 {
		t.Fatal("cross tenant delete")
	}
	if w := adminCall(s, "PATCH", "/admin/api/tenant/origins", `{"origin":"https://evil.example","action":"add"}`, cookie, "wrong"); w.Code != 403 {
		t.Fatal("CSRF")
	}
	call("POST", "/admin/api/browsers", `{"key_id":"web-test","origin":"https://php.example"}`, 409)
	call("POST", "/admin/api/browsers", `{"key_id":"bad","origin":"https://php.example/path"}`, 400)
	call("PATCH", "/admin/api/browsers/web-test", `{"origin":"https://new.example"}`, 200)
	if s.current().Security.Keys["web-test"].Secret != secret {
		t.Fatal("origin edit rotated credential")
	}
	call("PATCH", "/admin/api/browsers/web-test", `{"scopes":["api:admin"]}`, 400)
	call("PATCH", "/admin/api/browsers/web-test", `{"scopes":["route:command","api:invoke","api:admin"]}`, 200)
	rotated := call("PATCH", "/admin/api/browsers/web-test", `{"rotate":true}`, 200)["secret"].(string)
	if rotated == secret {
		t.Fatal("rotation reused secret")
	}
	if !strings.Contains(string(mustRead(t, c.Security.DeviceKeysFile)), rotated) {
		t.Fatal("rotation not persisted")
	}
	call("PATCH", "/admin/api/tenant/origins", `{"action":"delete","origin":"https://new.example"}`, 200)
	if allowedOriginValue(s.current().Tenants["a"].AllowedOrigins, "https://new.example") {
		t.Fatal("origin not removed")
	}
	// Device writes must preserve managed access and origins in the shared registry.
	call("POST", "/admin/api/devices", `{"equipment_id":"42"}`, 201)
	registry, err := config.ReadRegistry(c.Security.DeviceKeysFile)
	if err != nil {
		t.Fatal(err)
	}
	if registry.Browsers["web-test"].Secret != rotated {
		t.Fatal("device write lost browser")
	}
	reloaded := *s.current()
	if err := reloaded.RefreshDevices(); err != nil {
		t.Fatal(err)
	}
	if reloaded.Security.Keys["web-test"].Secret != rotated || allowedOriginValue(reloaded.Tenants["a"].AllowedOrigins, "https://new.example") {
		t.Fatal("reload lost access configuration")
	}
	call("DELETE", "/admin/api/browsers/web-test", `{}`, 200)
	if _, exists := s.current().Security.Keys["web-test"]; exists {
		t.Fatal("revocation ineffective")
	}
	if err := s.Reload(&reloaded); err != nil {
		t.Fatal(err)
	}
	if _, exists := s.current().Security.Keys["web-test"]; exists {
		t.Fatal("reload resurrected deleted key")
	}
}
func keyHasRole(key config.Key, role string) bool {
	for _, r := range key.Roles {
		if r == role {
			return true
		}
	}
	return false
}
func allowedOriginValue(origins []string, value string) bool {
	for _, o := range origins {
		if o == value {
			return true
		}
	}
	return false
}
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAdminTenantEditAndMaskedCredential(t *testing.T) {
	s, c := adminFixture(t, `{"user_type":-1,"token":"LOGIN"}`)
	cookie, csrf := loginAdmin(t, s, "a")
	secret := strings.Repeat("new-api-secret-", 3)
	body := fmt.Sprintf(`{"issuer":"new-issuer","base_url":"https://new.example/api","api_key":%q}`, secret)
	w := adminCall(s, "PATCH", "/admin/api/tenant", body, cookie, csrf)
	if w.Code != 200 || strings.Contains(w.Body.String(), secret) {
		t.Fatalf("edit %d %s", w.Code, w.Body.String())
	}
	tenant := s.current().Tenants["a"]
	if tenant.Issuer != "new-issuer" || tenant.WiseMed.BaseURL != "https://new.example/api" || tenant.WiseMed.APIKey != secret {
		t.Fatal("settings not applied")
	}
	if s.current().Tenants["b"].Issuer != c.Tenants["b"].Issuer {
		t.Fatal("other tenant changed")
	}
	if w := adminCall(s, "GET", "/admin/api/tenant", "", cookie, ""); w.Code != 401 {
		t.Fatal("tenant edit must require new login")
	}
	registry, err := config.ReadRegistry(c.Security.DeviceKeysFile)
	if err != nil {
		t.Fatal(err)
	}
	next, err := c.WithAccess(registry)
	if err != nil {
		t.Fatal(err)
	}
	if next.Tenants["a"].WiseMed.APIKey != secret {
		t.Fatal("tenant key not persisted")
	}
	for _, value := range []string{"", "abc", "abcdef", "abcdefg", "123456789"} {
		masked := maskCredential(value)
		if value != "" && masked == value {
			t.Fatal("full secret exposed")
		}
	}
	if maskCredential("123456789") != "123***789" {
		t.Fatal("wrong mask")
	}
}

func TestManagedBrowserJWTConnectionAndRotation(t *testing.T) {
	s, _ := adminFixture(t, `{"user_type":-1,"token":"LOGIN"}`)
	cookie, csrf := loginAdmin(t, s, "a")
	if w := adminCall(s, "POST", "/admin/api/browsers", `{"key_id":"web","origin":"https://php.example"}`, cookie, csrf); w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	host := httptest.NewTLSServer(s.routes())
	defer host.Close()
	dialer := websocket.Dialer{TLSClientConfig: host.Client().Transport.(*http.Transport).TLSClientConfig}
	claims := claimsFor("a", "browser", "php-browser")
	claims.Scopes = config.BrowserScopes()
	oldToken := signed(t, s.current(), claims, "web")
	endpoint := "wss" + strings.TrimPrefix(host.URL, "https") + "/ws"
	dial := func(token, origin string) (*websocket.Conn, *http.Response, error) {
		return dialer.Dial(endpoint, http.Header{"Authorization": []string{"Bearer " + token}, "Origin": []string{origin}})
	}
	ws, _, err := dial(oldToken, "https://php.example")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if err := ws.WriteJSON(Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "browser", "client_id": "php-browser"}}); err != nil {
		t.Fatal(err)
	}
	var ack Envelope
	if err := ws.ReadJSON(&ack); err != nil || ack.Type != "hello_ack" {
		t.Fatal("browser authentication", err, ack)
	}
	if denied, response, err := dial(oldToken, "https://wrong.example"); err == nil {
		denied.Close()
		t.Fatal("wrong origin accepted")
	} else if response.StatusCode != 403 {
		t.Fatal(response.StatusCode)
	}
	if w := adminCall(s, "PATCH", "/admin/api/browsers/web", `{"rotate":true}`, cookie, csrf); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	ws.SetReadDeadline(time.Now().Add(time.Second))
	// Presence events queued before revocation may still be buffered client-side.
	for {
		err = ws.ReadJSON(&ack)
		if err != nil {
			break
		}
	}
	if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("old browser socket survived rotation")
	}
	if denied, response, err := dial(oldToken, "https://php.example"); err == nil {
		denied.Close()
		t.Fatal("old signature accepted")
	} else if response.StatusCode != 401 {
		t.Fatal(response.StatusCode)
	}
	newToken := signed(t, s.current(), claims, "web")
	fresh, _, err := dial(newToken, "https://php.example")
	if err != nil {
		t.Fatal(err)
	}
	fresh.Close()
}

func TestRevealConfiguredBrowserKey(t *testing.T) {
	s, c := adminFixture(t, `{"user_type":-1,"token":"LOGIN"}`)
	readerOnly := c.Security.Keys["a-key"]
	readerOnly.Roles = []string{"reader"}
	c.Security.Keys["reader-only"] = readerOnly
	cookie, csrf := loginAdmin(t, s, "a")
	key := c.Security.Keys["a-key"]
	before := key.Secret
	response := adminCall(s, "POST", "/admin/api/browsers/key", `{"key_id":"a-key"}`, cookie, csrf)
	if response.Code != 200 {
		t.Fatalf("configured reveal %d %s", response.Code, response.Body.String())
	}
	var data struct {
		Secret  string `json:"secret"`
		Browser struct {
			Managed bool     `json:"managed"`
			Origins []string `json:"allowed_origins"`
		} `json:"browser"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data.Secret != before || data.Browser.Managed || len(data.Browser.Origins) == 0 {
		t.Fatal("wrong configured key metadata")
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("secret cached")
	}
	if w := adminCall(s, "GET", "/admin/api/browsers", "", cookie, ""); strings.Contains(w.Body.String(), before) {
		t.Fatal("list exposed secret")
	}
	for _, id := range []string{"b-key", "missing", "reader-only"} {
		body := fmt.Sprintf(`{"key_id":%q}`, id)
		if w := adminCall(s, "POST", "/admin/api/browsers/key", body, cookie, csrf); w.Code != 404 {
			t.Fatal("foreign/unknown key revealed")
		}
	}
	if w := adminCall(s, "POST", "/admin/api/browsers/key", `{"key_id":"a-key"}`, cookie, "wrong"); w.Code != 403 {
		t.Fatal("CSRF bypass")
	}
	if w := adminCall(s, "POST", "/admin/api/browsers/key", `{"key_id":"a-key"}`, nil, ""); w.Code != 401 {
		t.Fatal("anonymous reveal")
	}
	if w := adminCall(s, "PATCH", "/admin/api/browsers/a-key", `{"rotate":true}`, cookie, csrf); w.Code != 404 {
		t.Fatal("static key unexpectedly editable")
	}
	if s.current().Security.Keys["a-key"].Secret != before {
		t.Fatal("reveal modified credential")
	}
}
