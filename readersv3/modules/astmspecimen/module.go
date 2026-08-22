package astmspecimen

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "astm-specimen-settings" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	// The local-http SPA owns the settings route. This module only provides the
	// configuration API, keeping the ASTM editor visually consistent with Analize.
	rt.Handle("/api/settings/astm-specimens", http.HandlerFunc(m.api))
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (m *Module) api(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "settings": m.settings()})
	case http.MethodPut:
		var req struct {
			Default string `json:"default"`
			Rows    []struct {
				Source string `json:"source"`
				Target string `json:"target"`
			} `json:"rows"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "JSON invalid"})
			return
		}
		mapping := map[string]string{}
		for _, row := range req.Rows {
			if source, target := strings.ToUpper(strings.TrimSpace(row.Source)), strings.TrimSpace(row.Target); source != "" && target != "" {
				mapping[source] = target
			}
		}
		if err := config.Update(m.rt.ConfigPath(), map[string]interface{}{"modules.protocol-astm.specimen_code_default": firstNonEmpty(req.Default, "1"), "modules.protocol-astm.specimen_code_map": mapping}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "restart_required": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *Module) settings() map[string]interface{} {
	settings := m.rt.ModuleSettings("protocol-astm")
	rows := []map[string]string{}
	values, _ := settings["specimen_code_map"].(map[string]interface{})
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		rows = append(rows, map[string]string{"source": key, "target": strings.TrimSpace(toString(values[key]))})
	}
	return map[string]interface{}{"default": firstNonEmpty(toString(settings["specimen_code_default"]), "1"), "rows": rows}
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func toString(value interface{}) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
