package module

import (
	"context"
	"net/http"
)

type MenuEntry struct {
	ID    string `json:"id"`
	Group string `json:"group"`
	Label string `json:"label"`
	Path  string `json:"path"`
	Order int    `json:"order"`
}

type Runtime interface {
	ConfigPath() string
	ConfigDir() string
	ReaderID() string
	Logf(format string, args ...interface{})
	ModuleSettings(moduleID string) map[string]interface{}
	ResolvePath(path string) string
	AddMenu(entries ...MenuEntry)
	Handle(pattern string, handler http.Handler)
	Mux() *http.ServeMux
	RegisterService(name string, service interface{})
	Service(name string) (interface{}, bool)
}

type Module interface {
	ID() string
	Init(rt Runtime) error
}

type Starter interface {
	Start(ctx context.Context) error
}
