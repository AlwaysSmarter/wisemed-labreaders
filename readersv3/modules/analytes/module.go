package analytes

import (
	"net/http"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "analytes" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.AddMenu(
		module.MenuEntry{ID: "settings", Group: "admin", Label: "Setari", Path: "/settings", Order: 40},
		module.MenuEntry{ID: "settings-analytes", Group: "admin", Label: "Analize", Path: "/settings/analytes", Order: 41},
	)
	rt.RegisterService("analytes", map[string]interface{}{
		"features": []string{"catalog", "lookup", "protocol-binding"},
	})
	if binder, ok := rt.(interface{ Handle(string, http.Handler) }); ok {
		binder.Handle("/api/analytes/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"ok":true,"module":"analytes","capabilities":["list","edit","bind-qc-targets"]}`))
		}))
	}
	return nil
}
