package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeviceRegistryLoadsOnRestartAndCannotOverrideMaster(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "master.key"), []byte(strings.Repeat("m", 40)), 0600)
	body := `server:
  address: 127.0.0.1
  allow_insecure_loopback: true
admin:
  enabled: true
  public_origin: https://admin.example
security:
  device_keys_file: state/devices.yaml
  keys:
    master:
      tenant_id: a
      secret_file: master.key
      roles: [service]
      scopes: ["server:status"]
tenants:
  a:
    issuer: issuer-a
`
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(body), 0600)
	c, e := Load(path)
	if e != nil {
		t.Fatal(e)
	}
	records := map[string]Device{"device-1": {TenantID: "a", EquipmentID: "42", Subject: "device-1", Secret: strings.Repeat("d", 43)}}
	if e = WriteDevices(c.Security.DeviceKeysFile, records); e != nil {
		t.Fatal(e)
	}
	loaded, e := Load(path)
	if e != nil {
		t.Fatal(e)
	}
	if loaded.Security.Keys["device-1"].EquipmentID != "42" || loaded.Security.Keys["device-1"].Secret != records["device-1"].Secret || len(loaded.Security.Keys["device-1"].Roles) != 1 || loaded.Security.Keys["device-1"].Roles[0] != "reader" {
		t.Fatal("registry auth key not restored")
	}
	if strings.Contains(strings.Join(loaded.Security.Keys["device-1"].Scopes, ","), "api:") {
		t.Fatal("device unexpectedly privileged")
	}
	saved, _ := os.ReadFile(path)
	if string(saved) != body {
		t.Fatal("master config changed")
	}
	records["master"] = records["device-1"]
	delete(records, "device-1")
	WriteDevices(c.Security.DeviceKeysFile, records)
	if _, e = Load(path); e == nil {
		t.Fatal("registry overrode master key")
	}
}
func TestAdminConfigurationFailClosed(t *testing.T) {
	for _, origin := range []string{"http://public.example", "https://admin.example/", "https://user:password@admin.example", "https://admin.example?x=1", "*", ""} {
		c := &Config{Admin: Admin{Enabled: true, PublicOrigin: origin}, Security: Security{DeviceKeysFile: filepath.Join(t.TempDir(), "devices.yaml")}, Server: Server{Address: "127.0.0.1", AllowInsecureLoopback: true}}
		if e := c.prepareAdmin(""); e == nil {
			t.Errorf("accepted origin %q", origin)
		}
	}
	c := &Config{Admin: Admin{Enabled: true, PublicOrigin: "http://localhost:8090"}, Security: Security{DeviceKeysFile: filepath.Join(t.TempDir(), "devices.yaml")}, Server: Server{Address: "127.0.0.1", AllowInsecureLoopback: true}}
	if e := c.prepareAdmin(""); e != nil {
		t.Fatal("loopback development", e)
	}
	c.Server.Address = "0.0.0.0"
	c.Server.AllowInsecureHTTP = true
	if e := c.prepareAdmin(""); e == nil {
		t.Fatal("public HTTP admin accepted")
	}
}
func TestDeviceRegistryRejectsInvalidAndNeverLeaksSecretInErrors(t *testing.T) {
	secret := strings.Repeat("private", 10)
	c := &Config{Tenants: map[string]Tenant{"a": {}}, Security: Security{Keys: map[string]Key{}}}
	for _, record := range []Device{{TenantID: "other", EquipmentID: "42", Subject: "device", Secret: secret}, {TenantID: "a", EquipmentID: "../42", Subject: "device", Secret: secret}, {TenantID: "a", EquipmentID: "42", Subject: "device", Secret: "short"}} {
		_, e := c.WithDevices(map[string]Device{"device": record})
		if e == nil || strings.Contains(e.Error(), secret) {
			t.Fatal("invalid record accepted or secret leaked")
		}
	}
	path := filepath.Join(t.TempDir(), "devices.yaml")
	os.WriteFile(path, []byte("devices: {}\nunknown: "+secret), 0600)
	if _, e := ReadDevices(path); e == nil || strings.Contains(e.Error(), secret) {
		t.Fatal("bad registry accepted or disclosed")
	}
}
