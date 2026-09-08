package signingpad

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"wisemed-labreaders/readersv3/core/module"
)

type testRuntime struct {
	dir      string
	mux      *http.ServeMux
	settings map[string]interface{}
}

func (r *testRuntime) ConfigPath() string                           { return filepath.Join(r.dir, "config.yaml") }
func (r *testRuntime) ConfigDir() string                            { return r.dir }
func (r *testRuntime) ReaderID() string                             { return "esignature-server" }
func (r *testRuntime) Logf(string, ...interface{})                  {}
func (r *testRuntime) ModuleSettings(string) map[string]interface{} { return r.settings }
func (r *testRuntime) ResolvePath(s string) string                  { return filepath.Join(r.dir, s) }
func (r *testRuntime) AddMenu(...module.MenuEntry)                  {}
func (r *testRuntime) Handle(p string, h http.Handler)              { r.mux.Handle(p, h) }
func (r *testRuntime) Mux() *http.ServeMux                          { return r.mux }
func (r *testRuntime) RegisterService(string, interface{})          {}
func (r *testRuntime) Service(string) (interface{}, bool)           { return nil, false }

type fakePad struct{ closed chan struct{} }

func (p *fakePad) Count() (int32, error)    { return 1, nil }
func (p *fakePad) Start() error             { return nil }
func (p *fakePad) Retry() error             { return nil }
func (p *fakePad) Confirm() ([]byte, error) { return []byte("PNG bytes"), nil }
func (p *fakePad) Cancel() error            { return nil }
func (p *fakePad) Close()                   { close(p.closed) }

func TestWebSocketSessionOwnership(t *testing.T) {
	rt := &testRuntime{dir: t.TempDir(), mux: http.NewServeMux(), settings: map[string]interface{}{}}
	m := &Module{}
	if err := m.Init(rt); err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	closed2 := make(chan struct{})
	opens := 0
	m.driverFactory = func(string) (padDriver, error) {
		opens++
		if opens == 1 {
			return &fakePad{closed: closed}, nil
		}
		return &fakePad{closed: closed2}, nil
	}
	server := httptest.NewTLSServer(rt.mux)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	dialer := websocket.Dialer{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	defer server.Close()
	dial := func() *websocket.Conn {
		c, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	a, b := dial(), dial()
	send := func(c *websocket.Conn, action string) map[string]interface{} {
		t.Helper()
		c.SetReadDeadline(time.Now().Add(3 * time.Second))
		if err := c.WriteJSON(padCommand{ID: "42", Action: action}); err != nil {
			t.Fatal(err)
		}
		var result map[string]interface{}
		if err := c.ReadJSON(&result); err != nil {
			t.Fatal(err)
		}
		if result["id"] != "42" {
			t.Fatal(result)
		}
		return result
	}
	if r := send(a, "start"); r["ok"] != true {
		t.Fatal(r)
	}
	if r := send(b, "start"); r["ok"] != false {
		t.Fatal("second client acquired pad", r)
	}
	if r := send(a, "retry"); r["ok"] != true {
		t.Fatal(r)
	}
	if r := send(a, "confirm"); r["imageBase64"] != base64.StdEncoding.EncodeToString([]byte("PNG bytes")) {
		t.Fatal(r)
	}
	// Use a fresh fake channel for the next driver; Close has already run before response.

	if r := send(b, "start"); r["ok"] != true {
		t.Fatal(r)
	}
	b.Close()
	select {
	case <-closed2:
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect did not release pad")
	}
}
func TestOriginPolicy(t *testing.T) {
	m := &Module{rt: &testRuntime{settings: map[string]interface{}{"cors_allowed_origins": "https://trusted.example"}}}
	for _, tc := range []struct {
		origin string
		want   bool
	}{{"https://trusted.example", true}, {"http://localhost:19111", true}, {"https://evil.example", false}, {"null", false}} {
		req := httptest.NewRequest("GET", "http://localhost:19111/ws", nil)
		req.Header.Set("Origin", tc.origin)
		if got := m.originAllowed(req); got != tc.want {
			t.Errorf("origin %q: got %v", tc.origin, got)
		}
	}
}
