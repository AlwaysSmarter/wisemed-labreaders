package runner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/core/runtime"
	"wisemed-labreaders/readersv3/modules/builtin"
	"wisemed-labreaders/readersv3/shared/appmeta"
)

type RunOptions struct {
	Headless       bool
	HeadlessChild  bool
	InstallService bool
	Reconfigure    bool
	ShowLog        bool
}

func Run(configPath string, defaultModules []string, opts RunOptions) error {
	ensureResult, err := config.Ensure(configPath)
	if err != nil {
		return err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	normalizeLegacyConfig(cfg)
	if ensureResult.Created {
		log.Printf("configuration created from install template: %s", configPath)
	}
	if ensureResult.Merged {
		log.Printf("configuration merged with new defaults from %s", ensureResult.TemplatePath)
	}
	cfg.EnabledModules = append([]string(nil), defaultModules...)
	changed, err := ensureBootstrap(cfg, opts.Reconfigure)
	if err != nil {
		log.Printf("bootstrap: non-blocking startup warning: %v", err)
		startupConsolef("warning bootstrap: %v", err)
		changed = false
		cfg.ApplyDefaults()
	}
	logPath, closeLog, err := setupLogging(cfg, opts.ShowLog)
	if err != nil {
		return err
	}
	defer closeLog()
	log.Printf("runtime log file: %s", logPath)
	startupConsolef("versiune aplicatie: %s", appmeta.CurrentVersion())
	if changed {
		if !confirmSaveAndStart(cfg) {
			log.Printf("configuration aborted by user")
			return nil
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		log.Printf("configuration saved to %s", cfg.Path())
	}
	if opts.InstallService {
		return installService(cfg)
	}
	if err := checkForUpdates(cfg, opts); err != nil {
		if errors.Is(err, errUpdateStarted) {
			return nil
		}
		log.Printf("app-updates: non-blocking startup warning: %v", err)
		startupConsolef("warning update server: %v", err)
	}
	if opts.Headless && !opts.HeadlessChild {
		info, err := launchHeadlessProcess(configPath)
		if err != nil {
			return err
		}
		pidFile := filepath.Join(filepath.Dir(cfg.Path()), fmt.Sprintf("%s.pid", logBaseName(cfg)))
		fmt.Printf("Headless mode active. Background PID: %d\n", info.PID)
		fmt.Printf("Log file: %s\n", logPath)
		fmt.Printf("PID file: %s\n", pidFile)
		for _, line := range info.Instructions {
			fmt.Println(line)
		}
		return nil
	}
	reg := module.NewRegistry()
	builtin.RegisterAll(reg)
	logger := log.New(log.Writer(), "", log.LstdFlags|log.Lmicroseconds)
	app := runtime.New(cfg, logger, reg)
	if err := app.InitModules(cfg.EnabledModules); err != nil {
		return err
	}
	if handled, err := runServiceManager(cfg, app); handled || err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if opts.HeadlessChild {
		pidPath := filepath.Join(filepath.Dir(cfg.Path()), fmt.Sprintf("%s.pid", logBaseName(cfg)))
		if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); err != nil {
			log.Printf("failed to write pid file %s: %v", pidPath, err)
		} else {
			defer func() {
				if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
					log.Printf("failed to remove pid file %s: %v", pidPath, err)
				}
			}()
		}
	}
	return runAppLoop(ctx, configPath, defaultModules, logger)
}

func startupConsolef(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(msg)
	log.Print(msg)
}

func withoutModule(items []string, target string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(item, target) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func normalizeLegacyConfig(cfg *config.Config) {
	if cfg == nil {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.Analyzer.Protocol), "shimatzu-generic") {
		return
	}
	if cfg.Modules == nil {
		cfg.Modules = map[string]map[string]interface{}{}
	}
	appUpdates := cfg.ModuleSettings("app-updates")
	if appUpdates == nil {
		appUpdates = map[string]interface{}{}
		cfg.Modules["app-updates"] = appUpdates
	}
	currentAppID := strings.TrimSpace(fmt.Sprint(appUpdates["app_id"]))
	if strings.EqualFold(currentAppID, "shimatzu-generic-v3") || currentAppID == "" {
		appUpdates["app_id"] = "shimatzu-generic-reader"
		log.Printf("normalized legacy update app_id for SHIMATZU-GENERIC: %q -> %q", currentAppID, "shimatzu-generic-reader")
	}
}

type processControl struct {
	restartCh chan string
}

func (p *processControl) RequestRestart(reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "configuration updated"
	}
	select {
	case p.restartCh <- reason:
	default:
		select {
		case <-p.restartCh:
		default:
		}
		p.restartCh <- reason
	}
}

func runAppLoop(ctx context.Context, configPath string, defaultModules []string, logger *log.Logger) error {
	restartCh := make(chan string, 1)
	for {
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		cfg.EnabledModules = append([]string(nil), defaultModules...)

		reg := module.NewRegistry()
		builtin.RegisterAll(reg)
		app := runtime.New(cfg, logger, reg)
		app.RegisterService("process-control", &processControl{restartCh: restartCh})
		if err := app.InitModules(cfg.EnabledModules); err != nil {
			return err
		}

		runCtx, cancel := context.WithCancel(ctx)
		errCh := make(chan error, 1)
		go func() {
			errCh <- app.Start(runCtx)
		}()

		select {
		case <-ctx.Done():
			cancel()
			<-errCh
			return nil
		case reason := <-restartCh:
			log.Printf("full runtime restart requested: %s", reason)
			cancel()
			if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("runtime stopped during restart request: %v", err)
			}
		case err := <-errCh:
			cancel()
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}
