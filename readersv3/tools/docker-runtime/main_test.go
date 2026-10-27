package main

import (
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wisemed-labreaders/readersv3/shared/appmeta"
)

func writeConfig(t *testing.T, address string, tls bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	text := fmt.Sprintf("analyzer:\n  protocol: siui\nlocal_http:\n  enabled: true\n  address: %s\n  tls: %t\n", address, tls)
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPreflightRejectsWrongAppAndPublicListener(t *testing.T) {
	t.Setenv("WISEMED_DOCKER_APP", "siui-bridge")
	if err := run("check-config", writeConfig(t, "0.0.0.0:19112", false)); err == nil {
		t.Fatal("accepted public listener")
	}
	path := writeConfig(t, "127.0.0.1:19112", false)
	before, _ := os.ReadFile(path)
	if err := run("check-config", path); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("preflight modified config")
	}
	t.Setenv("WISEMED_DOCKER_APP", "siui-bridge-history")
	if err := run("check-config", path); err == nil {
		t.Fatal("accepted mismatched protocol")
	}
}

func TestHealthValidatesVersionAndTLS(t *testing.T) {
	t.Setenv("WISEMED_DOCKER_APP", "siui-bridge")
	version := appmeta.CurrentVersion()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/session" {
			t.Error("unexpected health route")
		}
		fmt.Fprintf(w, `{"ok":true,"reader":{"protocol":"siui","app_version":%q}}`, version)
	}))
	defer server.Close()
	path := writeConfig(t, strings.TrimPrefix(server.URL, "https://"), true)
	if err := run("health", path); err == nil {
		t.Fatal("accepted untrusted TLS")
	}
	dir := filepath.Join(filepath.Dir(path), "tls")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(filepath.Join(dir, "wisemed-local-root-ca.pem"), cert, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run("health", path); err != nil {
		t.Fatal(err)
	}
	version = "wrong-version"
	if err := run("health", path); err == nil {
		t.Fatal("accepted wrong running version")
	}
}
