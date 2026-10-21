package siui

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"wisemed-labreaders/readersv3/shared/apibridge"
	"wisemed-labreaders/readersv3/shared/history"
)

// Triggers capture transitions in the same transaction as the operation itself.
func initializeOperationAudit(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS siui_operation_events(seq INTEGER PRIMARY KEY AUTOINCREMENT,call_id TEXT NOT NULL,status TEXT NOT NULL,at TEXT NOT NULL,error TEXT NOT NULL DEFAULT '',snapshot INTEGER NOT NULL DEFAULT 0);
 CREATE INDEX IF NOT EXISTS siui_operation_events_call ON siui_operation_events(call_id,seq);
 CREATE TABLE IF NOT EXISTS siui_operation_sources(call_id TEXT PRIMARY KEY,transport TEXT NOT NULL,subject TEXT NOT NULL,source_ip TEXT NOT NULL,connection_id TEXT NOT NULL,tenant TEXT NOT NULL);`)
	if err != nil {
		return err
	}
	for _, table := range []string{"siui_calls", "siui_jobs"} {
		_, err = db.Exec(fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %s_audit_insert AFTER INSERT ON %s BEGIN INSERT INTO siui_operation_events(call_id,status,at,error) VALUES(NEW.id,NEW.status,NEW.created_at,NEW.error); END;
 CREATE TRIGGER IF NOT EXISTS %s_audit_update AFTER UPDATE OF status ON %s WHEN OLD.status<>NEW.status BEGIN INSERT INTO siui_operation_events(call_id,status,at,error) VALUES(NEW.id,NEW.status,strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ','now'),NEW.error); END;
 INSERT INTO siui_operation_events(call_id,status,at,error,snapshot) SELECT id,status,'',error,1 FROM %s WHERE NOT EXISTS(SELECT 1 FROM siui_operation_events WHERE call_id=%s.id);`, table, table, table, table, table, table))
		if err != nil {
			return err
		}
	}
	return nil
}

type operationSource struct {
	Transport  string `json:"transport"`
	Subject    string `json:"subject"`
	IP         string `json:"ip"`
	Connection string `json:"connection_id,omitempty"`
	Tenant     string `json:"tenant,omitempty"`
}

func (m *Module) operationSource(r *http.Request) operationSource {
	subject, _, _ := m.guard.SessionIdentity(r)
	s := operationSource{Transport: "http-session", Subject: subject, IP: r.RemoteAddr}
	if c, ok := r.Context().Value(apiIdentityKey{}).(apiClaims); ok {
		s.Transport = "http-jwt"
		s.Subject = c.Subject
	}
	if p, ok := apibridge.PrincipalFrom(r.Context()); ok {
		return operationSource{Transport: "wsm", Subject: p.Subject, IP: p.SourceIP, Connection: p.ConnectionID, Tenant: p.TenantID}
	}
	if ip, _, err := net.SplitHostPort(s.IP); err == nil {
		s.IP = ip
	}
	return s
}

type operationExecer interface {
	Exec(string, ...any) (sql.Result, error)
}

func (m *Module) saveOperationSource(db operationExecer, r *http.Request, id string) error {
	s := m.operationSource(r)
	_, err := db.Exec(`INSERT OR IGNORE INTO siui_operation_sources(call_id,transport,subject,source_ip,connection_id,tenant) VALUES(?,?,?,?,?,?)`, id, s.Transport, s.Subject, s.IP, s.Connection, s.Tenant)
	return err
}

type operationEvent struct {
	Status   string `json:"status"`
	At       string `json:"at,omitempty"`
	Error    string `json:"error,omitempty"`
	Snapshot bool   `json:"snapshot"`
}
type operationEntry struct {
	ID           string           `json:"id"`
	OperationID  string           `json:"operation_id"`
	Operation    string           `json:"operation"`
	Owner        string           `json:"owner"`
	Status       string           `json:"status"`
	Created      string           `json:"created_at"`
	Started      string           `json:"started_at,omitempty"`
	Completed    string           `json:"completed_at,omitempty"`
	Error        string           `json:"error,omitempty"`
	Source       operationSource  `json:"source"`
	History      historyDelivery  `json:"history"`
	RequestFiles []map[string]any `json:"request_files,omitempty"`
	Parameters   any              `json:"parameters,omitempty"`
	Result       any              `json:"result,omitempty"`
	Events       []operationEvent `json:"events,omitempty"`
	HistoryPath  string           `json:"history_path"`
}

