package signingpad

import (
	"encoding/json"
	"net/http"
)

// HTTP callers use the same session ownership and deadline rules as WiseMED WS.
func (m *Module) handleNativeCommand(w http.ResponseWriter, r *http.Request) {
	if !m.originAllowed(r) {
		http.Error(w, "origin not allowed", 403)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodOptions {
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var request struct {
		ID        string `json:"id"`
		Action    string `json:"action"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	response, handled, err := m.HandleWSAction("esignature."+request.Action, map[string]interface{}{"id": request.ID, "session_id": request.SessionID})
	if err != nil {
		m.writeJSON(w, 409, map[string]interface{}{"id": request.ID, "ok": false, "message": err.Error()})
		return
	}
	if !handled {
		http.Error(w, "unknown action", 400)
		return
	}
	response["id"] = request.ID
	m.writeJSON(w, 200, response)
}
