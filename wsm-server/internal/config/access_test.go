package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAccessRegistrySurvivesFullLoad(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "master.key"), []byte(strings.Repeat("m", 40)), 0600); err != nil {
		t.Fatal(err)
	}
	source := `server:
  address: 127.0.0.1
  allow_insecure_loopback: true
security:
  device_keys_file: registry.yaml
  keys:
    master:
      tenant_id: a
      secret_file: master.key
      roles: [browser]
      scopes: ["route:command"]
tenants:
  a:
    issuer: old-issuer
    allowed_origins: ["https://old.example"]
    wisemed:
      base_url: https://old.example/api
      api_key_file: intentionally-missing.key
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	issuer, baseURL, apiKey := "managed-issuer", "https://managed.example/api", strings.Repeat("api-", 10)
	browserSecret := strings.Repeat("b", 40)
	registry := Registry{Devices: map[string]Device{}, Browsers: map[string]Browser{"web": {TenantID: "a", Label: "web", Origin: "https://web.example", Secret: browserSecret, Scopes: BrowserScopes()}}, Origins: map[string][]string{"a": {}}, Tenants: map[string]TenantSettings{"a": {Issuer: &issuer, BaseURL: &baseURL, APIKey: &apiKey}}}
	if err := WriteRegistry(filepath.Join(dir, "registry.yaml"), registry); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Tenants["a"].Issuer != issuer || c.Tenants["a"].WiseMed.APIKey != apiKey || c.Tenants["a"].WiseMed.BaseURL != baseURL || len(c.Tenants["a"].AllowedOrigins) != 0 {
		t.Fatal("tenant overrides lost")
	}
	if c.Security.Keys["web"].Secret != browserSecret || !slices.Contains(c.Security.Keys["web"].Roles, "browser") {
		t.Fatal("generated browser unavailable after restart")
	}
	if err := WriteDevices(c.Security.DeviceKeysFile, map[string]Device{}); err != nil {
		t.Fatal(err)
	}
	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if again.Security.Keys["web"].Secret != browserSecret || again.Tenants["a"].Issuer != issuer {
		t.Fatal("device operation destroyed access settings")
	}
	info, err := os.Stat(c.Security.DeviceKeysFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("registry permissions")
	}
}

func TestAccessValidationAndImmutableCandidate(t *testing.T) {
	original := &Config{Tenants: map[string]Tenant{"a": {Issuer: "a", AllowedOrigins: []string{"https://original.example"}}, "b": {Issuer: "b"}}, Security: Security{Keys: map[string]Key{"static": {TenantID: "a", Secret: strings.Repeat("s", 40), Roles: []string{"browser"}}}}}
	browser := Browser{TenantID: "a", Label: "web", Origin: "https://web.example", Secret: strings.Repeat("s", 40), Scopes: BrowserScopes()}
	registry := Registry{Browsers: map[string]Browser{"web": browser}, Origins: map[string][]string{"a": {"https://web.example"}}}
	next, err := original.WithAccess(registry)
	if err != nil {
		t.Fatal(err)
	}
	if original.Tenants["a"].AllowedOrigins[0] != "https://original.example" || len(original.Security.Keys) != 1 {
		t.Fatal("candidate mutated active configuration")
	}
	if len(next.Security.Keys) != 2 {
		t.Fatal("browser not added")
	}
	browser.TenantID = "b"
	registry.Browsers["web"] = browser
	if _, err := original.WithAccess(registry); err == nil {
		t.Fatal("cross tenant shared secret accepted")
	}
	registry.Browsers = map[string]Browser{"static": browser}
	if _, err := original.WithAccess(registry); err == nil {
		t.Fatal("configured key collision accepted")
	}
	for _, origin := range []string{"https://ok.example?", "https://ok.example#", "https://ok.example/", "http://ok.example", "https://user:pass@ok.example", "*"} {
		if ValidOrigin(origin) {
			t.Fatalf("invalid origin %q", origin)
		}
	}
}
