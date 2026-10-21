package siuidata_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
	appruntime "wisemed-labreaders/readersv3/core/runtime"
	"wisemed-labreaders/readersv3/modules/builtin"
)

func TestRuntimeOverHTTPS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := `reader:
  id: wisemed-siui-data
analyzer:
  protocol: siui-data
  comm_type: utility
modules:
  local-http:
    enabled: true
  siui-data:
    snapshot: ./snapshot.json
    jwt_secret_env: SIUI_DATA_INTEGRATION_SECRET
`
	if e := os.WriteFile(path, []byte(raw), 0600); e != nil {
		t.Fatal(e)
	}
	secret := strings.Repeat("S", 40)
	t.Setenv("SIUI_DATA_INTEGRATION_SECRET", secret)
	cfg, e := config.Load(path)
	if e != nil {
		t.Fatal(e)
	}
	registry := module.NewRegistry()
	builtin.RegisterAll(registry)
	app := appruntime.New(cfg, log.New(io.Discard, "", 0), registry)
	if e = app.InitModules([]string{"local-http", "wisemed-api", "siui-data"}); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewTLSServer(app.Mux())
	defer server.Close()
	claims := jwt.MapClaims{"sub": "integration", "iss": "wisemed-siui-data", "aud": "siui-data-api", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(), "admin": true}
	token, e := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if e != nil {
		t.Fatal(e)
	}
	call := func(method, path, body string, auth bool) (int, []byte) {
		t.Helper()
		r, _ := http.NewRequest(method, server.URL+path, bytes.NewBufferString(body))
		if auth {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		response, e := server.Client().Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		return response.StatusCode, data
	}
	if status, _ := call("GET", "/api/siu-data/status", "", true); status != 404 {
		t.Fatalf("former API prefix still registered: %d", status)
	}
	if status, _ := call("GET", "/api/siui-data/status", "", false); status != 401 {
		t.Fatal("anonymous API exposed")
	}
	if status, _ := call("GET", "/api/siui-data/services/001", "", true); status != 503 {
		t.Fatal("uninitialized catalogue served")
	}
	if status, data := call("GET", "/api/siui-data/settings", "", true); status != 200 || !json.Valid(data) {
		t.Fatalf("setup unavailable %d %s", status, data)
	}
}
