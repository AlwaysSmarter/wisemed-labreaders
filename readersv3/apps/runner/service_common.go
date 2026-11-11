package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"wisemed-labreaders/readersv3/core/config"
)

type serviceInstallInfo struct {
	ServiceName string
	DisplayName string
	Description string
	Executable  string
	ConfigPath  string
	WorkDir     string
}

func buildServiceInstallInfo(cfg *config.Config) (serviceInstallInfo, error) {
	exePath, e := os.Executable()
	if e != nil {
		return serviceInstallInfo{}, e
	}
	exePath, e = filepath.Abs(exePath)
	if e != nil {
		return serviceInstallInfo{}, e
	}
	configPath, e := filepath.Abs(cfg.Path())
	if e != nil {
		return serviceInstallInfo{}, e
	}
	token := sanitizeLogToken(firstNonEmpty(cfg.Reader.ID, cfg.Reader.AnalyzerCode, cfg.Reader.Label, "reader"))
	if token == "" {
		token = "reader"
	}
	label := firstNonEmpty(cfg.Reader.Label, cfg.Reader.AnalyzerName, cfg.Reader.ID, "Reader")
	serviceName := "wisemed-" + token
	if token == "wisemed-ai" {
		serviceName = token
	}
	return serviceInstallInfo{ServiceName: serviceName, DisplayName: "WiseMED " + label, Description: fmt.Sprintf("WiseMED readersv3 %s service", label), Executable: exePath, ConfigPath: configPath, WorkDir: filepath.Dir(exePath)}, nil
}
