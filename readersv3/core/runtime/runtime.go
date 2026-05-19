package runtime

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"sync"

	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
)

type App struct {
	cfg      *config.Config
	logger   *log.Logger
	mux      *http.ServeMux
	registry *module.Registry

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
	return a.cfg.ModuleSettings(moduleID)
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
	errCh := make(chan error, len(a.modules))
	for _, item := range a.modules {
		starter, ok := item.(module.Starter)
		if !ok {
			continue
		}
		go func(st module.Starter) {
			errCh <- st.Start(ctx)
		}(starter)
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
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
