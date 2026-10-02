//go:build !windows

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"wisemed-labreaders/serverlast/wsm-server/internal/config"
	"wisemed-labreaders/serverlast/wsm-server/internal/server"
)

func watchReload(ctx context.Context, s *server.Server, path string) func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				cfg, err := config.Load(path)
				if err == nil {
					err = s.Reload(cfg)
				}
				if err != nil {
					log.Printf("reload rejected; previous configuration retained: %v", err)
				}
			}
		}
	}()
	return func() { signal.Stop(ch) }
}
