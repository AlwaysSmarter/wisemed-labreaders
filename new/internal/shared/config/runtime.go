package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type ReaderAgentConfig struct {
	Reader struct {
		ID            string `yaml:"id"`
		AnalyzerCode  string `yaml:"analyzer_code"`
		AnalyzerName  string `yaml:"analyzer_name"`
		AnalyzerType  string `yaml:"analyzer_type"`
		LicenseCode   string `yaml:"license_code"`
		APIKey        string `yaml:"api_key"`
		APIKeyRef     string `yaml:"api_key_ref"`
		MedicalUnitID string `yaml:"medical_unit_id"`
		DepartmentID  string `yaml:"department_id"`
		DeviceLabel   string `yaml:"device_label"`
	} `yaml:"reader"`
	Webservice struct {
		WSURL      string `yaml:"ws_url"`
		APIBaseURL string `yaml:"api_base_url"`
	} `yaml:"webservice"`
	Storage struct {
		LocalDBPath string `yaml:"local_db_path"`
	} `yaml:"storage"`
	Control struct {
		ReconnectSeconds int `yaml:"reconnect_seconds"`
		HeartbeatSeconds int `yaml:"heartbeat_seconds"`
	} `yaml:"control"`
}

type WiseMEDWSConfig struct {
	Server struct {
		Address string `yaml:"address"`
		Port    int    `yaml:"port"`
	} `yaml:"server"`
	Security struct {
		JWTSecretRef string `yaml:"jwt_secret_ref"`
	} `yaml:"security"`
	Control struct {
		CommandTimeoutMS int `yaml:"command_timeout_ms"`
	} `yaml:"control"`
}

func LoadReaderAgentConfig(path string) (*ReaderAgentConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &ReaderAgentConfig{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, err
	}
	cfg.applyDefaults()
	cfg.applyEnvOverrides()
	return cfg, cfg.validate()
}

func EnsureReaderConfigFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	template := `# Reader agent configuration
# This file is auto-generated at first run.
#
# Secrets policy:
# - recommended: use api_key_ref + environment variable
# - optional for local/dev: set reader.api_key directly

reader:
  # Unique reader id in WiseMED ecosystem
  id: "reader-maglumi-001"
  # Analyzer implementation code (must exist in adapter registry)
  analyzer_code: "maglumi-800"
  # Display name
  analyzer_name: "Maglumi 800"
  # Business type/category
  analyzer_type: "immunology"
  # License marker used by provisioning flow
  license_code: "DEMO-LICENSE"
  # Optional direct API key (local/dev)
  api_key: ""
  # Environment variable name containing reader API key
  api_key_ref: "WMR_READER_APIKEY"
  # Reader setup data (required to enable analyzer communication).
  # Can be auto-filled by WiseMEDWS registration_state.
  medical_unit_id: ""
  department_id: ""
  device_label: ""

webservice:
  # WiseMEDWS websocket endpoint (same VLAN with WiseMED)
  ws_url: "ws://127.0.0.1:8090/ws/readers"
  # WiseMEDWS HTTP API base URL used for worklist resolve.
  api_base_url: "http://127.0.0.1:8090"

storage:
  # Local SQLite DB path for offline-first operation
  local_db_path: "./reader.db"

control:
  # Reconnect interval to WiseMEDWS
  reconnect_seconds: 5
  # Heartbeat interval to WiseMEDWS
  heartbeat_seconds: 20
`
	return os.WriteFile(path, []byte(template), 0o600)
}

func LoadWiseMEDWSConfig(path string) (*WiseMEDWSConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &WiseMEDWSConfig{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, err
	}
	cfg.applyDefaults()
	cfg.applyEnvOverrides()
	return cfg, cfg.validate()
}

func (c *ReaderAgentConfig) applyDefaults() {
	if c.Control.ReconnectSeconds <= 0 {
		c.Control.ReconnectSeconds = 5
	}
	if c.Control.HeartbeatSeconds <= 0 {
		c.Control.HeartbeatSeconds = 20
	}
	if c.Storage.LocalDBPath == "" {
		c.Storage.LocalDBPath = "./reader.db"
	}
	if c.Webservice.APIBaseURL == "" {
		c.Webservice.APIBaseURL = "http://127.0.0.1:8090"
	}
}

