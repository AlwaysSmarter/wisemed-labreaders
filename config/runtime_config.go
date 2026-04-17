package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultControlReconnectSeconds = 5
	defaultControlHeartbeatSeconds = 20
)

type RuntimeConfig struct {
	Reader  RuntimeReaderConfig  `yaml:"reader"`
	WiseMED RuntimeWiseMEDConfig `yaml:"wisemed"`
	Auth    RuntimeAuthConfig    `yaml:"auth"`
	Control RuntimeControlConfig `yaml:"control"`
}

type RuntimeReaderConfig struct {
	ID           string `yaml:"id"`
	AnalyzerType string `yaml:"analyzer_type"`
}

type RuntimeWiseMEDConfig struct {
	APIBaseURL string `yaml:"api_base_url"`
	WSURL      string `yaml:"ws_url"`
	AuthURL    string `yaml:"auth_url"`
}

type RuntimeAuthConfig struct {
	ClientID  string `yaml:"client_id"`
	SecretRef string `yaml:"secret_ref"`
}

type RuntimeControlConfig struct {
	ReconnectSeconds int `yaml:"reconnect_seconds"`
	HeartbeatSeconds int `yaml:"heartbeat_seconds"`
}

func LoadRuntimeConfig(path string) (*RuntimeConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := RuntimeConfig{}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}

	cfg.applyDefaults()
	cfg.applyEnvOverrides()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (cfg *RuntimeConfig) Validate() error {
	if cfg == nil {
		return errors.New("runtime config is nil")
	}

	if strings.TrimSpace(cfg.Reader.ID) == "" {
		return errors.New("missing reader.id")
	}
	if strings.TrimSpace(cfg.WiseMED.APIBaseURL) == "" {
		return errors.New("missing wisemed.api_base_url")
	}
	if strings.TrimSpace(cfg.WiseMED.WSURL) == "" {
		return errors.New("missing wisemed.ws_url")
	}
	if strings.TrimSpace(cfg.WiseMED.AuthURL) == "" {
		return errors.New("missing wisemed.auth_url")
	}
	if strings.TrimSpace(cfg.Auth.ClientID) == "" {
		return errors.New("missing auth.client_id")
	}
	if strings.TrimSpace(cfg.Auth.SecretRef) == "" {
		return errors.New("missing auth.secret_ref")
	}

	if cfg.Control.ReconnectSeconds <= 0 {
		return fmt.Errorf("control.reconnect_seconds must be > 0 (got %d)", cfg.Control.ReconnectSeconds)
	}
	if cfg.Control.HeartbeatSeconds <= 0 {
		return fmt.Errorf("control.heartbeat_seconds must be > 0 (got %d)", cfg.Control.HeartbeatSeconds)
	}

	return nil
}

func (cfg *RuntimeConfig) applyDefaults() {
	if cfg.Control.ReconnectSeconds <= 0 {
		cfg.Control.ReconnectSeconds = defaultControlReconnectSeconds
	}
	if cfg.Control.HeartbeatSeconds <= 0 {
		cfg.Control.HeartbeatSeconds = defaultControlHeartbeatSeconds
	}
}

func (cfg *RuntimeConfig) applyEnvOverrides() {
	setStringFromEnv(&cfg.Reader.ID, "WMR_READER_ID")
	setStringFromEnv(&cfg.Reader.AnalyzerType, "WMR_READER_ANALYZER_TYPE")

	setStringFromEnv(&cfg.WiseMED.APIBaseURL, "WMR_WISEMED_API_BASE_URL")
	setStringFromEnv(&cfg.WiseMED.WSURL, "WMR_WISEMED_WS_URL")
	setStringFromEnv(&cfg.WiseMED.AuthURL, "WMR_WISEMED_AUTH_URL")

	setStringFromEnv(&cfg.Auth.ClientID, "WMR_AUTH_CLIENT_ID")
	setStringFromEnv(&cfg.Auth.SecretRef, "WMR_AUTH_SECRET_REF")

	setIntFromEnv(&cfg.Control.ReconnectSeconds, "WMR_CONTROL_RECONNECT_SECONDS")
	setIntFromEnv(&cfg.Control.HeartbeatSeconds, "WMR_CONTROL_HEARTBEAT_SECONDS")
}

func setStringFromEnv(target *string, envName string) {
	if val := strings.TrimSpace(os.Getenv(envName)); val != "" {
		*target = val
	}
}

func setIntFromEnv(target *int, envName string) {
	val := strings.TrimSpace(os.Getenv(envName))
	if val == "" {
		return
	}

	parsed, err := strconv.Atoi(val)
	if err != nil {
		return
	}
	*target = parsed
}
