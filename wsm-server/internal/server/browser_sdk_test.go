package server

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestPublicBrowserSDK(t *testing.T) {
	s := New(testConfig(t))
	defer s.Close()
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, httptest.NewRequest("GET", "/sdk/wsm.js", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "function WSMServer(") || rec.Header().Get("Content-Type") != "text/javascript; charset=utf-8" {
		t.Fatal("SDK not served", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("public asset CORS missing")
	}
	rec = httptest.NewRecorder()
	s.routes().ServeHTTP(rec, httptest.NewRequest("GET", "/sdk/siui-bridge.js", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "function SIUIBridge(") {
		t.Fatal("SIUI SDK not served")
	}
	for _, path := range []string{"/sdk/wsm.test.cjs", "/sdk/../auth.go"} {
		rec := httptest.NewRecorder()
		s.routes().ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 404 {
			t.Fatal("unexpected exposed file", path)
		}
	}
}

func TestBrowserSDKRealWebSocket(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required for browser SDK wire test")
	}
	cfg := testConfig(t)
	key := cfg.Security.Keys["a-key"]
	key.Scopes = append(key.Scopes, "api:invoke")
	cfg.Security.Keys["a-key"] = key
	s := New(cfg)
	defer s.Close()
	ts := httptest.NewTLSServer(s.routes())
	defer ts.Close()
	reader := claimsFor("a", "service", "siui-1")
	readerJWT := signed(t, cfg, reader, "a-key")
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	dialer := websocket.Dialer{TLSClientConfig: ts.Client().Transport.(*http.Transport).TLSClientConfig}
	ws, _, err := dialer.Dial(wsURL, map[string][]string{"Authorization": {"Bearer " + readerJWT}})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if err = ws.WriteJSON(Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "siui-bridge", "client_id": "siui-1"}}); err != nil {
		t.Fatal(err)
	}
	var hello Envelope
	if err = ws.ReadJSON(&hello); err != nil || hello.Type != "hello_ack" {
		t.Fatal("hello", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var m Envelope
			if ws.ReadJSON(&m) != nil {
				return
			}
			if m.Type != "command" {
				continue
			}
			_ = ws.WriteJSON(Envelope{Type: "reply", CorrelationID: m.RequestID, Target: &Target{Mode: "connection", ConnectionID: asString(m.Payload["sender_connection_id"])}, Payload: map[string]interface{}{"kind": "api.response", "status": 200, "content_type": "application/json", "body": map[string]interface{}{"ok": true}}})
		}
	}()
	browser := claimsFor("a", "browser", "web-test")
	browser.Scopes = append(browser.Scopes, "api:invoke")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	caPath := filepath.Join(t.TempDir(), "test-ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ts.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	script, _ := filepath.Abs("sdk/wire-test.cjs")
	cmd := exec.CommandContext(ctx, node, script)
	cmd.Env = append(os.Environ(), "NODE_EXTRA_CA_CERTS="+caPath, "WSM_TEST_URL="+wsURL, "WSM_TEST_TOKEN="+signed(t, cfg, browser, "a-key"))
	if output, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("SDK wire test: %v\n%s", e, output)
	}
	ws.Close()
	<-done
}

func TestPHPBrowserTokenAcceptedByWSM(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP required for token interoperability test")
	}
	cfg := testConfig(t)
	secretPath := filepath.Join(t.TempDir(), "browser.key")
	if err = os.WriteFile(secretPath, []byte(cfg.Security.Keys["a-key"].Secret+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	helper, err := filepath.Abs("../../examples/browser/wsm-token.php")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"secret_file", "secret"} {
		t.Run(source, func(t *testing.T) {
			program := `require getenv('WSM_HELPER'); echo wsmBrowserToken(['kid'=>'a-key','tenant_id'=>'a','issuer'=>'issuer-a','audience'=>'wsm-server',getenv('WSM_SECRET_SOURCE')=>getenv('WSM_SECRET_VALUE'),'allowed_scopes'=>['server:status']], 'user-123', ['server:status']);`
			cmd := exec.Command(php, "-n", "-r", program)
			value := secretPath
			if source == "secret" {
				value = cfg.Security.Keys["a-key"].Secret
			}
			cmd.Env = append(os.Environ(), "WSM_HELPER="+helper, "WSM_SECRET_SOURCE="+source, "WSM_SECRET_VALUE="+value)
			output, err := cmd.Output()
			if err != nil {
				t.Fatal("PHP token generation failed", err)
			}
			req := httptest.NewRequest("GET", "/ws", nil)
			req.Header.Set("Sec-WebSocket-Protocol", "wsm.v1, wsm.jwt."+string(output))
			claims, err := authenticate(cfg, req, true)
			if err != nil {
				t.Fatal("PHP token rejected", err)
			}
			if claims.Role != "browser" || claims.TenantID != "a" || claims.Subject != "user-123" || !strings.HasPrefix(claims.ClientID, "web-") {
				t.Fatal("unexpected PHP token identity")
			}
		})
	}
}