const operationSelect = `SELECT c.id,c.operation_id,c.operation,c.owner,c.status,c.created_at,c.error,COALESCE(o.started_at,''),COALESCE(o.completed_at,''),COALESCE(o.status,''),COALESCE(o.attempts,0),COALESCE(o.error,''),COALESCE(s.transport,''),COALESCE(s.subject,''),COALESCE(s.source_ip,o.source_ip,''),COALESCE(s.connection_id,''),COALESCE(s.tenant,'') FROM siui_history_calls_local c LEFT JOIN siui_history_outbox o ON o.call_id=c.id LEFT JOIN siui_operation_sources s ON s.call_id=c.id`

func scanOperation(row interface{ Scan(...any) error }) (operationEntry, error) {
	var c operationEntry
	err := row.Scan(&c.ID, &c.OperationID, &c.Operation, &c.Owner, &c.Status, &c.Created, &c.Error, &c.Started, &c.Completed, &c.History.Status, &c.History.Attempts, &c.History.Error, &c.Source.Transport, &c.Source.Subject, &c.Source.IP, &c.Source.Connection, &c.Source.Tenant)
	if c.Source.Transport == "" {
		if strings.HasPrefix(c.Owner, "wsm:") {
			c.Source.Transport = "wsm"
			if strings.HasPrefix(c.Source.IP, "wss:") {
				c.Source.Connection = strings.TrimPrefix(c.Source.IP, "wss:")
				c.Source.IP = ""
			}
		} else {
			c.Source.Transport = "http"
			c.Source.Subject = strings.TrimPrefix(c.Owner, "local:")
		}
	}
	c.HistoryPath = "/api/siui/operations/" + url.PathEscape(c.ID) + "/history"
	return c, err
}
func (m *Module) operationsHTTP(w http.ResponseWriter, r *http.Request, route, owner string, admin bool) {
	if r.Method != "GET" {
		fail(w, 405, "GET required")
		return
	}
	select {
	case <-m.ready:
	default:
		fail(w, 503, "SIUI starting")
		return
	}
	if route == "operations" {
		m.listOperations(w, r, owner, admin)
		return
	}
	parts := strings.Split(route, "/")
	if len(parts) < 2 || !history.ValidID(parts[1]) {
		fail(w, 404, "operation not found")
		return
	}
	c, err := scanOperation(m.db.QueryRow(operationSelect+` WHERE c.id=?`, parts[1]))
	if err == sql.ErrNoRows || err == nil && !admin && c.Owner != owner {
		fail(w, 404, "operation not found")
		return
	}
	if err != nil {
		fail(w, 500, "cannot read operation")
		return
	}
	if len(parts) == 3 && parts[2] == "file" {
		call, e := readServiceCall(m.db, c.Owner, c.ID)
		if e != nil {
			fail(w, 404, "local file unavailable")
			return
		}
		m.serviceFileHTTP(w, r, call)
		return
	}
	if len(parts) > 2 && parts[2] != "request" {
		m.operationHistoryHTTP(w, r, c, parts[2:])
		return
	}
	var parameters, result string
	err = m.db.QueryRow(`SELECT COALESCE(o.parameters,'{}'),c.result FROM siui_history_calls_local c LEFT JOIN siui_history_outbox o ON o.call_id=c.id WHERE c.id=?`, c.ID).Scan(&parameters, &result)
	if err != nil {
		fail(w, 500, "cannot read operation details")
		return
	}
	var requestParameters map[string]string
	_ = json.Unmarshal([]byte(parameters), &requestParameters)
	if len(parts) == 3 && parts[2] == "request" {
		files := history.RequestFiles(requestParameters)
		name := r.URL.Query().Get("name")
		data, ok := files[name]
		if !ok {
			fail(w, 404, "request file unavailable locally; check History")
			return
		}
		offset := int64(0)
		if r.URL.Query().Has("offset") {
			var e error
			offset, e = strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
			if e != nil || offset < 0 || offset > int64(len(data)) {
				fail(w, 400, "invalid offset")
				return
			}
			end := min(offset+history.ChunkSize, int64(len(data)))
			respond(w, 200, map[string]any{"ok": true, "data_base64": base64.StdEncoding.EncodeToString(data[offset:end]), "next_offset": end, "eof": end == int64(len(data))})
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": history.RequestFilename(name)}))
		_, _ = w.Write(data)
		return
	}
	c.Parameters = history.RequestPreview(requestParameters)
	requestFiles := history.RequestFiles(requestParameters)
	for _, name := range history.RequestFileNames(requestFiles) {
		data := requestFiles[name]
		c.RequestFiles = append(c.RequestFiles, map[string]any{"filename": history.RequestFilename(name), "size": len(data), "chunk_size": history.ChunkSize, "path": "/api/siui/operations/" + c.ID + "/request?name=" + url.QueryEscape(name)})
	}
	_ = json.Unmarshal([]byte(result), &c.Result)
	rows, err := m.db.Query(`SELECT status,at,error,snapshot FROM siui_operation_events WHERE call_id=? ORDER BY seq`, c.ID)
	if err != nil {
		fail(w, 500, "cannot read operation timeline")
		return
	}
	defer rows.Close()
	c.Events = []operationEvent{}
	for rows.Next() {
		var e operationEvent
		if err = rows.Scan(&e.Status, &e.At, &e.Error, &e.Snapshot); err != nil {
			fail(w, 500, "cannot read timeline")
			return
		}
		c.Events = append(c.Events, e)
	}
	if rows.Err() != nil {
		fail(w, 500, "cannot read timeline")
		return
	}
	respond(w, 200, map[string]any{"success": true, "call": c})
}
func (m *Module) listOperations(w http.ResponseWriter, r *http.Request, owner string, admin bool) {
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if from == "" && to == "" {
		now := time.Now()
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		from = day.Format(time.RFC3339)
		to = day.AddDate(0, 0, 1).Format(time.RFC3339)
	}
	a, e1 := time.Parse(time.RFC3339, from)
	b, e2 := time.Parse(time.RFC3339, to)
	if e1 != nil || e2 != nil || !b.After(a) {
		fail(w, 400, "from/to must be RFC3339 timestamps; to must be after from (exclusive)")
		return
	}
	where := ` WHERE julianday(c.created_at)>=julianday(?) AND julianday(c.created_at)<julianday(?)`
	args := []any{from, to}
	if !admin {
		where += ` AND c.owner=?`
		args = append(args, owner)
	}
	for _, filter := range []struct{ param, col string }{{"operation_id", "c.operation_id"}, {"operation", "c.operation"}, {"status", "c.status"}} {
		if v := q.Get(filter.param); v != "" {
			where += " AND " + filter.col + "=?"
			args = append(args, v)
		}
	}
	offset := 0
	if q.Get("offset") != "" {
		var err error
		offset, err = strconv.Atoi(q.Get("offset"))
		if err != nil || offset < 0 {
			fail(w, 400, "invalid offset")
			return
		}
	}
	var total int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM siui_history_calls_local c`+where, args...).Scan(&total); err != nil {
		fail(w, 500, "cannot count operations")
		return
	}
	rows, err := m.db.Query(operationSelect+where+` ORDER BY c.created_at DESC,c.id LIMIT 100 OFFSET ?`, append(args, offset)...)
	if err != nil {
		fail(w, 500, "cannot list operations")
		return
	}
	defer rows.Close()
	calls := []operationEntry{}
	for rows.Next() {
		c, e := scanOperation(rows)
		if e != nil {
			fail(w, 500, "cannot read operations")
			return
		}
		calls = append(calls, c)
	}
	if rows.Err() != nil {
		fail(w, 500, "cannot read operations")
		return
	}
	respond(w, 200, map[string]any{"success": true, "calls": calls, "total": total, "offset": offset, "next_offset": offset + len(calls), "from": from, "to": to})
}

// Proxy only the archived call already authorized locally. The History credential
// stays on the bridge; downloads are streamed one bounded chunk at a time.
func (m *Module) operationHistoryHTTP(w http.ResponseWriter, r *http.Request, c operationEntry, parts []string) {
	if parts[0] != "history" || len(parts) > 2 {
		fail(w, 404, "not found")
		return
	}
	m.history.mu.Lock()
	cfg := m.history.cfg
	m.history.mu.Unlock()
	client, err := m.historyClient(cfg)
	if err != nil {
		fail(w, 503, err.Error())
		return
	}
	defer client.CloseIdleConnections()
	var identity struct {
		Tenant string `json:"tenant"`
		Bridge string `json:"bridge"`
	}
	if err = historyRequest(r.Context(), client, cfg, "GET", "/api/history/identity", nil, &identity); err != nil {
		fail(w, 502, err.Error())
		return
	}
	var destination, raw string
	if err = m.db.QueryRow(`SELECT destination,manifest FROM siui_history_outbox WHERE call_id=?`, c.ID).Scan(&destination, &raw); err != nil {
		fail(w, 404, "History record unavailable")
		return
	}
	if destination == "" || destination != history.Digest(struct{ URL, Tenant, Bridge string }{cfg.URL, identity.Tenant, identity.Bridge}) {
		fail(w, 409, "call has not been sent to this History destination")
		return
	}
	path := "/api/history/calls/" + url.PathEscape(c.ID)
	if len(parts) == 1 || parts[1] == "request" {
		if len(parts) == 2 {
			path += "/request"
		}
		var data any
		if err = historyRequest(r.Context(), client, cfg, "GET", path, nil, &data); err != nil {
			fail(w, 502, err.Error())
			return
		}
		respond(w, 200, data)
		return
	}
	var record history.Record
	if json.Unmarshal([]byte(raw), &record) != nil {
		fail(w, 500, "invalid History manifest")
		return
	}
	var artifact *history.Artifact
	for i := range record.Artifacts {
		if record.Artifacts[i].Name == parts[1] {
			artifact = &record.Artifacts[i]
			break
		}
	}
	if artifact == nil {
		fail(w, 404, "artifact not found")
		return
	}
	// WSM callers use offset to obtain one chunk; HTTP download streams the file.
	offset := int64(0)
	chunked := r.URL.Query().Has("offset")
	if chunked {
		offset, err = strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
		if err != nil || offset < 0 || offset > artifact.Size {
			fail(w, 400, "invalid offset")
			return
		}
	}
	for offset < artifact.Size {
		req, e := http.NewRequestWithContext(r.Context(), "GET", cfg.URL+path+"/files/"+url.PathEscape(artifact.Name)+"?offset="+strconv.FormatInt(offset, 10), nil)
		if e != nil {
			fail(w, 500, "invalid History request")
			return
		}
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
		res, e := client.Do(req)
		if e != nil {
			if offset == 0 || chunked {
				fail(w, 502, e.Error())
			}
			return
		}
		if res.StatusCode != 200 {
			res.Body.Close()
			if offset == 0 || chunked {
				fail(w, 502, "History artifact unavailable")
			}
			return
		}
		if chunked {
			data, e := io.ReadAll(io.LimitReader(res.Body, min(int64(history.ChunkSize), artifact.Size-offset)))
			res.Body.Close()
			if e != nil || len(data) == 0 {
				fail(w, 502, "cannot read History chunk")
				return
			}
			respond(w, 200, map[string]any{"data_base64": base64.StdEncoding.EncodeToString(data), "next_offset": offset + int64(len(data)), "size": artifact.Size})
			return
		}
		contentType := res.Header.Get("Content-Type")
		switch contentType {
		case "application/zip", "application/xml", "application/gzip":
		default:
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
		filename := artifact.DownloadName()
		if _, params, err := mime.ParseMediaType(res.Header.Get("Content-Disposition")); err == nil && history.ValidFilename(params["filename"]) {
			filename = params["filename"]
		}
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
		w.Header().Set("X-Artifact-Size", strconv.FormatInt(artifact.Size, 10))
		w.Header().Set("X-Artifact-SHA256", artifact.SHA256)
		if !chunked {
			w.Header().Set("Content-Length", strconv.FormatInt(artifact.Size, 10))
		}
		n, e := io.CopyN(w, res.Body, min(int64(history.ChunkSize), artifact.Size-offset))
		res.Body.Close()
		if e != nil || n == 0 {
			return
		}
		offset += n
		if chunked {
			return
		}
	}
}
