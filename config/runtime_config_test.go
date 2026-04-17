package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRuntimeConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "reader.yaml")

	err := os.WriteFile(cfgPath, []byte(`
reader:
  id: "reader-01"
  analyzer_type: "biochemistry"
wisemed:
  api_base_url: "https://api.wisemed.eu"
  ws_url: "wss://ws.wisemed.eu/reader"
  auth_url: "https://api.wisemed.eu/auth/token"
auth:
  client_id: "reader-01"
  secret_ref: "WM_READER_SECRET"
`), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadRuntimeConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Reader.ID != "reader-01" {
		t.Fatalf("reader id mismatch: %q", cfg.Reader.ID)
	}
	if cfg.Control.ReconnectSeconds <= 0 {
		t.Fatalf("expected default reconnect_seconds > 0")
	}
	if cfg.Control.HeartbeatSeconds <= 0 {
		t.Fatalf("expected default heartbeat_seconds > 0")
	}
}

func TestLoadRuntimeConfig_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "reader.yaml")

	err := os.WriteFile(cfgPath, []byte(`
reader:
  id: "reader-from-file"
wisemed:
  api_base_url: "https://api.from.file"
  ws_url: "wss://ws.from.file"
  auth_url: "https://auth.from.file"
auth:
  client_id: "client-from-file"
  secret_ref: "SECRET_FROM_FILE"
control:
  reconnect_seconds: 4
  heartbeat_seconds: 11
`), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("WMR_READER_ID", "reader-from-env")
	t.Setenv("WMR_AUTH_CLIENT_ID", "client-from-env")
	t.Setenv("WMR_CONTROL_HEARTBEAT_SECONDS", "33")

	cfg, err := LoadRuntimeConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Reader.ID != "reader-from-env" {
		t.Fatalf("expected env override for reader id, got %q", cfg.Reader.ID)
	}
	if cfg.Auth.ClientID != "client-from-env" {
		t.Fatalf("expected env override for client id, got %q", cfg.Auth.ClientID)
	}
	if cfg.Control.HeartbeatSeconds != 33 {
		t.Fatalf("expected env override for heartbeat_seconds, got %d", cfg.Control.HeartbeatSeconds)
	}
}

func TestRuntimeConfigValidate_MissingRequired(t *testing.T) {
	cfg := &RuntimeConfig{}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation error")
	}
}
