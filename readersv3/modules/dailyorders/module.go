package dailyorders

import (
	"net/http"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "daily-orders" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.AddMenu(module.MenuEntry{ID: "orders", Group: "operations", Label: "Cereri analize", Path: "/orders", Order: 20})
	if binder, ok := rt.(interface{ Handle(string, http.Handler) }); ok {
		binder.Handle("/api/orders/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"ok":true,"module":"daily-orders","features":["list","rounds","racks","layout-customization"]}`))
		}))
	}
	return nil
}
