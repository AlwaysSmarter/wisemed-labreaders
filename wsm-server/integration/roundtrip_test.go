package integration

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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"wisemed-labreaders/readersv3/core/module"
	readerws "wisemed-labreaders/readersv3/modules/ws"
	"wisemed-labreaders/readersv3/shared/apibridge"
	"wisemed-labreaders/serverlast/wsm-server/internal/config"
	"wisemed-labreaders/serverlast/wsm-server/internal/server"
)

type api struct {
	secret      string
	initialized atomic.Bool
	calls       atomic.Int32
}

func (a *api) Settings() map[string]string {
	return map[string]string{"echipament_id": "42", "api_key_echipament": a.secret}
}
func (a *api) EnsureEquipmentInitialized() (map[string]interface{}, error) {
	a.initialized.Store(true)
	a.calls.Add(1)
	return map[string]interface{}{"echipament_id": "42"}, nil
}

type runtime struct {
	dir      string
	settings map[string]interface{}
	mux      *http.ServeMux
	mu       sync.RWMutex
	services map[string]interface{}
}

func (r *runtime) ConfigPath() string          { return filepath.Join(r.dir, "config.yaml") }
func (r *runtime) ConfigDir() string           { return r.dir }
func (r *runtime) ReaderID() string            { return "reader-one" }
func (r *runtime) Logf(string, ...interface{}) {}
func (r *runtime) ModuleSettings(id string) map[string]interface{} {
	if id == "wisemed-ws" {
		return r.settings
	}
	return map[string]interface{}{}
}
func (r *runtime) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(r.dir, path)
}
func (r *runtime) AddMenu(...module.MenuEntry)        {}
func (r *runtime) Handle(path string, h http.Handler) { r.mux.Handle(path, h) }
func (r *runtime) Mux() *http.ServeMux                { return r.mux }
func (r *runtime) RegisterService(n string, s interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.services[n] = s
}
func (r *runtime) Service(n string) (interface{}, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.services[n]
	return s, ok
}
func cert(t *testing.T, dir string) (string, string, *x509.CertPool) {
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(42), Subject: pkix.Name{CommonName: "localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &k.PublicKey, k)
	if e != nil {
		t.Fatal(e)
	}
	kb, _ := x509.MarshalPKCS8PrivateKey(k)
	cp := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	os.WriteFile(certPath, cp, 0600)
	os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}), 0600)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(cp)
	return certPath, keyPath, pool
}
func eventually(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition timeout")
}
func read(t *testing.T, ws *websocket.Conn, typ, correlation string) server.Envelope {
	t.Helper()
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var m server.Envelope
		if e := ws.ReadJSON(&m); e != nil {
			t.Fatal(e)
		}
		if m.Type == typ && (correlation == "" || m.CorrelationID == correlation) {
			return m
		}
	}
}
func TestProductionWSSReaderAPIAndReconnect(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, pool := cert(t, dir)
	secret := strings.Repeat("d", 40)
	t.Setenv("DEVICE_KEY", secret)
	t.Setenv("BROWSER_KEY", strings.Repeat("b", 40))
	probe, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()
	scopes := []string{"route:reply", "route:event", "route:command", "devices:debug", "connections:read", "server:status", "api:invoke", "api:admin"}
	cfg := &config.Config{Server: config.Server{Address: "127.0.0.1", Port: port, TLS: config.TLS{CertFile: certPath, KeyFile: keyPath}, MaxMessageBytes: 8 * 1024 * 1024}, Tenants: map[string]config.Tenant{"clinic": {Issuer: "issuer"}}, Security: config.Security{Keys: map[string]config.Key{"device": {TenantID: "clinic", SecretEnv: "DEVICE_KEY", Roles: []string{"reader"}, Scopes: scopes, ReaderID: "reader-one", EquipmentID: "42", Subject: "reader-one"}, "browser": {TenantID: "clinic", SecretEnv: "BROWSER_KEY", Roles: []string{"browser"}, Scopes: scopes}}}}
	if e = cfg.Prepare(dir); e != nil {
		t.Fatal(e)
	}
	s := server.New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverDone := make(chan error, 1)
	go func() { serverDone <- s.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case e := <-serverDone:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(6 * time.Second):
			t.Error("server did not stop")
		}
	})
	base := fmt.Sprintf("https://127.0.0.1:%d", port)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}, Timeout: time.Second}
	defer client.CloseIdleConnections()
	eventually(t, 3*time.Second, func() bool {
		r, e := client.Get(base + "/healthz")
		if e != nil {
			return false
		}
		r.Body.Close()
		return r.StatusCode == 200
	})
	api := &api{secret: secret}
	rt := &runtime{dir: dir, mux: http.NewServeMux(), services: map[string]interface{}{"wisemed-api": api}, settings: map[string]interface{}{"enabled": true, "url": "wss" + strings.TrimPrefix(base, "https") + "/ws", "key_id": "device", "tenant_id": "clinic", "issuer": "issuer", "ca_file": certPath, "scopes": strings.Join(scopes, ",")}}
	var mutations atomic.Int32
	rt.mux.HandleFunc("/api/example", func(w http.ResponseWriter, r *http.Request) {
		p, ok := apibridge.PrincipalFrom(r.Context())
		if !ok || p.Subject != "operator" || p.TenantID != "clinic" {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.Method == "POST" {
			mutations.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"value": mutations.Load(), "principal": p.Subject})
	})
	module := readerws.New().(*readerws.Module)
	if e = module.Init(rt); e != nil {
		t.Fatal(e)
	}
	readerDone := make(chan error, 1)
	go func() { readerDone <- module.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-readerDone:
		case <-time.After(5 * time.Second):
			t.Error("reader did not stop")
		}
	})
	eventually(t, 3*time.Second, module.Connected)
	if !api.initialized.Load() || module.Status()["equipment_id"] != "42" {
		t.Fatal("API before WSS identity missing")
	}
	now := time.Now()
	claims := server.AuthClaims{TenantID: "clinic", Role: "browser", ClientID: "ace", Scopes: scopes, RegisteredClaims: jwt.RegisteredClaims{Subject: "operator", Issuer: "issuer", Audience: jwt.ClaimStrings{"wsm-server"}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute))}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = "browser"
	jwtString, _ := token.SignedString([]byte(strings.Repeat("b", 40)))
	dialer := websocket.Dialer{TLSClientConfig: &tls.Config{RootCAs: pool}}
	browser, _, e := dialer.Dial("wss"+strings.TrimPrefix(base, "https")+"/ws", http.Header{"Authorization": []string{"Bearer " + jwtString}})
	if e != nil {
		t.Fatal(e)
	}
	defer browser.Close()
	browser.WriteJSON(server.Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "browser", "client_id": "ace"}})
	read(t, browser, "hello_ack", "")
	request := func(id, command string, args map[string]interface{}, target *server.Target) server.Envelope {
		t.Helper()
		if e := browser.WriteJSON(server.Envelope{Type: "command", RequestID: id, Target: target, Payload: map[string]interface{}{"command": command, "args": args}}); e != nil {
			t.Fatal(e)
		}
		return read(t, browser, "reply", id)
	}
	presence := request("presence", "equipment.status", map[string]interface{}{"equipment_id": "42"}, &server.Target{Mode: "server"})
	if presence.Payload["equipment"].([]interface{})[0].(map[string]interface{})["online"] != true {
		t.Fatal("equipment not online")
	}
	result := request("api", "api.request", map[string]interface{}{"method": "POST", "path": "/api/example", "body": map[string]interface{}{"x": 1}}, &server.Target{Mode: "equipment", EquipmentID: "42"})
	if result.Payload["status"] != float64(200) || mutations.Load() != 1 {
		t.Fatal(result)
	}
	// Click Open's ticket opens another socket; it can only call the selected device.
	ticket := request("ticket", "control.ticket", map[string]interface{}{"equipment_id": "42"}, &server.Target{Mode: "server"})
	ticketDialer := dialer
	ticketDialer.Subprotocols = []string{"wsm.v1", "wsm.ticket." + ticket.Payload["ticket"].(string)}
	control, _, e := ticketDialer.Dial("wss"+strings.TrimPrefix(base, "https")+"/ws", http.Header{"Origin": []string{base}})
	if e != nil {
		t.Fatal(e)
	}
	defer control.Close()
	control.WriteJSON(server.Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "browser", "client_id": "control"}})
	read(t, control, "hello_ack", "")
	control.WriteJSON(server.Envelope{Type: "command", RequestID: "control-api", Target: &server.Target{Mode: "equipment", EquipmentID: "42"}, Payload: map[string]interface{}{"command": "api.request", "args": map[string]interface{}{"method": "GET", "path": "/api/example"}}})
	reply := read(t, control, "reply", "control-api")
	if reply.Payload["status"] != float64(200) {
		t.Fatal(reply)
	}
	old := module.Status()["connection_id"]
	request("reconnect", "ws.reconnect", map[string]interface{}{}, &server.Target{Mode: "equipment", EquipmentID: "42"})
	eventually(t, 3*time.Second, func() bool { return module.Connected() && module.Status()["connection_id"] != old })
	module.Disconnect()
	eventually(t, time.Second, func() bool { return !module.Connected() })
	if module.Status()["phase"] != "paused" {
		t.Fatal(module.Status())
	}
	module.Reconnect()
	eventually(t, 3*time.Second, module.Connected)
	// A server reload simulates network loss: no manual retry; wait the actual 30s interval.
	if e = s.Reload(cfg); e != nil {
		t.Fatal(e)
	}
	eventually(t, 2*time.Second, func() bool { return !module.Connected() && module.Status()["retry_in_seconds"].(int) > 0 })
	remaining := module.Status()["retry_in_seconds"].(int)
	if remaining < 28 || remaining > 30 {
		t.Fatal("retry not 30 seconds", remaining)
	}
	eventually(t, 33*time.Second, module.Connected)
}
