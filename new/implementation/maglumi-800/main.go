package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"wisemed-labreaders/new/internal/readeragent/app"
	"wisemed-labreaders/new/internal/shared/config"
)

func main() {
	defaultCfgPath := defaultConfigPath()
	cfgPath := flag.String("config", defaultCfgPath, "Path to reader-agent yaml config")
	flag.Parse()

	if err := config.EnsureReaderConfigFile(*cfgPath); err != nil {
		log.Fatalf("ensure config file: %v", err)
	}

	cfg, err := config.LoadReaderAgentConfig(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if cfg.Reader.AnalyzerCode == "" {
		cfg.Reader.AnalyzerCode = "maglumi-800"
	}
	if cfg.Reader.AnalyzerName == "" {
		cfg.Reader.AnalyzerName = "Maglumi 800"
	}
	if cfg.Reader.AnalyzerType == "" {
		cfg.Reader.AnalyzerType = "immunology"
	}

	if err := app.Run(cfg); err != nil {
		log.Fatalf("run reader-agent: %v", err)
	}
}

func defaultConfigPath() string {
	exePath, err := os.Executable()
	if err != nil {
		return "reader-agent.yaml"
	}
	return filepath.Join(filepath.Dir(exePath), "reader-agent.yaml")
}
