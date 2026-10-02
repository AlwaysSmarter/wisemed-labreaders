package server

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"time"
)

func (s *Server) equipmentStatus(tenant, id string) map[string]interface{} {
	result := map[string]interface{}{"equipment_id": id, "online": false}
	for _, c := range s.hub.Snapshot(tenant) {
		if c.EquipmentID == id {
			result["online"] = true
			result["connection"] = c
			result["connected_seconds"] = int(time.Since(c.ConnectedAt).Seconds())
			break
		}
	}
	return result
}
func (s *Server) equipmentHTTP(w http.ResponseWriter, r *http.Request) {
	cfg := s.current()
	c, e := authenticate(cfg, r, false)
	if e != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	if !c.Has("connections:read") || !allowedOrigin(r, cfg.Tenants[c.TenantID]) {
		http.Error(w, "forbidden", 403)
		return
	}
	writeJSON(w, 200, s.equipmentStatus(c.TenantID, chi.URLParam(r, "id")))
}
