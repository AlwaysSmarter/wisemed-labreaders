package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"wisemed-labreaders/serverlast/wsm-server/internal/config"
)

var allScopes = []string{"route:command", "route:reply", "route:event", "route:broadcast", "topics:subscribe", "connections:read", "server:status", "wisemed:proxy"}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("WSM_TEST_A", strings.Repeat("a", 40))
	t.Setenv("WSM_TEST_B", strings.Repeat("b", 40))
	c := &config.Config{Server: config.Server{Address: "127.0.0.1", AllowInsecureLoopback: true}, Tenants: map[string]config.Tenant{"a": {Issuer: "issuer-a", AllowedOrigins: []string{"https://a.example"}}, "b": {Issuer: "issuer-b", AllowedOrigins: []string{"https://b.example"}}}, Security: config.Security{Keys: map[string]config.Key{"a-key": {TenantID: "a", SecretEnv: "WSM_TEST_A", Roles: []string{"browser", "reader", "service"}, Scopes: allScopes}, "b-key": {TenantID: "b", SecretEnv: "WSM_TEST_B", Roles: []string{"browser", "reader", "service"}, Scopes: allScopes}}}}
	if e := c.Prepare(t.TempDir()); e != nil {
		t.Fatal(e)
	}
	return c
}
func claimsFor(tenant, role, id string) AuthClaims {
	now := time.Now()
	c := AuthClaims{TenantID: tenant, Role: role, ClientID: id, Scopes: append([]string(nil), allScopes...), RegisteredClaims: jwt.RegisteredClaims{Subject: id, Issuer: "issuer-" + tenant, Audience: jwt.ClaimStrings{"wsm-server"}, IssuedAt: jwt.NewNumericDate(now.Add(-time.Second)), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute))}}
	if role == "reader" {
		c.ReaderID = id
		c.EquipmentID = id
	}
	return c
}
func signed(t *testing.T, cfg *config.Config, c AuthClaims, kid string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	token.Header["kid"] = kid
	s, e := token.SignedString([]byte(cfg.Security.Keys[kid].Secret))
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func authRequest(token string) *http.Request {
	r := httptest.NewRequest("GET", "https://example/ws", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}
func TestAuthentication(t *testing.T) {
	cfg := testConfig(t)
	cases := []struct {
		name   string
		change func(*AuthClaims)
		deny   bool
	}{
		{"valid", func(*AuthClaims) {}, false},
		{"tenant spoof", func(c *AuthClaims) { c.TenantID = "b" }, true},
		{"issuer", func(c *AuthClaims) { c.Issuer = "other" }, true},
		{"audience", func(c *AuthClaims) { c.Audience = jwt.ClaimStrings{"other"} }, true},
		{"missing expiry", func(c *AuthClaims) { c.ExpiresAt = nil }, true},
		{"expired", func(c *AuthClaims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute)) }, true},
		{"missing iat", func(c *AuthClaims) { c.IssuedAt = nil }, true},
		{"future iat", func(c *AuthClaims) { c.IssuedAt = jwt.NewNumericDate(time.Now().Add(time.Minute)) }, true},
		{"future nbf", func(c *AuthClaims) { c.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Minute)) }, true},
		{"excess TTL", func(c *AuthClaims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour)) }, true},
		{"unknown role", func(c *AuthClaims) { c.Role = "admin" }, true},
		{"unknown scope", func(c *AuthClaims) { c.Scopes = []string{"root"} }, true},
		{"missing identity", func(c *AuthClaims) { c.ClientID = "" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := claimsFor("a", "browser", "browser")
			tc.change(&c)
			_, e := authenticate(cfg, authRequest(signed(t, cfg, c, "a-key")), false)
			if (e != nil) != tc.deny {
				t.Fatalf("error=%v", e)
			}
		})
	}
	c := claimsFor("a", "browser", "browser")
	token := signed(t, cfg, c, "a-key")
	req := httptest.NewRequest("GET", "https://example/ws?token="+token, nil)
	if _, e := authenticate(cfg, req, true); e == nil {
		t.Fatal("query token enabled by default")
	}
	req.Header.Set("Sec-WebSocket-Protocol", "wsm.v1, wsm.jwt."+token)
	if _, e := authenticate(cfg, req, true); e != nil {
		t.Fatal(e)
	}
	if validateHelloAgainstClaims(HelloPayload{ClientType: "reader", ClientID: "browser", ReaderID: "r"}, &c) == nil {
		t.Fatal("role escalation")
	}
	key := cfg.Security.Keys["a-key"]
	key.Disabled = true
	cfg.Security.Keys["a-key"] = key
	if _, e := authenticate(cfg, authRequest(token), false); e == nil {
		t.Fatal("revoked key accepted")
	}
}

