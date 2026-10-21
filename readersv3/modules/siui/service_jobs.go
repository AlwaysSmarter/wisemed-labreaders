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
	"wisemed-labreaders/readersv3/shared/history"
)

const maxDownload = 64 * 1024 * 1024
const downloadChunk = 256 * 1024

type serviceWork struct {
	ID             string
	Operation      Operation
	Parameters     map[string]string
	NullParameters []string
	Config         settings
	Callback       *wmCallback
	CallbackPath   string
}
type serviceCall struct {
	ID          string           `json:"id"`
	OperationID string           `json:"operation_id"`
	Operation   string           `json:"operation"`
	Status      string           `json:"status"`
	Error       string           `json:"error,omitempty"`
	Result      json.RawMessage  `json:"result,omitempty"`
	History     *historyDelivery `json:"history,omitempty"`
	Callback    *wmDelivery      `json:"callwmapi,omitempty"`
}

func initializeServiceJobs(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS siui_calls(id TEXT PRIMARY KEY,owner TEXT NOT NULL,operation_id TEXT NOT NULL,operation TEXT NOT NULL,digest TEXT NOT NULL,status TEXT NOT NULL,error TEXT NOT NULL DEFAULT '',result TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL,UNIQUE(owner,operation_id)); UPDATE siui_calls SET status='unknown',error='Process stopped; reconcile with CNAS before submitting again' WHERE status IN ('queued','running')`)
	if err != nil {
		return err
	}
	if err = initializeHistory(db); err != nil {
		return err
	}
	return initializeCallbacks(db)
}
func readServiceCall(db *sql.DB, owner, id string) (serviceCall, error) {
	var c serviceCall
	var result string
	err := db.QueryRow(`SELECT id,operation_id,operation,status,error,result FROM siui_calls WHERE owner=? AND id=?`, owner, id).Scan(&c.ID, &c.OperationID, &c.Operation, &c.Status, &c.Error, &result)
	if result != "" {
		c.Result = json.RawMessage(result)
	}
	if err == nil {
		var delivery wmDelivery
		e := db.QueryRow(`SELECT status,attempts,http_status,error FROM siui_wm_callbacks WHERE call_id=?`, id).Scan(&delivery.Status, &delivery.Attempts, &delivery.HTTPStatus, &delivery.Error)
		if e == nil {
			c.Callback = &delivery
		} else if e != sql.ErrNoRows {
			return c, e
		}
	}
	if err == nil {
		var h historyDelivery
		if e := db.QueryRow(`SELECT status,attempts,error FROM siui_history_outbox WHERE call_id=?`, id).Scan(&h.Status, &h.Attempts, &h.Error); e == nil {
			c.History = &h
		}
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
	if strings.HasPrefix(route, "calls/") && strings.HasSuffix(route, "/callwmapi/retry") && r.Method == "POST" {
		id := strings.TrimSuffix(strings.TrimPrefix(route, "calls/"), "/callwmapi/retry")
		if _, err := readServiceCall(m.db, owner, id); err != nil {
			fail(w, 404, "call not found")
			return
		}
		result, err := m.db.Exec(`UPDATE siui_wm_callbacks SET status='pending',attempts=0,next_attempt=0,error='' WHERE call_id=? AND status='failed' AND body!=''`, id)
		if err != nil {
			fail(w, 500, "cannot retry callback")
			return
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			fail(w, 409, "only failed deliveries with a prepared body can be retried")
			return
		}
		respond(w, 202, map[string]any{"ok": true, "message": "WiseMED delivery queued; CNAS will not be called again"})
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
		OperationID string             `json:"operation_id"`
		Parameters  map[string]*string `json:"parameters"`
		Callback    *wmCallback        `json:"callwmapi,omitempty"`
	}
	if err := decode(w, r, &in); err != nil {
		fail(w, 400, err.Error())
		return
	}
	parameters := make(map[string]string, len(in.Parameters))
	var nullParameters []string
	for name, value := range in.Parameters {
		if value == nil {
			parameters[name] = ""
			nullParameters = append(nullParameters, name)
		} else {
			parameters[name] = *value
		}
	}
	if err := validateParameters(op, parameters, nullParameters...); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if !operationIDPattern.MatchString(in.OperationID) {
		fail(w, 400, "operation_id required (1–128 identifier characters)")
		return
	}
	var callbackPath, callbackDestination string
	if in.Callback != nil {
		if err := in.Callback.validate(); err != nil {
			fail(w, 400, err.Error())
			return
		}
		var err error
		callbackPath, err = in.Callback.path(parameters)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		sender, ok := m.callbackSender()
		if !ok {
			fail(w, 503, "WiseMED API unavailable")
			return
		}
		callbackDestination, err = sender.CallbackDestination()
		if err != nil {
			fail(w, 409, "configure WiseMED API before requesting callbacks")
			return
		}
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
		Parameters map[string]*string
		Config     settings
	}{op.Name, in.Parameters, cfg})
	if in.Callback != nil {
		b, _ = json.Marshal(struct {
			Request     json.RawMessage
			Callback    *wmCallback
			Destination string
		}{b, in.Callback, callbackDestination})
	}
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
	tx, err := m.db.Begin()
	if err != nil {
		fail(w, 500, "cannot reserve operation")
		return
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO siui_calls(id,owner,operation_id,operation,digest,status,created_at) VALUES(?,?,?,?,?,'queued',?)`, id, owner, in.OperationID, op.Name, digest, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		fail(w, 500, "cannot reserve operation")
		return
	}
	if err = m.saveOperationSource(tx, r, id); err != nil {
		fail(w, 500, "cannot persist operation source")
		return
	}
	parametersHistory, _ := json.Marshal(in.Parameters)
	_, err = tx.Exec(`INSERT INTO siui_history_outbox(call_id,parameters,source_ip) VALUES(?,?,?)`, id, string(parametersHistory), r.RemoteAddr)
	if err != nil {
		fail(w, 500, "cannot reserve History outbox")
		return
	}
	if in.Callback != nil {
		spec, sealErr := encodeWMCallback(cfg.Database, id, in.Callback)
		if sealErr != nil {
			fail(w, 500, "cannot protect WiseMED callback")
			return
		}
		parameters, _ := json.Marshal(in.Parameters)
		_, err = tx.Exec(`INSERT INTO siui_wm_callbacks(call_id,spec,parameters,destination) VALUES(?,?,?,?)`, id, string(spec), string(parameters), callbackDestination)
		if err != nil {
			fail(w, 500, "cannot reserve WiseMED callback")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		fail(w, 500, "cannot persist operation")
		return
	}
	m.pending++
	m.calls <- serviceWork{ID: id, Operation: op, Parameters: parameters, NullParameters: nullParameters, Config: cfg, Callback: in.Callback, CallbackPath: callbackPath}
	respond(w, 202, map[string]any{"ok": true, "call": serviceCall{ID: id, OperationID: in.OperationID, Operation: op.Name, Status: "queued"}})
}
func (m *Module) runServiceWork(parent context.Context, job serviceWork) {
	defer m.finished()
	if _, err := m.db.Exec(`UPDATE siui_calls SET status='running' WHERE id=?`, job.ID); err != nil {
		return
	}
	_, _ = m.db.Exec(`UPDATE siui_history_outbox SET started_at=? WHERE call_id=?`, time.Now().UTC().Format(time.RFC3339Nano), job.ID)
	ctx, cancel := context.WithTimeout(parent, 120*time.Second)
	defer cancel()
	c := client{cfg: job.Config, backend: m.factory(job.Config), logf: m.rt.Logf, session: &m.ocspSession, operation: job.Operation.Name, callID: job.ID}
	values, submitted, err := c.invokeOperation(ctx, job.Operation, job.Parameters, job.NullParameters...)
	result := map[string]any{"values": values, "ocsp": map[string]any{"reused": c.sessionReused, "age_seconds": c.sessionAge, "token_id": c.sessionID}}
	if err == nil {
		enrichInsuredResult(job.Operation.Name, values, result)
	}
	if err == nil && job.Operation.Download && (len(values) == 2 || (job.Operation.Name == "getEInvoice" && len(values) == 3)) && values[0] != "" {
		var file map[string]any
		file, err = m.downloadServiceFile(ctx, &c, job.ID, values, job.Operation.Service)
		if err == nil {
			file["validation_schema"] = resultFileSchema(job.Operation.Name, job.Parameters)
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
	var callbackBody []byte
	var callbackErr error
	if err == nil && job.Callback != nil {
		callbackBody, callbackErr = job.Callback.prepareBody(ctx, job.Config.Database, job.ID, result)
	}
	tx, persistErr := m.db.Begin()
	if persistErr == nil {
		defer tx.Rollback()
		_, persistErr = tx.Exec(`UPDATE siui_calls SET status=?,error=?,result=? WHERE id=?`, status, message, string(b), job.ID)
		if persistErr == nil {
			_, persistErr = tx.Exec(`UPDATE siui_history_outbox SET completed_at=? WHERE call_id=?`, time.Now().UTC().Format(time.RFC3339Nano), job.ID)
		}
		if persistErr == nil && job.Callback != nil {
			deliveryStatus, deliveryError, body := "skipped", "CNAS operation did not complete", []byte(nil)
			if err == nil {
				body = callbackBody
				transformErr := callbackErr
				if transformErr != nil {
					deliveryStatus, deliveryError = "failed", transformErr.Error()
				} else {
					deliveryStatus, deliveryError = "pending", ""
				}
			}
			_, persistErr = tx.Exec(`UPDATE siui_wm_callbacks SET status=?,error=?,path=?,body=? WHERE call_id=?`, deliveryStatus, deliveryError, job.CallbackPath, string(body), job.ID)
		}
		if persistErr == nil {
			persistErr = tx.Commit()
		}
	}
	if persistErr != nil {
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
	code, _, data, err := c.backend.Do(ctx, "GET", address, h, nil)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("CNAS download HTTP %d", code)
	}
	if len(data) == 0 || len(data) > maxDownload {
		return nil, errors.New("empty or oversized CNAS download")
	}
	downloadStorageMu.RLock()
	defer downloadStorageMu.RUnlock()
	path := datedDownloadPath(c.cfg.Database, id, time.Now().UTC())
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		return nil, errors.New("cannot save CNAS download")
	}
	filename, mime := history.DownloadIdentity(address, id, data)
	return map[string]any{"size": len(data), "content_type": mime, "filename": filename, "path": "/api/siui/calls/" + id + "/file", "chunk_size": downloadChunk}, nil
}
func (m *Module) serviceFileHTTP(w http.ResponseWriter, r *http.Request, call serviceCall) {
	downloadStorageMu.RLock()
	defer downloadStorageMu.RUnlock()
	var result struct {
		File *struct {
			Size             int64  `json:"size"`
			ValidationSchema string `json:"validation_schema"`
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
	if err != nil || offset < 0 || offset > processedFileLimit {
		fail(w, 400, "invalid download offset")
		return
	}
	m.mu.Lock()
	database := m.cfg.Database
	m.mu.Unlock()
	file, err := os.Open(existingDownloadPath(database, call.ID))
	if err != nil {
		if os.IsNotExist(err) {
			entry, e := scanOperation(m.db.QueryRow(operationSelect+" WHERE c.id=?", call.ID))
			if e == nil && entry.History.Status == "archived" {
				q := r.URL.Query()
				if q.Get("unzip") != "" || q.Get("entry") != "" || q.Has("schema") || q.Has("validateXSD") || q.Get("format") != "" {
					fail(w, 409, "local copy removed; download original from History before further processing")
					return
				}
				if !q.Has("offset") {
					cloned := r.Clone(r.Context())
					u := *r.URL
					q.Set("offset", "0")
					u.RawQuery = q.Encode()
					cloned.URL = &u
					r = cloned
				}
				m.operationHistoryHTTP(w, r, entry, []string{"history", "download"})
				return
			}
		}
		fail(w, 404, "download unavailable")
		return
	}
	defer file.Close()
	var source io.ReaderAt = file
	size := result.File.Size
	q := r.URL.Query()
	if q.Get("unzip") != "" || q.Get("entry") != "" || q.Has("schema") || q.Has("validateXSD") || q.Get("format") != "" {
		raw, readErr := io.ReadAll(io.LimitReader(file, processedFileLimit+1))
		if readErr != nil {
			fail(w, 500, "cannot read download")
			return
		}
		processed, processErr := processCallFile(r.Context(), raw, q, result.File.ValidationSchema)
		if processErr != nil {
			fail(w, 422, processErr.Error())
			return
		}
		source = bytes.NewReader(processed)
		size = int64(len(processed))
	}
	if offset > size {
		fail(w, 400, "invalid download offset")
		return
	}
	data, err := io.ReadAll(io.NewSectionReader(source, offset, downloadChunk))
	if err != nil {
		fail(w, 500, "cannot read download")
		return
	}
	respond(w, 200, map[string]any{"ok": true, "offset": offset, "next_offset": offset + int64(len(data)), "eof": offset+int64(len(data)) >= size, "data_base64": base64.StdEncoding.EncodeToString(data)})
}
