package genericfile

import (
	"net/http"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "protocol-generic-file" }
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	if binder, ok := rt.(interface{ Handle(string, http.Handler) }); ok {
		binder.Handle("/api/protocol/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"ok":true,"protocol":"generic-file"}`))
		}))
	}
	return nil
}
