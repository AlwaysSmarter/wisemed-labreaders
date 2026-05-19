package qc

import (
	"encoding/json"
	"net/http"
	"strings"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "qc" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	levels := m.configuredLevels()
	rt.AddMenu(
		module.MenuEntry{ID: "qc", Group: "operations", Label: "Controlul calitatii", Path: "/qc", Order: 30},
		module.MenuEntry{ID: "settings-qc", Group: "admin", Label: "Setari QC", Path: "/settings/qc", Order: 43},
		module.MenuEntry{ID: "settings-westgard", Group: "admin", Label: "Westgard", Path: "/settings/qc/westgard", Order: 46},
	)
	rt.RegisterService("qc", map[string]interface{}{
		"features": []string{"targets", "westgard", "statistics", "control-level-filtering"},
		"levels":   levels,
	})
	if binder, ok := rt.(interface{ Handle(string, http.Handler) }); ok {
		binder.Handle("/api/qc/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":       true,
				"module":   "qc",
				"features": []string{"targets", "westgard", "statistics"},
				"levels":   levels,
			})
		}))
	}
	return nil
}

func (m *Module) configuredLevels() []string {
	settings := m.rt.ModuleSettings(m.ID())
	raw, ok := settings["levels"]
	if !ok {
		return defaultQCLevels()
	}
	switch values := raw.(type) {
	case []interface{}:
		out := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				continue
			}
			if text = strings.TrimSpace(text); text != "" {
				out = append(out, text)
			}
		}
		if len(out) > 0 {
			return out
		}
	case []string:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				out = append(out, value)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return defaultQCLevels()
}

func defaultQCLevels() []string {
	return []string{"negativ", "pozitiv", "pcrescut", "pscazut", "nivel1", "nivel2", "nivel3", "nivel4", "nivel5"}
}
