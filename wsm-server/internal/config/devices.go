package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type Admin struct {
	Enabled           bool   `yaml:"enabled"`
	PublicOrigin      string `yaml:"public_origin"`
	SessionTTLSeconds int    `yaml:"session_ttl_seconds"`
}

// Device records belong to the separate writable registry, never the master config.
// JSON serialization intentionally excludes the stored secret.
type Device struct {
	TenantID    string    `yaml:"tenant_id" json:"tenant_id"`
	EquipmentID string    `yaml:"equipment_id" json:"equipment_id"`
	ReaderID    string    `yaml:"reader_id,omitempty" json:"reader_id"`
	Subject     string    `yaml:"subject" json:"subject"`
	Secret      string    `yaml:"secret" json:"-"`
	Scopes      *[]string `yaml:"scopes,omitempty" json:"-"`
}

func DeviceScopes() []string {
	return []string{"route:reply", "route:event", "route:command", "devices:debug", "connections:read", "server:status"}
}

// DevicePermissionCatalog is the bounded set editable for device credentials.
type DevicePermission struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

func DevicePermissionCatalog() []DevicePermission {
	return []DevicePermission{
		{"route:reply", "Trimite răspunsuri la comenzile primite; necesar pentru controlul remote al aparatului."},
		{"route:event", "Trimite evenimente către alți clienți din tenant."},
		{"route:command", "Trimite comenzi; necesar împreună cu api:invoke pentru Deschide."},
		{"devices:debug", "Trimite ping, reconnect și mesaje de diagnostic altor echipamente."},
		{"connections:read", "Vede lista și starea online a echipamentelor din tenant."},
		{"server:status", "Citește starea serverului WSM."},
		{"api:invoke", "Deschide console remote și apelează API-ul altor echipamente din același tenant, inclusiv operații care modifică date; necesită route:command."},
		{"api:admin", "Permite și operațiile API administrative remote; necesită api:invoke și route:command. Nu acordă login în consola admin WSM."},
		{"route:broadcast", "Trimite către mai mulți destinatari din tenant; necesită dreptul corespunzător de comandă sau eveniment."},
	}
}
func (d Device) EffectiveScopes() []string {
	if d.Scopes == nil {
		return DeviceScopes()
	}
	return slices.Clone(*d.Scopes)
}
func ValidateDeviceScopes(scopes []string) error {
	if len(scopes) == 0 {
		return errors.New("select at least one permission")
	}
	allowed := map[string]bool{}
	for _, p := range DevicePermissionCatalog() {
		allowed[p.ID] = true
	}
	seen := map[string]bool{}
	for _, scope := range scopes {
		if !allowed[scope] || seen[scope] {
			return errors.New("unknown or duplicate device permission")
		}
		seen[scope] = true
	}
	if seen["api:invoke"] && !seen["route:command"] {
		return errors.New("api:invoke requires route:command")
	}
	if seen["api:admin"] && !seen["api:invoke"] {
		return errors.New("api:admin requires api:invoke")
	}
	return nil
}
func (c *Config) prepareAdmin(base string) error {
	if c.Admin.SessionTTLSeconds == 0 {
		c.Admin.SessionTTLSeconds = 900
	}
	if c.Admin.SessionTTLSeconds < 60 || c.Admin.SessionTTLSeconds > 3600 {
		return errors.New("admin session TTL must be 60..3600 seconds")
	}
	if c.Security.DeviceKeysFile != "" {
		c.Security.DeviceKeysFile = resolve(base, c.Security.DeviceKeysFile)
	}
	if c.Admin.Enabled {
		u, e := url.Parse(c.Admin.PublicOrigin)
		if e != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("admin public_origin must be an exact origin")
		}
		if u.Scheme != "https" {
			host := net.ParseIP(u.Hostname())
			listener := net.ParseIP(c.Server.Address)
			if u.Scheme != "http" || listener == nil || !listener.IsLoopback() || !c.Server.AllowInsecureLoopback || (u.Hostname() != "localhost" && (host == nil || !host.IsLoopback())) {
				return errors.New("admin requires HTTPS origin (HTTP only for explicit loopback development)")
			}
		}
		if c.Security.DeviceKeysFile == "" {
			return errors.New("admin requires security.device_keys_file")
		}
	}
	return c.RefreshDevices()
}