type fixture struct {
	s      *Server
	http   *httptest.Server
	cfg    *config.Config
	dialer *websocket.Dialer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	cfg := testConfig(t)
	s := New(cfg)
	ts := httptest.NewTLSServer(s.routes())
	t.Cleanup(func() { s.Close(); ts.Close() })
	return &fixture{s, ts, cfg, &websocket.Dialer{TLSClientConfig: ts.Client().Transport.(*http.Transport).TLSClientConfig, HandshakeTimeout: time.Second}}
}
func (f *fixture) connect(t *testing.T, tenant, role, id string) (*websocket.Conn, string) {
	t.Helper()
	c := claimsFor(tenant, role, id)
	header := http.Header{"Authorization": []string{"Bearer " + signed(t, f.cfg, c, tenant+"-key")}}
	ws, _, e := f.dialer.Dial("wss"+strings.TrimPrefix(f.http.URL, "https")+"/ws", header)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { ws.Close() })
	send(t, ws, Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": role, "client_id": id, "reader_id": c.ReaderID}})
	ack := readType(t, ws, "hello_ack")
	return ws, ack.Payload["connection_id"].(string)
}
func send(t *testing.T, ws *websocket.Conn, m Envelope) {
	t.Helper()
	if e := ws.WriteJSON(m); e != nil {
		t.Fatal(e)
	}
}
func readType(t *testing.T, ws *websocket.Conn, typ string) Envelope {
	t.Helper()
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var m Envelope
		if e := ws.ReadJSON(&m); e != nil {
			t.Fatal(e)
		}
		if m.Type == typ {
			return m
		}
	}
}
func command(id string, target *Target) Envelope {
	return Envelope{Type: "command", RequestID: id, Target: target, Payload: map[string]interface{}{"command": "reader.status"}}
}
func TestWSSRoutingAndHTTPIsolation(t *testing.T) {
	f := newFixture(t)
	a, aid := f.connect(t, "a", "browser", "browser-a")
	reader, _ := f.connect(t, "a", "reader", "reader-1")
	foreign, fid := f.connect(t, "b", "reader", "reader-1")
	send(t, a, command("one", &Target{Mode: "reader", ReaderID: "reader-1"}))
	m := readType(t, reader, "command")
	if m.Payload["sender_connection_id"] != aid {
		t.Fatal("missing sender")
	}
	send(t, reader, Envelope{Type: "reply", CorrelationID: "one", Target: &Target{Mode: "connection", ConnectionID: aid}, Payload: map[string]interface{}{"ok": true}})
	if readType(t, a, "reply").CorrelationID != "one" {
		t.Fatal("correlation")
	}
	send(t, a, command("cross", &Target{Mode: "connection", ConnectionID: fid}))
	ack := readType(t, a, "command_ack")
	if ack.Payload["recipients"].(float64) != 0 {
		t.Fatal("cross tenant route")
	}
	send(t, a, Envelope{Type: "subscribe", Payload: map[string]interface{}{"topic": "results:reader-1"}})
	readType(t, a, "subscribe_ack")
	send(t, reader, Envelope{Type: "event", Target: &Target{Mode: "topic", Topic: "results:reader-1"}, Payload: map[string]interface{}{"event": "result_available"}})
	readType(t, a, "event")
	// A ping is a barrier: any foreign command/presence would precede its pong.
	send(t, foreign, Envelope{Type: "ping"})
	var fm Envelope
	foreign.SetReadDeadline(time.Now().Add(time.Second))
	for {
		if e := foreign.ReadJSON(&fm); e != nil {
			t.Fatal(e)
		}
		if fm.Type == "presence" && fm.Payload["connection_id"] == fid {
			continue
		}
		if fm.Type != "pong" {
			t.Fatalf("tenant leaked: %+v", fm)
		}
		break
	}
	req, _ := http.NewRequest("GET", f.http.URL+"/api/connections", nil)
	req.Header = authRequest(signed(t, f.cfg, claimsFor("a", "browser", "browser-a"), "a-key")).Header
	resp, e := f.http.Client().Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	var body struct{ Connections []ConnectionInfo }
	json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != 200 || len(body.Connections) != 2 {
		t.Fatal("diagnostics", resp.StatusCode, body)
	}
	for _, c := range body.Connections {
		if c.TenantID != "a" {
			t.Fatal("diagnostic leak")
		}
	}
	for path, want := range map[string]int{"/api/test-token": 404, "/test/": 404, "/api/connections": 401, "/api/debug/state": 401, "/healthz": 200} {
		r, e := f.http.Client().Get(f.http.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		r.Body.Close()
		if r.StatusCode != want {
			t.Errorf("%s: %d", path, r.StatusCode)
		}
	}
}
func TestBrowserSubprotocolOriginAndHello(t *testing.T) {
	f := newFixture(t)
	token := signed(t, f.cfg, claimsFor("a", "browser", "browser"), "a-key")
	url := "wss" + strings.TrimPrefix(f.http.URL, "https") + "/ws"
	d := *f.dialer
	d.Subprotocols = []string{"wsm.v1", "wsm.jwt." + token}
	ws, resp, e := d.Dial(url, http.Header{"Origin": []string{"https://evil.example"}})
	if e == nil {
		ws.Close()
		t.Fatal("origin accepted")
	}
	if resp.StatusCode != 403 {
		t.Fatal(resp.StatusCode)
	}
	resp.Body.Close()
	ws, _, e = d.Dial(url, http.Header{"Origin": []string{"https://a.example"}})
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	if ws.Subprotocol() != "wsm.v1" {
		t.Fatal("JWT subprotocol echoed or missing")
	}
	send(t, ws, Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "reader", "client_id": "browser", "reader_id": "reader-1"}})
	ws.SetReadDeadline(time.Now().Add(time.Second))
	_, _, e = ws.ReadMessage()
	if !websocket.IsCloseError(e, websocket.ClosePolicyViolation) {
		t.Fatalf("role escalation %v", e)
	}
}
func TestAuthorization(t *testing.T) {
	r := claimsFor("a", "reader", "r")
	for _, m := range []Envelope{command("x", &Target{Mode: "all"}), {Type: "event", Target: &Target{Mode: "topic", Topic: "results:other"}}, {Type: "reply", CorrelationID: "x", Target: &Target{Mode: "all"}}, {Type: "command", RequestID: "x"}} {
		if authorizeMessage(&r, m) == nil {
			t.Fatalf("allowed %+v", m)
		}
	}
	b := claimsFor("a", "browser", "b")
	b.Scopes = []string{"route:command"}
	if authorizeMessage(&b, command("x", &Target{Mode: "reader", ReaderID: "r"})) != nil {
		t.Fatal("single reader denied")
	}
	if authorizeMessage(&b, command("x", &Target{Mode: "all"})) == nil {
		t.Fatal("broadcast scope bypass")
	}
}
func TestExpiryReloadAndHelloTimeout(t *testing.T) {
	f := newFixture(t)
	f.cfg.Server.HelloTimeoutMS = 40
	tokenClaims := claimsFor("a", "browser", "short")
	tokenClaims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(2 * time.Second))
	url := "wss" + strings.TrimPrefix(f.http.URL, "https") + "/ws"
	ws, _, e := f.dialer.Dial(url, authRequest(signed(t, f.cfg, tokenClaims, "a-key")).Header)
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	ws.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, e = ws.ReadMessage(); e == nil {
		t.Fatal("hello timeout missing")
	}
	ws, _, e = f.dialer.Dial(url, authRequest(signed(t, f.cfg, tokenClaims, "a-key")).Header)
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	send(t, ws, Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "browser", "client_id": "short"}})
	readType(t, ws, "hello_ack")
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, _, e = ws.ReadMessage()
		if e != nil {
			break
		}
	}
	if ne, ok := e.(net.Error); ok && ne.Timeout() {
		t.Fatal("token expiry did not close")
	}
	active, _ := f.connect(t, "a", "browser", "active")
	next := testConfig(t)
	next.Server = f.cfg.Server
	k := next.Security.Keys["a-key"]
	k.Disabled = true
	next.Security.Keys["a-key"] = k
	if e = f.s.Reload(next); e != nil {
		t.Fatal(e)
	}
	active.SetReadDeadline(time.Now().Add(time.Second))
	for {
		_, _, e = active.ReadMessage()
		if e != nil {
			break
		}
	}
	if ne, ok := e.(net.Error); ok && ne.Timeout() {
		t.Fatal("reload did not close")
	}
	ws, resp, e := f.dialer.Dial(url, authRequest(signed(t, f.cfg, claimsFor("a", "browser", "revoked"), "a-key")).Header)
	if e == nil {
		ws.Close()
		t.Fatal("revoked accepted")
	}
	if resp.StatusCode != 401 {
		t.Fatal(resp.StatusCode)
	}
	resp.Body.Close()
}
func Test50RealWSSConnections(t *testing.T) {
	f := newFixture(t)
	sockets := []*websocket.Conn{}
	for i := 0; i < 50; i++ {
		ws, _ := f.connect(t, "a", "browser", fmt.Sprintf("browser-%d", i))
		sockets = append(sockets, ws)
	}
	send(t, sockets[0], command("broadcast", &Target{Mode: "all"}))
	var wg sync.WaitGroup
	for _, ws := range sockets {
		wg.Add(1)
		go func(ws *websocket.Conn) {
			defer wg.Done()
			ws.SetReadDeadline(time.Now().Add(5 * time.Second))
			for {
				var m Envelope
				if e := ws.ReadJSON(&m); e != nil {
					t.Error(e)
					return
				}
				if m.Type == "command" {
					if m.RequestID != "broadcast" {
						t.Error("wrong command")
					}
					return
				}
			}
		}(ws)
	}
	wg.Wait()
}
func certificateFiles(t *testing.T) (string, string, *x509.CertPool) {
	t.Helper()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	kb, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	cp := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	os.WriteFile(certPath, cp, 0600)
	os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}), 0600)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(cp)
	return certPath, keyPath, pool
}
func TestExternalTLSCertificateAndShutdown(t *testing.T) {
	cfg := testConfig(t)
	cert, key, pool := certificateFiles(t)
	cfg.Server.TLS = config.TLS{CertFile: cert, KeyFile: key}
	if e := cfg.Prepare(""); e != nil {
		t.Fatal(e)
	}
	s := New(cfg)
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &http.Server{Handler: s.routes(), TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return s.current().Certificate, nil }}}
	done := make(chan error, 1)
	go func() { done <- s.serve(ctx, srv, ln) }()
	dialer := &websocket.Dialer{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	ws, _, e := dialer.Dial("wss://"+ln.Addr().String()+"/ws", authRequest(signed(t, cfg, claimsFor("a", "browser", "tls"), "a-key")).Header)
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	send(t, ws, Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "browser", "client_id": "tls"}})
	readType(t, ws, "hello_ack")
	// Renew certificate on the same listener; a fresh client trusts only the new CA.
	cert2, key2, pool2 := certificateFiles(t)
	next := testConfig(t)
	next.Server = cfg.Server
	next.Server.TLS = config.TLS{CertFile: cert2, KeyFile: key2}
	if e = next.Prepare(""); e != nil {
		t.Fatal(e)
	}
	if e = s.Reload(next); e != nil {
		t.Fatal(e)
	}
	dialer.TLSClientConfig = &tls.Config{RootCAs: pool2, MinVersion: tls.VersionTLS12}
	ws, _, e = dialer.Dial("wss://"+ln.Addr().String()+"/ws", authRequest(signed(t, next, claimsFor("a", "browser", "tls-new"), "a-key")).Header)
	if e != nil {
		t.Fatal("renewed certificate", e)
	}
	defer ws.Close()
	send(t, ws, Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "browser", "client_id": "tls-new"}})
	readType(t, ws, "hello_ack")
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown stalled")
	}
	ws.SetReadDeadline(time.Now().Add(time.Second))
	for {
		_, _, e = ws.ReadMessage()
		if e != nil {
			break
		}
	}
	if ne, ok := e.(net.Error); ok && ne.Timeout() {
		t.Fatal("shutdown left WebSocket open")
	}
}

