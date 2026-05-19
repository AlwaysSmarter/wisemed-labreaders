package file

import (
	"context"
	"path/filepath"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "transport-file" }
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	settings := m.rt.ModuleSettings(m.ID())
	pattern, _ := settings["pattern"].(string)
	if pattern == "" {
		pattern = "*"
	}
	importDir, _ := settings["import_dir"].(string)
	if importDir == "" {
		importDir = "."
	}
	importDir = m.rt.ResolvePath(importDir)
	m.rt.RegisterService("transport-file", map[string]interface{}{
		"import_dir": importDir,
		"pattern":    pattern,
		"glob":       filepath.Join(importDir, pattern),
	})
	m.rt.Logf("file transport active import_dir=%s pattern=%s", importDir, pattern)
	<-ctx.Done()
	return nil
}
