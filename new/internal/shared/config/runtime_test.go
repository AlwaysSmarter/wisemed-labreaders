package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReaderAgentConfig(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "reader.yaml")
	err := os.WriteFile(p, []byte(`
reader:
  id: "r1"
  analyzer_code: "maglumi-800"
  api_key_ref: "WMR_READER_APIKEY"
webservice:
  ws_url: "ws://localhost:8090/ws/readers"
storage:
  local_db_path: "./reader.db"
control:
  reconnect_seconds: 2
  heartbeat_seconds: 3
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadReaderAgentConfig(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Reader.ID != "r1" {
		t.Fatalf("reader id mismatch")
	}
}
