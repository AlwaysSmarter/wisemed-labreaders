// Package config loads an immutable, validated configuration snapshot.
package config

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Server struct {
	ControlUIDir            string `yaml:"control_ui_dir"`
	Address                 string `yaml:"address"`
	Port                    int    `yaml:"port"`
	WriteTimeoutMS          int    `yaml:"write_timeout_ms"`
	ReadTimeoutMS           int    `yaml:"read_timeout_ms"`
	HelloTimeoutMS          int    `yaml:"hello_timeout_ms"`
	PingIntervalMS          int    `yaml:"ping_interval_ms"`
	SendQueueSize           int    `yaml:"send_queue_size"`
	MaxMessageBytes         int64  `yaml:"max_message_bytes"`
	MaxConnections          int    `yaml:"max_connections"`
	MaxConnectionsPerTenant int    `yaml:"max_connections_per_tenant"`
	MaxTopicsPerConnection  int    `yaml:"max_topics_per_connection"`
	MessagesPerSecond       int    `yaml:"messages_per_second"`
	MessageBurst            int    `yaml:"message_burst"`
	// Loopback-only opt-in, or explicit HTTP on a private proxy/container network.
	AllowInsecureLoopback bool `yaml:"allow_insecure_loopback"`
	AllowInsecureHTTP     bool `yaml:"allow_insecure_http"`
	TLS                   TLS  `yaml:"tls"`
}
type TLS struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}
type Security struct {
	DeviceKeysFile     string         `yaml:"device_keys_file"`
	Audience           string         `yaml:"audience"`
	MaxTokenTTLSeconds int            `yaml:"max_token_ttl_seconds"`
	AllowQueryToken    bool           `yaml:"allow_query_token"`
	Keys               map[string]Key `yaml:"keys"`
}
type Key struct {
	Subject     string   `yaml:"subject"`
	ReaderID    string   `yaml:"reader_id"`
	EquipmentID string   `yaml:"equipment_id"`
	TenantID    string   `yaml:"tenant_id"`
	SecretFile  string   `yaml:"secret_file"`
	SecretEnv   string   `yaml:"secret_env"`
	Disabled    bool     `yaml:"disabled"`
	Roles       []string `yaml:"roles"`
	Scopes      []string `yaml:"scopes"`
	Secret      string   `yaml:"-"`
}
type WiseMed struct {
	BaseURL    string `yaml:"base_url"`
	APIKeyFile string `yaml:"api_key_file"`
	APIKeyRef  string `yaml:"api_key_ref"`
	APIKey     string `yaml:"-"`
}
type Tenant struct {
	Issuer         string   `yaml:"issuer"`
	Disabled       bool     `yaml:"disabled"`
	AllowedOrigins []string `yaml:"allowed_origins"`
	WiseMed        WiseMed  `yaml:"wisemed"`
}
type Config struct {
	Admin       Admin             `yaml:"admin"`
	Devices     map[string]Device `yaml:"-"`
	Server      Server            `yaml:"server"`
	Security    Security          `yaml:"security"`
	Tenants     map[string]Tenant `yaml:"tenants"`
	Certificate *tls.Certificate  `yaml:"-"`
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

func ValidID(s string) bool { return identifier.MatchString(s) }
func ValidScope(s string) bool {
	switch s {
	case "route:command", "route:reply", "route:event", "route:broadcast", "topics:subscribe", "connections:read", "server:status", "wisemed:proxy", "devices:debug", "api:invoke", "api:admin":
		return true
	}
	return false
}
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d := yaml.NewDecoder(io.LimitReader(f, 1024*1024))
	d.KnownFields(true)
	var c Config
	if err = d.Decode(&c); err != nil {
		return nil, fmt.Errorf("configuration (legacy accepted_keys is unsupported; migrate to tenants/security.keys): %w", err)
	}
	var extra interface{}
	if err = d.Decode(&extra); err != io.EOF {
		return nil, errors.New("configuration must contain one YAML document")
	}
	if v := os.Getenv("WSM_SERVER_PORT"); v != "" {
		c.Server.Port, err = strconv.Atoi(v)
		if err != nil {
			return nil, errors.New("invalid WSM_SERVER_PORT")
		}
	}
	if err = c.Prepare(filepath.Dir(path)); err != nil {
		return nil, err
	}
	return &c, nil
}

