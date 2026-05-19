package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/core/runtime"
	"wisemed-labreaders/readersv3/modules/builtin"
	"wisemed-labreaders/readersv3/shared/appmeta"
)

func main() {
	cfgPath := flag.String("config", "deployments/config.yaml", "Path to update-server config")
	reconfigure := flag.Bool("reconfigure", false, "Run interactive bootstrap/configuration wizard before starting")
	showVersion := flag.Bool("version", false, "Print application version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(appmeta.CurrentVersion())
		return
	}

	ensureResult, err := config.Ensure(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	if ensureResult.Created {
		log.Printf("configuration created from install template: %s", *cfgPath)
	}
	if ensureResult.Merged {
		log.Printf("configuration merged with new defaults from %s", ensureResult.TemplatePath)
	}
	changed, err := ensureUpdateServerBootstrap(cfg, *reconfigure)
	if err != nil {
		log.Fatal(err)
	}
	if changed {
		if err := cfg.Save(); err != nil {
			log.Fatal(err)
		}
		log.Printf("configuration saved to %s", cfg.Path())
	}
	reg := module.NewRegistry()
	builtin.RegisterAll(reg)
	app := runtime.New(cfg, log.New(os.Stdout, "", log.LstdFlags), reg)
	if err := app.InitModules([]string{"wisemed-api", "login", "help", "app-update-server"}); err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Start(ctx); err != nil {
		log.Fatal(err)
	}
}
