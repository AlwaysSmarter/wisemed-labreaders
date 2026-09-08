package signingpad

import (
	"encoding/json"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"wisemed-labreaders/readersv3/core/config"
)

type padSettings struct {
	Manufacturer   string `json:"manufacturer"`
	PadType        string `json:"pad_type"`
	Model          string `json:"model"`
	TimeoutSeconds int    `json:"session_timeout_seconds"`
	DeviceIndex    int    `json:"device_index"`
}

func (m *Module) initNative() {
	s := m.rt.ModuleSettings(m.ID())
	m.nativeSettings = padSettings{firstNonEmpty(asString(s["manufacturer"]), "signotec"), firstNonEmpty(asString(s["pad_type"]), "omega"), asString(s["model"]), intSetting(s, "session_timeout_seconds", 180), intSetting(s, "device_index", 0)}
	m.driverFactory = openNative
}
func (m *Module) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return false
	}
	if parsed.Host == r.Host && ((r.TLS != nil && parsed.Scheme == "https") || (r.TLS == nil && parsed.Scheme == "http")) {
		return true
	}
	allowedOrigins := asString(m.rt.ModuleSettings(m.ID())["cors_allowed_origins"])
	if parseBool(asString(m.rt.ModuleSettings(m.ID())["shared_http"])) {
		if svc, ok := m.rt.Service("local-http-control"); ok {
			if provider, ok := svc.(interface{ CORSAllowedOrigins() string }); ok {
				allowedOrigins = provider.CORSAllowedOrigins()
			}
		}
	}
	for _, allowed := range strings.FieldsFunc(allowedOrigins, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '\r' }) {
		if strings.TrimSpace(allowed) == origin {
			return true
		}
	}
	return false
}
func (m *Module) handlePadSettings(w http.ResponseWriter, r *http.Request) {
	if !m.originAllowed(r) {
		http.Error(w, "origin not allowed", 403)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		s := m.nativeSettings
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&s); err != nil {
			http.Error(w, "invalid settings", 400)
			return
		}
		if s.Manufacturer != "signotec" || s.PadType != "omega" || len(s.Model) > 128 || s.TimeoutSeconds < 1 || s.TimeoutSeconds > 600 || s.DeviceIndex < 0 || s.DeviceIndex > 31 {
			http.Error(w, "use signotec/omega, a model up to 128 characters, timeout 1–600 seconds and device index 0–31", 400)
			return
		}
		if err := config.Update(m.rt.ConfigPath(), map[string]interface{}{"modules.signing-pad.manufacturer": s.Manufacturer, "modules.signing-pad.pad_type": s.PadType, "modules.signing-pad.model": s.Model, "modules.signing-pad.session_timeout_seconds": s.TimeoutSeconds, "modules.signing-pad.device_index": s.DeviceIndex}); err != nil {
			http.Error(w, "could not save settings", 500)
			return
		}
		m.nativeSettings = s
	} else if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	m.writeJSON(w, 200, m.nativeSettings)
}

type padCommand struct {
	ID     string `json:"id"`
	Action string `json:"action"`
}

func (m *Module) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !m.originAllowed(r) {
		http.Error(w, "origin not allowed", 403)
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: m.originAllowed}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-m.stopping:
			conn.Close()
		case <-finished:
		}
	}()
	conn.SetReadLimit(4096)
	timeout := m.sessionTimeout()
	if timeout < time.Second || timeout > 10*time.Minute {
		timeout = 180 * time.Second
	}
	// A session has an absolute deadline, so repeated commands cannot reserve the pad forever.
	conn.SetReadDeadline(time.Now().Add(timeout))
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	session := &padSession{m: m}
	defer session.close()
	var first json.RawMessage
	if err := conn.ReadJSON(&first); err != nil {
		return
	}
	var header struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal(first, &header); err != nil {
		return
	}
	if header.Cmd != "" {
		m.handleLegacyConnection(conn, first)
		return
	}
	for {
		var cmd padCommand
		if first != nil {
			if err := json.Unmarshal(first, &cmd); err != nil {
				return
			}
			first = nil
		} else if err := conn.ReadJSON(&cmd); err != nil {
			return
		}
		response := session.execute(cmd)
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteJSON(response); err != nil {
			return
		}
	}
}

func (m *Module) padSettingsSnapshot() padSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.nativeSettings
}
