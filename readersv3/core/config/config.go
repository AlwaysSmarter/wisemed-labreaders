package config

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	path           string                            `yaml:"-"`
	Reader         ReaderConfig                      `yaml:"reader"`
	LocalHTTP      LocalHTTPConfig                   `yaml:"local_http"`
	WiseMedWS      WiseMedWSConfig                   `yaml:"wisemed_ws"`
	Analyzer       AnalyzerConfig                    `yaml:"analyzer"`
	EnabledModules []string                          `yaml:"-"`
	Modules        map[string]map[string]interface{} `yaml:"modules"`
}

type ReaderConfig struct {
	ID           string `yaml:"id"`
	Label        string `yaml:"label"`
	AnalyzerName string `yaml:"analyzer_name"`
	AnalyzerCode string `yaml:"analyzer_code"`
	DBName       string `yaml:"db_name"`
}

type LocalHTTPConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Address  string `yaml:"address"`
	Language string `yaml:"language"`
	TLS      bool   `yaml:"tls"`
	CORS     string `yaml:"cors_allowed_origins"`
}

type WiseMedWSConfig struct {
	Enabled          bool   `yaml:"enabled"`
	URL              string `yaml:"url"`
	HeartbeatMS      int    `yaml:"heartbeat_ms"`
	ReconnectDelayMS int    `yaml:"reconnect_delay_ms"`
}

type AnalyzerConfig struct {
	CommType string `yaml:"comm_type"`
	Protocol string `yaml:"protocol"`
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
	cfg.path = path
	cfg.ApplyDefaults()
	return &cfg, nil
}

func Default() *Config {
	cfg := &Config{}
	cfg.ApplyDefaults()
	return cfg
}

func (c *Config) ApplyDefaults() {
	if c.Modules == nil {
		c.Modules = map[string]map[string]interface{}{}
	}
	syncLocalHTTPMirror(c)

	if c.Reader.Label == "" {
		c.Reader.Label = "Reader v3"
	}
	if c.Reader.AnalyzerName == "" {
		c.Reader.AnalyzerName = c.Reader.Label
	}
	if c.Reader.AnalyzerCode == "" {
		c.Reader.AnalyzerCode = strings.ToLower(strings.ReplaceAll(c.Reader.Label, " ", "-"))
	}
	if c.LocalHTTP.Address == "" {
		c.LocalHTTP.Address = "127.0.0.1:18080"
	}
	if c.LocalHTTP.Language == "" {
		c.LocalHTTP.Language = "ro"
	}
	if strings.TrimSpace(c.LocalHTTP.CORS) == "" {
		c.LocalHTTP.CORS = "https://ldse.wisemed.eu"
	}
	if !c.LocalHTTP.Enabled {
		c.LocalHTTP.Enabled = true
	}
	if c.WiseMedWS.HeartbeatMS <= 0 {
		c.WiseMedWS.HeartbeatMS = 15000
	}
	if c.WiseMedWS.ReconnectDelayMS <= 0 {
		c.WiseMedWS.ReconnectDelayMS = 5000
	}
	syncLocalHTTPMirror(c)
}

func (c *Config) ModuleSettings(moduleID string) map[string]interface{} {
	if c.Modules == nil {
		return map[string]interface{}{}
	}
	if item, ok := c.Modules[moduleID]; ok && item != nil {
		return item
	}
	return map[string]interface{}{}
}

func (c *Config) Path() string {
	return c.path
}

func (c *Config) Save() error {
	c.ApplyDefaults()
	blob, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, blob, 0o644)
}

func syncLocalHTTPMirror(c *Config) {
	if c == nil {
		return
	}
	if c.Modules == nil {
		c.Modules = map[string]map[string]interface{}{}
	}
	item, ok := c.Modules["local-http"]
	if !ok || item == nil {
		item = map[string]interface{}{}
		c.Modules["local-http"] = item
	}

	if strings.TrimSpace(c.LocalHTTP.Address) == "" {
		if value, _ := item["address"].(string); strings.TrimSpace(value) != "" {
			c.LocalHTTP.Address = strings.TrimSpace(value)
		}
	} else {
		item["address"] = c.LocalHTTP.Address
	}

	if strings.TrimSpace(c.LocalHTTP.Language) == "" {
		if value, _ := item["language"].(string); strings.TrimSpace(value) != "" {
			c.LocalHTTP.Language = strings.TrimSpace(value)
		}
	} else {
		item["language"] = c.LocalHTTP.Language
	}

	if enabled, ok := item["enabled"].(bool); !c.LocalHTTP.Enabled && ok {
		c.LocalHTTP.Enabled = enabled
	} else {
		item["enabled"] = c.LocalHTTP.Enabled
	}

	if tls, ok := item["tls"].(bool); !c.LocalHTTP.TLS && ok {
		c.LocalHTTP.TLS = tls
	} else {
		item["tls"] = c.LocalHTTP.TLS
	}

	if strings.TrimSpace(c.LocalHTTP.CORS) == "" {
		if value, _ := item["cors_allowed_origins"].(string); strings.TrimSpace(value) != "" {
			c.LocalHTTP.CORS = strings.TrimSpace(value)
		} else {
			c.LocalHTTP.CORS = "https://ldse.wisemed.eu"
		}
	}
	item["cors_allowed_origins"] = c.LocalHTTP.CORS
}
