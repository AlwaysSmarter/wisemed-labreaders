package analytemanagement

import (
	"net/http"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "analyte-management" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.AddMenu(module.MenuEntry{
		ID:    "settings-analyte-management",
		Group: "admin",
		Label: "Management analize",
		Path:  "/settings/analytes/manage",
		Order: 44,
	})
	rt.RegisterService("analyte-management", map[string]interface{}{
		"features": []string{
			"crud",
			"code-mapping",
			"qc-target-binding",
			"per-reader-customization",
		},
	})
	rt.Handle("/api/analytes/management/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"ok":true,"module":"analyte-management","capabilities":["crud","code-mapping","qc-target-binding","bulk-edit"]}`))
	}))
	return nil
}