// RefreshDevices must run while the caller excludes concurrent config publication.
// Reload reads the registry again under its admission gate, avoiding a stale config
// read overwriting a just-provisioned device.
func (c *Config) RefreshDevices() error {
	devices, err := ReadDevices(c.Security.DeviceKeysFile)
	if err != nil {
		return err
	}
	next, err := c.WithDevices(devices)
	if err != nil {
		return err
	}
	c.Security.Keys = next.Security.Keys
	c.Devices = next.Devices
	return nil
}
func ReadDevices(path string) (map[string]Device, error) {
	out := map[string]Device{}
	if path == "" {
		return out, nil
	}
	f, e := os.Open(path)
	if errors.Is(e, os.ErrNotExist) {
		return out, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, 4*1024*1024+1))
	if e != nil {
		return nil, e
	}
	if len(data) > 4*1024*1024 {
		return nil, errors.New("device registry too large")
	}
	var v struct {
		Devices map[string]Device `yaml:"devices"`
	}
	d := yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	if e = d.Decode(&v); e != nil {
		return nil, errors.New("invalid device registry")
	}
	var extra interface{}
	if d.Decode(&extra) != io.EOF {
		return nil, errors.New("device registry must contain one document")
	}
	if v.Devices != nil {
		out = v.Devices
	}
	return out, nil
}
func (c *Config) WithDevices(devices map[string]Device) (*Config, error) {
	next := *c
	next.Security = c.Security
	next.Security.Keys = map[string]Key{}
	next.Devices = map[string]Device{}
	for id, k := range c.Security.Keys {
		if _, generated := c.Devices[id]; !generated {
			next.Security.Keys[id] = k
		}
	}
	seen := map[string]bool{}
	for id, d := range devices {
		if !ValidID(id) || !ValidID(d.TenantID) || !ValidID(d.EquipmentID) || !ValidID(d.Subject) || (d.ReaderID != "" && !ValidID(d.ReaderID)) || len(d.Secret) < 32 || strings.TrimSpace(d.Secret) != d.Secret {
			return nil, errors.New("invalid generated device identity or secret")
		}
		if _, exists := next.Security.Keys[id]; exists {
			return nil, errors.New("generated key collides with configured key")
		}
		if _, exists := c.Tenants[d.TenantID]; !exists {
			return nil, errors.New("generated key references unknown tenant")
		}
		unique := d.TenantID + "\x00" + d.EquipmentID
		if seen[unique] {
			return nil, errors.New("duplicate generated equipment")
		}
		seen[unique] = true
		if err := ValidateDeviceScopes(d.EffectiveScopes()); err != nil {
			return nil, err
		}
		if d.Scopes != nil {
			scopes := d.EffectiveScopes()
			d.Scopes = &scopes
		}
		next.Devices[id] = d
		next.Security.Keys[id] = Key{TenantID: d.TenantID, EquipmentID: d.EquipmentID, ReaderID: d.ReaderID, Subject: d.Subject, Secret: d.Secret, Roles: []string{"reader"}, Scopes: d.EffectiveScopes()}
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
func WriteDevices(path string, devices map[string]Device) error {
	if path == "" {
		return errors.New("device registry is not configured")
	}
	data, e := yaml.Marshal(struct {
		Devices map[string]Device `yaml:"devices"`
	}{devices})
	if e != nil {
		return e
	}
	if len(data) > 4*1024*1024 {
		return errors.New("device registry is full")
	}
	dir := filepath.Dir(path)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".devices-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(tmp, path); e != nil {
		return fmt.Errorf("persist device registry: %w", e)
	}
	if d, e := os.Open(dir); e == nil {
		defer d.Close()
		_ = d.Sync()
	}
	return nil
}
