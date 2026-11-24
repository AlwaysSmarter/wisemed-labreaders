package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"

	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
)

type App struct {
	debugVerbosity atomic.Int32
	cfg            *config.Config
	logger         *log.Logger
	mux            *http.ServeMux
	registry       *module.Registry

	mu       sync.RWMutex
	modules  []module.Module
	menu     []module.MenuEntry
	services map[string]interface{}
}

func New(cfg *config.Config, logger *log.Logger, registry *module.Registry) *App {
	app := &App{
		cfg:      cfg,
		logger:   logger,
		mux:      http.NewServeMux(),
		registry: registry,
		services: map[string]interface{}{},
	}
	app.services["reader-config"] = map[string]interface{}{
		"id":            cfg.Reader.ID,
		"label":         cfg.Reader.Label,
		"analyzer_name": cfg.Reader.AnalyzerName,
		"analyzer_code": cfg.Reader.AnalyzerCode,
		"db_name":       cfg.Reader.DBName,
	}
	app.services["analyzer-config"] = map[string]interface{}{
		"comm_type": cfg.Analyzer.CommType,
		"protocol":  cfg.Analyzer.Protocol,
	}
	app.services["app-config"] = map[string]interface{}{
		"enabled_modules": append([]string(nil), cfg.EnabledModules...),
	}
	app.mux.HandleFunc("/healthz", app.handleHealth)
	app.mux.HandleFunc("/api/menu", app.handleMenu)
	app.mux.HandleFunc("/api/modules", app.handleModules)
	return app
}

func (a *App) ConfigPath() string { return a.cfg.Path() }
func (a *App) ConfigDir() string  { return filepath.Dir(a.cfg.Path()) }
func (a *App) ReaderID() string   { return a.cfg.Reader.ID }
func (a *App) Logf(format string, args ...interface{}) {
	a.logger.Printf(format, args...)
}

func (a *App) ModuleSettings(moduleID string) map[string]interface{} {
	settings := a.cfg.ModuleSettings(moduleID)
	if level := a.debugVerbosity.Load(); moduleID == "logging" && level != 0 {
		next := make(map[string]interface{}, len(settings)+1)
		for k, v := range settings {
			next[k] = v
		}
		next["verbose_level"] = int(level)
		return next
	}
	return settings
}

// ChangeDebugVerbosity updates logging in place without restarting modules or WSM.
func (a *App) ChangeDebugVerbosity(level int) error {
	if level < 1 || level > 5 {
		return errors.New("verbose_level must be an integer from 1 to 5")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := config.Update(a.ConfigPath(), map[string]interface{}{"logging.verbose_level": level, "modules.logging.verbose_level": level}); err != nil {
		return err
	}
	a.debugVerbosity.Store(int32(level))
	return nil
}

func (a *App) ResolvePath(path string) string {
	if path == "" {
		return a.ConfigDir()
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(a.ConfigDir(), path)
}

func (a *App) Mux() *http.ServeMux {
	return a.mux
}

func (a *App) AddMenu(entries ...module.MenuEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.menu = append(a.menu, entries...)
	sort.SliceStable(a.menu, func(i, j int) bool {
		if a.menu[i].Group == a.menu[j].Group {
			return a.menu[i].Order < a.menu[j].Order
		}
		return a.menu[i].Group < a.menu[j].Group
	})
}

func (a *App) Handle(pattern string, handler http.Handler) {
	a.mux.Handle(pattern, handler)
}

func (a *App) RegisterService(name string, service interface{}) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.services[name] = service
}

func (a *App) Service(name string) (interface{}, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	item, ok := a.services[name]
	return item, ok
}

func (a *App) InitModules(moduleIDs []string) error {
	for _, id := range moduleIDs {
		item, err := a.registry.Build(id)
		if err != nil {
			return err
		}
		if err := item.Init(a); err != nil {
			return err
		}
		a.modules = append(a.modules, item)
	}
	return nil
}

func (a *App) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, len(a.modules))
	var running sync.WaitGroup
	for _, item := range a.modules {
		starter, ok := item.(module.Starter)
		if !ok {
			continue
		}
		running.Add(1)
		go func(st module.Starter) {
			defer running.Done()
			errCh <- st.Start(ctx)
		}(starter)
	}

	var err error
	select {
	case <-ctx.Done():
	case err = <-errCh:
	}
	cancel()
	running.Wait()
	return err
}

func (a *App) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":     true,
		"reader": a.cfg.Reader,
	})
}

func (a *App) handleMenu(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":   true,
		"menu": a.menu,
	})
}

func (a *App) handleModules(w http.ResponseWriter, _ *http.Request) {
	items := make([]string, 0, len(a.modules))
	for _, item := range a.modules {
		items = append(items, item.ID())
	}
	a.mu.RLock()
	services := make([]string, 0, len(a.services))
	for name := range a.services {
		services = append(services, name)
	}
	a.mu.RUnlock()
	sort.Strings(services)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"modules":  items,
		"services": services,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
