package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExternalSecretsAndValidation(t *testing.T) {
	dir := t.TempDir()
	secret := strings.Repeat("s", 40)
	if e := os.WriteFile(filepath.Join(dir, "secret"), []byte(secret+"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	base := `server:
  address: 127.0.0.1
  allow_insecure_loopback: true
security:
  keys:
    key-a:
      tenant_id: a
      secret_file: secret
      roles: [browser]
      scopes: ["route:command"]
tenants:
  a:
    issuer: issuer-a
`
	path := filepath.Join(dir, "config.yaml")
	load := func(s string) (*Config, error) {
		t.Helper()
		if e := os.WriteFile(path, []byte(s), 0600); e != nil {
			t.Fatal(e)
		}
		return Load(path)
	}
	c, e := load(base)
	if e != nil {
		t.Fatal(e)
	}
	if c.Security.Keys["key-a"].Secret != secret || c.Server.Port != 8443 {
		t.Fatal("secret/defaults")
	}
	proxy := strings.Replace(base, "127.0.0.1", "0.0.0.0", 1)
	proxy = strings.Replace(proxy, "allow_insecure_loopback: true", "allow_insecure_http: true", 1)
	if c, e := load(proxy); e != nil || c.Certificate != nil {
		t.Fatalf("explicit HTTP proxy mode: %v", e)
	}
	tests := []struct{ name, s string }{
		{"plaintext public", strings.Replace(base, "127.0.0.1", "0.0.0.0", 1)},
		{"HTTP flag false", strings.Replace(proxy, "allow_insecure_http: true", "allow_insecure_http: false", 1)},
		{"HTTP flag cannot hide incomplete TLS", strings.Replace(proxy, "allow_insecure_http: true", "allow_insecure_http: true\n  tls:\n    cert_file: missing.pem", 1)},
		{"unknown field", base + "unknown: true\n"},
		{"legacy config", "security:\n  accepted_keys:\n    old: secret\n"},
		{"bad scope", strings.Replace(base, "route:command", "everything", 1)},
		{"bad role", strings.Replace(base, "browser", "admin", 1)},
		{"unknown tenant", strings.Replace(base, "tenant_id: a", "tenant_id: b", 1)},
		{"two sources", strings.Replace(base, "secret_file: secret", "secret_file: secret\n      secret_env: SECRET", 1)},
		{"multi document", base + "---\nserver: {}\n"},
		{"bad timeout", strings.Replace(base, "address: 127.0.0.1", "address: 127.0.0.1\n  read_timeout_ms: -1", 1)},
		{"invalid origin", strings.Replace(base, "issuer: issuer-a", "issuer: issuer-a\n    allowed_origins: ['*']", 1)},
		{"insecure upstream", strings.Replace(base, "issuer: issuer-a", "issuer: issuer-a\n    wisemed:\n      base_url: http://example.com\n      api_key_ref: SECRET", 1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, e := load(tc.s); e == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
	// Reload resolves new file content rather than retaining previous secrets.
	rotated := strings.Repeat("r", 40)
	os.WriteFile(filepath.Join(dir, "secret"), []byte(rotated), 0600)
	c, e = load(base)
	if e != nil || c.Security.Keys["key-a"].Secret != rotated {
		t.Fatal("rotation", e)
	}
	os.WriteFile(filepath.Join(dir, "secret"), []byte("short"), 0600)
	if _, e = load(base); e == nil {
		t.Fatal("short key accepted")
	}
}
func TestTenantsCannotShareSecrets(t *testing.T) {
	t.Setenv("KEY", strings.Repeat("k", 40))
	c := Config{Server: Server{Address: "127.0.0.1", AllowInsecureLoopback: true}, Tenants: map[string]Tenant{"a": {Issuer: "a"}, "b": {Issuer: "b"}}, Security: Security{Keys: map[string]Key{"ka": {TenantID: "a", SecretEnv: "KEY", Roles: []string{"browser"}, Scopes: []string{"route:command"}}, "kb": {TenantID: "b", SecretEnv: "KEY", Roles: []string{"browser"}, Scopes: []string{"route:command"}}}}}
	if e := c.Prepare(t.TempDir()); e == nil {
		t.Fatal("shared tenant secret accepted")
	}
}
