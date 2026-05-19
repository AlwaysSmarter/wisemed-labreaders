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
	exePath, err := os.Executable()
	if err != nil {
		return serviceInstallInfo{}, err
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return serviceInstallInfo{}, err
	}
	configPath, err := filepath.Abs(cfg.Path())
	if err != nil {
		return serviceInstallInfo{}, err
	}
	token := sanitizeLogToken(firstNonEmpty(cfg.Reader.ID, cfg.Reader.AnalyzerCode, cfg.Reader.Label, "reader"))
	if token == "" {
		token = "reader"
	}
	label := firstNonEmpty(cfg.Reader.Label, cfg.Reader.AnalyzerName, cfg.Reader.ID, "Reader")
	return serviceInstallInfo{
		ServiceName: "wisemed-" + token,
		DisplayName: "WiseMED " + label,
		Description: fmt.Sprintf("WiseMED readersv3 %s service", label),
		Executable:  exePath,
		ConfigPath:  configPath,
		WorkDir:     filepath.Dir(exePath),
	}, nil
}
