package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"wisemed-labreaders/serverlast/wsm-server/internal/config"
	"wisemed-labreaders/serverlast/wsm-server/internal/server"
)

func main() {
	cfgPath := flag.String("config", "deployments/config.yaml", "Path to config file")
	showLog := flag.Bool("showlog", false, "Print runtime logs to console in addition to the daily log file")
	flag.Parse()

	logPath, closeLog, err := setupLogging(*cfgPath, *showLog)
	if err != nil {
		log.Fatalf("setup logging: %v", err)
	}
	defer closeLog()
	log.Printf("runtime log file: %s", logPath)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	svc := server.New(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := svc.Run(ctx); err != nil {
		log.Fatalf("run server: %v", err)
	}
}

func setupLogging(cfgPath string, showLog bool) (string, func(), error) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	logDir := filepath.Dir(cfgPath)
	if logDir == "." || logDir == "" {
		logDir = "."
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", nil, err
	}
	fileName := fmt.Sprintf("%s-%s.log", logBaseName(cfgPath), time.Now().Format("20060102"))
	logPath := filepath.Join(logDir, fileName)
	fh, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", nil, err
	}
	var writer io.Writer = fh
	if showLog {
		writer = io.MultiWriter(os.Stderr, fh)
	}
	log.SetOutput(writer)
	return logPath, func() { _ = fh.Close() }, nil
}

func logBaseName(cfgPath string) string {
	name := strings.TrimSuffix(filepath.Base(cfgPath), filepath.Ext(cfgPath))
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "wisemedws"
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", ";", "-", ",", "-", ".", "-", "(", "", ")", "", "[", "", "]", "", "{", "", "}", "", "\"", "", "'", "")
	name = replacer.Replace(name)
	name = strings.Trim(name, "-_")
	if name == "" {
		return "wisemedws"
	}
	return name
}
