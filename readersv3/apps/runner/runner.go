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
	if ensureResult.Created {
		log.Printf("configuration created from install template: %s", configPath)
	}
	if ensureResult.Merged {
		log.Printf("configuration merged with new defaults from %s", ensureResult.TemplatePath)
	}
	cfg.EnabledModules = append([]string(nil), defaultModules...)
	changed, err := ensureBootstrap(cfg, opts.Reconfigure)
	if err != nil {
		return err
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
		return err
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
	logger := log.New(os.Stdout, "", log.LstdFlags)
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
	return app.Start(ctx)
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
