package siui

import (
	"net/http"
	"runtime"
)

func (m *Module) tokenPINHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfg
	if cfg.pin == nil {
		fail(w, 503, "PIN storage unavailable")
		return
	}
	if r.Method == "GET" {
		set, blocked, e := cfg.pin.status(cfg)
		if e != nil {
			fail(w, 500, "PIN storage unavailable")
			return
		}
		failures, e := cfg.pin.failureCount(cfg)
		if e != nil {
			fail(w, 500, "PIN storage unavailable")
			return
		}
		hint := ""
		if set {
			hint = "<hidden>"
		}
		respond(w, 200, map[string]any{"ok": true, "configured": set, "hint": hint, "blocked": blocked, "failed_attempts": failures, "max_attempts": pinFailureLimit, "manual_required": runtime.GOOS == "linux", "pkcs11_module": cfg.PKCS11Module})
		return
	}
	if r.Method != "PUT" {
		fail(w, 405, "method not allowed")
		return
	}
	if m.pending > 0 {
		fail(w, 409, "Așteptați încheierea operației active pe token")
		return
	}
	var in struct {
		PIN    string `json:"pin"`
		Forget bool   `json:"forget"`
		Manual bool   `json:"manual"`
	}
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, "Invalid PIN request")
		return
	}
	if e := cfg.pin.save(cfg, in.PIN, in.Forget, in.Manual); e != nil {
		fail(w, 400, e.Error())
		return
	}
	m.rt.Logf("SIUI token PIN settings updated (secret omitted)")
	respond(w, 200, map[string]any{"ok": true})
}
