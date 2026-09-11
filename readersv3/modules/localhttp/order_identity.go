package localhttp

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	model "wisemed-labreaders/readersv3/modules/core/model"
)

type orderIdentityStore interface {
	CorrectOrderID(model.OrderIDChange, string) (model.Order, error)
}

func (m *Module) handleOrderIDChange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return
	}
	if _, ok := m.currentSession(r); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Autentificare necesară"})
		return
	}
	var change model.OrderIDChange
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if err := json.NewDecoder(r.Body).Decode(&change); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Date invalide"})
		return
	}
	service, _ := m.rt.Service("storage")
	store, ok := service.(orderIdentityStore)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"error": "storage unavailable"})
		return
	}
	order, err := store.CorrectOrderID(change, m.auditActor(r))
	if err != nil {
		code := http.StatusInternalServerError
		switch {
		case errors.Is(err, model.ErrOrderIdentityInvalid):
			code = http.StatusBadRequest
		case errors.Is(err, model.ErrOrderIdentityConflict):
			code = http.StatusConflict
		case errors.Is(err, sql.ErrNoRows):
			code = http.StatusNotFound
		}
		writeJSON(w, code, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "order": order})
}
