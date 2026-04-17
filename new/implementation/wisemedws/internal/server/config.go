package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	path string `yaml:"-"`

	Server struct {
		Address string `yaml:"address"`
		Port    int    `yaml:"port"`
	} `yaml:"server"`
	Security struct {
		AdminJWTSecret    string `yaml:"admin_jwt_secret"`
		AdminJWTSecretRef string `yaml:"admin_jwt_secret_ref"`
	} `yaml:"security"`
	Readers struct {
		APIKeys  map[string]string        `yaml:"api_keys"`
		Profiles map[string]ReaderProfile `yaml:"profiles"`
	} `yaml:"readers"`
	Control struct {
		DefaultTimeoutMS int `yaml:"default_timeout_ms"`
	} `yaml:"control"`
	Worklist struct {
		SampleTagOverrides map[string][]string `yaml:"sample_tag_overrides"`
	} `yaml:"worklist"`
}

type ReaderProfile struct {
	ReaderID      string   `yaml:"reader_id" json:"reader_id"`
	MedicalUnitID string   `yaml:"medical_unit_id" json:"medical_unit_id"`
	DepartmentID  string   `yaml:"department_id" json:"department_id"`
	DeviceLabel   string   `yaml:"device_label" json:"device_label"`
	AllowedTags   []string `yaml:"allowed_tags" json:"allowed_tags"`
}

func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, err
	}
	cfg.path = path
	if cfg.Server.Address == "" {
		cfg.Server.Address = "0.0.0.0"
	}
	if cfg.Server.Port <= 0 {
		cfg.Server.Port = 8090
	}
	if cfg.Control.DefaultTimeoutMS <= 0 {
		cfg.Control.DefaultTimeoutMS = 10000
	}
	if cfg.Readers.APIKeys == nil {
		cfg.Readers.APIKeys = map[string]string{}
	}
	if cfg.Readers.Profiles == nil {
		cfg.Readers.Profiles = map[string]ReaderProfile{}
	}
	if cfg.Worklist.SampleTagOverrides == nil {
		cfg.Worklist.SampleTagOverrides = map[string][]string{}
	}
	if v := strings.TrimSpace(os.Getenv("WISEMEDWS_PORT")); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	return cfg, nil
}

func (c *Config) Save() error {
	if c.path == "" {
		return fmt.Errorf("config path is empty")
	}
	raw, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, raw, 0o600)
}

func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Address, c.Server.Port)
}

func EnsureConfigFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	template := `# WiseMEDWS configuration
# This file is auto-generated at first run.
#
# Reader authentication model:
# - each reader signs a JWT with its API key (HS256)
# - token claim sub=reader_id and role=reader
# - server validates signature using readers.api_keys[reader_id]
#
# Admin API authentication model:
# - set env var named in security.admin_jwt_secret_ref
# - admin JWT must contain role=admin

server:
  # Listen address for WiseMEDWS service
  address: "0.0.0.0"
  # Listen TCP port
  port: 8090

security:
  # Direct JWT secret value (dev/local only). Prefer admin_jwt_secret_ref in production.
  admin_jwt_secret: ""
  # Environment variable name that stores admin JWT HMAC secret
  admin_jwt_secret_ref: "WISEMEDWS_ADMIN_JWT_SECRET"

readers:
  # Static reader API keys (reader_id -> api_key)
  # Replace demo values in production.
  api_keys:
    reader-maglumi-001: "reader-key-demo-001"
    reader-cobas-pro-001: "reader-key-demo-002"
  # Reader setup profiles created after first connect.
  # Reader is considered fully configured when medical_unit_id is set.
  profiles:
    reader-maglumi-001:
      reader_id: "reader-maglumi-001"
      medical_unit_id: "MU-DEMO-01"
      department_id: "LAB"
      device_label: "Maglumi Main Lab"
      allowed_tags: ["TSH", "FT4", "CEA"]

control:
  # Default timeout for command/response over WS
  default_timeout_ms: 10000

worklist:
  # Optional per-sample tag whitelist override.
  sample_tag_overrides:
    SAMPLE-001: ["TSH", "FT4"]
`
	return os.WriteFile(path, []byte(template), 0o600)
}
