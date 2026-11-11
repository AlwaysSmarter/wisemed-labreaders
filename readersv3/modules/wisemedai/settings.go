package wisemedai

import (
	"gopkg.in/yaml.v3"
	"net/http"
	"os"
	"strings"
)

func (m *Module) openAIKey() string {
	if v := os.Getenv(m.cfg.OpenAIKeyEnv); v != "" {
		return v
	}
	b, _ := os.ReadFile(m.rt.ResolvePath(m.cfg.OpenAIKeyFile))
	return strings.TrimSpace(string(b))
}
func (m *Module) saveSettings(w http.ResponseWriter, r *http.Request, p principal) {
	var in struct {
		Provider  string  `json:"provider"`
		Model     string  `json:"model"`
		Budget    float64 `json:"monthly_budget_usd"`
		Input     float64 `json:"input_usd_per_million"`
		Output    float64 `json:"output_usd_per_million"`
		Retention int     `json:"retention_days"`
		Key       string  `json:"openai_key"`
	}
	if !decode(w, r, &in) {
		return
	}
	if (in.Provider != "mock" && in.Provider != "openai") || in.Model == "" || in.Budget <= 0 || in.Retention < 1 || in.Retention > 3650 || in.Input < 0 || in.Output < 0 || len(in.Key) > 512 {
		fail(w, 400, "invalid_settings")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, e := os.ReadFile(m.rt.ConfigPath())
	if e != nil {
		fail(w, 500, "config_read_failed")
		return
	}
	var cfg map[string]any
	if yaml.Unmarshal(raw, &cfg) != nil {
		fail(w, 500, "invalid_deployment_yaml")
		return
	}
	mods, ok := cfg["modules"].(map[string]any)
	if !ok {
		fail(w, 500, "invalid_deployment_modules")
		return
	}
	s, ok := mods[m.ID()].(map[string]any)
	if !ok {
		s = map[string]any{}
	}
	s["provider"] = in.Provider
	s["model"] = in.Model
	s["monthly_budget_usd"] = in.Budget
	s["input_usd_per_million"] = in.Input
	s["output_usd_per_million"] = in.Output
	s["retention_days"] = in.Retention
	mods[m.ID()] = s
	b, e := yaml.Marshal(cfg)
	if e != nil {
		fail(w, 500, "config_write_failed")
		return
	}
	if in.Key != "" {
		if e = os.WriteFile(m.rt.ResolvePath(m.cfg.OpenAIKeyFile), []byte(in.Key), 0600); e != nil {
			fail(w, 500, "secret_write_failed")
			return
		}
	}
	tmp := m.rt.ConfigPath() + ".ai-tmp"
	if e = os.WriteFile(tmp, b, 0600); e == nil {
		e = os.Rename(tmp, m.rt.ConfigPath())
	}
	if e != nil {
		fail(w, 500, "config_write_failed")
		return
	}
	tx, e := m.db.Begin(r.Context())
	if e == nil {
		defer tx.Rollback(r.Context())
		e = audit(r.Context(), tx, p, "settings.update", m.ID())
		if e == nil {
			e = tx.Commit(r.Context())
		}
	}
	if e != nil {
		fail(w, 503, "settings_saved_audit_failed")
		return
	}
	respond(w, 200, map[string]any{"ok": true, "restart_required": true})
}
