package signingpad

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (m *Module) openHistory() error {
	path := asString(m.rt.ModuleSettings(m.ID())["log_db_path"])
	if path == "" {
		return nil
	}
	path = m.rt.ResolvePath(path)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS signature_jobs (id INTEGER PRIMARY KEY, created_at TEXT NOT NULL, action TEXT NOT NULL, ok INTEGER NOT NULL, message TEXT NOT NULL)`); err != nil {
		db.Close()
		return err
	}
	m.history = db
	return nil
}
func (m *Module) recordAction(cmd padCommand, response map[string]interface{}) {
	if cmd.Action == "health" {
		return
	}
	ok := response["ok"] == true
	m.rt.Logf("esignature action=%s ok=%t", cmd.Action, ok)
	if m.history == nil {
		return
	}
	if _, err := m.history.Exec(`INSERT INTO signature_jobs(created_at,action,ok,message) VALUES(?,?,?,?)`, time.Now().UTC().Format(time.RFC3339), cmd.Action, ok, asString(response["message"])); err != nil {
		m.rt.Logf("esignature history: %v", err)
	}
}
func (m *Module) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !m.originAllowed(r) {
		http.Error(w, "origin not allowed", 403)
		return
	}
	jobs := []map[string]interface{}{}
	if m.history != nil {
		rows, err := m.history.Query(`SELECT created_at,action,ok,message FROM signature_jobs ORDER BY id DESC LIMIT 100`)
		if err != nil {
			http.Error(w, "history unavailable", 500)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var date, action, message string
			var ok bool
			if err := rows.Scan(&date, &action, &ok, &message); err != nil {
				http.Error(w, "history unavailable", 500)
				return
			}
			jobs = append(jobs, map[string]interface{}{"created_at": date, "action": action, "ok": ok, "message": message})
		}
		if rows.Err() != nil {
			http.Error(w, "history unavailable", 500)
			return
		}
	}
	m.writeJSON(w, 200, map[string]interface{}{"jobs": jobs})
}
func (m *Module) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !m.originAllowed(r) {
		http.Error(w, "origin not allowed", 403)
		return
	}
	stats := []map[string]interface{}{}
	if m.history != nil {
		rows, err := m.history.Query(`SELECT substr(created_at,1,10),sum(action='confirm' AND ok=1),sum(action='cancel' AND ok=1),sum(ok=0) FROM signature_jobs GROUP BY substr(created_at,1,10) ORDER BY 1 DESC LIMIT 30`)
		if err != nil {
			http.Error(w, "stats unavailable", 500)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var date string
			var confirmed, cancelled, failed int
			if err := rows.Scan(&date, &confirmed, &cancelled, &failed); err != nil {
				http.Error(w, "stats unavailable", 500)
				return
			}
			stats = append(stats, map[string]interface{}{"date": date, "confirmed": confirmed, "cancelled": cancelled, "failed": failed})
		}
		if rows.Err() != nil {
			http.Error(w, "stats unavailable", 500)
			return
		}
	}
	m.writeJSON(w, 200, map[string]interface{}{"stats": stats})
}
