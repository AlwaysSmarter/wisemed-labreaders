package siuihistory

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

type archiveTotals struct {
	Calls         int64 `json:"calls"`
	Archived      int64 `json:"archived"`
	Pending       int64 `json:"pending"`
	Files         int64 `json:"files"`
	PayloadBytes  int64 `json:"payload_bytes"`
	ManifestBytes int64 `json:"manifest_bytes"`
	PendingBytes  int64 `json:"pending_bytes"`
}
type bridgeLoad struct {
	ID            string `json:"id"`
	Authorized    bool   `json:"authorized"`
	Calls         int64  `json:"calls"`
	Completed     int64  `json:"completed"`
	Failed        int64  `json:"failed"`
	Unknown       int64  `json:"unknown"`
	Pending       int64  `json:"pending"`
	ArchivedBytes int64  `json:"archived_bytes"`
	LastReceived  string `json:"last_received,omitempty"`
	LastSeen      string `json:"last_seen,omitempty"`
}
type archiveBucket struct {
	At     string `json:"at"`
	Status string `json:"status"`
	Count  int64  `json:"count"`
}
type pendingArchive struct {
	Bridge     string `json:"bridge"`
	CallID     string `json:"call_id"`
	Operation  string `json:"operation"`
	CreatedAt  string `json:"created_at"`
	ReceivedAt string `json:"received_at"`
}

