package ecard

import (
	"bytes"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Capture bounded JSON only: incomplete payloads are omitted, never logged raw.
type callCapture struct {
	bytes.Buffer
	overflow bool
}

func (b *callCapture) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len()+n > 65536 {
		b.overflow = true
		return n, nil
	}
	if !b.overflow {
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}

type callWriter struct {
	http.ResponseWriter
	body   callCapture
	status int
}

func (w *callWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}
func (w *callWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	_, _ = w.body.Write(p)
	return w.ResponseWriter.Write(p)
}

// Keep synchronous batch headers visible while the caller answers PIN prompts.
func (w *callWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (w *callWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func cleanHistory(v interface{}) interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		for k, v := range x {
			key := strings.ToLower(k)
			// These flags describe execution, without carrying PIN/token values.
			if _, boolean := v.(bool); boolean && (key == "pin_attempted" || key == "_offline_token") {
				continue
			}
			if strings.Contains(key, "pin") || strings.Contains(key, "password") || strings.Contains(key, "secret") || strings.Contains(key, "token") || key == "challenge" || key == "data_base64" || strings.HasSuffix(key, "key") || strings.HasSuffix(key, "keys") || key == "authorization" {
				x[k] = "[redacted]"
			} else {
				x[k] = cleanHistory(v)
			}
		}
	case []interface{}:
		for i, v := range x {
			x[i] = cleanHistory(v)
		}
	}
	return v
}
func capturedJSON(b *callCapture) interface{} {
	if b.overflow {
		return "[payload exceeds 64 KiB; omitted]"
	}
	if b.Len() == 0 {
		return nil
	}
	var v interface{}
	if json.Unmarshal(b.Bytes(), &v) != nil {
		return "[non-JSON payload omitted]"
	}
	return cleanHistory(v)
}
func (m *Module) recordCall(w http.ResponseWriter, r *http.Request, owner string) (http.ResponseWriter, func()) {
	req := &callCapture{}
	if r.Body != nil {
		r.Body = struct {
			io.Reader
			io.Closer
		}{io.TeeReader(r.Body, req), r.Body}
	}
	out := &callWriter{ResponseWriter: w}
	start := time.Now()
	return out, func() {
		request := capturedJSON(req)
		if strings.HasSuffix(r.URL.Path, "/pin") {
			request = "[PIN response omitted]"
		}
		status := out.status
		if status == 0 {
			status = 200
		}
		// Query values are not stored: console callers may put secrets in arbitrary URLs.
		entry := map[string]interface{}{"time": start.UTC().Format(time.RFC3339Nano), "method": r.Method, "path": r.URL.Path, "status": status, "duration_ms": time.Since(start).Milliseconds(), "request": request, "response": capturedJSON(&out.body)}
		id := uuid.NewString()
		b, e := m.store.seal(entry, id)
		if e == nil {
			_, e = m.store.db.Exec(`INSERT INTO call_history(id,owner,created,payload) VALUES(?,?,?,?)`, id, owner, start.UnixNano(), b)
		}
		if e != nil {
			log.Printf("ecard: could not store call history: %v", e)
		}
		if e == nil {
			_, _ = m.store.db.Exec(`DELETE FROM call_history WHERE owner=? AND id NOT IN (SELECT id FROM call_history WHERE owner=? ORDER BY created DESC LIMIT 1000)`, owner, owner)
		}
	}
}
func historyOffset(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if n < 0 {
		return 0
	}
	return n
}
func (m *Module) history(w http.ResponseWriter, r *http.Request, owner string) {
	offset := historyOffset(r)
	rows, e := m.store.db.Query(`SELECT id,payload FROM call_history WHERE owner=? ORDER BY created DESC LIMIT 50 OFFSET ?`, owner, offset)
	if e != nil {
		fail(w, 500, e)
		return
	}
	defer rows.Close()
	entries := []interface{}{}
	for rows.Next() {
		var id string
		var b []byte
		var v interface{}
		if e = rows.Scan(&id, &b); e != nil {
			fail(w, 500, e)
			return
		}
		if e = m.store.open(b, id, &v); e != nil {
			fail(w, 500, e)
			return
		}
		entries = append(entries, v)
	}
	if e = rows.Err(); e != nil {
		fail(w, 500, e)
		return
	}
	respond(w, 200, map[string]interface{}{"items": entries, "next_offset": offset + len(entries), "limit": 50})
}
func (m *Module) listJobs(w http.ResponseWriter, r *http.Request, owner string) {
	offset := historyOffset(r)
	rows, e := m.store.db.Query(`SELECT id,state,operation,sdk,total,completed,created,error FROM jobs WHERE owner=? ORDER BY created DESC,id LIMIT 50 OFFSET ?`, owner, offset)
	if e != nil {
		fail(w, 500, e)
		return
	}
	defer rows.Close()
	items := []Job{}
	for rows.Next() {
		var j Job
		if e = rows.Scan(&j.ID, &j.State, &j.Operation, &j.SDK, &j.Total, &j.Completed, &j.Created, &j.Error); e != nil {
			fail(w, 500, e)
			return
		}
		items = append(items, j)
	}
	if e = rows.Err(); e != nil {
		fail(w, 500, e)
		return
	}
	respond(w, 200, map[string]interface{}{"items": items, "next_offset": offset + len(items)})
}
