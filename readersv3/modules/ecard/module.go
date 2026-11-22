package ecard

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

//go:embed ui/index.html
var assets embed.FS

type sessionGuard interface {
	RequireSession(http.Handler) http.Handler
	SessionIdentity(*http.Request) (string, bool, bool)
}
type activeJob struct {
	cancel    context.CancelFunc
	pin       chan string
	prompt    string
	challenge string
	pinSeen   bool
}
type Module struct {
	rt       module.Runtime
	guard    sessionGuard
	settings Settings
	store    *store
	mu       sync.Mutex // admission, upload, commit and PIN handoff
	active   map[string]*activeJob
	queue    chan string
	ctx      context.Context
	factory  func(context.Context, string, Settings) (backend, error)
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "ecard" }
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	var e error
	m.settings, e = readSettings(rt.ModuleSettings(m.ID()))
	if e != nil {
		return e
	}
	for _, p := range []*string{&m.settings.SDKRoot, &m.settings.HostDir, &m.settings.StateDir} {
		var err error
		*p, err = filepath.Abs(rt.ResolvePath(*p))
		if err != nil {
			return err
		}
	}
	m.store, e = openStore(m.settings.StateDir)
	if e != nil {
		return e
	}
	service, ok := rt.Service("local-http-control")
	m.guard, ok = service.(sessionGuard)
	if !ok {
		m.store.db.Close()
		return errors.New("ecard requires local-http session control")
	}
	m.active = map[string]*activeJob{}
	m.queue = make(chan string, m.settings.MaxJobs)
	m.factory = newBackend
	rt.Handle("/api/ecard/", m.guard.RequireSession(http.HandlerFunc(m.handle)))
	rt.Handle("/ecard/", m.guard.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := assets.ReadFile("ui/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(b)
	})))
	rt.RegisterService("http-server-options", func(server *http.Server) { server.WriteTimeout = time.Duration(m.settings.JobTimeout+60) * time.Second })
	rt.AddMenu(module.MenuEntry{ID: "ecard", Group: "primary", Label: "PIAS eCard", Path: "/ecard/", Order: 5})
	return nil
}
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	defer m.store.db.Close()
	timer := time.NewTicker(time.Hour)
	defer timer.Stop()
	_ = m.store.cleanup(m.settings.RetentionHours)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			_ = m.store.cleanup(m.settings.RetentionHours)
		case id := <-m.queue:
			m.run(ctx, id)
		}
	}
}
func ownerKey(r *http.Request, user string) string {
	if p, ok := apibridge.PrincipalFrom(r.Context()); ok {
		return "wsm:" + p.TenantID + ":" + p.Subject
	}
	return "local:" + user
}
func respond(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, e error) {
	respond(w, status, map[string]interface{}{"ok": false, "error": e.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v interface{}) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return errors.New("invalid JSON or request larger than 1 MiB")
	}
	var extra interface{}
	if d.Decode(&extra) != io.EOF {
		return errors.New("one JSON value required")
	}
	return nil
}
func (m *Module) handle(w http.ResponseWriter, r *http.Request) {
	user, admin, ok := m.guard.SessionIdentity(r)
	if !ok || user == "" {
		fail(w, 401, errors.New("authentication required"))
		return
	}
	owner := ownerKey(r, user)
	if r.Method != "GET" {
		if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			fail(w, 415, errors.New("application/json required"))
			return
		}
		if o := r.Header.Get("Origin"); o != "" {
			u, e := url.Parse(o)
			if e != nil || u.Host != r.Host {
				fail(w, 403, errors.New("same origin required"))
				return
			}
		}
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/ecard/")
	if path == "ui" && r.Method == "GET" {
		b, _ := assets.ReadFile("ui/index.html")
		respond(w, 200, map[string]string{"html": string(b)})
		return
	}
	if path == "sign/verify" && r.Method == "POST" {
		m.verify(w, r)
		return
	}
	if path == "sign/batch" {
		m.batch(w, r, owner)
		return
	}
	if path == "settings" && r.Method == "PUT" {
		if !admin {
			fail(w, 403, errors.New("administrator required"))
			return
		}
		m.saveSettings(w, r)
		return
	}
	if path == "settings" && r.Method == "GET" {
		respond(w, 200, m.settings)
		return
	}
	if path == "health" && r.Method == "GET" {
		respond(w, 200, map[string]interface{}{"ok": true, "hardware_supported": runtime.GOOS == "windows", "sdk_order": m.settings.SDKOrder, "batch_parallelism_per_card": 1})
		return
	}
	if strings.HasPrefix(path, "tdx/") {
		if !admin {
			fail(w, 403, errors.New("administrator required"))
			return
		}
		m.handleTDX(w, r, strings.TrimPrefix(path, "tdx/"))
		return
	}
	if path == "jobs" && r.Method == "POST" {
		var req Request
		if e := decode(w, r, &req); e != nil {
			fail(w, 400, e)
			return
		}
		if e := req.Validate(); e != nil {
			fail(w, 400, e)
			return
		}
		if (req.Operation == "enroll" || req.Operation == "reset-pin") && !admin {
			fail(w, 403, errors.New("administrator required"))
			return
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.ctx == nil || m.ctx.Err() != nil {
			fail(w, 503, errors.New("service starting or stopping"))
			return
		}
		var count int
		if e := m.store.db.QueryRow(`SELECT count(*) FROM jobs`).Scan(&count); e != nil {
			fail(w, 500, errors.New("storage unavailable"))
			return
		}
		if count >= m.settings.MaxJobs {
			fail(w, 429, errors.New("job capacity reached; delete completed jobs"))
			return
		}
		id := uuid.NewString()
		b, e := m.store.seal(req, id)
		if e == nil {
			_, e = m.store.db.Exec(`INSERT INTO jobs(id,owner,operation,state,created,request) VALUES(?,?,?,'uploading',?,?)`, id, owner, req.Operation, time.Now().Unix(), b)
		}
		if e != nil {
			fail(w, 500, errors.New("cannot store job"))
			return
		}
		j, _ := m.store.job(id, owner)
		respond(w, 201, j)
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] != "jobs" {
		fail(w, 404, errors.New("route not found"))
		return
	}
	id := parts[1]
	m.mu.Lock()
	defer m.mu.Unlock()
	j, e := m.store.job(id, owner)
	if e != nil {
		fail(w, 404, errors.New("job not found"))
		return
	}
	suffix := ""
	if len(parts) == 3 {
		suffix = parts[2]
	} else if len(parts) > 3 {
		fail(w, 404, errors.New("route not found"))
		return
	}
	switch {
	case suffix == "" && r.Method == "GET":
		out := map[string]interface{}{"job": j}
		if a := m.active[id]; a != nil && a.prompt != "" {
			out["pin_request"] = map[string]string{"prompt": a.prompt, "challenge": a.challenge}
		}
		respond(w, 200, out)
	case suffix == "" && r.Method == "DELETE":
		if j.State == "running" || j.State == "waiting_pin" || j.State == "queued" {
			fail(w, 409, errors.New("cancel and await terminal state first"))
			return
		}
		tx, e := m.store.db.Begin()
		if e != nil {
			fail(w, 500, e)
			return
		}
		defer tx.Rollback()
		_, e = tx.Exec(`DELETE FROM items WHERE job=?`, id)
		if e == nil {
			_, e = tx.Exec(`DELETE FROM jobs WHERE id=?`, id)
		}
		if e == nil {
			e = tx.Commit()
		}
		if e != nil {
			fail(w, 500, errors.New("delete failed"))
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
	case suffix == "items" && r.Method == "POST":
		m.upload(w, r, j)
	case suffix == "start" && r.Method == "POST":
		if j.State != "uploading" {
			fail(w, 409, errors.New("job already started; never replayed"))
			return
		}
		if j.Operation == "sign" && j.Total == 0 {
			fail(w, 400, errors.New("upload texts first"))
			return
		}
		if len(m.queue) >= cap(m.queue) {
			fail(w, 429, errors.New("queue full"))
			return
		}
		if e = m.store.state(id, "queued", ""); e != nil {
			fail(w, 500, e)
			return
		}
		m.queue <- id
		respond(w, 202, map[string]string{"id": id, "state": "queued"})
	case suffix == "cancel" && r.Method == "POST":
		if a := m.active[id]; a != nil {
			a.cancel()
		} else if j.State == "queued" || j.State == "uploading" {
			_ = m.store.state(id, "cancelled", "")
		}
		respond(w, 202, map[string]bool{"ok": true})
	case suffix == "pin" && r.Method == "POST":
		var p struct {
			Challenge string `json:"challenge"`
			Value     string `json:"value"`
		}
		if e := decode(w, r, &p); e != nil {
			fail(w, 400, e)
			return
		}
		a := m.active[id]
		if a == nil || a.challenge == "" || p.Challenge != a.challenge {
			fail(w, 409, errors.New("PIN challenge expired"))
			return
		}
		if len(p.Value) < 1 || len(p.Value) > 32 {
			fail(w, 400, errors.New("invalid input length"))
			return
		}
		select {
		case a.pin <- p.Value:
			a.challenge = ""
			a.prompt = ""
			respond(w, 200, map[string]bool{"ok": true})
		default:
			fail(w, 409, errors.New("input already supplied"))
		}
	case suffix == "results" && r.Method == "GET":
		m.results(w, r, id)
	default:
		fail(w, 405, errors.New("method not allowed"))
	}
}
func (m *Module) upload(w http.ResponseWriter, r *http.Request, j Job) {
	if j.State != "uploading" || j.Operation != "sign" {
		fail(w, 409, errors.New("job does not accept texts"))
		return
	}
	var in struct {
		Offset int    `json:"offset"`
		Items  []Item `json:"items"`
	}
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, e)
		return
	}
	if in.Offset != j.Total {
		fail(w, 409, errors.New("offset mismatch; inspect total before retry"))
		return
	}
	if len(in.Items) == 0 || len(in.Items) > 500 || j.Total+len(in.Items) > m.settings.MaxItems {
		fail(w, 400, errors.New("chunk or job item limit exceeded"))
		return
	}
	var blob []byte
	var used int64
	if e := m.store.db.QueryRow(`SELECT request,bytes FROM jobs WHERE id=?`, j.ID).Scan(&blob, &used); e != nil {
		fail(w, 500, e)
		return
	}
	var req Request
	if e := m.store.open(blob, j.ID, &req); e != nil {
		fail(w, 500, errors.New("cannot decrypt job"))
		return
	}
	tx, e := m.store.db.Begin()
	if e != nil {
		fail(w, 500, e)
		return
	}
	defer tx.Rollback()
	for i, item := range in.Items {
		if item.ID == "" || len(item.ID) > 128 || len(item.Text) > m.settings.MaxItemBytes {
			fail(w, 400, errors.New("invalid item id or text size"))
			return
		}
		if req.Encoding == "ascii" {
			for _, c := range item.Text {
				if c > 127 {
					fail(w, 400, errors.New("non-ASCII text; explicitly select utf-8"))
					return
				}
			}
		}
		used += int64(len(item.Text))
		if used > m.settings.MaxJobBytes {
			fail(w, 400, errors.New("job byte limit exceeded"))
			return
		}
		b, e := m.store.seal(item, j.ID+":"+strconv.Itoa(j.Total+i))
		if e != nil {
			fail(w, 500, e)
			return
		}
		if _, e = tx.Exec(`INSERT INTO items(job,n,id,payload) VALUES(?,?,?,?)`, j.ID, j.Total+i, item.ID, b); e != nil {
			fail(w, 409, errors.New("duplicate item id or storage error"))
			return
		}
	}
	if _, e = tx.Exec(`UPDATE jobs SET total=total+?,bytes=? WHERE id=?`, len(in.Items), used, j.ID); e == nil {
		e = tx.Commit()
	}
	if e != nil {
		fail(w, 500, errors.New("upload failed"))
		return
	}
	respond(w, 200, map[string]int{"total": j.Total + len(in.Items)})
}
func (m *Module) results(w http.ResponseWriter, r *http.Request, id string) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if offset < 0 {
		offset = 0
	}
	if limit < 1 || limit > 20 {
		limit = 20
	}
	rows, e := m.store.db.Query(`SELECT n,result FROM items WHERE job=? AND n>=? AND result IS NOT NULL ORDER BY n LIMIT ?`, id, offset, limit)
	if e != nil {
		fail(w, 500, e)
		return
	}
	defer rows.Close()
	out := []interface{}{}
	next := offset
	for rows.Next() {
		var n int
		var b []byte
		if e = rows.Scan(&n, &b); e != nil {
			break
		}
		var v interface{}
		e = m.store.open(b, id+":result:"+strconv.Itoa(n), &v)
		if e != nil {
			break
		}
		out = append(out, v)
		next = n + 1
	}
	if e == nil {
		e = rows.Err()
	}
	if e != nil {
		fail(w, 500, errors.New("cannot read results"))
		return
	}
	respond(w, 200, map[string]interface{}{"items": out, "next_offset": next})
}

var _ = sql.ErrNoRows