func (m *Module) dashboardHTTP(w http.ResponseWriter, r *http.Request, tenant string) {
	if r.Method != "GET" {
		failure(w, 405, errors.New("GET required"))
		return
	}
	from, to, err := callPeriod(r)
	if err != nil || from == "" || to == "" {
		failure(w, 400, errors.New("from and to must be RFC3339 timestamps"))
		return
	}
	a, _ := time.Parse(time.RFC3339Nano, from)
	b, _ := time.Parse(time.RFC3339Nano, to)
	if !b.After(a) || b.Sub(a) > 366*24*time.Hour {
		failure(w, 400, errors.New("interval must be positive, maximum 366 days"))
		return
	}
	m.mu.Lock()
	db, err := m.database(r.Context())
	keys := append([]bridgeKey(nil), m.cfg.Keys...)
	storage := m.cfg.Storage
	m.mu.Unlock()
	if err != nil {
		failure(w, 503, err)
		return
	}
	if !validName(tenant) {
		failure(w, 409, errors.New("configure tenant first"))
		return
	}
	// At most eight artifacts are allowed by the ingestion protocol. Aggregate in
	// SQL rather than loading every manifest/file into the dashboard process.
	bytesSQL := "0"
	for i := 0; i < 8; i++ {
		bytesSQL += fmt.Sprintf("+COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(record,'$.artifacts[%d].size')) AS UNSIGNED),0)", i)
	}
	bucketSeconds := 3600
	if b.Sub(a) > 48*time.Hour {
		bucketSeconds = 86400
	}
	query := `SELECT bridge,status,JSON_UNQUOTE(JSON_EXTRACT(record,'$.status')) AS result_status,
 CASE WHEN ct>=? AND ct<? THEN FLOOR(TIMESTAMPDIFF(SECOND,STR_TO_DATE(LEFT(?,19),'%Y-%m-%dT%H:%i:%s'),STR_TO_DATE(LEFT(ct,19),'%Y-%m-%dT%H:%i:%s'))/?) ELSE -1 END AS bucket,
 COUNT(*),COALESCE(SUM(` + bytesSQL + `),0),COALESCE(SUM(JSON_LENGTH(record,'$.artifacts')),0),COALESCE(SUM(OCTET_LENGTH(record)),0),MAX(received_at)
 FROM (SELECT bridge,status,record,received_at,` + strings.ReplaceAll(callTimeSQL, "created_at", "received_at") + ` AS ct FROM siui_history_calls WHERE tenant=?) c
 GROUP BY bridge,status,result_status,bucket`
	rows, err := db.QueryContext(r.Context(), query, from, to, from, bucketSeconds, tenant)
	if err != nil {
		failure(w, 500, err)
		return
	}
	totals := archiveTotals{}
	counts := map[string]int64{"completed": 0, "failed": 0, "unknown": 0}
	loads := map[string]*bridgeLoad{}
	getBridge := func(id string) *bridgeLoad {
		if loads[id] == nil {
			loads[id] = &bridgeLoad{ID: id}
		}
		return loads[id]
	}
	for _, key := range keys {
		if key.Tenant == tenant {
			getBridge(key.Bridge).Authorized = true
		}
	}
	hourly := []archiveBucket{}
	for rows.Next() {
		var bridge, status, result, received string
		var bucket int
		var n, bytes, files, manifest int64
		if err = rows.Scan(&bridge, &status, &result, &bucket, &n, &bytes, &files, &manifest, &received); err != nil {
			break
		}
		load := getBridge(bridge)
		totals.Calls += n
		latestTime(&load.LastReceived, received)
		if status == "archived" {
			totals.Archived += n
			totals.Files += files
			totals.PayloadBytes += bytes
			totals.ManifestBytes += manifest
			load.ArchivedBytes += bytes + manifest
		}
		if status == "pending" {
			totals.Pending += n
			totals.PendingBytes += bytes
			load.Pending += n
		}
		if bucket >= 0 {
			counts[result] += n
			load.Calls += n
			switch result {
			case "completed":
				load.Completed += n
			case "failed":
				load.Failed += n
			case "unknown":
				load.Unknown += n
			}
			hourly = append(hourly, archiveBucket{At: a.Add(time.Duration(bucket*bucketSeconds) * time.Second).UTC().Format(time.RFC3339), Status: result, Count: n})
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		failure(w, 500, err)
		return
	}
	queue := []pendingArchive{}
	rows, err = db.QueryContext(r.Context(), `SELECT bridge,call_id,operation,created_at,received_at FROM siui_history_calls WHERE tenant=? AND status='pending' ORDER BY `+strings.ReplaceAll(callTimeSQL, "created_at", "received_at")+` ASC,bridge,call_id LIMIT 50`, tenant)
	if err != nil {
		failure(w, 500, err)
		return
	}
	for rows.Next() {
		var item pendingArchive
		if err = rows.Scan(&item.Bridge, &item.CallID, &item.Operation, &item.CreatedAt, &item.ReceivedAt); err != nil {
			break
		}
		queue = append(queue, item)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		failure(w, 500, err)
		return
	}
	now := time.Now()
	requests, requestErrors, receivedBytes := int64(0), int64(0), int64(0)
	recentErrors := []debugRequest{}
	m.debugMu.Lock()
	for i := len(m.requests) - 1; i >= 0; i-- {
		req := m.requests[i]
		if req.Tenant != tenant {
			continue
		}
		if req.Bridge != "" {
			latestTime(&getBridge(req.Bridge).LastSeen, req.At)
		}
		at, _ := time.Parse(time.RFC3339Nano, req.At)
		if !at.Before(now.Add(-5 * time.Minute)) {
			requests++
			receivedBytes += req.Bytes
			if req.Status >= 400 {
				requestErrors++
			}
		}
		if req.Status >= 400 && len(recentErrors) < 6 {
			recentErrors = append(recentErrors, req)
		}
	}
	m.debugMu.Unlock()
	bridges := []*bridgeLoad{}
	authorized, observed, senders := 0, 0, 0
	for _, load := range loads {
		if load.Authorized {
			authorized++
		}
		if load.Calls > 0 {
			senders++
		}
		latestTime(&load.LastSeen, load.LastReceived)
		at, _ := time.Parse(time.RFC3339Nano, load.LastSeen)
		if !at.IsZero() && !at.Before(now.Add(-15*time.Minute)) {
			observed++
		}
		bridges = append(bridges, load)
	}
	sort.Slice(bridges, func(i, j int) bool {
		if bridges[i].Calls != bridges[j].Calls {
			return bridges[i].Calls > bridges[j].Calls
		}
		return bridges[i].ID < bridges[j].ID
	})
	output(w, 200, map[string]any{"success": true, "generated_at": now.UTC().Format(time.RFC3339), "from": from, "to": to, "storage": storage, "period_basis": "received_at", "totals": totals, "counts": counts, "hourly": hourly, "bridges": bridges, "authorized_bridges": authorized, "observed_bridges": observed, "sending_bridges": senders, "queue": queue, "recent_errors": recentErrors, "traffic": map[string]any{"requests": requests, "errors": requestErrors, "received_bytes": receivedBytes, "window_seconds": 300, "sample_limit": 1000}})
}
func latestTime(current *string, candidate string) {
	next, e := time.Parse(time.RFC3339Nano, candidate)
	if e != nil {
		return
	}
	old, _ := time.Parse(time.RFC3339Nano, *current)
	if old.IsZero() || next.After(old) {
		*current = candidate
	}
}
