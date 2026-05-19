package help

import (
	"io/fs"
	"net/http"
	"os"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "help" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.AddMenu(module.MenuEntry{ID: "help", Group: "secondary", Label: "Ajutor", Path: "/help", Order: 50})
	settings := rt.ModuleSettings(m.ID())
	helpDir, _ := settings["help_dir"].(string)
	if helpDir == "" {
		helpDir = "./help"
	}
	helpDir = rt.ResolvePath(helpDir)
	rt.RegisterService("help", map[string]interface{}{
		"dir": helpDir,
	})
	rt.Handle("/help", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/help/", http.StatusTemporaryRedirect)
	}))
	if stat, err := os.Stat(helpDir); err == nil && stat.IsDir() {
		sub, err := fs.Sub(os.DirFS(helpDir), ".")
		if err == nil {
			rt.Handle("/help/", http.StripPrefix("/help/", http.FileServer(http.FS(sub))))
			return nil
		}
	}
	rt.Handle("/help/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Help content directory not found. Configure modules.help.help_dir per reader."))
	}))
	return nil
}
