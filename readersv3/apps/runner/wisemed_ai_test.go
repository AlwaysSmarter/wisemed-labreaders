package runner

import (
	"testing"
	"wisemed-labreaders/readersv3/core/config"
)

func TestWiseMEDAIUtilityDefaults(t *testing.T) {
	cfg, e := config.Load("../wisemed-ai/deployments/config.install.yaml")
	if e != nil {
		t.Fatal(e)
	}
	if defaultCommType(cfg) != "utility" {
		t.Fatal("wrong transport")
	}
	s := cfg.ModuleSettings("app-updates")
	if !boolSetting(s, "enabled") || !boolSetting(s, "auto_download") || strSetting(s, "app_id") != "wisemed-ai" || strSetting(s, "channel") != "stable" || strSetting(s, "base_url") == "" {
		t.Fatal("incorrect update settings")
	}
	info, e := buildServiceInstallInfo(cfg)
	if e != nil || info.ServiceName != "wisemed-ai" {
		t.Fatal(info, e)
	}
}
