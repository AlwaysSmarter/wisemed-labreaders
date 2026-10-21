package siuidata

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"wisemed-labreaders/readersv3/modules/ws"
)

type bridgeClient interface {
	Connected() bool
	Exchange(context.Context, ws.Envelope) (map[string]interface{}, error)
}
type bridgePeer struct {
	ID         string `json:"id"`
	ClientType string `json:"client_type"`
	ReaderID   string `json:"reader_id"`
	Label      string `json:"label"`
}

func (m *Module) bridgeClient() (bridgeClient, bool) {
	service, ok := m.rt.Service("wisemed-ws-client")
	if !ok {
		return nil, false
	}
	client, ok := service.(bridgeClient)
	return client, ok && client.Connected()
}
func bridgePeers(ctx context.Context, client bridgeClient) ([]bridgePeer, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	response, e := client.Exchange(ctx, ws.Envelope{Type: "list_connections"})
	if e != nil {
		return nil, e
	}
	b, _ := json.Marshal(response["connections"])
	var all []bridgePeer
	if e = json.Unmarshal(b, &all); e != nil {
		return nil, errors.New("invalid WSM connections response")
	}
	peers := []bridgePeer{}
	for _, peer := range all {
		if peer.ClientType == "siui-bridge" {
			peers = append(peers, peer)
		}
	}
	return peers, nil
}

