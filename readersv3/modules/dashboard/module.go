package dashboard

import (
	"net/http"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "dashboard" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.AddMenu(module.MenuEntry{ID: "dashboard", Group: "operations", Label: "Dashboard", Path: "/dashboard", Order: 11})
	rt.RegisterService("dashboard", map[string]interface{}{
		"features": []string{"daily-stats", "qc-summary", "connectivity", "timeline"},
	})
	rt.Handle("/api/dashboard/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"ok":true,"module":"dashboard","features":["daily-stats","qc-summary","connectivity","timeline"]}`))
	}))
	return nil
}
