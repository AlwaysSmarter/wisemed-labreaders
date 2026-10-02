package siui

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

type sessionGuard interface {
	RequireSession(http.Handler) http.Handler
	SessionIdentity(*http.Request) (string, bool, bool)
}
type Module struct {
	ready           chan struct{}
	rt              module.Runtime
	guard           sessionGuard
	db              *sql.DB
	mu              sync.Mutex
	cfg             settings
	pending         int
	checking        bool
	nativeSupported bool
	queue           chan work
	factory         func(settings) backend
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "siui" }
func value(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	s := rt.ModuleSettings(m.ID())
	base := value(s["base_url"])
	if base == "" {
		base = "https://www.siui.ro"
	}
	if e := validateBase(base); e != nil {
		return e
	}
	store := value(s["certificate_store"])
	if store == "" {
		store = "CurrentUser"
	}
	if store != "CurrentUser" && store != "LocalMachine" {
		return errors.New("invalid certificate_store")
	}
	database := value(s["database"])
	if database == "" {
		database = "./siui-jobs.db"
	}
	m.cfg = settings{Username: value(s["username"]), Licence: value(s["licence"]), Thumbprint: value(s["certificate_thumbprint"]), Store: store, BaseURL: base, Database: rt.ResolvePath(database)}
	svc, ok := rt.Service("local-http-control")
	m.guard, _ = svc.(sessionGuard)
	if !ok || m.guard == nil {
		return errors.New("SIUI requires the shared authenticated HTTP service")
	}
	m.ready = make(chan struct{})
	m.queue = make(chan work, 20)
	if m.factory == nil {
		m.factory = func(s settings) backend { return nativeBackend{cfg: s} }
		m.nativeSupported = runtime.GOOS == "windows"
	}
	rt.Handle("/api/siui/", m.guard.RequireSession(http.HandlerFunc(m.handle)))
	rt.AddMenu(module.MenuEntry{ID: "siui", Group: "settings", Label: "CNAS - validare 72h", Path: "/settings/siui", Order: 35})
	return nil
}
func (m *Module) Start(ctx context.Context) error {
	if e := os.MkdirAll(filepath.Dir(m.cfg.Database), 0700); e != nil {
		return e
	}
	lock, e := os.OpenFile(m.cfg.Database+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if e = lockFile(lock); e == nil {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("another SIUI instance is using this database")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(100 * time.Millisecond):
		}
	}
	m.db, e = openJobs(m.cfg.Database)
	if e != nil {
		return e
	}
	defer m.db.Close()
	close(m.ready)
	for {
		select {
		case <-ctx.Done():
			return nil
		case job := <-m.queue:
			if ctx.Err() != nil {
				return nil
			}
			_, e := m.db.Exec(`UPDATE siui_jobs SET status='running',updated_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339Nano), job.ID)
			if e != nil {
				m.rt.Logf("SIUI job state write failed")
				m.finished()
				continue
			}
			m.rt.Logf("SIUI validation started job=%s", job.ID)
			callCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
			c := client{cfg: job.Config, backend: m.factory(job.Config)}
			result, submitted, err := c.validate(callCtx, job.XML)
			cancel()
			state, message, encoded := "completed", "", ""
			if err != nil {
				state = "failed"
				if submitted {
					state = "unknown"
				}
				message = err.Error()
			} else {
				b, _ := json.Marshal(result)
				encoded = string(b)
			}
			_, e = m.db.Exec(`UPDATE siui_jobs SET status=?,updated_at=?,error=?,result=? WHERE id=?`, state, time.Now().UTC().Format(time.RFC3339Nano), message, encoded, job.ID)
			if e != nil {
				m.rt.Logf("SIUI result persistence failed job=%s", job.ID)
			} else {
				m.rt.Logf("SIUI validation finished job=%s status=%s", job.ID, state)
			}
			m.finished()
		}
	}
}
func (m *Module) finished() { m.mu.Lock(); m.pending--; m.mu.Unlock() }
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]any{"ok": false, "error": message})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("application/json required")
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*maxData))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return errors.New("invalid or oversized JSON")
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return errors.New("one JSON object required")
	}
	return nil
}
func (m *Module) handle(w http.ResponseWriter, r *http.Request) {
	subject, admin, ok := m.guard.SessionIdentity(r)
	if !ok {
		fail(w, 401, "authentication required")
		return
	}
	owner := "local:" + subject
	if p, remote := apibridge.PrincipalFrom(r.Context()); remote {
		owner = "wsm:" + strconv.Quote(p.TenantID) + ":" + strconv.Quote(p.Subject)
		admin = p.Admin
	} else if r.Method != "GET" {
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			if e != nil || u.Host != r.Host || u.Scheme != scheme {
				fail(w, 403, "origin rejected")
				return
			}
		}
	}
	route := strings.TrimPrefix(r.URL.Path, "/api/siui/")
	if strings.HasPrefix(route, "validations") {
		select {
		case <-m.ready:
		default:
			fail(w, 503, "SIUI worker is starting")
			return
		}
	}
	switch {
	case route == "status" && r.Method == "GET":
		m.mu.Lock()
		cfg, pending := m.cfg, m.pending
		m.mu.Unlock()
		respond(w, 200, map[string]any{"ok": true, "platform": runtime.GOOS, "native_supported": runtime.GOOS == "windows", "configured": cfg.Username != "" && cfg.Thumbprint != "" && cfg.Licence != "", "pending": pending, "report_type": "PARA", "request_type": "RQ_PARA_SRV"})
	case route == "ocsp-test" && r.Method == "POST":
		m.ocspHTTP(w, r)
	case route == "personalization" && r.Method == "POST":
		if !admin {
			fail(w, 403, "administrator required")
			return
		}
		m.personalizationHTTP(w, r)
	case route == "insured" && r.Method == "POST":
		m.insuredHTTP(w, r)
	case route == "settings":
		if !admin {
			fail(w, 403, "administrator required")
			return
		}
		m.settingsHTTP(w, r)
	case route == "certificates" && r.Method == "GET":
		if !admin {
			fail(w, 403, "administrator required")
			return
		}
		store := r.URL.Query().Get("store")
		if store == "" {
			m.mu.Lock()
			store = m.cfg.Store
			m.mu.Unlock()
		}
		list, e := listCertificates(store)
		if e != nil {
			fail(w, 503, e.Error())
			return
		}
		respond(w, 200, map[string]any{"ok": true, "certificates": list})
	case route == "validations" && r.Method == "POST":
		var in struct {
			OperationID string `json:"operation_id"`
			XML         string `json:"xml"`
		}
		if e := decode(w, r, &in); e != nil {
			fail(w, 400, e.Error())
			return
		}
		if _, e := requestIDs(in.XML); e != nil {
			fail(w, 400, e.Error())
			return
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.cfg.Username == "" || m.cfg.Thumbprint == "" || m.cfg.Licence == "" {
			fail(w, 409, "select the CNAS certificate and configure username and licence first")
			return
		}
		if m.checking {
			fail(w, 409, "an insured lookup is using the token; retry after it completes")
			return
		}
		// Reservation and queue admission share a lock with configuration changes.
		job, created, e := insertJob(m.db, owner, in.OperationID, in.XML)
		if e != nil {
			fail(w, 409, e.Error())
			return
		}
		if created {
			select {
			case m.queue <- work{job.ID, in.XML, m.cfg}:
				m.pending++
			default:
				m.db.Exec(`UPDATE siui_jobs SET status='failed',error='Local queue full; nothing sent to CNAS' WHERE id=?`, job.ID)
				fail(w, 503, "local queue full; nothing sent to CNAS")
				return
			}
		}
		status := http.StatusAccepted
		if !created {
			status = http.StatusOK
		}
		respond(w, status, map[string]any{"ok": true, "duplicate": !created, "job": job})
	case route == "validations" && r.Method == "GET":
		rows, e := m.db.Query(`SELECT id,operation_id,status,created_at,updated_at FROM siui_jobs WHERE owner=? ORDER BY created_at DESC LIMIT 50`, owner)
		if e != nil {
			fail(w, 500, "cannot read validation history")
			return
		}
		defer rows.Close()
		jobs := []Job{}
		for rows.Next() {
			var j Job
			if e = rows.Scan(&j.ID, &j.OperationID, &j.Status, &j.CreatedAt, &j.UpdatedAt); e != nil {
				fail(w, 500, "cannot read validation history")
				return
			}
			jobs = append(jobs, j)
		}
		if rows.Err() != nil {
			fail(w, 500, "cannot read validation history")
			return
		}
		respond(w, 200, map[string]any{"ok": true, "jobs": jobs})
	case strings.HasPrefix(route, "validations/") && r.Method == "GET":
		job, e := readJob(m.db, owner, strings.TrimPrefix(route, "validations/"))
		if e == sql.ErrNoRows {
			fail(w, 404, "validation not found")
			return
		}
		if e != nil {
			fail(w, 500, "cannot read validation")
			return
		}
		respond(w, 200, map[string]any{"ok": true, "job": job})
	default:
		fail(w, 405, "unsupported route or method")
	}
}

// Short values are fully masked to avoid revealing an entire secret.
func licenceHint(licence string) string {
	chars := []rune(licence)
	if len(chars) == 0 {
		return ""
	}
	if len(chars) <= 6 {
		return "••••••"
	}
	return string(chars[:3]) + "…" + string(chars[len(chars)-3:])
}

func (m *Module) settingsHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.Method == "GET" {
		respond(w, 200, map[string]any{"ok": true, "settings": map[string]any{"licence_configured": m.cfg.Licence != "", "licence_hint": licenceHint(m.cfg.Licence), "username": m.cfg.Username, "base_url": m.cfg.BaseURL, "certificate_store": m.cfg.Store, "certificate_thumbprint": m.cfg.Thumbprint}})
		return
	}
	if r.Method != "PUT" {
		fail(w, 405, "method not allowed")
		return
	}
	if m.pending > 0 {
		fail(w, 409, "wait for queued validations before changing the certificate")
		return
	}
	var in struct {
		Username   string  `json:"username"`
		Licence    *string `json:"licence"`
		BaseURL    string  `json:"base_url"`
		Store      string  `json:"certificate_store"`
		Thumbprint string  `json:"certificate_thumbprint"`
	}
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, e.Error())
		return
	}
	if e := validateBase(in.BaseURL); e != nil {
		fail(w, 400, e.Error())
		return
	}
	current, _ := url.Parse(m.cfg.BaseURL)
	requested, _ := url.Parse(in.BaseURL)
	if requested.Host != current.Host && requested.Host != "www.siui.ro" && requested.Host != "www.siui.ro:444" && requested.Host != "testsiui.siui.ro" && requested.Host != "testsiui.siui.ro:444" {
		fail(w, 400, "configure custom CNAS origins locally in config.yaml")
		return
	}
	if in.Username == "" || strings.ContainsAny(in.Username, ":\r\n") {
		fail(w, 400, "invalid CNAS username")
		return
	}
	if in.Store != "CurrentUser" && in.Store != "LocalMachine" {
		fail(w, 400, "invalid certificate store")
		return
	}
	licence := m.cfg.Licence
	if in.Licence != nil && strings.TrimSpace(*in.Licence) != "" {
		licence = strings.TrimSpace(*in.Licence)
		if len(licence) > 4096 || strings.ContainsAny(licence, "\r\n") {
			fail(w, 400, "invalid CNAS licence")
			return
		}
	}
	// Saving general settings is possible before a Windows token is selected.
	if in.Thumbprint != "" && (in.Thumbprint != m.cfg.Thumbprint || in.Store != m.cfg.Store) {
		certs, e := listCertificates(in.Store)
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		found := false
		for _, c := range certs {
			if strings.EqualFold(c.Thumbprint, strings.ReplaceAll(in.Thumbprint, " ", "")) && c.Valid && c.HasPrivateKey {
				in.Thumbprint = c.Thumbprint
				found = true
				break
			}
		}
		if !found {
			fail(w, 400, "select an available, valid certificate with a private key")
			return
		}
	}
	cfg, e := config.Load(m.rt.ConfigPath())
	if e != nil {
		fail(w, 500, "cannot load configuration")
		return
	}
	s := cfg.ModuleSettings("siui")
	s["username"] = in.Username
	s["licence"] = licence
	delete(s, "licence_file")
	s["base_url"] = in.BaseURL
	s["certificate_store"] = in.Store
	s["certificate_thumbprint"] = in.Thumbprint
	cfg.Modules["siui"] = s
	if e = os.Chmod(m.rt.ConfigPath(), 0600); e != nil {
		fail(w, 500, "cannot protect configuration")
		return
	}
	if e = cfg.Save(); e != nil {
		fail(w, 500, "cannot save configuration")
		return
	}
	m.cfg.Username = in.Username
	m.cfg.Licence = licence
	m.cfg.BaseURL = in.BaseURL
	m.cfg.Store = in.Store
	m.cfg.Thumbprint = in.Thumbprint
	m.rt.Logf("SIUI configuration updated; certificate reference saved without export")
	respond(w, 200, map[string]any{"ok": true, "licence_hint": licenceHint(m.cfg.Licence)})
}
