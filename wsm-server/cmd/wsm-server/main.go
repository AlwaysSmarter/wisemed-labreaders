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
		return fmt.Errorf(`load config: %w

If this failure follows a tenant ID rename, update all three places:
1. Main configuration (%s): rename the entry under tenants and update
   security.keys.<key-name>.tenant_id to the same tenant ID.
   The key name itself (for example, local-backend) can remain unchanged.
2. Generated equipment registry: open the file configured by
   security.device_keys_file (relative paths are resolved from the config directory).
   Back it up first, then update tenant_id on every affected device entry.
   Preserve all secrets, key IDs and other fields. Edit while WSM is stopped.
3. Each reader: Settings -> Debug WSS -> Tenant ID, then save and apply.
   External JWT issuers must also use the new tenant ID.

Validate with -config <configuration-file> -check-config, then restart WSM
and select the renamed tenant in the admin login.
For other configuration failures, correct the specific error reported above.`, err, *path)
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
