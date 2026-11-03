package config

import (
	"bytes"
	"errors"
	"io"
	"net/url"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Browser secrets stay in the protected registry and never appear in JSON lists.
type Browser struct {
	TenantID string   `yaml:"tenant_id" json:"tenant_id"`
	Label    string   `yaml:"label" json:"label"`
	Origin   string   `yaml:"origin" json:"origin"`
	Secret   string   `yaml:"secret" json:"-"`
	Scopes   []string `yaml:"scopes" json:"scopes"`
}
type TenantSettings struct {
	Issuer  *string `yaml:"issuer,omitempty"`
	BaseURL *string `yaml:"base_url,omitempty"`
	APIKey  *string `yaml:"api_key,omitempty" json:"-"`
}
type Registry struct {
	Tenants  map[string]TenantSettings `yaml:"tenant_settings,omitempty"`
	Devices  map[string]Device         `yaml:"devices"`
	Browsers map[string]Browser        `yaml:"browsers,omitempty"`
	Origins  map[string][]string       `yaml:"tenant_origins,omitempty"`
}

func ReadRegistry(path string) (Registry, error) {
	out := Registry{Tenants: map[string]TenantSettings{}, Devices: map[string]Device{}, Browsers: map[string]Browser{}, Origins: map[string][]string{}}
	if path == "" {
		return out, nil
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 4*1024*1024+1))
	if err != nil {
		return out, err
	}
	if len(data) > 4*1024*1024 {
		return out, errors.New("device registry too large")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err = decoder.Decode(&out); err != nil {
		return out, errors.New("invalid device registry")
	}
	var extra interface{}
	if decoder.Decode(&extra) != io.EOF {
		return out, errors.New("device registry must contain one document")
	}
	if out.Tenants == nil {
		out.Tenants = map[string]TenantSettings{}
	}
	if out.Devices == nil {
		out.Devices = map[string]Device{}
	}
	if out.Browsers == nil {
		out.Browsers = map[string]Browser{}
	}
	if out.Origins == nil {
		out.Origins = map[string][]string{}
	}
	return out, nil
}
func ValidOrigin(origin string) bool {
	u, err := url.Parse(origin)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.Hostname() != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && !strings.HasSuffix(origin, "#")
}
func BrowserScopes() []string {
	return []string{"route:reply", "route:event", "route:command", "connections:read", "server:status", "api:invoke", "topics:subscribe"}
}
func (c *Config) WithAccess(registry Registry) (*Config, error) {
	next := *c
	next.Security = c.Security
	next.Security.Keys = map[string]Key{}
	for id, key := range c.Security.Keys {
		if _, old := c.Browsers[id]; !old {
			next.Security.Keys[id] = key
		}
	}
	next.Browsers = map[string]Browser{}
	next.Tenants = map[string]Tenant{}
	for id, tenant := range c.Tenants {
		tenant.AllowedOrigins = slices.Clone(tenant.AllowedOrigins)
		next.Tenants[id] = tenant
	}
	next.ManagedTenants = map[string]TenantSettings{}
	for id, settings := range registry.Tenants {
		tenant, exists := next.Tenants[id]
		if !exists {
			return nil, errors.New("tenant settings reference unknown tenant")
		}
		if settings.Issuer != nil {
			if strings.TrimSpace(*settings.Issuer) == "" || len(*settings.Issuer) > 2048 {
				return nil, errors.New("issuer required (max 2048 characters)")
			}
			tenant.Issuer = *settings.Issuer
		}
		if settings.BaseURL != nil {
			u, err := url.Parse(*settings.BaseURL)
			if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
				return nil, errors.New("WiseMED API must be an HTTPS URL without credentials/query/fragment")
			}
			tenant.WiseMed.BaseURL = *settings.BaseURL
		}
		if settings.APIKey != nil {
			if len(*settings.APIKey) < 32 || strings.TrimSpace(*settings.APIKey) != *settings.APIKey {
				return nil, errors.New("WiseMED API key requires at least 32 characters without surrounding whitespace")
			}
			tenant.WiseMed.APIKey = *settings.APIKey
		}
		next.Tenants[id] = tenant
		next.ManagedTenants[id] = settings
	}
	for id, origins := range registry.Origins {
		tenant, exists := next.Tenants[id]
		if !exists {
			return nil, errors.New("origin override references unknown tenant")
		}
		if len(origins) > 100 {
			return nil, errors.New("too many tenant origins")
		}
		seen := map[string]bool{}
		for _, origin := range origins {
			if !ValidOrigin(origin) || seen[origin] {
				return nil, errors.New("invalid or duplicate origin")
			}
			seen[origin] = true
		}
		tenant.AllowedOrigins = slices.Clone(origins)
		next.Tenants[id] = tenant
	}
	for id, browser := range registry.Browsers {
		if !ValidID(id) || !ValidOrigin(browser.Origin) || len(browser.Label) == 0 || len(browser.Label) > 128 || len(browser.Secret) < 32 || strings.TrimSpace(browser.Secret) != browser.Secret {
			return nil, errors.New("invalid browser registration")
		}
		if _, exists := next.Tenants[browser.TenantID]; !exists {
			return nil, errors.New("unknown browser tenant")
		}
		if _, exists := next.Security.Keys[id]; exists {
			return nil, errors.New("browser key collision")
		}
		if err := ValidateDeviceScopes(browser.Scopes); err != nil {
			return nil, err
		}
		browser.Scopes = slices.Clone(browser.Scopes)
		next.Browsers[id] = browser
		next.Security.Keys[id] = Key{TenantID: browser.TenantID, Secret: browser.Secret, Roles: []string{"browser", "service"}, Scopes: slices.Clone(browser.Scopes)}
	}
	secrets := map[string]string{}
	for _, key := range next.Security.Keys {
		if key.Secret == "" {
			continue
		}
		if tenant, exists := secrets[key.Secret]; exists && tenant != key.TenantID {
			return nil, errors.New("different tenants must not share a signing secret")
		}
		secrets[key.Secret] = key.TenantID
	}
	return &next, nil
}
