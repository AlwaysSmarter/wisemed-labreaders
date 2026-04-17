package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"wisemed-labreaders/new/implementation/wisemedws/internal/server"
)

func main() {
	cfgPath := flag.String("config", "deployments/wisemedws.yaml", "Path to wisemedws config")
	flag.Parse()

	if err := server.EnsureConfigFile(*cfgPath); err != nil {
		log.Fatalf("ensure config file: %v", err)
	}

	cfg, err := server.LoadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	s, err := server.New(cfg)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := s.Run(ctx); err != nil {
		log.Fatalf("run error: %v", err)
	}
}
