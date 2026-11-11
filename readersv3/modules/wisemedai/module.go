package wisemedai

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"wisemed-labreaders/readersv3/core/module"
)

//go:embed web/dist
var assets embed.FS

//go:embed migrations/*.sql
var migrations embed.FS

type Module struct {
	modelWorkMu sync.Mutex
	compiled    map[[32]byte]json.RawMessage
	rt          module.Runtime
	cfg         Settings
	db          *pgxpool.Pool
	box         *cipherBox
	mu          sync.Mutex
	loginMu     sync.Mutex
	attempts    map[string]loginAttempt
	client      *http.Client
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "wisemed-ai" }
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	var e error
	m.cfg, e = readSettings(rt.ModuleSettings(m.ID()))
	if e != nil {
		return e
	}
	m.box, e = loadBox(rt.ResolvePath(m.cfg.EncryptionKeyFile))
	if e != nil {
		return e
	}
	m.client = &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	m.attempts = map[string]loginAttempt{}
	if m.cfg.DatabaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		c, e := pgxpool.ParseConfig(m.cfg.DatabaseURL)
		if e != nil {
			return errInvalidDatabase
		}
		c.MaxConns = 10
		c.ConnConfig.ConnectTimeout = 5 * time.Second
		m.db, e = pgxpool.NewWithConfig(ctx, c)
		if e != nil {
			return errInvalidDatabase
		}
		if e = m.db.Ping(ctx); e != nil {
			return errInvalidDatabase
		}
		if e = m.migrate(ctx); e != nil {
			return e
		}
		if e = m.bootstrap(ctx); e != nil {
			return e
		}
		if e = m.ensureSpeechModels(ctx); e != nil {
			return e
		}
		if e = m.ensureCatalog(ctx); e != nil {
			return e
		}
	}
	rt.Handle("/ai/voice", http.HandlerFunc(m.browserSpeechPage))
	rt.Handle("/v1/", http.HandlerFunc(m.serve))
	rt.Handle("/api/wisemed-ai/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := r.Clone(r.Context())
		u := *r.URL
		c.URL = &u
		c.URL.Path = strings.TrimPrefix(r.URL.Path, "/api/wisemed-ai")
		m.serve(w, c)
	}))
	rt.Handle("/ai", http.RedirectHandler("/ai/", http.StatusTemporaryRedirect))
	sub, _ := fs.Sub(assets, "web/dist")
	rt.Handle("/ai/", http.StripPrefix("/ai/", http.FileServer(http.FS(sub))))
	rt.Handle("/health/ai", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"ok": true, "service": "wisemed-ai"})
	}))
	rt.Handle("/ready/ai", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, c := context.WithTimeout(r.Context(), 2*time.Second)
		defer c()
		if m.db == nil || m.db.Ping(ctx) != nil {
			fail(w, 503, "database_unavailable")
			return
		}
		respond(w, 200, map[string]any{"ok": true})
	}))
	rt.RegisterService("http-server-options", func(s *http.Server) {
		s.ReadTimeout = 15 * time.Second
		s.WriteTimeout = 65 * time.Second
		s.IdleTimeout = 60 * time.Second
		s.MaxHeaderBytes = 32 << 10
	})
	rt.AddMenu(module.MenuEntry{ID: m.ID(), Group: "core", Label: "WiseMED AI Dashboard", Path: "/ai/", Order: 1})
	return nil
}
func (m *Module) Start(ctx context.Context) error {
	lastBillingCheck := time.Time{}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if m.db != nil {
				m.db.Close()
			}
			return nil
		case <-ticker.C:
			if m.db != nil {
				go m.modelMirrorWorker(ctx)
			}
			if m.db != nil {
				c, cancel := context.WithTimeout(ctx, 30*time.Second)
				if time.Since(lastBillingCheck) >= time.Minute {
					m.subscriptionWorker(c)
					lastBillingCheck = time.Now()
				}
				m.dispatchOutbox(c)
				if time.Now().Minute() == 0 {
					m.retain(c)
				}
				cancel()
			}
		}
	}
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code string) {
	respond(w, status, map[string]any{"error": code})
}
