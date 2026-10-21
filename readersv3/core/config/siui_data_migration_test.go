package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSIUIDataConfigurationMigration(t *testing.T) {
	cfg := loadTestConfig(t, `analyzer:
  protocol: siu-data
modules:
  siu-data:
    postgres_host: db.example
    postgres_database: cnas
    postgres_password: retained-password
    jwt_secret_env: CUSTOM_GLOBAL_KEY
    client_jwt_secrets:
      "7991962": retained-client-key
  siui-data:
    postgres_host: new.example
`)
	cfg.EnabledModules = []string{"local-http", legacySIUIDataModule}
	for i := 0; i < 2; i++ {
		cfg.ApplyDefaults()
		settings := cfg.ModuleSettings(siuiDataModule)
		if cfg.Analyzer.Protocol != siuiDataModule || cfg.EnabledModules[1] != siuiDataModule {
			t.Fatal("protocol or enabled module was not migrated")
		}
		if _, exists := cfg.Modules[legacySIUIDataModule]; exists {
			t.Fatal("legacy settings remain")
		}
		if settings["postgres_host"] != "new.example" || settings["postgres_database"] != "cnas" || settings["postgres_password"] != "retained-password" || settings["jwt_secret_env"] != "CUSTOM_GLOBAL_KEY" {
			t.Fatal("operator settings were lost")
		}
		keys := settings["client_jwt_secrets"].(map[string]interface{})
		if keys["7991962"] != "retained-client-key" {
			t.Fatal("client key was lost")
		}
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(cfg.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), legacySIUIDataModule+":") || strings.Contains(string(raw), "protocol: "+legacySIUIDataModule+"\n") {
		t.Fatal("saved legacy identifiers")
	}
	info, err := os.Stat(cfg.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("configuration containing keys must be private")
	}
}

func TestSIUIDataInstallerMigratesBeforeAddingDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`analyzer:
  protocol: siu-data
modules:
  siu-data:
    postgres_host: existing.example
    postgres_database: cnas
    postgres_user: reader
    postgres_password: retained-password
    client_jwt_secrets:
      "7991962": retained-client-key
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(InstallTemplatePath(path), []byte(`analyzer:
  protocol: siui-data
modules:
  siui-data:
    postgres_host: localhost
    postgres_database: ""
    postgres_user: ""
    postgres_password: ""
    client_jwt_secrets: {}
    jwt_secret_env: WISEMED_SIUI_DATA_JWT_SECRET
`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Ensure(path)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Merged {
		t.Fatal("migration was not persisted")
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.ModuleSettings(siuiDataModule)
	if cfg.Analyzer.Protocol != siuiDataModule || s["postgres_host"] != "existing.example" || s["postgres_database"] != "cnas" || s["postgres_password"] != "retained-password" || s["jwt_secret_env"] != "WISEMED_SIUI_DATA_JWT_SECRET" {
		t.Fatal("installer defaults replaced operator configuration")
	}
	if s["client_jwt_secrets"].(map[string]interface{})["7991962"] != "retained-client-key" {
		t.Fatal("installer lost client keys")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("installer left private configuration readable")
	}
	result, err = Ensure(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Merged {
		t.Fatal("migration is not idempotent")
	}
}
