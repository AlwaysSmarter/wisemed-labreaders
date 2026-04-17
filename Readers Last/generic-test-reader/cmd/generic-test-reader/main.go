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

	"wisemed-labreaders/readerslast/generic-test-reader/internal/config"
	"wisemed-labreaders/readerslast/generic-test-reader/internal/reader"
	"wisemed-labreaders/readerslast/generic-test-reader/internal/storage"
	"wisemed-labreaders/readerslast/generic-test-reader/internal/webui"
)

func main() {
	cfgPath := flag.String("config", "deployments/config.yaml", "Path to reader config")
	reconfigure := flag.Bool("reconfigure", false, "Run interactive bootstrap/configuration wizard before starting")
	showLog := flag.Bool("showlog", false, "Print runtime logs to console in addition to the daily log file")
	flag.Parse()

	flag.Usage = func() {
		log.Printf("Generic Test Reader\n\nFlags:\n")
		flag.PrintDefaults()
		log.Printf(`
Reader capabilities:
- bootstrap against WiseMed API using JWT-signed calls
- interactive selection of medical unit
- persistent YAML config and SQLite database
- communication modes: file, serial, network
- file import mode is operational
- serial/network configs are stored and exposed for later implementations
- WebSocket connection to WiseMedWS with authenticated reader JWT
- WS admin commands for status, logs, analytes, orders and results
`)
	}

	cfg, err := config.LoadOrCreate(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	changed, err := reader.EnsureBootstrap(cfg, *reconfigure)
	if err != nil {
		log.Fatalf("bootstrap reader: %v", err)
	}
	logPath, closeLog, err := setupLogging(cfg, *showLog)
	if err != nil {
		log.Fatalf("setup logging: %v", err)
	}
	defer closeLog()
	fmt.Printf("Runtime log file: %s\n", logPath)
	if changed {
		if !reader.ConfirmSaveAndStart(cfg) {
			log.Printf("configuration aborted by user")
			return
		}
		if err := cfg.Save(); err != nil {
			log.Fatalf("save config: %v", err)
		}
		log.Printf("configuration saved to %s", cfg.ConfigPath())
	}
	log.Printf("starting reader with communication=%s protocol=%s", cfg.Comm.Type, cfg.Comm.Protocol)

	store, err := storage.Open(cfg.DBPath())
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	app := reader.New(cfg, store)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if cfg.LocalHTTP.Enabled {
		uiServer, err := webui.New(cfg, app)
		if err != nil {
			log.Fatalf("build local http ui: %v", err)
		}
		if err := uiServer.Start(ctx); err != nil {
			log.Fatalf("start local http ui: %v", err)
		}
		log.Printf("local http ui available at http://%s", cfg.LocalHTTP.Address)
	}

	if err := app.Run(ctx); err != nil {
		log.Fatalf("run generic test reader: %v", err)
	}
}

func setupLogging(cfg *config.Config, showLog bool) (string, func(), error) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	logDir := filepath.Dir(cfg.ConfigPath())
	if logDir == "." || logDir == "" {
		logDir = "."
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", nil, err
	}
	fileName := fmt.Sprintf("%s-%s.log", logBaseName(cfg), time.Now().Format("20060102"))
	logPath := filepath.Join(logDir, fileName)
	fh, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", nil, err
	}

	var writer io.Writer = fh
	if showLog {
		writer = io.MultiWriter(os.Stdout, fh)
	}
	log.SetOutput(writer)
	return logPath, func() { _ = fh.Close() }, nil
}

func logBaseName(cfg *config.Config) string {
	for _, value := range []string{cfg.Reader.ID, cfg.Reader.AnalyzerCode, cfg.Reader.Label, "reader"} {
		if token := sanitizeLogToken(value); token != "" {
			return token
		}
	}
	return "reader"
}

func sanitizeLogToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", ";", "-", ",", "-", ".", "-", "(", "", ")", "", "[", "", "]", "", "{", "", "}", "", "\"", "", "'", "")
	value = replacer.Replace(value)
	value = strings.Trim(value, "-_")
	return value
}
