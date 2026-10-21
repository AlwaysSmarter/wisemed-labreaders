package siuidata

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	_ "time/tzdata"

	"github.com/golang-jwt/jwt/v5"
	"wisemed-labreaders/readersv3/core/module"
)

const prefix = "/api/siui-data/"

type guard interface {
	RequireSession(http.Handler) http.Handler
	SessionIdentity(*http.Request) (string, bool, bool)
}
type claims struct {
	jwt.RegisteredClaims
	Admin bool   `json:"admin"`
	CUI   string `json:"cui,omitempty"`
}
type Module struct {
	rt         module.Runtime
	guard      guard
	cfg        settings
	settingsMu sync.Mutex
	cfgMu      sync.RWMutex
	dbMu       sync.Mutex
	db         *pgxpool.Pool
	workMu     sync.Mutex
	workWG     sync.WaitGroup
	running    int
	closing    bool
	workCtx    context.Context
	workCancel context.CancelFunc
	loc        *time.Location
	key        []byte
	requests   atomic.Uint64
	started    time.Time
	metricsMu  sync.RWMutex
	metrics    map[string]any
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "siui-data" }
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	m.started = time.Now()
	cfg := defaultSettings()
	b, e := json.Marshal(rt.ModuleSettings(m.ID()))
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &cfg); e != nil {
		return e
	}
	if e = cfg.validate(); e != nil {
		return e
	}
	m.cfg = cfg
	m.workCtx, m.workCancel = context.WithCancel(context.Background())
	loc, e := time.LoadLocation("Europe/Bucharest")
	if e != nil {
		return e
	}
	m.loc = loc
	if cfg.JWTSecretEnv != "" {
		m.key = []byte(os.Getenv(cfg.JWTSecretEnv))
	}
	if len(m.key) > 0 && len(m.key) < 32 {
		return errors.New("JWT secret must have at least 32 bytes")
	}
	if len(m.key) == 0 {
		m.key = make([]byte, 32)
		if _, e = rand.Read(m.key); e != nil {
			return e
		}
	}
	for _, key := range cfg.ClientJWTSecrets {
		if string(m.key) == key {
			return errors.New("client JWT keys must differ from the global key")
		}
	}
	svc, ok := rt.Service("local-http-control")
	m.guard, _ = svc.(guard)
	if !ok || m.guard == nil {
		return errors.New("siui-data requires authenticated local-http")
	}
	m.metrics = map[string]any{"cpuPercent": nil, "rssBytes": nil}
	rt.Handle("/siui-data-clients", http.HandlerFunc(m.clientsHTTP))
	rt.Handle("/siui-data-clients/", http.HandlerFunc(m.clientsHTTP))
	rt.AddMenu(module.MenuEntry{ID: "siui-data-clients", Group: "core", Label: "Clienți · personalizări CNAS", Path: "/siui-data-clients", Order: 4})
	rt.Handle("/upload", m.authenticate(http.HandlerFunc(m.uploadHTTP)))
	rt.Handle(prefix, m.authenticate(http.HandlerFunc(m.serve)))
	rt.Handle("/siui-data-dashboard", m.guard.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, dashboard)
	})))
	rt.Handle("/siui-data-settings", m.guard.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, admin := m.identity(r)
		if !admin {
			failure(w, 403, "administrator required")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, settingsPage)
	})))
	rt.AddMenu(module.MenuEntry{ID: "siui-data", Group: "core", Label: "SIUI Data", Path: "/siui-data-dashboard", Order: 1})
	rt.Handle("/siui-data-versions", m.guard.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, versionsPage)
	})))
	rt.Handle("/siui-data-debug", m.guard.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, debugPage)
	})))
	rt.AddMenu(module.MenuEntry{ID: "siui-data-debug", Group: "core", Label: "Debug API SIUI Data", Path: "/siui-data-debug", Order: 3})
	rt.AddMenu(module.MenuEntry{ID: "siui-data-versions", Group: "core", Label: "Versiuni nomenclatoare", Path: "/siui-data-versions", Order: 2})
	rt.AddMenu(module.MenuEntry{ID: "siui-data-settings", Group: "settings", Label: "SIUI Data · PostgreSQL", Path: "/siui-data-settings", Order: 10})
	return nil
}
func (m *Module) Start(ctx context.Context) error {
	localDone := make(chan struct{})
	go func() { defer close(localDone); m.watchLocal(ctx) }()
	defer func() {
		m.workMu.Lock()
		m.closing = true
		m.workCancel()
		m.workMu.Unlock()
		<-localDone
		m.workWG.Wait()
		m.dbMu.Lock()
		if m.db != nil {
			m.db.Close()
		}
		m.dbMu.Unlock()
	}()
	previous, _, _ := processUsage()
	last := time.Now()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			cpu, rss, e := processUsage()
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			metrics := map[string]any{"cpuPercent": nil, "rssBytes": nil, "heapBytes": mem.HeapAlloc, "goReservedBytes": mem.Sys, "logicalCPUs": runtime.NumCPU()}
			if e == nil {
				percent := 100 * (cpu - previous) / now.Sub(last).Seconds()
				if percent < 0 {
					percent = 0
				}
				metrics["cpuPercent"] = percent
				metrics["rssBytes"] = rss
			}
			previous = cpu
			last = now
			m.metricsMu.Lock()
			m.metrics = metrics
			m.metricsMu.Unlock()
		}
	}
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func output(w http.ResponseWriter, status int, v any) {
	writeJSON(w, status, map[string]any{"success": true, "data": v})
}
func listOutput(w http.ResponseWriter, status int, rows any, total int64, meta map[string]any) {
	if meta == nil {
		meta = map[string]any{}
	}
	meta["success"] = true
	meta["rows"] = rows
	meta["totalCount"] = total
	writeJSON(w, status, meta)
}
func failure(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"success": false, "error": message})
}

