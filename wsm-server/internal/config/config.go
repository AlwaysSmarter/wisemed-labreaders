package config

import (
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Address         string `yaml:"address"`
		Port            int    `yaml:"port"`
		WriteTimeoutMS  int    `yaml:"write_timeout_ms"`
		ReadTimeoutMS   int    `yaml:"read_timeout_ms"`
		PingIntervalMS  int    `yaml:"ping_interval_ms"`
		SendQueueSize   int    `yaml:"send_queue_size"`
		MaxMessageBytes int64  `yaml:"max_message_bytes"`
	} `yaml:"server"`
	Security struct {
		AcceptedKeys map[string]string `yaml:"accepted_keys"`
	} `yaml:"security"`
	WiseMed struct {
		BaseURL   string `yaml:"base_url"`
		APIKey    string `yaml:"api_key"`
		APIKeyRef string `yaml:"api_key_ref"`
	} `yaml:"wisemed"`
}

func Load(path string) (*Config, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(blob, &cfg); err != nil {
		return nil, err
	}

	if cfg.Server.Address == "" {
		cfg.Server.Address = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8090
	}
	if cfg.Server.WriteTimeoutMS <= 0 {
		cfg.Server.WriteTimeoutMS = 5000
	}
	if cfg.Server.ReadTimeoutMS <= 0 {
		cfg.Server.ReadTimeoutMS = 60000
	}
	if cfg.Server.PingIntervalMS <= 0 {
		cfg.Server.PingIntervalMS = 25000
	}
	if cfg.Server.SendQueueSize <= 0 {
		cfg.Server.SendQueueSize = 128
	}
	if cfg.Server.MaxMessageBytes <= 0 {
		cfg.Server.MaxMessageBytes = 1024 * 1024
	}
	if cfg.Security.AcceptedKeys == nil {
		cfg.Security.AcceptedKeys = map[string]string{}
	}
	if v := os.Getenv("WSM_SERVER_PORT"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			cfg.Server.Port = parsed
		}
	}
	if cfg.WiseMed.APIKey == "" && cfg.WiseMed.APIKeyRef != "" {
		cfg.WiseMed.APIKey = os.Getenv(cfg.WiseMed.APIKeyRef)
	}

	return &cfg, nil
}
