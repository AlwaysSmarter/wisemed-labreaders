package localhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"wisemed-labreaders/readersv3/modules/ws"
)

type wssControl interface {
	Status() map[string]interface{}
	Settings() map[string]interface{}
	SaveSettings(map[string]interface{}) error
	Trace() []ws.Trace
	Reconnect()
	Disconnect()
	DebugRequest(context.Context, string, string, string) (map[string]interface{}, error)
	Exchange(context.Context, ws.Envelope) (map[string]interface{}, error)
	Request(context.Context, string, map[string]interface{}) (map[string]interface{}, error)
}

func (m *Module) wss() wssControl {
	svc, ok := m.rt.Service("wisemed-ws-client")
	if !ok {
		return nil
	}
	client, _ := svc.(wssControl)
	return client
}
func (m *Module) handleWSS(w http.ResponseWriter, r *http.Request) {
	sess, authenticated := m.currentSession(r)
	if !authenticated {
		writeJSON(w, 401, map[string]interface{}{"ok": false, "error": "authentication required"})
		return
	}
	if r.URL.Path != "/api/wss/status" && sess.UserType > 0 {
		writeJSON(w, 403, map[string]interface{}{"ok": false, "error": "WSS administration requires an administrator"})
		return
	}
	c := m.wss()
	if c == nil {
		writeJSON(w, 503, map[string]interface{}{"ok": false, "error": "WSS module unavailable"})
		return
	}
	fail := func(e error) { writeJSON(w, 400, map[string]interface{}{"ok": false, "error": e.Error()}) }
	// Require same-origin JSON for mutation in addition to the existing session.
	if r.Method != "GET" {
		if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			fail(errors.New("application/json required"))
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host != r.Host {
				fail(errors.New("same-origin request required"))
				return
			}
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	var result interface{}
	switch r.URL.Path {
	case "/api/wss/status":
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		settings := map[string]interface{}{}
		trace := []ws.Trace{}
		if sess.UserType <= 0 {
			publicSettings := c.Settings()
			for _, key := range []string{"enabled", "url", "auth_mode", "tenant_id", "key_id", "issuer", "audience", "subject", "client_id", "secret_file", "device_secret_configured", "token_file", "token_path", "ca_file", "scopes"} {
				if v, ok := publicSettings[key]; ok {
					settings[key] = v
				}
			}
			trace = c.Trace()
		}
		writeJSON(w, 200, map[string]interface{}{"ok": true, "status": c.Status(), "settings": settings, "trace": trace})
		return
	case "/api/wss/settings":
		if r.Method != "PUT" {
			w.WriteHeader(405)
			return
		}
		var settings map[string]interface{}
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768)).Decode(&settings); e != nil {
			fail(e)
			return
		}
		if e := c.SaveSettings(settings); e != nil {
			fail(e)
			return
		}
		result = c.Status()
	case "/api/wss/control", "/api/wss/debug", "/api/wss/open":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var in struct {
			Action       string `json:"action"`
			ConnectionID string `json:"connection_id"`
			EquipmentID  string `json:"equipment_id"`
			Text         string `json:"text"`
		}
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768)).Decode(&in); e != nil {
			fail(e)
			return
		}
		switch r.URL.Path {
		case "/api/wss/control":
			switch in.Action {
			case "disconnect":
				c.Disconnect()
			case "reconnect":
				c.Reconnect()
			default:
				fail(errors.New("invalid control action"))
				return
			}
			result = c.Status()
		case "/api/wss/debug":
			var e error
			result, e = c.DebugRequest(ctx, in.Action, in.ConnectionID, in.Text)
			if e != nil {
				fail(e)
				return
			}
		case "/api/wss/open":
			data, e := c.Request(ctx, "control.ticket", map[string]interface{}{"equipment_id": in.EquipmentID})
			if e != nil {
				fail(e)
				return
			}
			parsed, e := url.Parse(asString(c.Settings()["url"]))
			if e != nil || parsed.Scheme != "wss" {
				fail(errors.New("WSS URL unavailable"))
				return
			}
			parsed.Scheme = "https"
			parsed.Path = "/control/"
			origin := "http://" + r.Host
			if r.TLS != nil {
				origin = "https://" + r.Host
			}
			parsed.RawQuery = url.Values{"equipment_id": []string{in.EquipmentID}, "opener_origin": []string{origin}}.Encode()
			writeJSON(w, 200, map[string]interface{}{"ok": true, "url": parsed.String(), "ticket": data["ticket"]})
			return
		}
	case "/api/wss/bridge":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var in map[string]interface{}
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 6*1024*1024)).Decode(&in); e != nil {
			fail(e)
			return
		}
		id := asString(in["connection_id"])
		if id == "" {
			fail(errors.New("select a connected equipment"))
			return
		}
		delete(in, "connection_id")
		var e error
		result, e = c.Exchange(ctx, ws.Envelope{Type: "command", Target: &ws.Target{Mode: "connection", ConnectionID: id}, Payload: map[string]interface{}{"command": "api.request", "args": in}})
		if e != nil {
			fail(e)
			return
		}
	default:
		w.WriteHeader(404)
		return
	}
	writeJSON(w, 200, map[string]interface{}{"ok": true, "data": result})
}

func (m *Module) wssStatus() map[string]interface{} {
	if c := m.wss(); c != nil {
		return c.Status()
	}
	return map[string]interface{}{"connected": false, "phase": "disabled"}
}