func TestConnectionLimitRateAndRepeatedHello(t *testing.T) {
	f := newFixture(t)
	f.cfg.Server.MaxConnectionsPerTenant = 1
	f.cfg.Server.MessagesPerSecond = 1
	f.cfg.Server.MessageBurst = 3
	a, _ := f.connect(t, "a", "browser", "a1")
	url := "wss" + strings.TrimPrefix(f.http.URL, "https") + "/ws"
	ws, r, e := f.dialer.Dial(url, authRequest(signed(t, f.cfg, claimsFor("a", "browser", "a2"), "a-key")).Header)
	if e == nil {
		ws.Close()
		t.Fatal("tenant limit bypass")
	}
	if r.StatusCode != 503 {
		t.Fatal(r.StatusCode)
	}
	r.Body.Close()
	b, _ := f.connect(t, "b", "browser", "b")
	send(t, b, Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "browser", "client_id": "b"}})
	b.SetReadDeadline(time.Now().Add(time.Second))
	for {
		_, _, e = b.ReadMessage()
		if e != nil {
			break
		}
	}
	if !websocket.IsCloseError(e, websocket.ClosePolicyViolation) {
		t.Fatal("repeated hello", e)
	}
	for i := 0; i < 5; i++ {
		_ = a.WriteJSON(Envelope{Type: "ping"})
	}
	a.SetReadDeadline(time.Now().Add(time.Second))
	for {
		_, _, e = a.ReadMessage()
		if e != nil {
			break
		}
	}
	if !websocket.IsCloseError(e, websocket.ClosePolicyViolation) {
		t.Fatal("rate limit", e)
	}
}
func TestRejectInvalidReloadRetainsConfig(t *testing.T) {
	f := newFixture(t)
	a, _ := f.connect(t, "a", "browser", "a")
	next := testConfig(t)
	next.Server.Port++
	if f.s.Reload(next) == nil {
		t.Fatal("listener change accepted")
	}
	if f.s.current() != f.cfg {
		t.Fatal("failed reload replaced config")
	}
	send(t, a, Envelope{Type: "ping"})
	readType(t, a, "pong")
}
func TestRejectWrongAlgorithmUnknownKeyAndScopeEscalation(t *testing.T) {
	cfg := testConfig(t)
	c := claimsFor("a", "browser", "b")
	token := jwt.NewWithClaims(jwt.SigningMethodHS384, c)
	token.Header["kid"] = "a-key"
	raw, _ := token.SignedString([]byte(cfg.Security.Keys["a-key"].Secret))
	if _, e := authenticate(cfg, authRequest(raw), false); e == nil {
		t.Fatal("HS384 accepted")
	}
	token = jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	token.Header["kid"] = "missing"
	raw, _ = token.SignedString([]byte(cfg.Security.Keys["a-key"].Secret))
	if _, e := authenticate(cfg, authRequest(raw), false); e == nil {
		t.Fatal("unknown kid accepted")
	}
	token.Header["kid"] = "a-key"
	raw, _ = token.SignedString([]byte("wrong-secret"))
	if _, e := authenticate(cfg, authRequest(raw), false); e == nil {
		t.Fatal("wrong secret accepted")
	}
	k := cfg.Security.Keys["a-key"]
	k.Scopes = []string{"server:status"}
	cfg.Security.Keys["a-key"] = k
	if _, e := authenticate(cfg, authRequest(signed(t, cfg, c, "a-key")), false); e == nil {
		t.Fatal("scope escalation")
	}
}

