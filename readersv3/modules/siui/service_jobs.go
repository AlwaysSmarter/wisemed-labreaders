package siui

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const maxDownload = 64 * 1024 * 1024
const downloadChunk = 256 * 1024

type serviceWork struct {
	ID         string
	Operation  Operation
	Parameters map[string]string
	Config     settings
}
type serviceCall struct {
	ID          string          `json:"id"`
	OperationID string          `json:"operation_id"`
	Operation   string          `json:"operation"`
	Status      string          `json:"status"`
	Error       string          `json:"error,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
}

func initializeServiceJobs(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS siui_calls(id TEXT PRIMARY KEY,owner TEXT NOT NULL,operation_id TEXT NOT NULL,operation TEXT NOT NULL,digest TEXT NOT NULL,status TEXT NOT NULL,error TEXT NOT NULL DEFAULT '',result TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL,UNIQUE(owner,operation_id)); UPDATE siui_calls SET status='unknown',error='Process stopped; reconcile with CNAS before submitting again' WHERE status IN ('queued','running')`)
	return err
}
func readServiceCall(db *sql.DB, owner, id string) (serviceCall, error) {
	var c serviceCall
	var result string
	err := db.QueryRow(`SELECT id,operation_id,operation,status,error,result FROM siui_calls WHERE owner=? AND id=?`, owner, id).Scan(&c.ID, &c.OperationID, &c.Operation, &c.Status, &c.Error, &result)
	if result != "" {
		c.Result = json.RawMessage(result)
	}
	return c, err
}
func (m *Module) serviceHTTP(w http.ResponseWriter, r *http.Request, route, owner string, admin bool) {
	if route == "general/ocsp" && (r.Method == "POST" || r.Method == "GET") {
		if !admin {
			fail(w, 403, "administrator required to reveal CNAS session token")
			return
		}
		if r.Method == "GET" {
			m.mu.Lock()
			cfg := m.cfg
			m.mu.Unlock()
			c := client{cfg: cfg}
			respond(w, 200, map[string]any{"ok": true, "debug": c.ocspDebug()})
			return
		}
		var in struct{}
		if err := decode(w, r, &in); err != nil {
			fail(w, 400, err.Error())
			return
		}
		c, release, ok := m.passiveClient(w, "general/ocsp")
		if !ok {
			return
		}
		defer release()
		debug := c.ocspDebug()

		ctx, cancel := context.WithTimeout(r.Context(), ocspTimeout)
		defer cancel()
		headers, err := c.sessionHeaders(ctx)
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		respond(w, 200, map[string]any{"ok": true, "token": headers.Get("OSCP_RESPONSE"), "debug": debug, "authenticated": true, "reused": c.sessionReused, "age_seconds": c.sessionAge, "token_id": c.sessionID, "message": fmt.Sprintf("Folosesc token-ul OCSP %s cu vechime de %d secunde", c.sessionID, c.sessionAge)})
		return
	}
	select {
	case <-m.ready:
	default:
		fail(w, 503, "SIUI worker is starting")
		return
	}
	if strings.HasPrefix(route, "calls/") && r.Method == "GET" {
		id := strings.TrimPrefix(route, "calls/")
		wantFile := strings.HasSuffix(id, "/file")
		if wantFile {
			id = strings.TrimSuffix(id, "/file")
		}
		call, err := readServiceCall(m.db, owner, id)
		if err == sql.ErrNoRows {
			fail(w, 404, "call not found")
			return
		}
		if err != nil {
			fail(w, 500, "cannot read call")
			return
		}
		if wantFile {
			m.serviceFileHTTP(w, r, call)
			return
		}
		respond(w, 200, map[string]any{"ok": true, "call": call})
		return
	}
	group, name, ok := strings.Cut(route, "/")
	op, found := findOperation(group, name)
	if !ok || !found {
		fail(w, 404, "unsupported CNAS operation")
		return
	}
	if r.Method != "POST" {
		fail(w, 405, "POST required")
		return
	}
	if op.Mutating && !admin {
		fail(w, 403, "administrator required for operations that modify CNAS data")
		return
	}
	var in struct {
		OperationID string            `json:"operation_id"`
		Parameters  map[string]string `json:"parameters"`
	}
	if err := decode(w, r, &in); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := validateParameters(op, in.Parameters); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if !operationIDPattern.MatchString(in.OperationID) {
		fail(w, 400, "operation_id required (1–128 identifier characters)")
		return
	}
	if !m.nativeSupported {
		fail(w, 503, "Real CNAS operations require Windows or macOS and a configured token")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfg
	// Bind deduplication to credentials/reference and destinations without storing them.
	b, _ := json.Marshal(struct {
		Operation  string
		Parameters map[string]string
		Config     settings
	}{op.Name, in.Parameters, cfg})
	sum := sha256.Sum256(b)
	digest := hex.EncodeToString(sum[:])
	var existingID, existingDigest string
	err := m.db.QueryRow(`SELECT id,digest FROM siui_calls WHERE owner=? AND operation_id=?`, owner, in.OperationID).Scan(&existingID, &existingDigest)
	if err == nil {
		if existingDigest != digest {
			fail(w, 409, "operation_id belongs to a different request or configuration")
			return
		}
		c, e := readServiceCall(m.db, owner, existingID)
		if e != nil {
			fail(w, 500, "cannot read call")
			return
		}
		respond(w, 200, map[string]any{"ok": true, "duplicate": true, "call": c})
		return
	}
	if err != sql.ErrNoRows {
		fail(w, 500, "cannot reserve operation")
		return
	}
	if cfg.Username == "" || cfg.Licence == "" || cfg.Thumbprint == "" {
		fail(w, 409, "configure CNAS credentials and certificate first")
		return
	}
	if m.checking || len(m.calls) == cap(m.calls) {
		fail(w, 409, "CNAS token or queue busy")
		return
	}
	random := make([]byte, 16)
	if _, err = rand.Read(random); err != nil {
		fail(w, 500, "cannot generate call ID")
		return
	}
	id := hex.EncodeToString(random)
	_, err = m.db.Exec(`INSERT INTO siui_calls(id,owner,operation_id,operation,digest,status,created_at) VALUES(?,?,?,?,?,'queued',?)`, id, owner, in.OperationID, op.Name, digest, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		fail(w, 500, "cannot reserve operation")
		return
	}
	m.pending++
	m.calls <- serviceWork{id, op, in.Parameters, cfg}
	respond(w, 202, map[string]any{"ok": true, "call": serviceCall{ID: id, OperationID: in.OperationID, Operation: op.Name, Status: "queued"}})
}
func (m *Module) runServiceWork(parent context.Context, job serviceWork) {
	defer m.finished()
	if _, err := m.db.Exec(`UPDATE siui_calls SET status='running' WHERE id=?`, job.ID); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 120*time.Second)
	defer cancel()
	c := client{cfg: job.Config, backend: m.factory(job.Config), logf: m.rt.Logf, session: &m.ocspSession, operation: job.Operation.Name, callID: job.ID}
	values, submitted, err := c.invokeOperation(ctx, job.Operation, job.Parameters)
	result := map[string]any{"values": values, "ocsp": map[string]any{"reused": c.sessionReused, "age_seconds": c.sessionAge, "token_id": c.sessionID}}
	if err == nil && job.Operation.Download && (len(values) == 2 || (job.Operation.Name == "getEInvoice" && len(values) == 3)) && values[0] != "" {
		var file map[string]any
		file, err = m.downloadServiceFile(ctx, &c, job.ID, values, job.Operation.Service)
		if err == nil {
			result["file"] = file
		}
	}
	status, message := "completed", ""
	if err != nil {
		status = "failed"
		if submitted {
			status = "unknown"
		}
		message = err.Error()
		for _, secret := range []string{job.Config.Licence} {
			if secret != "" {
				message = strings.ReplaceAll(message, secret, "[redacted]")
			}
		}
		if len(message) > 4096 {
			message = message[:4096]
		}
	}
	b := []byte("")
	if err == nil {
		b, _ = json.Marshal(result)
	}
	if _, e := m.db.Exec(`UPDATE siui_calls SET status=?,error=?,result=? WHERE id=?`, status, message, string(b), job.ID); e != nil {
		m.rt.Logf("SIUI call persistence failed id=%s", job.ID)
	}
	if err != nil {
		m.rt.Logf("[ERROR] SIUI operation=%s id=%s status=%s", job.Operation.Name, job.ID, status)
	} else {
		m.rt.Logf("SIUI operation=%s id=%s status=%s", job.Operation.Name, job.ID, status)
	}
}
func (m *Module) downloadServiceFile(ctx context.Context, c *client, id string, values []string, service ...string) (result map[string]any, err error) {
	target := ""
	defer func() { err = withConnectionError(target, err) }()
	size, err := strconv.ParseInt(values[1], 10, 64)
	if err != nil || size < 1 || size > maxDownload {
		return nil, errors.New("CNAS download size invalid or exceeds 64 MiB")
	}
	base := c.cfg.BaseURL
	if len(service) > 0 {
		base = c.cfg.serviceBaseURL(service[0])
	}
	address, err := downloadAddress(values[0], base)
	if err != nil {
		return nil, err
	}
	h, err := c.sessionHeaders(ctx)
	if err != nil {
		return nil, err
	}
	h.Del("SOAPAction")
	h.Del("Content-Type")
	target = address
	code, headers, data, err := c.backend.Do(ctx, "GET", address, h, nil)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("CNAS download HTTP %d", code)
	}
	if len(data) == 0 || len(data) > maxDownload {
		return nil, errors.New("empty or oversized CNAS download")
	}
	dir := filepath.Join(filepath.Dir(c.cfg.Database), "siui-downloads")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(dir, id), data, 0600); err != nil {
		return nil, errors.New("cannot save CNAS download")
	}
	filename := id + ".download"
	if bytes.HasPrefix(data, []byte("PK")) {
		filename = id + ".zip"
	} else if bytes.HasPrefix(bytes.TrimSpace(data), []byte("<")) {
		filename = id + ".xml"
	}
	mime := headers.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}
	return map[string]any{"size": len(data), "content_type": mime, "filename": filename, "path": "/api/siui/calls/" + id + "/file", "chunk_size": downloadChunk}, nil
}
func (m *Module) serviceFileHTTP(w http.ResponseWriter, r *http.Request, call serviceCall) {
	var result struct {
		File *struct {
			Size int64 `json:"size"`
		} `json:"file"`
	}
	if json.Unmarshal(call.Result, &result) != nil || result.File == nil || call.Status != "completed" {
		fail(w, 404, "download unavailable")
		return
	}
	offset := int64(0)
	var err error
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.ParseInt(raw, 10, 64)
	}
	if err != nil || offset < 0 || offset > result.File.Size {
		fail(w, 400, "invalid download offset")
		return
	}
	m.mu.Lock()
	dir := filepath.Dir(m.cfg.Database)
	m.mu.Unlock()
	file, err := os.Open(filepath.Join(dir, "siui-downloads", call.ID))
	if err != nil {
		fail(w, 404, "download unavailable")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.NewSectionReader(file, offset, downloadChunk))
	if err != nil {
		fail(w, 500, "cannot read download")
		return
	}
	respond(w, 200, map[string]any{"ok": true, "offset": offset, "next_offset": offset + int64(len(data)), "eof": offset+int64(len(data)) >= result.File.Size, "data_base64": base64.StdEncoding.EncodeToString(data)})
}
