package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"wisemed-labreaders/serverlast/wsm-server/internal/config"
	"wisemed-labreaders/serverlast/wsm-server/internal/server"
)

var buildVersion = "dev"

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run() error {
	version := flag.Bool("version", false, "Print build version and exit")
	path := flag.String("config", "deployments/config.yaml", "Configuration file")
	check := flag.Bool("check-config", false, "Validate configuration, secrets and TLS files, then exit")
	_ = flag.Bool("showlog", true, "Compatibility flag; logs always go to stderr/journal")
	flag.Parse()
	if *version {
		fmt.Println(buildVersion)
		return nil
	}
	log.SetFlags(log.LstdFlags | log.LUTC)
	absPath, err := filepath.Abs(*path)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	*path = absPath
	log.Printf("wsm-server version=%s", buildVersion)
	log.Printf("loading configuration: %s", *path)
	cfg, err := config.Load(*path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if *check {
		fmt.Println("configuration valid")
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	svc := server.New(cfg)
	stopReload := watchReload(ctx, svc, *path)
	defer stopReload()
	return svc.Run(ctx)
}