func TestEquipmentStatusAndScopedControlTickets(t *testing.T) {
	f := newFixture(t)
	key := f.cfg.Security.Keys["a-key"]
	key.Scopes = append(append([]string(nil), key.Scopes...), "api:invoke", "api:admin")
	f.cfg.Security.Keys["a-key"] = key
	reader, _ := f.connect(t, "a", "reader", "device-42")
	parent := claimsFor("a", "browser", "operator")
	parent.Scopes = append(parent.Scopes, "api:invoke", "api:admin")
	result, e := f.s.issueControlTicket(f.cfg, &parent, "device-42")
	if e != nil {
		t.Fatal(e)
	}
	ticket := result["ticket"].(string)
	d := *f.dialer
	d.Subprotocols = []string{"wsm.v1", "wsm.ticket." + ticket}
	ws, _, e := d.Dial("wss"+strings.TrimPrefix(f.http.URL, "https")+"/ws", http.Header{"Origin": []string{f.http.URL}})
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	send(t, ws, Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "browser", "client_id": "control"}})
	readType(t, ws, "hello_ack")
	req := command("api-1", &Target{Mode: "equipment", EquipmentID: "device-42"})
	req.Payload = map[string]interface{}{"command": "api.request", "args": map[string]interface{}{"method": "GET", "path": "/api/status"}, "sender_subject": "spoof", "sender_scopes": []string{"root"}}
	send(t, ws, req)
	received := readType(t, reader, "command")
	if received.Payload["sender_subject"] != "operator" {
		t.Fatal("untrusted subject forwarded")
	}
	req.RequestID = "escape"
	req.Target.EquipmentID = "other-device"
	send(t, ws, req)
	readType(t, ws, "error")
	replay, resp, e := d.Dial("wss"+strings.TrimPrefix(f.http.URL, "https")+"/ws", http.Header{"Origin": []string{f.http.URL}})
	if e == nil {
		replay.Close()
		t.Fatal("ticket replay")
	}
	if resp.StatusCode != 401 {
		t.Fatal(resp.StatusCode)
	}
	resp.Body.Close()
	if f.s.equipmentStatus("a", "device-42")["online"] != true || f.s.equipmentStatus("b", "device-42")["online"] != false {
		t.Fatal("equipment tenant isolation")
	}
	payload, e := f.s.handleServerCommand(context.Background(), f.cfg.Tenants["a"], &Connection{ConnectionInfo: ConnectionInfo{TenantID: "a"}}, Envelope{Payload: map[string]interface{}{"command": "equipment.status", "args": map[string]interface{}{"equipment_ids": []interface{}{"device-42", "offline-1"}}}})
	if e != nil || len(payload["equipment"].([]map[string]interface{})) != 2 {
		t.Fatal(payload, e)
	}
}
