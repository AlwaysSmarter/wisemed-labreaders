package siui

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"wisemed-labreaders/readersv3/shared/apibridge"

	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/shared/history"
)

type historyDelivery struct {
	Status   string `json:"status"`
	Attempts int    `json:"attempts"`
	Error    string `json:"error,omitempty"`
}
type historySettings struct {
	Enabled      bool   `json:"enabled"`
	URL          string `json:"url"`
	Token        string `json:"token"`
	CAFile       string `json:"ca_file"`
	RetrySeconds int    `json:"retry_seconds"`
}
type historySync struct {
	database string
	mu       sync.Mutex
	cfg      historySettings
}

func initializeHistory(db *sql.DB) error {
	_, e := db.Exec(`CREATE TABLE IF NOT EXISTS siui_history_outbox(call_id TEXT PRIMARY KEY,parameters TEXT NOT NULL DEFAULT '{}',source_ip TEXT NOT NULL DEFAULT '',started_at TEXT NOT NULL DEFAULT '',completed_at TEXT NOT NULL DEFAULT '',status TEXT NOT NULL DEFAULT 'pending',attempts INTEGER NOT NULL DEFAULT 0,next_attempt TEXT NOT NULL DEFAULT '',error TEXT NOT NULL DEFAULT '',manifest TEXT NOT NULL DEFAULT '',destination TEXT NOT NULL DEFAULT ''); CREATE VIEW IF NOT EXISTS siui_history_calls_local AS SELECT id,owner,operation_id,operation,status,error,result,created_at FROM siui_calls UNION ALL SELECT id,owner,operation_id,'validateReport',status,error,result,created_at FROM siui_jobs; UPDATE siui_history_outbox SET completed_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE completed_at='' AND call_id IN (SELECT id FROM siui_history_calls_local WHERE status IN ('completed','failed','unknown'));`)
	if e != nil {
		return e
	}
	return initializeOperationAudit(db)
}
func (m *Module) initHistory() {
	m.history = &historySync{database: m.cfg.Database, cfg: historySettings{RetrySeconds: 30}}
	b, _ := json.Marshal(m.rt.ModuleSettings("siui-history-sync"))
	_ = json.Unmarshal(b, &m.history.cfg)
	m.rt.Handle("/api/siui-history-sync/", m.authenticate(http.HandlerFunc(m.historyHTTP)))
	m.rt.Handle("/history-sync-ui", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, historySyncPage)
	}))
}
func (m *Module) historyHTTP(w http.ResponseWriter, r *http.Request) {
	_, admin, ok := m.guard.SessionIdentity(r)
	if c, yes := r.Context().Value(apiIdentityKey{}).(apiClaims); yes {
		admin, ok = c.Admin, true
	}
	if r.Method != "GET" && r.Header.Get("Origin") != "" {
		u, e := url.Parse(r.Header.Get("Origin"))
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if e != nil || u.Host != r.Host || u.Scheme != scheme {
			fail(w, 403, "origin rejected")
			return
		}
	}
	if !ok || !admin {
		fail(w, 403, "administrator required")
		return
	}
	select {
	case <-m.ready:
	default:
		fail(w, 503, "SIUI starting")
		return
	}
	m.history.mu.Lock()
	defer m.history.mu.Unlock()
	route := strings.TrimPrefix(r.URL.Path, "/api/siui-history-sync/")
	if route == "settings" && r.Method == "GET" {
		c := m.history.cfg
		c.Token = ""
		respond(w, 200, c)
		return
	}
	if route == "settings" && r.Method == "PUT" {
		var c historySettings
		if e := decode(w, r, &c); e != nil {
			fail(w, 400, e.Error())
			return
		}
		if c.Token == "" {
			c.Token = m.history.cfg.Token
		}
		c.URL = strings.TrimRight(c.URL, "/")
		if c.Enabled {
			u, e := url.Parse(c.URL)
			if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || c.Token == "" {
				fail(w, 400, "HTTPS History URL and bridge token required")
				return
			}
		}
		if c.RetrySeconds < 5 {
			c.RetrySeconds = 30
		}
		cfg, e := config.Load(m.rt.ConfigPath())
		if e == nil {
			b, _ := json.Marshal(c)
			var v map[string]interface{}
			_ = json.Unmarshal(b, &v)
			cfg.Modules["siui-history-sync"] = v
			e = cfg.Save()
			if e == nil {
				e = os.Chmod(m.rt.ConfigPath(), 0600)
			}
		}
		if e != nil {
			fail(w, 500, e.Error())
			return
		}
		m.history.cfg = c
		respond(w, 200, map[string]bool{"success": true})
		return
	}
	if route == "status" && r.Method == "GET" {
		rows, e := m.db.Query(`SELECT status,COUNT(*) FROM siui_history_outbox GROUP BY status`)
		if e != nil {
			fail(w, 500, e.Error())
			return
		}
		counts := map[string]int{}
		for rows.Next() {
			var s string
			var n int
			_ = rows.Scan(&s, &n)
			counts[s] = n
		}
		rows.Close()
		var last string
		_ = m.db.QueryRow(`SELECT error FROM siui_history_outbox WHERE error<>'' ORDER BY next_attempt DESC LIMIT 1`).Scan(&last)
		respond(w, 200, map[string]any{"success": true, "counts": counts, "last_error": last, "enabled": m.history.cfg.Enabled, "message": historySyncMessage(m.history.cfg.Enabled)})
		return
	}
	if route == "retry" && r.Method == "POST" {
		_, e := m.db.Exec(`UPDATE siui_history_outbox SET next_attempt='' WHERE status='pending'`)
		if e != nil {
			fail(w, 500, e.Error())
			return
		}
		respond(w, 200, map[string]bool{"success": true})
		return
	}
	if route == "test" && r.Method == "POST" {
		c := m.history.cfg
		client, e := m.historyClient(c)
		if e == nil {
			e = historyRequest(r.Context(), client, c, "GET", "/api/history/identity", nil, nil)
		}
		if e != nil {
			fail(w, 502, e.Error())
			return
		}
		respond(w, 200, map[string]any{"success": true, "enabled": c.Enabled, "message": historySyncMessage(c.Enabled)})
		return
	}
	fail(w, 404, "not found")
}
func (m *Module) historyClient(c historySettings) (*http.Client, error) {
	u, e := url.Parse(c.URL)
	if e != nil || u.Scheme != "https" || u.Host == "" || c.Token == "" {
		return nil, errors.New("configure HTTPS History URL and token")
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if c.CAFile != "" {
		b, e := os.ReadFile(m.rt.ResolvePath(c.CAFile))
		if e != nil {
			return nil, e
		}
		pool, e := x509.SystemCertPool()
		if e != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(b) {
			return nil, errors.New("invalid History CA PEM")
		}
		tr.TLSClientConfig.RootCAs = pool
	}
	return &http.Client{Transport: tr, Timeout: 5 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("History redirects are disabled") }}, nil
}
func historyRequest(ctx context.Context, client *http.Client, c historySettings, method, path string, data []byte, result any) error {
	req, e := http.NewRequestWithContext(ctx, method, c.URL+path, bytes.NewReader(data))
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	if method == "PUT" {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	res, e := client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	b, e := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if e != nil {
		return e
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var msg struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &msg)
		return fmt.Errorf("History HTTP %d: %s", res.StatusCode, msg.Error)
	}
	if result != nil {
		return json.Unmarshal(b, result)
	}
	return nil
}
func (m *Module) historyWorker(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.syncHistory(ctx)
		}
	}
}
func (m *Module) syncHistory(ctx context.Context) {
	downloadStorageMu.RLock()
	defer downloadStorageMu.RUnlock()
	m.history.mu.Lock()
	c := m.history.cfg
	m.history.mu.Unlock()
	if !c.Enabled {
		return
	}
	var id, parameters, ip, started, completed, manifest, destination string
	var attempts int
	e := m.db.QueryRowContext(ctx, `SELECT o.call_id,o.parameters,o.source_ip,o.started_at,o.completed_at,o.manifest,o.destination,o.attempts FROM siui_history_outbox o JOIN siui_history_calls_local c ON c.id=o.call_id WHERE o.status='pending' AND o.completed_at<>'' AND c.status IN ('completed','failed','unknown') AND (o.next_attempt='' OR o.next_attempt<=?) ORDER BY c.created_at LIMIT 1`, time.Now().UTC().Format(time.RFC3339Nano)).Scan(&id, &parameters, &ip, &started, &completed, &manifest, &destination, &attempts)
	if e != nil {
		return
	}
	client, e := m.historyClient(c)
	if e != nil {
		m.historyFailure(id, attempts, e, c.RetrySeconds)
		return
	}
	defer client.CloseIdleConnections()
	var identity struct {
		Tenant string `json:"tenant"`
		Bridge string `json:"bridge"`
	}
	if e = historyRequest(ctx, client, c, "GET", "/api/history/identity", nil, &identity); e != nil {
		m.historyFailure(id, attempts, e, c.RetrySeconds)
		return
	}
	if !history.ValidID(identity.Tenant) || !history.ValidID(identity.Bridge) {
		m.historyFailure(id, attempts, errors.New("invalid History identity"), c.RetrySeconds)
		return
	}
	destinationNow := history.Digest(struct{ URL, Tenant, Bridge string }{c.URL, identity.Tenant, identity.Bridge})
	// Once delivery has started, changing destination must not silently move protected data.
	if destination != "" && destination != destinationNow {
		m.historyFailure(id, attempts, errors.New("History destination changed; restore original URL/bridge identity or explicitly migrate the pending outbox"), c.RetrySeconds)
		return
	}
	var rec history.Record
	if manifest == "" {
		rec, e = m.prepareHistory(ctx, id, parameters, ip, started, completed)
		if e == nil {
			b, _ := json.Marshal(rec)
			manifest = string(b)
			_, e = m.db.ExecContext(ctx, `UPDATE siui_history_outbox SET manifest=?,destination=? WHERE call_id=?`, manifest, destinationNow, id)
		}
	} else {
		e = json.Unmarshal([]byte(manifest), &rec)
	}
	if e == nil {
		e = m.uploadHistory(ctx, client, c, rec)
	}
	if e != nil {
		m.historyFailure(id, attempts, e, c.RetrySeconds)
		return
	}
	if _, e = m.db.ExecContext(ctx, `UPDATE siui_history_outbox SET status='archived',error='',attempts=attempts+1 WHERE call_id=?`, id); e == nil {
		_ = os.RemoveAll(filepath.Join(filepath.Dir(m.history.database), "siui-history-outbox", id))
	}
}
func (m *Module) historyFailure(id string, attempts int, e error, retry int) {
	if retry < 5 {
		retry = 30
	}
	delay := time.Duration(retry) * time.Second * time.Duration(1<<min(attempts, 7))
	if delay > time.Hour {
		delay = time.Hour
	}
	msg := e.Error()
	if len(msg) > 2000 {
		msg = msg[:2000]
	}
	_, _ = m.db.Exec(`UPDATE siui_history_outbox SET attempts=attempts+1,error=?,next_attempt=? WHERE call_id=?`, msg, time.Now().Add(delay).UTC().Format(time.RFC3339Nano), id)
}
func (m *Module) prepareHistory(ctx context.Context, id, parameters, ip, started, completed string) (history.Record, error) {
	var rec history.Record
	var result, message string
	rec.CallID = id
	rec.SourceIP = ip
	rec.StartedAt = started
	rec.CompletedAt = completed
	e := m.db.QueryRowContext(ctx, `SELECT operation_id,operation,owner,created_at,status,error,result FROM siui_history_calls_local WHERE id=?`, id).Scan(&rec.OperationID, &rec.Operation, &rec.Owner, &rec.CreatedAt, &rec.Status, &message, &result)
	if e != nil {
		return rec, e
	}
	dir := filepath.Join(filepath.Dir(m.history.database), "siui-history-outbox", id)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return rec, e
	}
	// Store the original request/result compressed; do not expand catalogues or ZIP downloads into JSON.
	var params any
	_ = json.Unmarshal([]byte(parameters), &params)
	var res map[string]any
	if result != "" {
		_ = json.Unmarshal([]byte(result), &res)
	}
	payload := map[string]any{"parameters": params, "result": res, "error": message}
	if entry, err := scanOperation(m.db.QueryRowContext(ctx, operationSelect+" WHERE c.id=?", id)); err == nil {
		payload["source"] = entry.Source
		if entry.Source.IP != "" {
			rec.SourceIP = entry.Source.IP
		} else if entry.Source.Transport == "wsm" {
			rec.SourceIP = ""
		}
	}
	events, err := m.db.QueryContext(ctx, `SELECT status,at,error,snapshot FROM siui_operation_events WHERE call_id=? ORDER BY seq`, id)
	if err != nil {
		return rec, err
	}
	timeline := []operationEvent{}
	for events.Next() {
		var event operationEvent
		if err = events.Scan(&event.Status, &event.At, &event.Error, &event.Snapshot); err != nil {
			events.Close()
			return rec, err
		}
		timeline = append(timeline, event)
	}
	err = events.Err()
	events.Close()
	if err != nil {
		return rec, err
	}
	payload["events"] = timeline
	f, e := os.OpenFile(filepath.Join(dir, "record-gzip"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return rec, e
	}
	gz := gzip.NewWriter(f)
	e = json.NewEncoder(gz).Encode(payload)
	ce := gz.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = f.Sync()
	}
	ce = f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return rec, e
	}
	a, e := hashHistoryFile(filepath.Join(dir, "record-gzip"), "record-gzip")
	if e != nil {
		return rec, e
	}
	a.Filename = "record.json.gz"
	a.ContentType = "application/gzip"
	rec.Artifacts = append(rec.Artifacts, a)
	var requestParameters map[string]string
	if e = json.Unmarshal([]byte(parameters), &requestParameters); e != nil {
		return rec, e
	}
	requestFiles := history.RequestFiles(requestParameters)
	for _, name := range history.RequestFileNames(requestFiles) {
		path := filepath.Join(dir, name)
		if e = os.WriteFile(path, requestFiles[name], 0600); e != nil {
			return rec, e
		}
		artifact, err := hashHistoryFile(path, name)
		if err != nil {
			return rec, err
		}
		artifact.Filename = history.RequestFilename(name)
		artifact.ContentType = "application/xml"
		if name == "request-invoiceFile" {
			artifact.ContentType = "application/zip"
		}
		rec.Artifacts = append(rec.Artifacts, artifact)
	}
	if res != nil {
		if details, ok := res["insured"]; ok {
			rec.Details, _ = json.Marshal(details)
		}
		if _, ok := res["file"]; ok {
			a, e = hashHistoryFile(existingDownloadPath(m.history.database, id), "download")
			if e != nil {
				return rec, e
			}
			address, fallback := "", id
			if values, ok := res["values"].([]any); ok && len(values) > 0 {
				address, _ = values[0].(string)
			}
			if file, ok := res["file"].(map[string]any); ok {
				if filename, ok := file["filename"].(string); ok {
					fallback = filename
				}
			}
			original, err := os.Open(existingDownloadPath(m.history.database, id))
			if err != nil {
				return rec, err
			}
			head, err := io.ReadAll(io.LimitReader(original, 512))
			original.Close()
			if err != nil {
				return rec, err
			}
			a.Filename, a.ContentType = history.DownloadIdentity(address, fallback, head)
			rec.Artifacts = append(rec.Artifacts, a)
		}
	}
	return rec, rec.Validate()
}
func hashHistoryFile(path, name string) (history.Artifact, error) {
	a := history.Artifact{Name: name}
	f, e := os.Open(path)
	if e != nil {
		return a, e
	}
	defer f.Close()
	h := sha256.New()
	a.Size, e = io.Copy(h, f)
	a.SHA256 = hex.EncodeToString(h.Sum(nil))
	return a, e
}
func (m *Module) uploadHistory(ctx context.Context, client *http.Client, c historySettings, rec history.Record) error {
	b, _ := json.Marshal(rec)
	var progress history.Progress
	if e := historyRequest(ctx, client, c, "POST", "/api/history/calls", b, &progress); e != nil {
		return e
	}
	if !progress.Success || (progress.Status != "pending" && progress.Status != "archived") {
		return errors.New("History did not acknowledge the manifest")
	}
	if progress.Status == "archived" {
		return nil
	}
	for _, a := range rec.Artifacts {
		offset := progress.Offsets[a.Name]
		if offset < 0 || offset > a.Size {
			return errors.New("invalid remote offset")
		}
		path := filepath.Join(filepath.Dir(m.history.database), "siui-history-outbox", rec.CallID, a.Name)
		if a.Name == "download" {
			path = existingDownloadPath(m.history.database, rec.CallID)
		}
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		_, e = f.Seek(offset, 0)
		buf := make([]byte, history.ChunkSize)
		for e == nil && offset < a.Size {
			n, readErr := io.ReadFull(f, buf[:min(int64(len(buf)), a.Size-offset)])
			if readErr != nil {
				e = readErr
				break
			}
			e = historyRequest(ctx, client, c, "PUT", fmt.Sprintf("/api/history/calls/%s/files/%s?offset=%d", rec.CallID, a.Name, offset), buf[:n], nil)
			offset += int64(n)
		}
		f.Close()
		if e != nil {
			return e
		}
	}
	if e := historyRequest(ctx, client, c, "POST", "/api/history/calls/"+rec.CallID+"/commit", nil, &progress); e != nil {
		return e
	}
	if !progress.Success || progress.Status != "archived" {
		return errors.New("History did not confirm durable archival")
	}
	return nil
}

