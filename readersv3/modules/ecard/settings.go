package ecard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"wisemed-labreaders/readersv3/core/config"
)

func (m *Module) saveSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CEASOffline *bool    `json:"ceas_offline,omitempty"`
		SDKOrder    []string `json:"sdk_order"`
		Reader      string   `json:"reader_name"`
		UMHost      string   `json:"um_host"`
		UMPort      int      `json:"um_port"`
	}
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, e)
		return
	}
	b, _ := json.Marshal(in)
	var values map[string]interface{}
	_ = json.Unmarshal(b, &values)
	if _, e := readSettings(values); e != nil {
		fail(w, 400, e)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.active) != 0 || len(m.queue) != 0 {
		fail(w, 409, errors.New("wait for queued/running card jobs"))
		return
	}
	cfg, e := config.Load(m.rt.ConfigPath())
	if e != nil {
		fail(w, 500, errors.New("cannot load config"))
		return
	}
	next := m.settings
	next.SDKOrder, next.Reader, next.UMHost, next.UMPort = in.SDKOrder, in.Reader, in.UMHost, in.UMPort
	if in.CEASOffline != nil {
		next.CEASOffline = *in.CEASOffline
	}
	firewallChanged := ceasFirewallChanged(m.settings, next)
	if firewallChanged {
		if e = applyCEASFirewall(r.Context(), next); e != nil {
			rollback := applyCEASFirewall(context.Background(), m.settings)
			if rollback != nil {
				e = errors.Join(e, rollback)
			}
			fail(w, 500, e)
			return
		}
	}
	settings := cfg.ModuleSettings("ecard")
	for k, v := range values {
		settings[k] = v
	}
	if e = cfg.Save(); e != nil {
		err := errors.New("cannot save config")
		if firewallChanged {
			err = errors.Join(err, applyCEASFirewall(context.Background(), m.settings))
		}
		fail(w, 500, err)
		return
	}
	// Publish only editable fields after persistence, under the admission lock.
	m.settings.CEASOffline = next.CEASOffline
	m.settings.SDKOrder = append([]string(nil), in.SDKOrder...)
	m.settings.Reader = in.Reader
	m.settings.UMHost = in.UMHost
	m.settings.UMPort = in.UMPort
	respond(w, 200, map[string]interface{}{"ok": true, "restart_required": false, "saved": in})
}

func ceasFirewallChanged(previous, next Settings) bool {
	return previous.CEASOffline != next.CEASOffline ||
		(next.CEASOffline && (previous.UMHost != next.UMHost || previous.UMPort != next.UMPort))
}

func (m *Module) settingsSnapshot() Settings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings
}