func (c *WiseMEDWSConfig) applyDefaults() {
	if c.Server.Address == "" {
		c.Server.Address = "0.0.0.0"
	}
	if c.Server.Port <= 0 {
		c.Server.Port = 8090
	}
	if c.Control.CommandTimeoutMS <= 0 {
		c.Control.CommandTimeoutMS = 10000
	}
}

func (c *ReaderAgentConfig) applyEnvOverrides() {
	overrideString(&c.Reader.ID, "WMR_READER_ID")
	overrideString(&c.Reader.AnalyzerCode, "WMR_ANALYZER_CODE")
	overrideString(&c.Reader.AnalyzerName, "WMR_ANALYZER_NAME")
	overrideString(&c.Reader.AnalyzerType, "WMR_ANALYZER_TYPE")
	overrideString(&c.Reader.LicenseCode, "WMR_LICENSE_CODE")
	overrideString(&c.Reader.APIKey, "WMR_READER_APIKEY")
	overrideString(&c.Reader.APIKeyRef, "WMR_READER_APIKEY_REF")
	overrideString(&c.Reader.MedicalUnitID, "WMR_MEDICAL_UNIT_ID")
	overrideString(&c.Reader.DepartmentID, "WMR_DEPARTMENT_ID")
	overrideString(&c.Reader.DeviceLabel, "WMR_DEVICE_LABEL")
	overrideString(&c.Webservice.WSURL, "WMR_WEBSERVICE_WS_URL")
	overrideString(&c.Webservice.APIBaseURL, "WMR_WEBSERVICE_API_BASE_URL")
	overrideString(&c.Storage.LocalDBPath, "WMR_LOCAL_DB_PATH")
	overrideInt(&c.Control.ReconnectSeconds, "WMR_CONTROL_RECONNECT_SECONDS")
	overrideInt(&c.Control.HeartbeatSeconds, "WMR_CONTROL_HEARTBEAT_SECONDS")
}

func (c *WiseMEDWSConfig) applyEnvOverrides() {
	overrideString(&c.Server.Address, "WMWS_ADDRESS")
	overrideInt(&c.Server.Port, "WMWS_PORT")
	overrideString(&c.Security.JWTSecretRef, "WMWS_JWT_SECRET_REF")
	overrideInt(&c.Control.CommandTimeoutMS, "WMWS_COMMAND_TIMEOUT_MS")
}

func (c *ReaderAgentConfig) validate() error {
	if strings.TrimSpace(c.Reader.ID) == "" {
		return errors.New("missing reader.id")
	}
	if strings.TrimSpace(c.Reader.AnalyzerCode) == "" {
		return errors.New("missing reader.analyzer_code")
	}
	if strings.TrimSpace(c.Webservice.WSURL) == "" {
		return errors.New("missing webservice.ws_url")
	}
	if strings.TrimSpace(c.Reader.APIKey) == "" && strings.TrimSpace(c.Reader.APIKeyRef) == "" {
		return errors.New("missing reader api key: set reader.api_key or reader.api_key_ref")
	}
	if c.Control.ReconnectSeconds <= 0 || c.Control.HeartbeatSeconds <= 0 {
		return errors.New("invalid control timing")
	}
	return nil
}

func (c *WiseMEDWSConfig) validate() error {
	if c.Server.Port <= 0 {
		return errors.New("invalid server port")
	}
	if strings.TrimSpace(c.Security.JWTSecretRef) == "" {
		return errors.New("missing security.jwt_secret_ref")
	}
	if c.Control.CommandTimeoutMS <= 0 {
		return errors.New("invalid control.command_timeout_ms")
	}
	return nil
}

func (c *ReaderAgentConfig) ReconnectInterval() time.Duration {
	return time.Duration(c.Control.ReconnectSeconds) * time.Second
}

func (c *ReaderAgentConfig) HeartbeatInterval() time.Duration {
	return time.Duration(c.Control.HeartbeatSeconds) * time.Second
}

func (c *WiseMEDWSConfig) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Address, c.Server.Port)
}

func overrideString(target *string, env string) {
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		*target = v
	}
}

func overrideInt(target *int, env string) {
	v := strings.TrimSpace(os.Getenv(env))
	if v == "" {
		return
	}
	n, err := strconv.Atoi(v)
	if err == nil {
		*target = n
	}
}
