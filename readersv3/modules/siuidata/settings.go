package siuidata

import (
	"encoding/json"
	"errors"
	"gopkg.in/yaml.v3"
	"net/http"
	"os"
	"path/filepath"

	"wisemed-labreaders/readersv3/core/config"
)

type settings struct {
	ClientJWTSecrets map[string]string `json:"client_jwt_secrets"`
	JWTSecretEnv     string            `json:"jwt_secret_env"`
	PGHost           string            `json:"postgres_host"`
	PGPort           int               `json:"postgres_port"`
	PGDatabase       string            `json:"postgres_database"`
	PGUser           string            `json:"postgres_user"`
	PGPassword       string            `json:"postgres_password"`
	PGSSLMode        string            `json:"postgres_sslmode"`
	PGRootCert       string            `json:"postgres_root_cert"`
	PGMaxConns       int               `json:"postgres_max_conns"`
	UploadDir        string            `json:"upload_dir"`
	MaxUploadMiB     int64             `json:"max_upload_mib"`
	MaxXMLMiB        int64             `json:"max_xml_mib"`
	ImportMinutes    int               `json:"import_timeout_minutes"`
	LocalDir         string            `json:"local_import_dir"`
	LocalPollSeconds int               `json:"local_poll_seconds"`
}

func defaultSettings() settings {
	return settings{JWTSecretEnv: "WISEMED_SIUI_DATA_JWT_SECRET", PGHost: "127.0.0.1", PGPort: 5432, PGSSLMode: "prefer", PGMaxConns: 20, UploadDir: "./catalogue-uploads", MaxUploadMiB: 4096, MaxXMLMiB: 16384, ImportMinutes: 120, LocalPollSeconds: 30}
}
func (c settings) validate() error {
	if e := validateClientKeys(c.ClientJWTSecrets); e != nil {
		return e
	}
	if c.LocalPollSeconds < 1 || c.LocalPollSeconds > 3600 {
		return errors.New("local polling interval must be 1..3600 seconds")
	}
	if c.PGHost == "" || c.PGPort < 1 || c.PGPort > 65535 || c.PGMaxConns < 4 || c.PGMaxConns > 500 {
		return errors.New("invalid PostgreSQL host, port or pool size (4..500)")
	}
	switch c.PGSSLMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return errors.New("invalid PostgreSQL sslmode")
	}
	if c.UploadDir == "" || c.MaxUploadMiB < 1 || c.MaxUploadMiB > 1<<20 || c.MaxXMLMiB < 1 || c.MaxXMLMiB > 1<<20 || c.ImportMinutes < 1 || c.ImportMinutes > 1440 {
		return errors.New("invalid upload limits or import timeout")
	}
	return nil
}

func (m *Module) settingsHTTP(w http.ResponseWriter, r *http.Request) {
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()
	_, admin := m.identity(r)
	if !admin {
		failure(w, 403, "administrator required")
		return
	}
	m.cfgMu.RLock()
	c := m.cfg
	m.cfgMu.RUnlock()
	if r.Method == "GET" {
		configured := c.PGPassword != ""
		c.PGPassword = ""
		c.ClientJWTSecrets = nil
		output(w, 200, map[string]any{"settings": c, "postgres_password_configured": configured})
		return
	}
	if r.Method != "PUT" {
		failure(w, 405, "GET or PUT required")
		return
	}
	// Blank/omitted password preserves the existing secret. It is never echoed.
	oldKeys := c.ClientJWTSecrets
	c.ClientJWTSecrets = nil
	oldPassword := c.PGPassword
	c.PGPassword = ""
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if e := d.Decode(&c); e != nil {
		failure(w, 400, "invalid settings JSON")
		return
	}
	if c.ClientJWTSecrets != nil {
		failure(w, 403, "manage client keys only in the internal Clients page")
		return
	}
	c.ClientJWTSecrets = oldKeys
	if c.PGPassword == "" {
		c.PGPassword = oldPassword
	}
	if e := c.validate(); e != nil {
		failure(w, 400, e.Error())
		return
	}
	m.workMu.Lock()
	defer m.workMu.Unlock()
	if m.running != 0 {
		failure(w, 409, "wait for active uploads/imports before changing settings")
		return
	}
	cfg, e := config.Load(m.rt.ConfigPath())
	if e != nil {
		failure(w, 500, "cannot load configuration")
		return
	}
	b, _ := json.Marshal(c)
	var values map[string]any
	_ = json.Unmarshal(b, &values)
	// Keep unrelated module keys intact during migration from the old snapshot.
	for k, v := range values {
		cfg.ModuleSettings(m.ID())[k] = v
	}
	if cfg.Modules[m.ID()] == nil {
		cfg.Modules[m.ID()] = values
	}
	// Atomically save with private permissions, including on first creation.
	cfg.ApplyDefaults()
	b, e = yaml.Marshal(cfg)
	if e == nil {
		e = writePrivateConfig(m.rt.ConfigPath(), b)
	}
	if e != nil {
		failure(w, 500, "cannot save configuration")
		return
	}
	m.dbMu.Lock()
	old := m.db
	m.db = nil
	m.cfgMu.Lock()
	m.cfg = c
	m.cfgMu.Unlock()
	m.dbMu.Unlock()
	if old != nil {
		old.Close()
	}
	output(w, 200, map[string]any{})
}

func writePrivateConfig(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".siui-config-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
