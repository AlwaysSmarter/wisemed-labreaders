package server

import (
	"net/http"
	"strconv"
)

func (s *Server) adminLogDates(w http.ResponseWriter, r *http.Request) {
	_, _, ok := s.requireAdmin(w, r, false)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if s.logs == nil {
		adminError(w, 503, "daily log storage unavailable")
		return
	}
	dates, err := s.logs.Dates()
	if err != nil {
		adminError(w, 500, "cannot list daily logs")
		return
	}
	writeJSON(w, 200, map[string]interface{}{"dates": dates, "today": s.logs.Today(), "timezone": "UTC"})
}
func (s *Server) adminLogs(w http.ResponseWriter, r *http.Request) {
	_, sess, ok := s.requireAdmin(w, r, false)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if s.logs == nil {
		adminError(w, 503, "daily log storage unavailable")
		return
	}
	day := r.URL.Query().Get("date")
	if day == "" {
		day = s.logs.Today()
	}
	cursor := int64(-1)
	if value := r.URL.Query().Get("cursor"); value != "" {
		var err error
		cursor, err = strconv.ParseInt(value, 10, 64)
		if err != nil || cursor < -1 {
			adminError(w, 400, "invalid log cursor")
			return
		}
	}
	if previous := r.URL.Query().Get("cursor_date"); previous != "" && previous != day {
		cursor = 0
	}
	page, err := s.logs.Read(day, cursor, sess.TenantID)
	if err != nil {
		adminError(w, 400, "cannot read log; use a valid YYYY-MM-DD date")
		return
	}
	writeJSON(w, 200, page)
}
