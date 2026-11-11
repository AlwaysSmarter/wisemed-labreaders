package wisemedai

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/shared/appmeta"
	"wisemed-labreaders/readersv3/shared/appupdates"
)

var runtimeKeys = map[string][]string{
	"wisemed-api": {"cfg_wisemed_protocol", "cfg_wisemed_ip", "cfg_wisemed_port", "cfg_wisemed_path", "cfg_wisemed_key", "unitate_medicala_id", "tip_de_echipament_id", "cod_echipament", "numar_serial_echipament"},
	"wisemed-ws":  {"enabled", "url", "auth_mode", "tenant_id", "key_id", "issuer", "audience", "subject", "client_id", "device_secret", "secret_file", "token_file", "token_path", "ca_file", "scopes"},
	"app-updates": {"enabled", "app_id", "channel", "base_url", "auto_download", "download_dir"},
	"wisemed-ai":  {"database_url", "secure_cookies", "wisemed_login_enabled", "wsm_identity_enabled", "connector_allowed_hosts"},
}

func (m *Module) runtimeSettings(w http.ResponseWriter, r *http.Request, p principal, action string) {
	if !p.InstanceAdmin {
		fail(w, 403, "instance_admin_required")
		return
	}
	cfg, e := config.Load(m.rt.ConfigPath())
	if e != nil {
		fail(w, 500, "config_read_failed")
		return
	}
	if action == "updates/check" {
		if r.Method != "POST" {
			fail(w, 405, "method_not_allowed")
			return
		}
		s := cfg.ModuleSettings("app-updates")
		key, _ := cfg.ModuleSettings("wisemed-api")["cfg_wisemed_key"].(string)
		base, _ := s["base_url"].(string)
		channel, _ := s["channel"].(string)
		if key == "" {
			respond(w, 200, map[string]any{"status": "not_configured", "message": "Configurați cheia WiseMED API pentru autentificarea la update-server.", "current_version": appmeta.CurrentVersion()})
			return
		}
		client := appupdates.NewClient(base, key, "wisemed-ai", channel)
		client.Client = m.client
		response, e := client.Check(appmeta.CurrentVersion(), runtime.GOOS, runtime.GOARCH)
		if e != nil {
			fail(w, 502, "update_server_unavailable")
			return
		}
		respond(w, 200, response)
		return
	}
	if action == "wsm/reconnect" {
		if r.Method != "POST" {
			fail(w, 405, "method_not_allowed")
			return
		}
		if svc, ok := m.rt.Service("wisemed-ws-client"); ok {
			if client, ok := svc.(interface{ Reconnect() }); ok {
				client.Reconnect()
				respond(w, 200, map[string]bool{"ok": true})
				return
			}
		}
		fail(w, 503, "wsm_unavailable")
		return
	}
	if r.Method == "GET" {
		out := map[string]any{}
		for section, keys := range runtimeKeys {
			raw := cfg.ModuleSettings(section)
			s := map[string]any{}
			for _, k := range keys {
				if k == "database_url" || k == "cfg_wisemed_key" || k == "device_secret" {
					s[k+"_configured"] = raw[k] != nil && raw[k] != ""
					continue
				}
				if v, ok := raw[k]; ok {
					s[k] = v
				}
			}
			out[section] = s
		}
		if svc, ok := m.rt.Service("wisemed-ws-status"); ok {
			if status, ok := svc.(interface{ Status() map[string]interface{} }); ok {
				out["wsm_status"] = status.Status()
			}
		}
		out["version"] = appmeta.CurrentVersion()
		respond(w, 200, out)
		return
	}
	if r.Method != "PUT" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		Sections map[string]map[string]any `json:"sections"`
	}
	if !decode(w, r, &in) {
		return
	}
	patch := map[string]any{}
	for section, values := range in.Sections {
		allowed, ok := runtimeKeys[section]
		if !ok {
			fail(w, 400, "unsupported_section")
			return
		}
		for k, v := range values {
			known := false
			for _, a := range allowed {
				known = known || a == k
			}
			if !known {
				fail(w, 400, "unsupported_setting")
				return
			}
			if k == "database_url" || k == "cfg_wisemed_key" || k == "device_secret" {
				secret, ok := v.(string)
				if !ok {
					fail(w, 400, "secret_must_be_string")
					return
				}
				if strings.TrimSpace(secret) == "" {
					continue
				}
			}
			if section == "app-updates" && k == "app_id" && v != "wisemed-ai" {
				fail(w, 400, "app_id_must_be_wisemed_ai")
				return
			}
			if k == "enabled" || k == "auto_download" || k == "secure_cookies" || k == "wisemed_login_enabled" || k == "wsm_identity_enabled" {
				if _, ok := v.(bool); !ok {
					fail(w, 400, "boolean_setting_required")
					return
				}
			}
			if k == "database_url" {
				if _, e := pgxpool.ParseConfig(v.(string)); e != nil {
					fail(w, 400, "invalid_database_url")
					return
				}
			}
			if k == "connector_allowed_hosts" {
				hosts, ok := v.([]any)
				if !ok {
					fail(w, 400, "hosts_array_required")
					return
				}
				for _, host := range hosts {
					h, ok := host.(string)
					u, e := url.Parse("https://" + h)
					if !ok || e != nil || u.Host != h || u.User != nil || u.Path != "" || u.Hostname() == "" {
						fail(w, 400, "invalid_connector_host")
						return
					}
				}
			}
			if section == "app-updates" && k == "base_url" {
				raw, ok := v.(string)
				u, e := url.Parse(raw)
				if !ok || e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))) {
					fail(w, 400, "invalid_update_server_url")
					return
				}
			}
			patch["modules."+section+"."+k] = v
		}
	}
	// Validate all values before writing; WSM validation and save are delegated to the real shared client.
	if values, ok := in.Sections["wisemed-api"]; ok {
		if scheme, ok := values["cfg_wisemed_protocol"]; ok && scheme != "https" {
			fail(w, 400, "wisemed_requires_https")
			return
		}
		for _, v := range values {
			if _, ok := v.(string); !ok {
				fail(w, 400, "wisemed_settings_must_be_strings")
				return
			}
		}
	}
	if values, ok := in.Sections["wisemed-ws"]; ok {
		svc, exists := m.rt.Service("wisemed-ws-client")
		client, valid := svc.(interface {
			SaveSettings(map[string]interface{}) error
		})
		if !exists || !valid {
			fail(w, 503, "wsm_unavailable")
			return
		}
		if e = client.SaveSettings(values); e != nil {
			fail(w, 400, "invalid_wsm_configuration")
			return
		}
		for k := range values {
			delete(patch, "modules.wisemed-ws."+k)
		}
	}
	m.mu.Lock()
	e = config.Update(m.rt.ConfigPath(), patch)
	m.mu.Unlock()
	if e != nil {
		fail(w, 500, "config_write_failed")
		return
	}
	tx, e := m.db.Begin(r.Context())
	if e == nil {
		defer tx.Rollback(r.Context())
		e = audit(r.Context(), tx, p, "runtime_settings.update", m.ID())
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
