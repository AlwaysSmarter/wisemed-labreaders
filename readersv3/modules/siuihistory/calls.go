package siuihistory

import (
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wisemed-labreaders/readersv3/shared/history"
)

// Fixed-width UTC timestamps keep fractional seconds and timezone offsets comparable.
const callTimeSQL = `CONCAT(DATE_FORMAT(CONVERT_TZ(STR_TO_DATE(LEFT(created_at,19),'%Y-%m-%dT%H:%i:%s'),IF(RIGHT(created_at,1)='Z','+00:00',RIGHT(created_at,6)),'+00:00'),'%Y-%m-%dT%H:%i:%s'),'.',RPAD(IF(SUBSTRING(created_at,20,1)='.',SUBSTRING_INDEX(SUBSTRING_INDEX(SUBSTRING_INDEX(SUBSTRING(created_at,21),'Z',1),'+',1),'-',1),''),9,'0'),'Z')`

func callPeriod(r *http.Request) (string, string, error) {
	bounds := [2]string{}
	var times [2]time.Time
	for i, key := range []string{"from", "to"} {
		if v := r.URL.Query().Get(key); v != "" {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				return "", "", fmt.Errorf("invalid %s timestamp", key)
			}
			times[i] = t
			bounds[i] = t.UTC().Format("2006-01-02T15:04:05.000000000Z")
		}
	}
	if !times[0].IsZero() && !times[1].IsZero() && times[1].Before(times[0]) {
		return "", "", errors.New("to must be at or after from")
	}
	return bounds[0], bounds[1], nil
}

