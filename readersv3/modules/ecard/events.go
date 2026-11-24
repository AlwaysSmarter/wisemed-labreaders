package ecard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (m *Module) emitEvent(owner, job, source, kind string, data interface{}) {
	if job == "" && source == "pcsc" && (kind == "card_removed" || kind == "card_inserted" || kind == "reader_disconnected" || kind == "monitor_unavailable") {
		m.cardEpoch.Add(1)
		m.rememberedPIN.clear()
		m.mu.Lock()
		for _, a := range m.active {
			if a.cachedPIN != nil {
				a.cachedPIN.clear()
			}
		}
		m.mu.Unlock()
	}
	// Correlate hardware observations with operations active at that moment.
	if source == "pcsc" && job == "" {
		type target struct{ owner, job, sdk string }
		targets := []target{}
		m.mu.Lock()
		for id, a := range m.active {
			targets = append(targets, target{a.owner, id, a.sdk})
		}
		m.mu.Unlock()
		for _, t := range targets {
			m.emitEvent(t.owner, t.job, source, kind, map[string]interface{}{"hardware": data, "sdk": t.sdk})
		}
	}
	v := map[string]interface{}{"time": time.Now().UTC().Format(time.RFC3339Nano), "job": job, "source": source, "event": kind, "type": eventType(source, kind), "subtype": kind, "data": data}
	raw, e := json.Marshal(v)
	full := raw
	if len(raw) > 65536 {
		v["data"] = "[event payload exceeds 64 KiB; see operation results]"
		raw, e = json.Marshal(v)
	}
	if e != nil {
		return
	}
	var safe interface{}
	if json.Unmarshal(raw, &safe) != nil {
		return
	}
	explainErrors(safe)
	b, e := m.store.seal(cleanHistory(safe), "ecard-event:"+owner)
	if e != nil {
		return
	}
	jobBlob := b
	if job != "" && len(full) > 65536 {
		var value interface{}
		if e = json.Unmarshal(full, &value); e != nil {
			return
		}
		explainErrors(value)
		if jobBlob, e = m.store.seal(cleanHistory(value), "ecard-event:"+owner); e != nil {
			return
		}
	}
	tx, e := m.store.db.Begin()
	if e != nil {
		log.Printf("ecard event persistence failed: %v", e)
		return
	}
	defer tx.Rollback()
	result, e := tx.Exec(`INSERT INTO events(owner,payload) VALUES(?,?)`, owner, b)
	var eventID int64
	if e == nil {
		eventID, e = result.LastInsertId()
	}
	if e == nil && job != "" {
		_, e = tx.Exec(`INSERT INTO job_events(job,id,owner,payload) VALUES(?,?,?,?)`, job, eventID, owner, jobBlob)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		log.Printf("ecard event persistence failed: %v", e)
		return
	}
	visible := cleanHistory(safe).(map[string]interface{})
	visible["id"] = eventID
	m.publishEvent(owner, visible)
	_, _ = m.store.db.Exec(`DELETE FROM events WHERE id <= (SELECT max(id)-5000 FROM events)`)
}
func (m *Module) events(w http.ResponseWriter, r *http.Request, owner string) {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if after < 0 {
		after = 0
	}
	job := r.URL.Query().Get("job_id")
	if job != "" {
		if _, e := m.store.job(job, owner); e != nil {
			fail(w, 404, errors.New("job not found"))
			return
		}
	}
	rows, e := m.eventRows(owner, job, after)
	if e != nil {
		fail(w, 500, e)
		return
	}
	defer rows.Close()
	verbose := m.verboseEvents()
	items := []interface{}{}
	cursor := after
	used := 0
	for rows.Next() {
		var id int64
		var own string
		var b []byte
		var v map[string]interface{}
		if e = rows.Scan(&id, &own, &b); e != nil {
			fail(w, 500, e)
			return
		}
		if e = m.store.open(b, "ecard-event:"+own, &v); e != nil {
			fail(w, 500, e)
			return
		}
		if job != "" && v["job"] != job {
			cursor = id
			continue
		}
		if !verbose && !minimalLiveEvent(v) {
			cursor = id
			continue
		}
		if used+len(b) > 3<<20 {
			if len(items) == 0 {
				respond(w, 413, map[string]interface{}{"error": "event_exceeds_live_page_limit", "report_parts_path": "/api/ecard/jobs/" + job + "/report?part=0"})
				return
			}
			break
		}
		used += len(b)
		cursor = id
		v["id"] = id
		items = append(items, v)
		if len(items) == 100 {
			break
		}
	}
	if e = rows.Err(); e != nil {
		fail(w, 500, e)
		return
	}
	respond(w, 200, map[string]interface{}{"items": items, "cursor": cursor})
}
func (m *Module) monitorReaders(ctx context.Context) {
	previous := map[string]bool{}
	lastError := ""
	initialized := false
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			current, e := readerPresence()
			if e != nil {
				if e.Error() != lastError {
					m.emitEvent("", "", "pcsc", "monitor_unavailable", map[string]string{"reason": e.Error()})
					lastError = e.Error()
				}
				continue
			}
			if lastError != "" {
				m.emitEvent("", "", "pcsc", "monitor_restored", nil)
				lastError = ""
			}
			for name, present := range current {
				old, exists := previous[name]
				if !exists {
					m.emitEvent("", "", "pcsc", "reader_connected", map[string]interface{}{"reader": name, "card_present": present, "initial_snapshot": !initialized})
				}
				if (!exists && present) || (exists && old != present) {
					kind := "card_removed"
					if present {
						kind = "card_inserted"
					}
					m.emitEvent("", "", "pcsc", kind, map[string]interface{}{"reader": name, "initial_snapshot": !initialized})
				}
			}
			for name := range previous {
				if _, ok := current[name]; !ok {
					m.emitEvent("", "", "pcsc", "reader_disconnected", map[string]string{"reader": name})
				}
			}
			previous = current
			initialized = true
		}
	}
}

// Wait for the monitor before closing its database.
func (m *Module) startEventMonitor(ctx context.Context) func() {
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); m.monitorReaders(ctx) }()
	return func() { cancel(); wg.Wait() }
}

// Also consult the retained global ring for operations created before job_events.
func (m *Module) eventRows(owner, job string, after int64) (*sql.Rows, error) {
	if job == "" {
		return m.store.db.Query(`SELECT id,owner,payload FROM events WHERE id>? AND (owner=? OR owner='') ORDER BY id LIMIT 100`, after, owner)
	}
	return m.store.db.Query(`SELECT id,owner,payload FROM job_events WHERE job=? AND owner=? AND id>? UNION ALL SELECT id,owner,payload FROM events WHERE owner=? AND id>? AND id NOT IN (SELECT id FROM job_events WHERE job=?) ORDER BY id`, job, owner, after, owner, after, job)
}

// Stable categories independent of the selected SDK version.
func eventType(source, kind string) string {
	if source == "pcsc" {
		return "pcsc"
	}
	if source == "worker" {
		return "operation"
	}
	lower := strings.ToLower(kind)
	if strings.Contains(lower, "um") {
		return "um"
	}
	if strings.HasPrefix(lower, "pin_") || strings.Contains(lower, "autentificare") {
		return "authentication"
	}
	return "sdk"
}
