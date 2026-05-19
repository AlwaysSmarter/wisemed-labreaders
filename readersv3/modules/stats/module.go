package stats

import (
	"net/http"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "stats" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.AddMenu(module.MenuEntry{ID: "stats-daily", Group: "operations", Label: "Statistici zilnice", Path: "/stats/daily", Order: 30})
	rt.RegisterService("stats", map[string]interface{}{
		"features": []string{"daily", "dashboard", "qc", "trend-series"},
	})
	if binder, ok := rt.(interface{ Handle(string, http.Handler) }); ok {
		binder.Handle("/api/stats/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"ok":true,"module":"stats","features":["daily","dashboard","qc"]}`))
		}))
	}
	return nil
}