func validName(s string) bool { return history.ValidID(s) }
func (m *Module) stage(tenant, bridge, id, name string) string {
	return filepath.Join(m.rt.ResolvePath(m.cfg.StagingPath), tenant, bridge, id, name)
}
func objectKey(tenant, bridge string, rec history.Record, name string) string {
	t, _ := time.Parse(time.RFC3339Nano, rec.CreatedAt)
	return tenant + "/" + t.UTC().Format("2006/01/02") + "/" + bridge + "/" + rec.CallID + "/" + name
}
func (m *Module) progress(tenant, bridge string, rec history.Record, status string) history.Progress {
	p := history.Progress{Success: true, Status: status, Offsets: map[string]int64{}}
	for _, a := range rec.Artifacts {
		if status == "archived" {
			p.Offsets[a.Name] = a.Size
		} else if st, e := os.Stat(m.stage(tenant, bridge, rec.CallID, a.Name)); e == nil {
			p.Offsets[a.Name] = st.Size()
		} else {
			p.Offsets[a.Name] = 0
		}
	}
	return p
}
func (m *Module) calls(w http.ResponseWriter, r *http.Request, db *sql.DB, route, tenant, bridge string, admin bool) {
	if !validName(tenant) {
		failure(w, 409, errors.New("configure tenant first"))
		return
	}
	if route == "calls" && r.Method == "POST" {
		if admin {
			bridge = r.URL.Query().Get("bridge")
		}
		if !validName(bridge) {
			failure(w, 403, errors.New("use a bridge credential to ingest calls"))
			return
		}
		var rec history.Record
		if e := parse(w, r, &rec); e != nil {
			failure(w, 400, e)
			return
		}
		if entry, ok := r.Context().Value(debugContextKey{}).(*debugRequest); ok {
			entry.Operation = rec.Operation
			entry.OperationID = rec.OperationID
		}
		if e := rec.Validate(); e != nil {
			failure(w, 400, e)
			return
		}
		b, _ := json.Marshal(rec)
		digest := history.Digest(rec)
		_, e := db.ExecContext(r.Context(), `INSERT IGNORE INTO siui_history_calls(tenant,bridge,call_id,operation_id,operation,created_at,received_at,status,digest,record) VALUES(?,?,?,?,?,?,?,'pending',?,?)`, tenant, bridge, rec.CallID, rec.OperationID, rec.Operation, rec.CreatedAt, time.Now().UTC().Format(time.RFC3339Nano), digest, string(b))
		if e != nil {
			failure(w, 500, e)
			return
		}
		var existing, status string
		e = db.QueryRowContext(r.Context(), `SELECT digest,status FROM siui_history_calls WHERE tenant=? AND bridge=? AND call_id=?`, tenant, bridge, rec.CallID).Scan(&existing, &status)
		if e != nil {
			failure(w, 500, e)
			return
		}
		if existing != digest {
			failure(w, 409, errors.New("call already exists with different content"))
			return
		}
		output(w, 200, m.progress(tenant, bridge, rec, status))
		return
	}
	if route == "calls" && r.Method == "GET" {
		timeSQL := callTimeSQL
		field := r.URL.Query().Get("time_field")
		if field == "received_at" {
			timeSQL = strings.ReplaceAll(callTimeSQL, "created_at", "received_at")
		} else if field != "" && field != "created_at" {
			failure(w, 400, errors.New("invalid time_field"))
			return
		}
		from, to, err := callPeriod(r)
		if err != nil {
			failure(w, 400, err)
			return
		}
		query := `SELECT bridge,call_id,operation_id,operation,created_at,received_at,status,record FROM siui_history_calls WHERE tenant=?`
		args := []any{tenant}
		if bridge != "" {
			query += " AND bridge=?"
			args = append(args, bridge)
		} else if v := r.URL.Query().Get("bridge"); v != "" {
			query += " AND bridge=?"
			args = append(args, v)
		}
		for _, field := range []string{"operation", "operation_id", "call_id", "status"} {
			if v := r.URL.Query().Get(field); v != "" {
				query += " AND " + field + "=?"
				args = append(args, v)
			}
		}
		if from != "" {
			query += " AND " + timeSQL + ">=?"
			args = append(args, from)
		}
		if to != "" {
			if r.URL.Query().Get("to_exclusive") == "1" {
				query += " AND " + timeSQL + "<?"
			} else {
				query += " AND " + timeSQL + "<=?"
			}
			args = append(args, to)
		}
		if v := r.URL.Query().Get("operation_status"); v != "" {
			if v != "completed" && v != "failed" && v != "unknown" {
				failure(w, 400, errors.New("invalid operation_status"))
				return
			}
			query += " AND JSON_UNQUOTE(JSON_EXTRACT(record,'$.status'))=?"
			args = append(args, v)
		}
		for _, field := range []string{"pid", "cid"} {
			if v := r.URL.Query().Get(field); v != "" {
				query += " AND EXISTS (SELECT 1 FROM siui_history_details d WHERE d.tenant=siui_history_calls.tenant AND d.bridge=siui_history_calls.bridge AND d.call_id=siui_history_calls.call_id AND d." + field + "=?)"
				args = append(args, v)
			}
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if offset < 0 {
			offset = 0
		}
		query += " ORDER BY " + timeSQL + " DESC,bridge,call_id LIMIT 101 OFFSET ?"
		args = append(args, offset)
		rows, e := db.QueryContext(r.Context(), query, args...)
		if e != nil {
			failure(w, 500, e)
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var b, id, oid, op, created, received, status, raw string
			if e = rows.Scan(&b, &id, &oid, &op, &created, &received, &status, &raw); e != nil {
				failure(w, 500, e)
				return
			}
			var rec history.Record
			if e = json.Unmarshal([]byte(raw), &rec); e != nil {
				failure(w, 500, e)
				return
			}
			items = append(items, map[string]any{"owner": rec.Owner, "source_ip": rec.SourceIP, "operation_status": rec.Status, "artifacts": rec.Artifacts, "bridge": b, "call_id": id, "operation_id": oid, "operation": op, "created_at": created, "received_at": received, "status": status})
		}
		if e = rows.Err(); e != nil {
			failure(w, 500, e)
			return
		}
		hasMore := len(items) > 100
		if hasMore {
			items = items[:100]
		}
		output(w, 200, map[string]any{"success": true, "calls": items, "next_offset": offset + len(items), "has_more": hasMore})
		return
	}
	parts := strings.Split(route, "/")
	if len(parts) < 2 || parts[0] != "calls" || !validName(parts[1]) {
		failure(w, 404, errors.New("not found"))
		return
	}
	id := parts[1]
	if admin {
		bridge = r.URL.Query().Get("bridge")
	}
	if !validName(bridge) {
		failure(w, 400, errors.New("bridge query parameter required"))
		return
	}
	var raw, status string
	e := db.QueryRowContext(r.Context(), `SELECT record,status FROM siui_history_calls WHERE tenant=? AND bridge=? AND call_id=?`, tenant, bridge, id).Scan(&raw, &status)
	if e == sql.ErrNoRows {
		failure(w, 404, errors.New("call not found"))
		return
	}
	if e != nil {
		failure(w, 500, e)
		return
	}
	var rec history.Record
	if e = json.Unmarshal([]byte(raw), &rec); e != nil {
		failure(w, 500, e)
		return
	}
	if r.Method == "GET" && status == "archived" {
		m.populateFileNames(r.Context(), db, tenant, bridge, &rec)
	}
	if len(parts) == 3 && parts[2] == "request" && r.Method == "GET" {
		if status != "archived" {
			failure(w, 409, errors.New("request archive not ready"))
			return
		}
		store, err := m.archiveStore(r.Context(), db, tenant, bridge, id)
		if err != nil {
			failure(w, 503, err)
			return
		}
		f, err := store.Open(r.Context(), objectKey(tenant, bridge, rec, "record-gzip"), 0)
		if err != nil {
			failure(w, 502, err)
			return
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			failure(w, 502, err)
			return
		}
		defer gz.Close()
		var payload struct {
			Parameters map[string]string `json:"parameters"`
		}
		if err = json.NewDecoder(io.LimitReader(gz, 64<<20)).Decode(&payload); err != nil {
			failure(w, 422, errors.New("request preview unavailable; download record-gzip for full content"))
			return
		}
		output(w, 200, map[string]any{"success": true, "parameters": history.RequestPreview(payload.Parameters), "archive": "record-gzip"})
		return
	}
	if len(parts) == 2 && r.Method == "GET" {
		var details, parseError string
		e = db.QueryRowContext(r.Context(), `SELECT details,parse_error FROM siui_history_details WHERE tenant=? AND bridge=? AND call_id=?`, tenant, bridge, id).Scan(&details, &parseError)
		var parsed any
		if details != "" {
			_ = json.Unmarshal([]byte(details), &parsed)
		}
		if e != nil && e != sql.ErrNoRows {
			failure(w, 500, e)
			return
		}
		output(w, 200, map[string]any{"success": true, "record": rec, "details": parsed, "parse_error": parseError, "delivery": m.progress(tenant, bridge, rec, status)})
		return
	}
	if len(parts) == 3 && parts[2] == "commit" && r.Method == "POST" {
		if status == "archived" {
			output(w, 200, m.progress(tenant, bridge, rec, status))
			return
		}
		store, e := m.store()
		if e != nil {
			failure(w, 503, e)
			return
		}
		for _, a := range rec.Artifacts {
			f, e := os.Open(m.stage(tenant, bridge, id, a.Name))
			if e != nil {
				failure(w, 409, e)
				return
			}
			h := sha256.New()
			n, e := io.Copy(h, f)
			if e == nil && (n != a.Size || hex.EncodeToString(h.Sum(nil)) != a.SHA256) {
				e = errors.New("artifact size/hash mismatch")
			}
			if e == nil {
				_, e = f.Seek(0, 0)
			}
			if e == nil {
				e = store.Put(r.Context(), objectKey(tenant, bridge, rec, a.Name), f, a.Size)
			}
			f.Close()
			if e != nil {
				failure(w, 502, e)
				return
			}
		}
		if e = m.indexDetails(r.Context(), db, tenant, bridge, rec); e != nil {
			failure(w, 500, e)
			return
		}
		// Preserve the manifest alongside originals, enabling recovery without the SQL index.
		if e = store.Put(r.Context(), objectKey(tenant, bridge, rec, "manifest.json"), strings.NewReader(raw), int64(len(raw))); e != nil {
			failure(w, 502, e)
			return
		}
		if e = m.saveLocation(r.Context(), db, tenant, bridge, id); e != nil {
			failure(w, 500, e)
			return
		}
		_, e = db.ExecContext(r.Context(), `UPDATE siui_history_calls SET status='archived' WHERE tenant=? AND bridge=? AND call_id=?`, tenant, bridge, id)
		if e != nil {
			failure(w, 500, e)
			return
		}
		_ = os.RemoveAll(filepath.Dir(m.stage(tenant, bridge, id, "unused")))
		output(w, 200, m.progress(tenant, bridge, rec, "archived"))
		return
	}
	if len(parts) == 4 && parts[2] == "files" {
		var a *history.Artifact
		for i := range rec.Artifacts {
			if rec.Artifacts[i].Name == parts[3] {
				a = &rec.Artifacts[i]
				break
			}
		}
		if a == nil {
			failure(w, 404, errors.New("artifact not found"))
			return
		}
		offset, e := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
		if r.URL.Query().Get("offset") == "" {
			offset = 0
			e = nil
		}
		if e != nil || offset < 0 || offset > a.Size {
			failure(w, 400, errors.New("invalid offset"))
			return
		}
		if r.Method == "PUT" && status != "archived" {
			data, e := io.ReadAll(http.MaxBytesReader(w, r.Body, history.ChunkSize))
			if e != nil || len(data) == 0 || offset+int64(len(data)) > a.Size {
				failure(w, 400, errors.New("invalid chunk size"))
				return
			}
			path := m.stage(tenant, bridge, id, a.Name)
			if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
				failure(w, 500, e)
				return
			}
			f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
			if e != nil {
				failure(w, 500, e)
				return
			}
			defer f.Close()
			st, e := f.Stat()
			if e != nil {
				failure(w, 500, e)
				return
			}
			if st.Size() != offset {
				failure(w, 409, fmt.Errorf("expected offset %d", st.Size()))
				return
			}
			_, e = f.WriteAt(data, offset)
			if e == nil {
				e = f.Sync()
			}
			if e != nil {
				failure(w, 500, e)
				return
			}
			output(w, 200, map[string]any{"success": true, "offset": offset + int64(len(data))})
			return
		}
		if r.Method == "GET" && status == "archived" {
			store, e := m.archiveStore(r.Context(), db, tenant, bridge, id)
			if e != nil {
				failure(w, 503, e)
				return
			}
			f, e := store.Open(r.Context(), objectKey(tenant, bridge, rec, a.Name), offset)
			if e != nil {
				failure(w, 502, e)
				return
			}
			defer f.Close()
			if r.URL.Query().Get("format") == "json" {
				data, err := io.ReadAll(io.LimitReader(f, min(int64(history.ChunkSize), a.Size-offset)))
				if err != nil {
					failure(w, 502, err)
					return
				}
				output(w, 200, map[string]any{"success": true, "data_base64": base64.StdEncoding.EncodeToString(data), "next_offset": offset + int64(len(data)), "filename": a.DownloadName(), "content_type": a.ContentType})
				return
			}
			contentType := a.ContentType
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			w.Header().Set("Content-Type", contentType)
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": a.DownloadName()}))
			w.Header().Set("X-Artifact-Size", strconv.FormatInt(a.Size, 10))
			w.Header().Set("X-Artifact-SHA256", a.SHA256)
			_, _ = io.CopyN(w, f, min(int64(history.ChunkSize), a.Size-offset))
			return
		}
	}
	failure(w, 405, errors.New("unsupported method or archive state"))
}
