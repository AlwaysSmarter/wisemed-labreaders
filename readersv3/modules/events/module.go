package events

import (
	"net/http"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "events" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.RegisterService("events", map[string]interface{}{
		"features": []string{"log-stream", "broadcast", "ui-refresh-hooks"},
	})
	rt.Handle("/api/events/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"ok":true,"module":"events","features":["log-stream","broadcast","ui-refresh-hooks"]}`))
	}))
	return nil
}