func (m *Module) bridgeHTTP(w http.ResponseWriter, r *http.Request, route string) {
	client, connected := m.bridgeClient()
	if route == "bridge/connections" {
		if r.Method != "GET" {
			failure(w, 405, "GET required")
			return
		}
		f, e := parseFilters(r.URL.Query())
		if e != nil {
			failure(w, 400, e.Error())
			return
		}
		limit, start, e := pagination(f)
		if e != nil {
			failure(w, 400, e.Error())
			return
		}
		if !connected {
			listOutput(w, 200, []bridgePeer{}, 0, map[string]any{"connected": false, "limit": limit, "start": start})
			return
		}
		peers, e := bridgePeers(r.Context(), client)
		if e != nil {
			failure(w, 503, "cannot list WSM bridges; check connections:read permission")
			return
		}
		total := len(peers)
		lo := start
		if lo > total {
			lo = total
		}
		hi := lo + limit
		if hi > total {
			hi = total
		}
		listOutput(w, 200, peers[lo:hi], int64(total), map[string]any{"connected": true, "limit": limit, "start": start})
		return
	}
	if r.Method != "POST" {
		failure(w, 405, "POST required")
		return
	}
	_, admin := m.identity(r)
	if !admin {
		failure(w, 403, "administrator required")
		return
	}
	if !connected {
		failure(w, 503, "WSM is not connected")
		return
	}
	var in struct {
		Category     string `json:"partnerCategory"`
		ConnectionID string `json:"connection_id"`
	}
	if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); e != nil {
		failure(w, 400, "invalid sync request")
		return
	}
	typ, e := catalogueType(in.Category)
	if e != nil {
		failure(w, 400, "import is currently supported for CLIN, PARA, PHM and PHYS")
		return
	}
	peers, e := bridgePeers(r.Context(), client)
	if e != nil {
		failure(w, 503, "cannot list WSM bridges; check permissions")
		return
	}
	if in.ConnectionID == "" && len(peers) == 1 {
		in.ConnectionID = peers[0].ID
	}
	found := false
	for _, peer := range peers {
		if peer.ID == in.ConnectionID {
			found = true
		}
	}
	if !found {
		failure(w, 400, "select a connected siui-bridge")
		return
	}
	m.workMu.Lock()
	if m.closing || m.running >= 2 {
		m.workMu.Unlock()
		failure(w, 409, "import capacity busy")
		return
	}
	m.running++
	m.workWG.Add(1)
	m.workMu.Unlock()
	transferred := false
	defer func() {
		if !transferred {
			m.finishWork()
		}
	}()
	m.cfgMu.RLock()
	cfg := m.cfg
	m.cfgMu.RUnlock()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	db, e := m.database(ctx)
	if e != nil {
		failure(w, 503, e.Error())
		return
	}
	conn, e := db.Acquire(ctx)
	if e != nil {
		failure(w, 503, "PostgreSQL busy")
		return
	}
	key := catalogueLockKey(typ)
	var locked bool
	if e = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(1936288117,$1)`, key).Scan(&locked); e != nil || !locked {
		conn.Release()
		failure(w, 409, "an import of this category is already running")
		return
	}
	defer func() {
		if !transferred {
			releaseImportConn(conn, key)
		}
	}()
	if _, e = conn.Exec(ctx, `UPDATE siui_data_imports SET status='failed',finished_at=now(),error='worker interrupted before activation' WHERE catalogue_type=$1 AND status IN ('queued','parsing','downloading')`, typ); e != nil {
		failure(w, 500, "cannot recover interrupted sync")
		return
	}
	var newest *string
	if e = conn.QueryRow(ctx, `SELECT max(source_date)::text FROM siui_data_imports WHERE catalogue_type=$1 AND status='ready'`, typ).Scan(&newest); e != nil {
		failure(w, 503, "cannot read current catalogue date")
		return
	}
	var lastChecked *time.Time
	if e = conn.QueryRow(ctx, `SELECT max(created_at) FROM siui_data_imports WHERE catalogue_type=$1 AND source_kind='wsm' AND status IN ('ready','unchanged')`, typ).Scan(&lastChecked); e != nil {
		failure(w, 503, "cannot read last successful WSM check")
		return
	}
	start := catalogueCheckStart(time.Now().In(m.loc), lastChecked)
	id := uuid.NewString()
	asOf := time.Now().In(m.loc).Format("2006-01-02")
	if _, e = conn.Exec(ctx, `INSERT INTO siui_data_imports(id,catalogue_type,status,as_of,source_kind,source_filename) VALUES($1,$2,'downloading',$3,'wsm',$4)`, id, typ, asOf, in.ConnectionID); e != nil {
		failure(w, 500, "cannot create sync job")
		return
	}
	transferred = true
	go func() {
		defer m.finishWork()
		defer releaseImportConn(conn, key)
		ctx, cancel := context.WithTimeout(m.workCtx, time.Duration(cfg.ImportMinutes)*time.Minute)
		defer cancel()
		if e := m.syncBridge(ctx, client, conn, cfg, id, typ, asOf, in.ConnectionID, start, newest); e != nil {
			m.rt.Logf("siui-data WSM sync %s failed: %v", id, e)
			m.markImportFailed(id, e)
		}
	}()
	output(w, 202, map[string]any{"importId": id, "status": "downloading", "statusUrl": prefix + "imports/" + id})
}

func callBridge(ctx context.Context, client bridgeClient, target, method, path string, body any) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	args := map[string]any{"method": method, "path": path}
	if body != nil {
		args["body"] = body
	}
	response, e := client.Exchange(ctx, ws.Envelope{Type: "command", Target: &ws.Target{Mode: "connection", ConnectionID: target}, Payload: map[string]interface{}{"command": "api.request", "args": args}})
	if e != nil {
		return nil, e
	}
	raw, _ := json.Marshal(response)
	var wire struct {
		Status int            `json:"status"`
		Body   map[string]any `json:"body"`
	}
	if json.Unmarshal(raw, &wire) != nil || wire.Status == 0 {
		return nil, errors.New("invalid bridge API response")
	}
	if wire.Status < 200 || wire.Status >= 300 {
		return nil, fmt.Errorf("bridge HTTP %d: %v", wire.Status, wire.Body["error"])
	}
	return wire.Body, nil
}

type bridgeCall struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error"`
	Result struct {
		Values []string `json:"values"`
		File   *struct {
			Size     int64  `json:"size"`
			Filename string `json:"filename"`
		} `json:"file"`
	} `json:"result"`
}

func decodeBridgeCall(body map[string]any) (bridgeCall, error) {
	var call bridgeCall
	b, _ := json.Marshal(body["call"])
	if e := json.Unmarshal(b, &call); e != nil || call.ID == "" {
		return call, errors.New("invalid CNAS call response")
	}
	return call, nil
}

