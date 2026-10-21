package server

import (
	"crypto/tls"
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

func TestPHPHTTPWrapperEndToEnd(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP unavailable")
	}
	if err := exec.Command(php, "-r", `exit(extension_loaded('curl') ? 0 : 1);`).Run(); err != nil {
		t.Skip("PHP ext-curl unavailable")
	}
	cfg := testConfig(t)
	key := cfg.Security.Keys["a-key"]
	key.Scopes = append(append([]string(nil), key.Scopes...), "api:invoke")
	cfg.Security.Keys["a-key"] = key
	s := New(cfg)
	ts := httptest.NewUnstartedServer(s.routes())
	// The handler must override an ordinary short write timeout during its wait.
	ts.Config.WriteTimeout = 250 * time.Millisecond
	// Exercise the TLS 1.2 path supported by production and older PHP cURL builds.
	ts.TLS = &tls.Config{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12}
	ts.StartTLS()
	t.Cleanup(func() { s.Close(); ts.Close() })
	f := &fixture{s: s, http: ts, cfg: cfg, dialer: &websocket.Dialer{TLSClientConfig: ts.Client().Transport.(*http.Transport).TLSClientConfig}}
	peer, _ := f.connect(t, "a", "reader", "php-test-reader")
	claims := claimsFor("a", "service", "php-backend")
	claims.Scopes = append(claims.Scopes, "api:invoke")
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ts.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	wrapper, err := filepath.Abs("../../examples/php/WSMHttpClient.php")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(php, "-d", "curl.cainfo="+caPath, "-r", `
require getenv('WSM_TEST_WRAPPER');
$wsm = new WSMHttpClient(getenv('WSM_TEST_URL'), getenv('WSM_TEST_JWT'), [
    'connectTimeout'=>500, 'requestTimeout'=>2000, 'totalTimeout'=>5000,
], getenv('WSM_TEST_CA'));
$client = $wsm->firstClient('reader');
$response = $wsm->callAPI($client, 'GET', '/api/status');
if ($response['status'] !== 422 || $response['body']['error'] !== 'fixture_error') exit(2);
echo 'PHP_HTTP_OK';
`)
	command.Env = append(os.Environ(), "WSM_TEST_WRAPPER="+wrapper, "WSM_TEST_URL="+ts.URL, "WSM_TEST_CA="+caPath, "WSM_TEST_JWT="+signed(t, cfg, claims, "a-key"))
	done := make(chan struct {
		output []byte
		err    error
	}, 1)
	go func() {
		out, err := command.CombinedOutput()
		done <- struct {
			output []byte
			err    error
		}{out, err}
	}()
	m := readType(t, peer, "command")
	if m.Payload["command"] != "api.request" || m.Payload["sender_subject"] != "php-backend" {
		t.Fatalf("incorrect forwarded command: %+v", m)
	}
	args := m.Payload["args"].(map[string]interface{})
	if args["path"] != "/api/status" || args["method"] != "GET" {
		t.Fatalf("incorrect args: %+v", args)
	}
	time.Sleep(350 * time.Millisecond)
	send(t, peer, Envelope{Type: "reply", CorrelationID: m.RequestID, Target: &Target{Mode: "connection", ConnectionID: asString(m.Payload["sender_connection_id"])}, Payload: map[string]interface{}{"kind": "api.response", "status": 422, "body": map[string]interface{}{"error": "fixture_error"}}})
	select {
	case result := <-done:
		if result.err != nil || !strings.Contains(string(result.output), "PHP_HTTP_OK") {
			t.Fatalf("PHP: %v: %s", result.err, result.output)
		}
	case <-time.After(6 * time.Second):
		_ = command.Process.Kill()
		t.Fatal("PHP did not finish")
	}
}
