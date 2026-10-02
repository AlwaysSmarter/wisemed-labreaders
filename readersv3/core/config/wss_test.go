package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadTestConfig(t *testing.T, content string) *Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestWSSModuleSettingsRemainAuthoritative(t *testing.T) {
	cfg := loadTestConfig(t, `wisemed_ws:
  enabled: true
  url: wss://old.example/ws
  heartbeat_ms: 15000
modules:
  wisemed-ws:
    enabled: false
    url: wss://tenant.example/ws
    heartbeat_ms: 25000
    reconnect_delay_ms: 5000
    tenant_id: tenant-42
    auth_mode: token_file
    token_file: ./private/token.jwt
`)
	for i := 0; i < 3; i++ {
		cfg.ApplyDefaults()
		if cfg.WiseMedWS.Enabled || cfg.WiseMedWS.URL != "wss://tenant.example/ws" || cfg.WiseMedWS.HeartbeatMS != 25000 {
			t.Fatalf("module settings overwritten: %+v", cfg.WiseMedWS)
		}
		if cfg.WiseMedWS.ReconnectDelayMS != 30000 {
			t.Fatal("retry must be 30s")
		}
		ws := cfg.ModuleSettings("wisemed-ws")
		if ws["tenant_id"] != "tenant-42" || ws["auth_mode"] != "token_file" || ws["token_file"] != "./private/token.jwt" {
			t.Fatal("auth configuration lost")
		}
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		var err error
		cfg, err = Load(cfg.Path())
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestWSSLegacyTopLevelMirrored(t *testing.T) {
	cfg := loadTestConfig(t, `wisemed_ws:
  enabled: true
  url: wss://custom.example/ws
  heartbeat_ms: 22000
modules:
  wisemed-ws:
    tenant_id: own-tenant
`)
	ws := cfg.ModuleSettings("wisemed-ws")
	if ws["enabled"] != true || ws["url"] != "wss://custom.example/ws" || ws["heartbeat_ms"] != 22000 || ws["auth_mode"] != "device_key" || ws["tenant_id"] != "own-tenant" {
		t.Fatalf("bad mirror: %+v", ws)
	}
}

func TestInstallMergePreservesLegacyConnectionSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	current := `reader:
  id: signing-pad-utility
wisemed_ws:
  enabled: true
  url: wss://configured.example/ws
local_http:
  address: 0.0.0.0:19110
modules:
  signing-pad:
    address: 127.0.0.1:23456
    tls: true
`
	template := `modules:
  wisemed-ws:
    enabled: false
    url: wss://default.example/ws
    tenant_id: ""
  signing-pad:
    shared_http: true
`
	if err := os.WriteFile(path, []byte(current), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(InstallTemplatePath(path), []byte(template), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(path); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.WiseMedWS.Enabled || cfg.WiseMedWS.URL != "wss://configured.example/ws" {
		t.Fatalf("installer replaced WSS: %+v", cfg.WiseMedWS)
	}
	if cfg.LocalHTTP.Address != "127.0.0.1:23456" || !cfg.LocalHTTP.TLS || cfg.ModuleSettings("signing-pad")["shared_http"] != true {
		t.Fatal("signing-pad listener migration lost settings")
	}
	result, err := Ensure(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Merged {
		t.Fatal("migration not idempotent")
	}
}

func TestAllEquipmentAppsHaveWSSModulesAndInstallSettings(t *testing.T) {
	paths, err := filepath.Glob("../../apps/*/main.go")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, path := range paths {
		app := filepath.Base(filepath.Dir(path))
		if app == "update-server" {
			continue
		} // distribution infrastructure, not a tenant equipment client
		t.Run(app, func(t *testing.T) {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{`"local-http"`, `"wisemed-api"`, `"wisemed-ws"`} {
				if !strings.Contains(string(content), required) {
					t.Errorf("missing module %s", required)
				}
			}
			cfg, err := Load(filepath.Join(filepath.Dir(path), "deployments/config.install.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			ws := cfg.ModuleSettings("wisemed-ws")
			if ws["auth_mode"] != "device_key" || ws["reconnect_delay_ms"] != 30000 {
				t.Fatalf("bad WSS defaults: %+v", ws)
			}
			if app == "signing-pad-utility" && cfg.ModuleSettings("signing-pad")["shared_http"] != true {
				t.Fatal("competing signing-pad listener")
			}
		})
		count++
	}
	if count < 26 {
		t.Fatalf("unexpected coverage count: %d", count)
	}
}