// Legacy synchronous routes participate in the same durable outbox as catalog calls.
func (m *Module) beginPassiveHistory(r *http.Request, operation string, parameters map[string]string) (string, error) {
	select {
	case <-m.ready:
	default:
		return "", errors.New("SIUI database is starting")
	}
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	id := hex.EncodeToString(b)
	subject, _, _ := m.guard.SessionIdentity(r)
	owner := "local:" + subject
	if c, ok := r.Context().Value(apiIdentityKey{}).(apiClaims); ok {
		owner = c.Owner
		if owner == "" {
			owner = "local:" + c.Subject
		}
	}
	if p, ok := apibridge.PrincipalFrom(r.Context()); ok {
		owner = "wsm:" + strconv.Quote(p.TenantID) + ":" + strconv.Quote(p.Subject)
	}
	params, _ := json.Marshal(parameters)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, e := m.db.Begin()
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	_, e = tx.Exec(`INSERT INTO siui_calls(id,owner,operation_id,operation,digest,status,created_at) VALUES(?,?,?,?,?,'running',?)`, id, owner, id, operation, history.Digest(parameters), now)
	if e == nil {
		_, e = tx.Exec(`INSERT INTO siui_history_outbox(call_id,parameters,source_ip,started_at) VALUES(?,?,?,?)`, id, string(params), r.RemoteAddr, now)
	}
	if e == nil {
		e = m.saveOperationSource(tx, r, id)
	}
	if e == nil {
		e = tx.Commit()
	}
	return id, e
}
func (m *Module) finishPassiveHistory(id string, result any, callErr error) error {
	status, message := "completed", ""
	if callErr != nil {
		status = "failed"
		message = callErr.Error()
		m.mu.Lock()
		licence := m.cfg.Licence
		m.mu.Unlock()
		if licence != "" {
			message = strings.ReplaceAll(message, licence, "[redacted]")
		}
	}
	b, _ := json.Marshal(result)
	tx, e := m.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.Exec(`UPDATE siui_calls SET status=?,result=?,error=? WHERE id=?`, status, string(b), message, id)
	if e == nil {
		_, e = tx.Exec(`UPDATE siui_history_outbox SET completed_at=? WHERE call_id=?`, time.Now().UTC().Format(time.RFC3339Nano), id)
	}
	if e == nil {
		e = tx.Commit()
	}
	return e
}

func historySyncMessage(enabled bool) string {
	if !enabled {
		return "Conexiunea poate funcționa, dar sincronizarea este dezactivată: operațiile pending nu sunt trimise. Activează Sincronizare activă și salvează setările."
	}
	return "Sincronizarea este activă. Testul verifică identitatea; verifică starea cozii pentru erori de transfer sau arhivare."
}