// A successful check uses its request start time so changes published during a
// long download are not skipped. Failed checks never advance the watermark.
func catalogueCheckStart(now time.Time, lastChecked *time.Time) string {
	if lastChecked != nil {
		return lastChecked.UTC().Format(time.RFC3339)
	}
	return time.Date(now.Year()-1, time.January, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
}

func (m *Module) syncBridge(ctx context.Context, client bridgeClient, conn *pgxpool.Conn, c settings, id, typ, asOf, target, start string, newest *string) error {
	// CNAS may return its latest catalogue for a historical start. Only the
	// downloaded XML/filename determines source_date and whether to import.
	operation, parameters := "getCatalogues", map[string]string{"partnerCategory": typ, "start": start}
	if typ == "PHYS" {
		operation = "getPhysicianCatalogues"
		parameters = map[string]string{}
	}
	body, e := callBridge(ctx, client, target, "POST", "/api/siui/general/"+operation, map[string]any{"operation_id": "siui-data-" + id, "parameters": parameters})
	if e != nil {
		return e
	}
	call, e := decodeBridgeCall(body)
	if e != nil {
		return e
	}
	for call.Status == "queued" || call.Status == "running" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
		body, e = callBridge(ctx, client, target, "GET", "/api/siui/calls/"+url.PathEscape(call.ID), nil)
		if e != nil {
			return e
		}
		call, e = decodeBridgeCall(body)
		if e != nil {
			return e
		}
	}
	if call.Status != "completed" {
		return fmt.Errorf("CNAS call %s: %s", call.Status, call.Error)
	}
	if call.Result.File == nil {
		if len(call.Result.Values) > 0 && call.Result.Values[0] != "" {
			return errors.New("bridge completed without a downloadable catalogue file")
		}
		return markUnchanged(ctx, conn, id, "Bridge-ul nu a returnat un nomenclator nou.")
	}
	if call.Result.File.Size < 1 || call.Result.File.Size > c.MaxUploadMiB<<20 {
		return errors.New("bridge download exceeds configured ZIP limit")
	}
	dir := m.rt.ResolvePath(c.UploadDir)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, "wsm-*.zip")
	if e != nil {
		return e
	}
	defer f.Close()
	defer os.Remove(f.Name())
	hash := sha256.New()
	offset := int64(0)
	for offset < call.Result.File.Size {
		body, e = callBridge(ctx, client, target, "GET", fmt.Sprintf("/api/siui/calls/%s/file?offset=%d", url.PathEscape(call.ID), offset), nil)
		if e != nil {
			return e
		}
		b, _ := json.Marshal(body)
		var chunk struct {
			Offset int64  `json:"offset"`
			Next   int64  `json:"next_offset"`
			EOF    bool   `json:"eof"`
			Data   string `json:"data_base64"`
		}
		if e = json.Unmarshal(b, &chunk); e != nil {
			return e
		}
		if len(chunk.Data) > 1<<20 {
			return errors.New("oversized bridge chunk")
		}
		data, e := base64.StdEncoding.DecodeString(chunk.Data)
		if e != nil {
			return errors.New("invalid bridge chunk encoding")
		}
		if chunk.Offset != offset || len(data) == 0 || chunk.Next != offset+int64(len(data)) || chunk.Next > call.Result.File.Size || chunk.EOF != (chunk.Next == call.Result.File.Size) {
			return errors.New("invalid or stalled bridge download offsets")
		}
		if _, e = io.MultiWriter(f, hash).Write(data); e != nil {
			return e
		}
		offset = chunk.Next
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if sourceType, _ := sourceFilename(call.Result.File.Filename); sourceType != "" && sourceType != typ {
		return errors.New("bridge file does not match requested partnerCategory")
	}
	filename, issue, e := prepareBridgeCatalogue(ctx, f, c.MaxXMLMiB<<20, call.Result.File.Filename, typ)
	if e != nil {
		return e
	}
	if filename != f.Name() {
		defer os.Remove(filename)
	}
	if issue == "" {
		return errors.New("download has no valid issueDate or dated CNAS filename; cannot prove it is newer")
	}
	if _, e = conn.Exec(ctx, `UPDATE siui_data_imports SET source_date=$2,source_filename=$3,source_sha256=$4 WHERE id=$1`, id, issue, filepath.Base(call.Result.File.Filename), hex.EncodeToString(hash.Sum(nil))); e != nil {
		return e
	}
	if typ == "PHYS" {
		dates, e := physicianArchiveDates(f, c.MaxXMLMiB<<20)
		if e != nil {
			return e
		}
		var previous map[string]string
		e = conn.QueryRow(ctx, `SELECT COALESCE((SELECT i.source_dates FROM siui_data_active a JOIN siui_data_imports i ON i.id=a.import_id WHERE a.catalogue_type='PHYS'),'{}'::jsonb)`).Scan(&previous)
		if e != nil {
			return e
		}
		newer := false
		for source, day := range dates {
			if previous[source] != "" && day < previous[source] {
				return fmt.Errorf("downloaded physician source %s is older than the active version", source)
			}
			if day > previous[source] {
				newer = true
			}
		}
		if _, e = conn.Exec(ctx, `UPDATE siui_data_imports SET source_dates=$2 WHERE id=$1`, id, dates); e != nil {
			return e
		}
		if !newer {
			return markUnchanged(ctx, conn, id, "Niciuna dintre sursele CNAS/OPSNAJ nu este mai nouă decât versiunea activă.")
		}
	} else if newest != nil && issue <= *newest {
		return markUnchanged(ctx, conn, id, "Nomenclatorul disponibil nu este mai nou decât ultima versiune importată.")
	}
	return runImport(ctx, conn, id, typ, asOf, filename, c.MaxXMLMiB<<20)
}