// Prepare applies defaults, resolves secret/certificate files and fails closed.
func (c *Config) Prepare(base string) error {
	s := &c.Server
	if s.ControlUIDir != "" {
		s.ControlUIDir = resolve(base, s.ControlUIDir)
	}
	if s.Address == "" {
		s.Address = "0.0.0.0"
	}
	if s.Port == 0 {
		s.Port = 8443
	}
	if s.WriteTimeoutMS == 0 {
		s.WriteTimeoutMS = 5000
	}
	if s.ReadTimeoutMS == 0 {
		s.ReadTimeoutMS = 60000
	}
	if s.HelloTimeoutMS == 0 {
		s.HelloTimeoutMS = 10000
	}
	if s.PingIntervalMS == 0 {
		s.PingIntervalMS = 25000
	}
	if s.SendQueueSize == 0 {
		s.SendQueueSize = 128
	}
	if s.MaxMessageBytes == 0 {
		s.MaxMessageBytes = 1048576
	}
	if s.MaxConnections == 0 {
		s.MaxConnections = 2000
	}
	if s.MaxConnectionsPerTenant == 0 {
		s.MaxConnectionsPerTenant = 200
	}
	if s.MaxTopicsPerConnection == 0 {
		s.MaxTopicsPerConnection = 64
	}
	if s.MessagesPerSecond == 0 {
		s.MessagesPerSecond = 50
	}
	if s.MessageBurst == 0 {
		s.MessageBurst = 100
	}
	if s.Port < 1 || s.Port > 65535 || s.WriteTimeoutMS < 1 || s.ReadTimeoutMS < 1 || s.HelloTimeoutMS < 1 || s.PingIntervalMS < 1 || s.PingIntervalMS >= s.ReadTimeoutMS || s.SendQueueSize < 1 || s.SendQueueSize > 4096 || s.MaxMessageBytes < 1 || s.MaxMessageBytes > 16*1024*1024 || s.MaxConnections < 1 || s.MaxConnectionsPerTenant < 1 || s.MaxTopicsPerConnection < 1 || s.MessagesPerSecond < 1 || s.MessageBurst < 1 {
		return errors.New("invalid server limits/timeouts")
	}
	if (s.TLS.CertFile == "") != (s.TLS.KeyFile == "") {
		return errors.New("TLS requires both cert_file and key_file")
	}
	if s.TLS.CertFile == "" {
		ip := net.ParseIP(s.Address)
		if !s.AllowInsecureHTTP && (!s.AllowInsecureLoopback || ip == nil || !ip.IsLoopback()) {
			return errors.New("TLS required; use allow_insecure_loopback on loopback, or explicitly allow_insecure_http behind a private TLS proxy")
		}
	} else {
		s.TLS.CertFile = resolve(base, s.TLS.CertFile)
		s.TLS.KeyFile = resolve(base, s.TLS.KeyFile)
		cert, err := tls.LoadX509KeyPair(s.TLS.CertFile, s.TLS.KeyFile)
		if err != nil {
			return fmt.Errorf("TLS key pair: %w", err)
		}
		c.Certificate = &cert
	}
	if c.Security.Audience == "" {
		c.Security.Audience = "wsm-server"
	}
	if c.Security.MaxTokenTTLSeconds == 0 {
		c.Security.MaxTokenTTLSeconds = 900
	}
	if c.Security.MaxTokenTTLSeconds < 1 || c.Security.MaxTokenTTLSeconds > 86400 {
		return errors.New("max_token_ttl_seconds must be 1..86400")
	}
	if err := c.prepareAdmin(base); err != nil {
		return err
	}
	if len(c.Tenants) == 0 || len(c.Security.Keys) == 0 {
		return errors.New("at least one tenant and key are required")
	}
	for id, t := range c.Tenants {
		if !ValidID(id) || t.Issuer == "" {
			return fmt.Errorf("invalid tenant ID or missing issuer: %q", id)
		}
		for _, o := range t.AllowedOrigins {
			u, e := url.Parse(o)
			if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
				return fmt.Errorf("tenant %s: origins must be exact https origins", id)
			}
		}
		if t.WiseMed.BaseURL != "" {
			u, e := url.Parse(t.WiseMed.BaseURL)
			if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return fmt.Errorf("tenant %s: WiseMED URL must be HTTPS without credentials/query/fragment", id)
			}
			secret, e := readSecret(base, t.WiseMed.APIKeyFile, t.WiseMed.APIKeyRef)
			if e != nil {
				return fmt.Errorf("tenant %s WiseMED credential: %w", id, e)
			}
			t.WiseMed.APIKey = secret
		}
		c.Tenants[id] = t
	}
	seen := map[string]string{}
	active := 0
	for kid, k := range c.Security.Keys {
		if !ValidID(kid) || !ValidID(k.TenantID) {
			return errors.New("invalid key/tenant identifier")
		}
		t, ok := c.Tenants[k.TenantID]
		if !ok {
			return fmt.Errorf("key %s references unknown tenant", kid)
		}
		if k.Disabled || t.Disabled {
			continue
		}
		if len(k.Roles) == 0 || len(k.Scopes) == 0 {
			return fmt.Errorf("key %s requires explicit roles and scopes", kid)
		}
		for _, r := range k.Roles {
			if r != "browser" && r != "reader" && r != "service" {
				return fmt.Errorf("key %s: invalid role", kid)
			}
		}
		for _, scope := range k.Scopes {
			if !ValidScope(scope) {
				return fmt.Errorf("key %s: invalid scope", kid)
			}
		}
		secret := k.Secret
		var e error
		if _, generated := c.Devices[kid]; !generated {
			secret, e = readSecret(base, k.SecretFile, k.SecretEnv)
		}
		if e != nil {
			return fmt.Errorf("key %s: %w", kid, e)
		}
		if len(secret) < 32 {
			return fmt.Errorf("key %s: secret must contain at least 32 bytes", kid)
		}
		if prior, ok := seen[secret]; ok && prior != k.TenantID {
			return errors.New("different tenants must not share a signing secret")
		}
		seen[secret] = k.TenantID
		k.Secret = secret
		c.Security.Keys[kid] = k
		active++
	}
	if active == 0 {
		return errors.New("at least one active tenant key is required")
	}
	return nil
}
func resolve(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}
func readSecret(base, file, env string) (string, error) {
	if (file == "") == (env == "") {
		return "", errors.New("configure exactly one secret file or environment reference")
	}
	var v string
	if file != "" {
		b, e := os.ReadFile(resolve(base, file))
		if e != nil {
			return "", e
		}
		v = string(b)
	} else {
		v = os.Getenv(env)
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", errors.New("empty secret")
	}
	return v, nil
}