func (m *Module) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Origin") != "" {
			u, e := url.Parse(r.Header.Get("Origin"))
			scheme := "https"
			if r.TLS == nil {
				scheme = "http"
			}
			if e != nil || u.Host != r.Host || u.Scheme != scheme {
				failure(w, 403, "origin rejected")
				return
			}
		}
		h := r.Header.Get("Authorization")
		if h != "" {
			if r.TLS == nil || !strings.HasPrefix(h, "Bearer ") || len(h) > 8192 {
				failure(w, 401, "Bearer JWT requires HTTPS")
				return
			}
			c := &claims{}
			token, e := jwt.ParseWithClaims(strings.TrimPrefix(h, "Bearer "), c, m.jwtVerificationKey, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("wisemed-siui-data"), jwt.WithAudience("siui-data-api"), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
			if e != nil || !token.Valid || c.Subject == "" || c.IssuedAt == nil || c.ExpiresAt == nil || c.ExpiresAt.Sub(c.IssuedAt.Time) > 15*time.Minute || !c.ExpiresAt.After(c.IssuedAt.Time) {
				failure(w, 401, "invalid or expired JWT")
				return
			}
			if c.CUI != "" {
				cui, _, ok := targetedPath(r.URL.Path)
				if !ok || cui != c.CUI || (r.Method != "GET" && r.Method != "HEAD") {
					failure(w, 403, "JWT is restricted to its own CUI targeted services")
					return
				}
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, c)))
			return
		}
		m.sessionAPI(next).ServeHTTP(w, r)
	})
}

// Keep authentication failures on module APIs in the same response contract.
func (m *Module) sessionAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := m.guard.SessionIdentity(r); !ok {
			failure(w, 401, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type identityKey struct{}

func (m *Module) identity(r *http.Request) (string, bool) {
	if c, ok := r.Context().Value(identityKey{}).(*claims); ok {
		return c.Subject, c.Admin
	}
	name, admin, _ := m.guard.SessionIdentity(r)
	return name, admin
}
func (m *Module) serve(w http.ResponseWriter, r *http.Request) {
	m.requests.Add(1)
	route := strings.TrimPrefix(r.URL.Path, prefix)
	subject, admin := m.identity(r)
	switch {
	case route == "auth/token" && r.Method == "POST":
		m.issueTokenHTTP(w, r, subject, admin)
	case strings.HasPrefix(route, "targeted_services/") || strings.HasPrefix(route, "targeted-simple-services/"):
		m.targetedHTTP(w, r)

	case route == "debug" && r.Method == "GET":
		output(w, 200, map[string]any{"html": debugPage})
	case route == "settings":
		m.settingsHTTP(w, r)
	case route == "bridge/connections" || route == "bridge/sync":
		m.bridgeHTTP(w, r, route)
	case route == "categories" && r.Method == "GET":
		m.categoriesHTTP(w, r)
	case route == "upload" || route == "import":
		m.uploadHTTP(w, r)
	default:
		m.queryHTTP(w, r, route)
	}
}
