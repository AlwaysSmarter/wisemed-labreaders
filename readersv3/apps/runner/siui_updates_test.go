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
