package runner

import (
	"testing"

	"wisemed-labreaders/readersv3/core/config"
)

func TestSigningPadLegacyListenerUsesSharedHTTP(t *testing.T) {
	cfg := &config.Config{
		Reader:   config.ReaderConfig{ID: "custom-installation"},
		Analyzer: config.AnalyzerConfig{Protocol: "signing-pad"},
		Modules: map[string]map[string]interface{}{
			"signing-pad": {"address": "127.0.0.1:22222", "tls": true},
		},
	}
	cfg.ApplyDefaults()
	normalizeLegacyConfig(cfg)
	if cfg.ModuleSettings("signing-pad")["shared_http"] != true || cfg.LocalHTTP.Address != "127.0.0.1:22222" || !cfg.LocalHTTP.TLS {
		t.Fatal("legacy listener not migrated")
	}
	// Once migrated, the common settings page controls the listener address.
	cfg.LocalHTTP.Address = "127.0.0.1:33333"
	normalizeLegacyConfig(cfg)
	if cfg.LocalHTTP.Address != "127.0.0.1:33333" {
		t.Fatal("legacy signing-pad settings overwrote shared HTTP settings")
	}
}

func TestBootstrapPreservesWSSAuthAndUpdatedURL(t *testing.T) {
	cfg := config.Default()
	cfg.Modules["wisemed-ws"]["tenant_id"] = "tenant-custom"
	cfg.Modules["wisemed-ws"]["token_file"] = "./private/token"
	// Explicit bootstrap edits should update the legacy mirror and runtime module.
	cfg.WiseMedWS.URL = "wss://configured.example/ws"
	cfg.WiseMedWS.Enabled = true
	syncModuleMirrors(cfg)
	cfg.ApplyDefaults()
	ws := cfg.ModuleSettings("wisemed-ws")
	if ws["url"] != cfg.WiseMedWS.URL || ws["enabled"] != true || ws["tenant_id"] != "tenant-custom" || ws["token_file"] != "./private/token" {
		t.Fatal("bootstrap lost WSS config")
	}
}
