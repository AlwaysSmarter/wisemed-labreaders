package ws

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"wisemed-labreaders/readersv3/core/module"
)

type secretRuntime struct {
	module.Runtime
	path     string
	settings map[string]interface{}
}

func (r secretRuntime) ConfigPath() string                           { return r.path }
func (r secretRuntime) ReaderID() string                             { return "reader-test" }
func (r secretRuntime) ModuleSettings(string) map[string]interface{} { return r.settings }
func (r secretRuntime) RegisterService(string, interface{})          {}
func (r secretRuntime) ResolvePath(path string) string {
	return filepath.Join(filepath.Dir(r.path), path)
}

type secretEquipmentAPI struct{ secret string }

func (a secretEquipmentAPI) Settings() map[string]string {
	return map[string]string{"api_key_echipament": a.secret}
}
func (secretEquipmentAPI) EnsureEquipmentInitialized() (map[string]interface{}, error) {
	return nil, nil
}

func secretModule(t *testing.T) *Module {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("reader:\n  id: preserve-reader\nmodules:\n  other:\n    preserve: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m := &Module{}
	if err := m.Init(secretRuntime{path: path, settings: map[string]interface{}{"auth_mode": "device_key", "tenant_id": "tenant", "issuer": "issuer", "key_id": "key"}}); err != nil {
		t.Fatal(err)
	}
	return m
}
func TestDeviceSecretPersistsWithoutDisclosureAndBlankPreserves(t *testing.T) {
	m := secretModule(t)
	secret := strings.Repeat("private-key-", 4)
	for _, input := range []string{secret, "", "   "} {
		if err := m.SaveSettings(map[string]interface{}{"device_secret": input}); err != nil {
			t.Fatal(err)
		}
		if m.settingsSnapshot()["device_secret"] != secret {
			t.Fatal("blank password erased stored key")
		}
	}
	data, err := os.ReadFile(m.rt.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	modules := cfg["modules"].(map[string]interface{})
	if modules["wisemed-ws"].(map[string]interface{})["device_secret"] != secret || modules["other"].(map[string]interface{})["preserve"] != true || cfg["reader"].(map[string]interface{})["id"] != "preserve-reader" {
		t.Fatal("configuration not preserved")
	}
	m.record("out", Envelope{Type: "command", Payload: map[string]interface{}{"command": "api.request", "args": map[string]interface{}{"device_secret": secret}}})
	for name, value := range map[string]interface{}{"settings": m.Settings(), "status": m.Status(), "trace": m.Trace()} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), secret) {
			t.Fatalf("%s leaked key", name)
		}
	}
	public := m.Settings()
	if _, exists := public["device_secret"]; exists || public["device_secret_configured"] != true {
		t.Fatal("public settings should expose only configured flag")
	}
	before := string(data)
	for _, invalid := range []interface{}{"too-short", 123, true} {
		if err := m.SaveSettings(map[string]interface{}{"device_secret": invalid}); err == nil {
			t.Fatal("invalid secret accepted")
		}
		data, err = os.ReadFile(m.rt.ConfigPath())
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != before || m.settingsSnapshot()["device_secret"] != secret {
			t.Fatal("invalid save modified config")
		}
	}
}
func TestDeviceSecretJWTPrecedence(t *testing.T) {
	m := secretModule(t)
	explicit, file, fallback := strings.Repeat("explicit", 5), strings.Repeat("file-key", 5), strings.Repeat("api-key-", 5)
	if err := os.WriteFile(m.rt.ResolvePath("secret.key"), []byte(file), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, secret, path, want string }{
		{"explicit", explicit, "missing.key", explicit},
		{"file", "", "secret.key", file},
		{"legacy API", "", "", fallback},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := m.settingsSnapshot()
			settings["device_secret"], settings["secret_file"] = tc.secret, tc.path
			token, err := m.accessToken(context.Background(), settings, secretEquipmentAPI{fallback}, "42")
			if err != nil {
				t.Fatal(err)
			}
			parts := strings.Split(token, ".")
			if len(parts) != 3 {
				t.Fatal("invalid JWT")
			}
			mac := hmac.New(sha256.New, []byte(tc.want))
			mac.Write([]byte(parts[0] + "." + parts[1]))
			if parts[2] != base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) {
				t.Fatal("wrong signing key precedence")
			}
		})
	}
	settings := m.settingsSnapshot()
	settings["device_secret"] = "short"
	settings["secret_file"] = "secret.key"
	if _, err := m.accessToken(context.Background(), settings, secretEquipmentAPI{fallback}, "42"); err == nil {
		t.Fatal("invalid explicit key silently fell back to another key")
	}
}