func markUnchanged(ctx context.Context, conn *pgxpool.Conn, id, message string) error {
	_, e := conn.Exec(ctx, `UPDATE siui_data_imports SET status='unchanged',finished_at=now(),error=$2 WHERE id=$1`, id, message)
	return e
}

// Convert raw/GZIP XML to a bounded disk file; ZIPs remain ZIPs. The issue date
// is read from the root without decoding the entire catalogue twice.
func prepareBridgeCatalogue(ctx context.Context, f *os.File, maxXML int64, name string, types ...string) (string, string, error) {
	info, e := f.Stat()
	if e != nil {
		return "", "", e
	}
	magic := make([]byte, 4)
	f.ReadAt(magic, 0)
	var reader io.ReadCloser
	filename := f.Name()
	if string(magic[:2]) == "PK" {
		members, e := archiveMembers(f, info.Size(), maxXML)
		if e != nil {
			return "", "", e
		}
		var issue string
		physicians := len(types) > 0 && types[0] == "PHYS"
		for _, member := range members {
			r, e := member.Open()
			if e != nil {
				return "", "", e
			}
			day, e := xmlIssueDate(r)
			r.Close()
			if e != nil {
				return "", "", e
			}
			if day != "" {
				if issue != "" && issue != day && !physicians {
					return "", "", errors.New("ZIP XML issue dates differ")
				}
				if day > issue {
					issue = day
				}
			}
		}
		if issue == "" {
			_, issue = sourceFilename(name)
		}
		return filename, issue, nil
	}
	reader = io.NopCloser(io.NewSectionReader(f, 0, info.Size()))
	if magic[0] == 0x1f && magic[1] == 0x8b {
		reader, e = gzip.NewReader(reader)
		if e != nil {
			return "", "", e
		}
	}
	defer reader.Close()
	tmp, e := os.CreateTemp(filepath.Dir(f.Name()), "wsm-*.xml")
	if e != nil {
		return "", "", e
	}
	n, e := io.Copy(tmp, io.LimitReader(contextReader{ctx, reader}, maxXML+1))
	closeErr := tmp.Close()
	if e != nil || closeErr != nil || n > maxXML {
		os.Remove(tmp.Name())
		return "", "", errors.New("invalid or oversized expanded bridge XML")
	}
	x, e := os.Open(tmp.Name())
	if e != nil {
		os.Remove(tmp.Name())
		return "", "", e
	}
	defer x.Close()
	issue, e := xmlIssueDate(x)
	if e != nil {
		os.Remove(tmp.Name())
		return "", "", e
	}
	if issue == "" {
		_, issue = sourceFilename(name)
	}
	return tmp.Name(), issue, nil
}
func xmlIssueDate(r io.Reader) (string, error) {
	d := xml.NewDecoder(io.LimitReader(r, 1<<20))
	for {
		token, e := d.Token()
		if e != nil {
			return "", e
		}
		switch v := token.(type) {
		case xml.Directive:
			return "", errors.New("XML directives forbidden")
		case xml.StartElement:
			if v.Name.Local != "Catalogues" {
				return "", errors.New("expected Catalogues root")
			}
			for _, a := range v.Attr {
				if a.Name.Local == "issueDate" {
					if len(a.Value) < 10 {
						return "", errors.New("invalid issueDate")
					}
					return date(a.Value[:10])
				}
			}
			return "", nil
		}
	}
}
