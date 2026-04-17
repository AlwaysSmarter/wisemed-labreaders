package main

import (
	"flag"
	"log"

	"wisemed-labreaders/new/internal/readeragent/app"
	"wisemed-labreaders/new/internal/shared/config"
)

func main() {
	cfgPath := flag.String("config", "deployments/reader-agent.yaml", "Path to reader-agent yaml config")
	flag.Parse()

	if err := config.EnsureReaderConfigFile(*cfgPath); err != nil {
		log.Fatalf("ensure config file: %v", err)
	}

	cfg, err := config.LoadReaderAgentConfig(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := app.Run(cfg); err != nil {
		log.Fatalf("run reader-agent: %v", err)
	}
}
