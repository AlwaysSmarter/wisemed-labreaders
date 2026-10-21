package siui

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type endpointCheck struct {
	Service    string `json:"service"`
	URL        string `json:"url"`
	CheckedAt  string `json:"checked_at,omitempty"`
	Reachable  bool   `json:"reachable"`
	HTTPStatus int    `json:"http_status,omitempty"`
	LatencyMS  int64  `json:"latency_ms,omitempty"`
	Error      string `json:"error,omitempty"`
}

func dashboardEndpoints(cfg settings) []endpointCheck {
	out := []endpointCheck{{Service: "OCSP", URL: strings.TrimRight(cfg.BaseURL, "/") + "/OCSP/validator"}}
	seen := map[string]bool{}
	for _, op := range operations() {
		if !seen[op.Service] {
			seen[op.Service] = true
			out = append(out, endpointCheck{Service: op.Service, URL: strings.TrimRight(cfg.serviceBaseURL(op.Service), "/") + "/svapntws/services/" + op.Service})
		}
	}
	return out
}
func probeEndpoint(ctx context.Context, client *http.Client, endpoint endpointCheck) endpointCheck {
	started := time.Now()
	endpoint.CheckedAt = started.UTC().Format(time.RFC3339)
	req, err := http.NewRequestWithContext(ctx, "HEAD", endpoint.URL, nil)
	if err == nil {
		var res *http.Response
		res, err = client.Do(req)
		if res != nil {
			endpoint.HTTPStatus = res.StatusCode
			res.Body.Close()
		}
	}
	endpoint.LatencyMS = time.Since(started).Milliseconds()
	if err != nil {
		endpoint.Error = err.Error()
	} else {
		endpoint.Reachable = true
	}
	return endpoint
}
func (m *Module) dashboardChecksHTTP(w http.ResponseWriter, r *http.Request, admin bool) {
	if !admin {
		fail(w, 403, "administrator required")
		return
	}
	if r.Method != "POST" {
		fail(w, 405, "POST required")
		return
	}
	m.mu.Lock()
	cfg := m.cfg
	m.mu.Unlock()
	key := ocspConfigKey(cfg)
	m.dashboardMu.Lock()
	if m.dashboardChecking {
		m.dashboardMu.Unlock()
		fail(w, 409, "connectivity check in progress")
		return
	}
	if m.dashboardKey == key && time.Since(m.dashboardChecked) < time.Minute {
		checks := append([]endpointCheck(nil), m.dashboardChecks...)
		m.dashboardMu.Unlock()
		respond(w, 200, map[string]any{"success": true, "cached": true, "endpoints": checks})
		return
	}
	m.dashboardChecking = true
	m.dashboardMu.Unlock()
	defer func() { m.dashboardMu.Lock(); m.dashboardChecking = false; m.dashboardMu.Unlock() }()
	// No client certificate, no PIN, no SOAP request and no TLS bypass. A response
	// proves HTTPS reachability only, not an authenticated CNAS operation.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	checks := dashboardEndpoints(cfg)
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range checks {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-r.Context().Done():
				return
			}
			defer func() { <-sem }()
			checks[i] = probeEndpoint(r.Context(), client, checks[i])
		}(i)
	}
	wg.Wait()
	m.dashboardMu.Lock()
	m.dashboardChecks = checks
	m.dashboardChecked = time.Now()
	m.dashboardKey = key
	m.dashboardMu.Unlock()
	respond(w, 200, map[string]any{"success": true, "endpoints": checks})
}
func (m *Module) dashboardHTTP(w http.ResponseWriter, r *http.Request, owner string, admin bool) {
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
	now := time.Now()
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if from == "" && to == "" {
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		from = day.Format(time.RFC3339)
		to = day.AddDate(0, 0, 1).Format(time.RFC3339)
	}
	a, e1 := time.Parse(time.RFC3339, from)
	b, e2 := time.Parse(time.RFC3339, to)
	if e1 != nil || e2 != nil || !b.After(a) || b.Sub(a) > (31*24+2)*time.Hour {
		fail(w, 400, "invalid interval (RFC3339, maximum 31 days)")
		return
	}
	scoped := ""
	scopeArgs := []any{}
	if !admin {
		scoped = " AND c.owner=?"
		scopeArgs = append(scopeArgs, owner)
	}
	where := ` WHERE julianday(c.created_at)>=julianday(?) AND julianday(c.created_at)<julianday(?)` + scoped
	args := append([]any{from, to}, scopeArgs...)
	counts := map[string]int{}
	byOperation := []map[string]any{}
	rows, err := m.db.Query(`SELECT c.operation,c.status,COUNT(*),COALESCE(AVG(CASE WHEN o.started_at<>'' AND o.completed_at<>'' THEN MAX(0,(julianday(o.completed_at)-julianday(o.started_at))*86400) END),0) FROM siui_history_calls_local c LEFT JOIN siui_history_outbox o ON o.call_id=c.id`+where+` GROUP BY c.operation,c.status`, args...)
	if err != nil {
		fail(w, 500, "cannot read dashboard metrics")
		return
	}
	for rows.Next() {
		var op, status string
		var n int
		var duration float64
		if err = rows.Scan(&op, &status, &n, &duration); err != nil {
			break
		}
		counts[status] += n
		byOperation = append(byOperation, map[string]any{"operation": op, "status": status, "count": n, "average_seconds": duration})
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		fail(w, 500, "cannot read metrics")
		return
	}
	buckets := []map[string]any{}
	rows, err = m.db.Query(`SELECT CAST((unixepoch(c.created_at)-unixepoch(?))/3600 AS INTEGER),c.status,COUNT(*) FROM siui_history_calls_local c`+where+` GROUP BY 1,c.status ORDER BY 1`, append([]any{from}, args...)...)
	if err != nil {
		fail(w, 500, "cannot read chart")
		return
	}
	for rows.Next() {
		var hour, n int
		var status string
		if err = rows.Scan(&hour, &status, &n); err != nil {
			break
		}
		buckets = append(buckets, map[string]any{"at": a.Add(time.Duration(hour) * time.Hour).UTC().Format(time.RFC3339), "status": status, "count": n})
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		fail(w, 500, "cannot read chart")
		return
	}
	// Live queue is independent of the selected reporting period.
	active := []operationEntry{}
	rows, err = m.db.Query(operationSelect+` WHERE c.status IN ('queued','running')`+scoped+` ORDER BY c.created_at LIMIT 100`, scopeArgs...)
	if err != nil {
		fail(w, 500, "cannot read queue")
		return
	}
	for rows.Next() {
		c, e := scanOperation(rows)
		if e != nil {
			err = e
			break
		}
		active = append(active, c)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		fail(w, 500, "cannot read queue")
		return
	}
	backlog := map[string]map[string]int{}
	for _, table := range []string{"siui_history_outbox", "siui_wm_callbacks"} {
		rows, e := m.db.Query(`SELECT o.status,COUNT(*) FROM `+table+` o JOIN siui_history_calls_local c ON c.id=o.call_id WHERE 1=1`+scoped+` GROUP BY o.status`, scopeArgs...)
		if e != nil {
			fail(w, 500, "cannot read delivery queues")
			return
		}
		statuses := map[string]int{}
		for rows.Next() {
			var s string
			var n int
			if e = rows.Scan(&s, &n); e != nil {
				break
			}
			statuses[s] = n
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			fail(w, 500, "cannot read delivery queues")
			return
		}
		backlog[table] = statuses
	}
	recent := []operationEntry{}
	rows, err = m.db.Query(operationSelect+` WHERE c.status IN ('failed','unknown')`+scoped+` ORDER BY c.created_at DESC LIMIT 6`, scopeArgs...)
	if err != nil {
		fail(w, 500, "cannot read failures")
		return
	}
	for rows.Next() {
		c, e := scanOperation(rows)
		if e != nil {
			err = e
			break
		}
		recent = append(recent, c)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		fail(w, 500, "cannot read failures")
		return
	}
	m.mu.Lock()
	cfg := m.cfg
	pending, checking := m.pending, m.checking
	queued := len(m.calls) + len(m.queue)
	capacity := cap(m.calls) + cap(m.queue)
	m.mu.Unlock()
	endpoints := dashboardEndpoints(cfg)
	m.dashboardMu.Lock()
	if m.dashboardKey == ocspConfigKey(cfg) {
		for i := range endpoints {
			for _, e := range m.dashboardChecks {
				if endpoints[i].URL == e.URL {
					endpoints[i] = e
				}
			}
		}
	}
	probing := m.dashboardChecking
	m.dashboardMu.Unlock()
	// Last completed operation is historical evidence, not a current health probe.
	evidence := map[string]map[string]string{}
	rows, err = m.db.Query(`SELECT c.operation,MAX(CASE WHEN c.status='completed' THEN c.created_at ELSE '' END),MAX(CASE WHEN c.status IN ('failed','unknown') THEN c.created_at ELSE '' END) FROM siui_history_calls_local c WHERE 1=1`+scoped+` GROUP BY c.operation`, scopeArgs...)
	if err != nil {
		fail(w, 500, "cannot read service evidence")
		return
	}
	serviceByOp := map[string]string{}
	for _, op := range operations() {
		serviceByOp[op.Name] = op.Service
	}
	serviceByOp["validateReport"] = "SiuiValidateWS"
	serviceByOp["ocsp"] = "OCSP"
	for rows.Next() {
		var op, good, bad string
		if err = rows.Scan(&op, &good, &bad); err != nil {
			break
		}
		service := serviceByOp[op]
		if service == "" {
			continue
		}
		v := evidence[service]
		if v == nil {
			v = map[string]string{}
		}
		if good > v["last_completed"] {
			v["last_completed"] = good
		}
		if bad > v["last_failed"] {
			v["last_failed"] = bad
		}
		evidence[service] = v
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		fail(w, 500, "cannot read service evidence")
		return
	}
	sort.Slice(byOperation, func(i, j int) bool {
		return fmt.Sprint(byOperation[i]["operation"]) < fmt.Sprint(byOperation[j]["operation"])
	})
	ocsp := m.ocspSession.dashboardState(cfg)
	pinSet, pinBlocked, _ := cfg.pin.status(cfg)
	historyEnabled := false
	if m.history != nil {
		m.history.mu.Lock()
		historyEnabled = m.history.cfg.Enabled
		m.history.mu.Unlock()
	}
	respond(w, 200, map[string]any{"success": true, "generated_at": now.UTC().Format(time.RFC3339), "from": from, "to": to, "scope": map[bool]string{true: "all", false: "own"}[admin], "admin": admin, "counts": counts, "hourly": buckets, "by_operation": byOperation, "queue": active, "recent_failures": recent, "deliveries": backlog, "history_enabled": historyEnabled, "runtime": map[string]any{"pending": pending, "queued": queued, "capacity": capacity, "active": max(0, pending-queued), "token_checking": checking}, "ocsp": ocsp, "configured": cfg.Username != "" && cfg.Thumbprint != "" && cfg.Licence != "", "pin": map[string]bool{"saved": pinSet, "suspended": pinBlocked}, "endpoints": endpoints, "endpoint_evidence": evidence, "probing": probing})
}
