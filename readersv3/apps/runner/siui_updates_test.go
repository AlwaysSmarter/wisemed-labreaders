package runner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/shared/appupdates"
)

func TestSIUIInstallationUsesSharedUpdater(t *testing.T) {
	cfg, err := config.Load("../siui-bridge/deployments/config.install.yaml")
	if err != nil {
		t.Fatal(err)
	}
	settings := cfg.ModuleSettings("app-updates")
	if !boolSetting(settings, "enabled") || !boolSetting(settings, "auto_download") || strSetting(settings, "base_url") == "" {
		t.Fatal("SIUI utility updates are disabled or unconfigured")
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/public/check-update" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("shared update request missing authentication or route")
		}
		var req appupdates.CheckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.AppID != "siui-bridge" || req.Channel != "stable" || req.OS == "" || req.Arch == "" {
			t.Errorf("wrong update identity/platform: %+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"up_to_date"}`))
	}))
	defer server.Close()
	settings["base_url"] = server.URL
	cfg.Modules["wisemed-api"]["cfg_wisemed_key"] = "synthetic-update-test-key"
	if err := checkForUpdates(cfg, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("SIUI skipped the shared updater")
	}
}

func TestHistoryInstallationUsesSharedUpdater(t *testing.T) {
	cfg, err := config.Load("../siui-bridge-history/deployments/config.install.yaml")
	if err != nil {
		t.Fatal(err)
	}
	settings := cfg.ModuleSettings("app-updates")
	if !boolSetting(settings, "enabled") || !boolSetting(settings, "auto_download") || strSetting(settings, "app_id") != "siui-bridge-history" || strSetting(settings, "channel") != "stable" || strSetting(settings, "base_url") == "" {
		t.Fatal("History updater defaults missing")
	}
	if defaultCommType(cfg) != "utility" {
		t.Fatal("History must be a utility")
	}
}

func TestHistoryBootstrapRequiresEquipmentConfiguration(t *testing.T) {
	cfg, err := config.Load("../siui-bridge-history/deployments/config.install.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ws := cfg.Modules["wisemed-api"]
	ws["cfg_wisemed_ip"] = "example.test"
	ws["cfg_wisemed_key"] = "test-key"
	if !needsBootstrap(cfg) {
		t.Fatal("History skipped equipment configuration")
	}
	for _, key := range []string{"unitate_medicala_id", "tip_de_echipament_id", "cod_echipament", "producator_echipament", "numar_serial_echipament", "nume_pe_raport_final", "nr_rackuri", "pozitii_pe_rack"} {
		ws[key] = "1"
	}
	if needsBootstrap(cfg) {
		t.Fatal("complete History configuration was rejected")
	}
}
