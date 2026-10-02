package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"wisemed-labreaders/serverlast/wsm-server/internal/config"
)

func adminFixture(t *testing.T, response string) (*Server, *config.Config) {
	t.Helper()
	up := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" || r.URL.Path != "/administrative/login" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("wrong upstream contract")
		}
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		if in["username"] != "operator" || in["password"] != "PASSWORD-PRIVATE" {
			t.Error("login payload")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, response)
	}))
	t.Cleanup(up.Close)
	c := testConfig(t)
	c.Admin = config.Admin{Enabled: true, PublicOrigin: "https://admin.example", SessionTTLSeconds: 900}
	c.Security.DeviceKeysFile = filepath.Join(t.TempDir(), "state", "devices.yaml")
	for id, tenant := range c.Tenants {
		tenant.WiseMed = config.WiseMed{BaseURL: up.URL, APIKey: strings.Repeat("p", 40)}
		c.Tenants[id] = tenant
	}
	s := New(c)
	s.upstreamClient = up.Client()
	s.upstreamClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	t.Cleanup(s.Close)
	return s, c
}
func adminCall(s *Server, method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://admin.example"+path, strings.NewReader(body))
	r.Header.Set("Origin", "https://admin.example")
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if csrf != "" {
		r.Header.Set("X-CSRF-Token", csrf)
	}
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	return w
}
func loginAdmin(t *testing.T, s *Server, tenant string) (*http.Cookie, string) {
	t.Helper()
	w := adminCall(s, "POST", "/admin/api/login", fmt.Sprintf(`{"tenant_id":%q,"username":"operator","password":"PASSWORD-PRIVATE"}`, tenant), nil, "")
	if w.Code != 200 {
		t.Fatalf("login %d %s", w.Code, w.Body.String())
	}
	var data map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &data)
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/admin" || cookie.MaxAge != 900 {
		t.Fatalf("cookie flags %+v", cookie)
	}
	if strings.Contains(w.Body.String(), "PASSWORD-PRIVATE") || strings.Contains(w.Body.String(), "TOKEN-PRIVATE") {
		t.Fatal("credentials leaked")
	}
	return cookie, data["csrf_token"].(string)
}
func TestAdminStrictWiseMEDUserType(t *testing.T) {
	for _, value := range []string{"0", "-2", "1", "null", "false", `""`, `"-2"`, `"-01"`, "-1.0", "{}", "[]", "MISSING"} {
		t.Run(value, func(t *testing.T) {
			field := `"user_type":` + value + `,`
			if value == "MISSING" {
				field = ""
			}
			s, _ := adminFixture(t, `{`+field+`"login_token":"TOKEN-PRIVATE"}`)
			w := adminCall(s, "POST", "/admin/api/login", `{"tenant_id":"a","username":"operator","password":"PASSWORD-PRIVATE"}`, nil, "")
			if w.Code != 401 || len(w.Result().Cookies()) != 0 {
				t.Fatalf("accepted %s: %d", value, w.Code)
			}
		})
	}
	for _, response := range []string{`{"user_type":-1,"login_token":"TOKEN-PRIVATE"}`, `{"user_type":"-1","token":"TOKEN-PRIVATE"}`} {
		s, _ := adminFixture(t, response)
		loginAdmin(t, s, "a")
	}
	for _, response := range []string{`{"user_type":-1}`, `{"user_type":-1,"login_token":123}`, `{"user_type":-1,"login_token":"TOKEN-PRIVATE","ok":false}`, `{"user_type":-1,"login_token":"TOKEN-PRIVATE","status":"failure"}`, `{"user_type":-1,"login_token":"TOKEN-PRIVATE","error":"PASSWORD-PRIVATE"}`} {
		s, _ := adminFixture(t, response)
		w := adminCall(s, "POST", "/admin/api/login", `{"tenant_id":"a","username":"operator","password":"PASSWORD-PRIVATE"}`, nil, "")
		if w.Code != 401 || strings.Contains(w.Body.String(), "PRIVATE") {
			t.Fatalf("failure accepted or leaked %s", w.Body.String())
		}
	}
}
func TestAdminOriginCSRFSessionAndRate(t *testing.T) {
	s, c := adminFixture(t, `{"user_type":-1,"login_token":"TOKEN-PRIVATE"}`)
	cookie, csrf := loginAdmin(t, s, "a")
	for _, origin := range []string{"", "https://evil.example", "https://admin.example.evil"} {
		r := httptest.NewRequest("POST", "https://admin.example/admin/api/devices", strings.NewReader(`{"equipment_id":"42"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("origin %q accepted", origin)
		}
	}
	if w := adminCall(s, "POST", "/admin/api/devices", `{"equipment_id":"42"}`, cookie, "wrong"); w.Code != 403 {
		t.Fatal("CSRF not checked")
	}
	if w := adminCall(s, "GET", "/admin/api/devices", "", nil, ""); w.Code != 401 {
		t.Fatal("anonymous devices")
	}
	if w := adminCall(s, "POST", "/admin/api/logout", "{}", cookie, csrf); w.Code != 200 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout failed")
	}
	if w := adminCall(s, "GET", "/admin/api/devices", "", cookie, ""); w.Code != 401 {
		t.Fatal("logout retained session")
	}
	cookie, csrf = loginAdmin(t, s, "a")
	s.admin.mu.Lock()
	for id, v := range s.admin.sessions {
		v.ExpiresAt = time.Now().Add(-time.Second)
		s.admin.sessions[id] = v
	}
	s.admin.mu.Unlock()
	if w := adminCall(s, "POST", "/admin/api/devices", `{"equipment_id":"42"}`, cookie, csrf); w.Code != 401 {
		t.Fatal("expired session accepted")
	}
	cookie, _ = loginAdmin(t, s, "a")
	clone := *c
	if err := s.Reload(&clone); err != nil {
		t.Fatal(err)
	}
	if w := adminCall(s, "GET", "/admin/api/devices", "", cookie, ""); w.Code != 401 {
		t.Fatal("reload retained session")
	}
	for i := 0; i < 8; i++ {
		adminCall(s, "POST", "/admin/api/login", `{"tenant_id":"a","username":"operator","password":"PASSWORD-PRIVATE"}`, nil, "")
	}
	if w := adminCall(s, "POST", "/admin/api/login", `{"tenant_id":"a","username":"operator","password":"PASSWORD-PRIVATE"}`, nil, ""); w.Code != 429 {
		t.Fatalf("rate limit %d", w.Code)
	}
	c2 := *s.current()
	c2.Admin.Enabled = false
	s.cfg = &c2
	if w := adminCall(s, "GET", "/admin/", "", nil, ""); w.Code != 404 {
		t.Fatal("disabled UI accessible")
	}
}
func TestAdminDevicePersistenceRevealIsolationAndSocketRevocation(t *testing.T) {
	s, c := adminFixture(t, `{"user_type":-1,"login_token":"TOKEN-PRIVATE"}`)
	ca, csrfA := loginAdmin(t, s, "a")
	cb, csrfB := loginAdmin(t, s, "b")
	w := adminCall(s, "POST", "/admin/api/devices", `{"equipment_id":"42"}`, ca, csrfA)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created struct {
		Device struct {
			KeyID    string `json:"key_id"`
			Subject  string `json:"subject"`
			ReaderID string `json:"reader_id"`
		}
		Secret string
	}
	if json.Unmarshal(w.Body.Bytes(), &created) != nil || len(created.Secret) < 32 || created.Device.ReaderID != "" {
		t.Fatal("ID-only creation")
	}
	current := s.current()
	key := current.Security.Keys[created.Device.KeyID]
	if key.EquipmentID != "42" || key.TenantID != "a" || strings.Contains(strings.Join(key.Scopes, ","), "api:") {
		t.Fatal("binding/scopes")
	}
	data, err := config.ReadDevices(c.Security.DeviceKeysFile)
	if err != nil || data[created.Device.KeyID].Secret != created.Secret {
		t.Fatal("persistence", err)
	}
	st, _ := os.Stat(c.Security.DeviceKeysFile)
	if st.Mode().Perm() != 0600 {
		t.Fatal("registry permission")
	}
	restarted, err := c.WithDevices(data)
	if err != nil || restarted.Security.Keys[created.Device.KeyID].Secret != created.Secret {
		t.Fatal("restart")
	}
	list := adminCall(s, "GET", "/admin/api/devices", "", ca, "")
	if strings.Contains(list.Body.String(), created.Secret) || !strings.Contains(list.Body.String(), `"online":false`) {
		t.Fatal("list redaction/offline")
	}
	if w := adminCall(s, "GET", "/admin/api/devices", "", cb, ""); strings.Contains(w.Body.String(), "42") {
		t.Fatal("cross tenant list")
	}
	if w := adminCall(s, "POST", "/admin/api/devices/key", `{"equipment_id":"42"}`, cb, csrfB); w.Code != 404 {
		t.Fatal("cross tenant key")
	}
	if w := adminCall(s, "POST", "/admin/api/devices/key", `{"equipment_id":"42"}`, ca, csrfA); w.Code != 200 || !strings.Contains(w.Body.String(), created.Secret) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("reveal")
	}
	if w := adminCall(s, "POST", "/admin/api/devices/key", `{"equipment_id":"a-key"}`, ca, csrfA); w.Code != 404 {
		t.Fatal("master key revealed")
	}
	if w := adminCall(s, "POST", "/admin/api/devices", `{"equipment_id":"42"}`, ca, csrfA); w.Code != 409 {
		t.Fatal("duplicate")
	}
	httpServer := httptest.NewTLSServer(s.routes())
	defer httpServer.Close()
	dialer := websocket.Dialer{TLSClientConfig: httpServer.Client().Transport.(*http.Transport).TLSClientConfig}
	claims := claimsFor("a", "reader", "local-reader")
	claims.Subject = created.Device.Subject
	claims.EquipmentID = "42"
	claims.Scopes = config.DeviceScopes()
	token := signed(t, current, claims, created.Device.KeyID)
	headers := http.Header{"Authorization": []string{"Bearer " + token}}
	ws, _, err := dialer.Dial("wss"+strings.TrimPrefix(httpServer.URL, "https")+"/ws", headers)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	ws.WriteJSON(Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "reader", "client_id": "local-reader", "reader_id": "local-reader", "equipment_id": "42"}})
	var ack Envelope
	if err = ws.ReadJSON(&ack); err != nil || ack.Type != "hello_ack" {
		t.Fatal("generated key handshake", ack, err)
	}
	list = adminCall(s, "GET", "/admin/api/devices", "", ca, "")
	if !strings.Contains(list.Body.String(), `"online":true`) {
		t.Fatal("online status")
	}
	if w := adminCall(s, "DELETE", "/admin/api/devices/"+created.Device.KeyID, "{}", cb, csrfB); w.Code != 404 {
		t.Fatal("cross tenant delete")
	}
	otherClaims := claimsFor("b", "browser", "browser-b")
	otherHeaders := http.Header{"Authorization": []string{"Bearer " + signed(t, current, otherClaims, "b-key")}}
	other, _, err := dialer.Dial("wss"+strings.TrimPrefix(httpServer.URL, "https")+"/ws", otherHeaders)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	other.WriteJSON(Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "browser", "client_id": "browser-b"}})
	if err = other.ReadJSON(&ack); err != nil || ack.Type != "hello_ack" {
		t.Fatal("unrelated socket setup", err)
	}
	parentClaims := claimsFor("a", "browser", "operator-browser")
	parentClaims.Scopes = append(parentClaims.Scopes, "api:invoke")
	issued, err := s.issueControlTicket(s.current(), &parentClaims, "42")
	if err != nil {
		t.Fatal("issue ticket", err)
	}
	if w := adminCall(s, "POST", "/admin/api/devices", `{"equipment_id":"43"}`, ca, csrfA); w.Code != 201 {
		t.Fatal("second create", w.Code, w.Body.String())
	}
	request := httptest.NewRequest("GET", "https://admin.example/ws", nil)
	request.Header.Set("Sec-WebSocket-Protocol", "wsm.v1, wsm.ticket."+issued["ticket"].(string))
	if _, err := s.authenticateSocket(s.current(), request); err != nil {
		t.Fatal("unrelated provisioning invalidated ticket", err)
	}
	wrong := claims
	wrong.EquipmentID = "other-equipment"
	if _, err := authenticate(s.current(), authRequest(signed(t, s.current(), wrong, created.Device.KeyID)), true); err == nil {
		t.Fatal("device equipment binding bypassed")
	}
	wrong = claims
	wrong.Scopes = append(append([]string(nil), wrong.Scopes...), "api:invoke")
	if _, err := authenticate(s.current(), authRequest(signed(t, s.current(), wrong, created.Device.KeyID)), true); err == nil {
		t.Fatal("device scope escalation")
	}
	// A stale config snapshot loaded before deletion must not resurrect its key.
	stale := *current
	if w := adminCall(s, "DELETE", "/admin/api/devices/"+created.Device.KeyID, "{}", ca, csrfA); w.Code != 200 {
		t.Fatal("revoke", w.Body.String())
	}
	ws.SetReadDeadline(time.Now().Add(time.Second))
	for {
		if e := ws.ReadJSON(&ack); e != nil {
			if timeout, ok := e.(net.Error); ok && timeout.Timeout() {
				t.Fatal("revoked socket not closed")
			}
			break
		}
	}
	other.SetReadDeadline(time.Now().Add(time.Second))
	other.WriteJSON(Envelope{Type: "ping", RequestID: "still-open"})
	for {
		if err = other.ReadJSON(&ack); err != nil {
			t.Fatal("unrelated socket was closed", err)
		}
		if ack.Type == "pong" {
			break
		}
	}
	if _, err = authenticate(s.current(), authRequest(token), true); err == nil {
		t.Fatal("revoked key authenticates")
	}
	if err = s.Reload(&stale); err != nil {
		t.Fatal(err)
	}
	if _, exists := s.current().Security.Keys[created.Device.KeyID]; exists {
		t.Fatal("stale reload resurrected key")
	}
}
func TestAdminConcurrentCreatesRemainDurable(t *testing.T) {
	s, c := adminFixture(t, `{"user_type":-1,"login_token":"TOKEN-PRIVATE"}`)
	cookie, csrf := loginAdmin(t, s, "a")
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for retry := 0; retry < 30; retry++ {
				w := adminCall(s, "POST", "/admin/api/devices", fmt.Sprintf(`{"equipment_id":"eq-%d"}`, i), cookie, csrf)
				if w.Code == 201 {
					return
				}
				if w.Code != 409 {
					t.Errorf("create %d %s", w.Code, w.Body.String())
					return
				}
			}
			t.Error("retries exhausted")
		}(i)
	}
	wg.Wait()
	records, e := config.ReadDevices(c.Security.DeviceKeysFile)
	if e != nil || len(records) != 12 || len(s.current().Devices) != 12 {
		t.Fatalf("lost update %d %v", len(records), e)
	}
}
func TestAdminPersistenceFailurePublishesNoKey(t *testing.T) {
	s, c := adminFixture(t, `{"user_type":-1,"login_token":"TOKEN-PRIVATE"}`)
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if e := os.WriteFile(blocker, []byte("blocked"), 0600); e != nil {
		t.Fatal(e)
	}
	c.Security.DeviceKeysFile = filepath.Join(blocker, "devices.yaml")
	cookie, csrf := loginAdmin(t, s, "a")
	before := len(s.current().Security.Keys)
	w := adminCall(s, "POST", "/admin/api/devices", `{"equipment_id":"42"}`, cookie, csrf)
	if w.Code != 500 || len(s.current().Security.Keys) != before || len(s.current().Devices) != 0 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("persistence failure leaked/published %d %s", w.Code, w.Body.String())
	}
}

func TestAdminUpdateReaderBinding(t *testing.T) {
	s, c := adminFixture(t, `{"user_type":-1,"login_token":"TOKEN-PRIVATE"}`)
	ca, csrf := loginAdmin(t, s, "a")
	cb, csrfB := loginAdmin(t, s, "b")
	w := adminCall(s, "POST", "/admin/api/devices", `{"equipment_id":"42"}`, ca, csrf)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created struct {
		Device struct {
			KeyID   string `json:"key_id"`
			Subject string `json:"subject"`
		}
		Secret string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	path := "/admin/api/devices/" + created.Device.KeyID
	for _, tc := range []struct {
		body   string
		cookie *http.Cookie
		csrf   string
		status int
	}{
		{`{"reader_id":"reader-new"}`, nil, "", 401},
		{`{"reader_id":"reader-new"}`, ca, "", 403},
		{`{"reader_id":"reader-new"}`, cb, csrfB, 404},
		{`{}`, ca, csrf, 400}, {`{"reader_id":null}`, ca, csrf, 400},
		{`{"reader_id":5}`, ca, csrf, 400}, {`{"reader_id":"../bad"}`, ca, csrf, 400},
		{`{"reader_id":"ok","secret":"replace"}`, ca, csrf, 400},
	} {
		if got := adminCall(s, "PATCH", path, tc.body, tc.cookie, tc.csrf); got.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.body, got.Code, got.Body.String())
		}
	}
	claims := claimsFor("a", "reader", "local-reader")
	claims.Subject = created.Device.Subject
	claims.EquipmentID = "42"
	claims.Scopes = config.DeviceScopes()
	token := signed(t, s.current(), claims, created.Device.KeyID)
	ts := httptest.NewTLSServer(s.routes())
	defer ts.Close()
	dialer := websocket.Dialer{TLSClientConfig: ts.Client().Transport.(*http.Transport).TLSClientConfig}
	ws, _, err := dialer.Dial("wss"+strings.TrimPrefix(ts.URL, "https")+"/ws", http.Header{"Authorization": []string{"Bearer " + token}})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if err := ws.WriteJSON(Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "reader", "client_id": "local-reader", "reader_id": "local-reader", "equipment_id": "42"}}); err != nil {
		t.Fatal(err)
	}
	var ack Envelope
	if err := ws.ReadJSON(&ack); err != nil || ack.Type != "hello_ack" {
		t.Fatal("hello", err, ack)
	}
	for _, readerID := range []string{"new-reader", ""} {
		w = adminCall(s, "PATCH", path, fmt.Sprintf(`{"reader_id":%q}`, readerID), ca, csrf)
		if w.Code != 200 || strings.Contains(w.Body.String(), created.Secret) {
			t.Fatal("update", w.Code, w.Body.String())
		}
		devices, err := config.ReadDevices(c.Security.DeviceKeysFile)
		if err != nil {
			t.Fatal(err)
		}
		d := devices[created.Device.KeyID]
		if d.ReaderID != readerID || d.Secret != created.Secret || d.Subject != claims.Subject {
			t.Fatal("identity/secret not preserved")
		}
		restored, err := c.WithDevices(devices)
		if err != nil || restored.Security.Keys[created.Device.KeyID].ReaderID != readerID {
			t.Fatal("restart", err)
		}
		_, err = authenticate(s.current(), authRequest(token), true)
		if (readerID == "") != (err == nil) {
			t.Fatal("reader binding not enforced", err)
		}
		if readerID != "" {
			ws.SetReadDeadline(time.Now().Add(time.Second))
			for {
				if err := ws.ReadJSON(&ack); err != nil {
					if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
						t.Fatal("changed device socket stayed open")
					}
					break
				}
			}
		}
	}
	// A failed write must leave both the credential and its old binding usable.
	before := s.current()
	oldPath := before.Security.DeviceKeysFile
	// Mutate only a private snapshot, with no active readers of the original config.
	broken := *before
	broken.Security = before.Security
	broken.Security.DeviceKeysFile = filepath.Join(t.TempDir(), "block", "devices.yaml")
	if err := os.WriteFile(filepath.Dir(broken.Security.DeviceKeysFile), []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	s.gate.Lock()
	s.cfg = &broken
	s.gate.Unlock()
	for _, method := range []string{"PATCH", "DELETE"} {
		w = adminCall(s, method, path, `{"reader_id":"another"}`, ca, csrf)
		if w.Code != 500 {
			t.Fatal("expected persist failure", method, w.Code)
		}
		if s.current().Devices[created.Device.KeyID].ReaderID != "" {
			t.Fatal("published failed mutation")
		}
	}
	stored, err := config.ReadDevices(oldPath)
	if err != nil || stored[created.Device.KeyID].Secret != created.Secret {
		t.Fatal("original registry damaged", err)
	}
}

func TestAdminPermissionEditing(t *testing.T) {
	s, c := adminFixture(t, `{"user_type":-1,"login_token":"TOKEN-PRIVATE"}`)
	cookie, csrf := loginAdmin(t, s, "a")
	other, otherCSRF := loginAdmin(t, s, "b")
	w := adminCall(s, "POST", "/admin/api/devices", `{"equipment_id":"42","reader_id":"reader-one"}`, cookie, csrf)
	if w.Code != 201 {
		t.Fatal(w.Code)
	}
	var result struct {
		Device struct {
			KeyID string `json:"key_id"`
		}
		Secret string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	id := result.Device.KeyID
	path := "/admin/api/devices/" + id
	for _, body := range []string{`{"scopes":[]}`, `{"scopes":["unknown"]}`, `{"scopes":["api:invoke"]}`, `{"scopes":["api:admin","route:command"]}`, `{"scopes":["route:reply","route:reply"]}`, `{"scopes":null}`} {
		if got := adminCall(s, "PATCH", path, body, cookie, csrf); got.Code != 400 {
			t.Fatal(body, got.Code)
		}
	}
	body := `{"scopes":["route:reply","route:command","api:invoke","api:admin"]}`
	if got := adminCall(s, "PATCH", path, body, other, otherCSRF); got.Code != 404 {
		t.Fatal("tenant", got.Code)
	}
	if got := adminCall(s, "PATCH", path, body, cookie, ""); got.Code != 403 {
		t.Fatal("csrf", got.Code)
	}
	w = adminCall(s, "PATCH", path, body, cookie, csrf)
	if w.Code != 200 || strings.Contains(w.Body.String(), result.Secret) {
		t.Fatal("permission update", w.Code)
	}
	stored, err := config.ReadDevices(c.Security.DeviceKeysFile)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := c.WithDevices(stored)
	if err != nil {
		t.Fatal(err)
	}
	k := restored.Security.Keys[id]
	if k.ReaderID != "reader-one" || k.Secret != result.Secret || !slices.Contains(k.Scopes, "api:admin") {
		t.Fatal("persistence/identity")
	}
	claims := claimsFor("a", "reader", "reader-one")
	claims.Subject = k.Subject
	claims.EquipmentID = "42"
	claims.Scopes = k.Scopes
	oldToken := signed(t, restored, claims, id)
	if _, err := authenticate(s.current(), authRequest(oldToken), true); err != nil {
		t.Fatal("new permission not authorized", err)
	}
	list := adminCall(s, "GET", "/admin/api/devices", "", cookie, "")
	if !strings.Contains(list.Body.String(), `"permissions"`) || !strings.Contains(list.Body.String(), `"api:invoke"`) || strings.Contains(list.Body.String(), result.Secret) {
		t.Fatal("catalog/list")
	}
	s.ticketMu.Lock()
	s.tickets["owned"] = controlTicket{claims: claims}
	otherClaims := claims
	otherClaims.Subject = "someone-else"
	s.tickets["other"] = controlTicket{claims: otherClaims}
	s.ticketMu.Unlock()
	w = adminCall(s, "PATCH", path, `{"scopes":["route:reply"]}`, cookie, csrf)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if _, err := authenticate(s.current(), authRequest(oldToken), true); err == nil {
		t.Fatal("removed permissions still authorized")
	}
	s.ticketMu.Lock()
	_, owned := s.tickets["owned"]
	_, unrelated := s.tickets["other"]
	s.ticketMu.Unlock()
	if owned || !unrelated {
		t.Fatal("ticket revocation isolation")
	}
	// Binding-only updates preserve customized rights.
	w = adminCall(s, "PATCH", path, `{"reader_id":""}`, cookie, csrf)
	if w.Code != 200 || !slices.Equal(s.current().Security.Keys[id].Scopes, []string{"route:reply"}) {
		t.Fatal("rights lost on reader edit")
	}
}
