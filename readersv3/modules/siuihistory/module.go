package siuihistory

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

type bridgeKey struct {
	Tenant string `json:"tenant"`
	Bridge string `json:"bridge"`
	Hash   string `json:"hash"`
}
type settings struct {
	MySQLHost     string      `json:"mysql_host"`
	MySQLPort     string      `json:"mysql_port"`
	MySQLDatabase string      `json:"mysql_database"`
	MySQLUser     string      `json:"mysql_user"`
	MySQLPassword string      `json:"mysql_password"`
	MySQLTLS      string      `json:"mysql_tls"`
	Tenant        string      `json:"tenant"`
	Storage       string      `json:"storage"`
	BasePath      string      `json:"base_path"`
	StagingPath   string      `json:"staging_path"`
	Endpoint      string      `json:"s3_endpoint"`
	Bucket        string      `json:"s3_bucket"`
	Region        string      `json:"s3_region"`
	Prefix        string      `json:"s3_prefix"`
	AccessKey     string      `json:"s3_access_key"`
	SecretKey     string      `json:"s3_secret_key"`
	SessionToken  string      `json:"s3_session_token"`
	S3Insecure    bool        `json:"s3_insecure"`
	Keys          []bridgeKey `json:"bridge_keys"`
}
type guard interface {
	RequireSession(http.Handler) http.Handler
	SessionIdentity(*http.Request) (string, bool, bool)
}
type Module struct {
	rt       module.Runtime
	guard    guard
	mu       sync.Mutex
	cfg      settings
	db       *sql.DB
	debugMu  sync.Mutex
	requests []debugRequest
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "siui-history" }
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	m.cfg = settings{MySQLHost: "127.0.0.1", MySQLPort: "3306", MySQLTLS: "false", Storage: "local", BasePath: "./history", StagingPath: "./history-staging"}
	b, _ := json.Marshal(rt.ModuleSettings(m.ID()))
	if e := json.Unmarshal(b, &m.cfg); e != nil {
		return e
	}
	if s, ok := rt.Service("local-http-control"); ok {
		m.guard, _ = s.(guard)
	}
	rt.Handle("/api/history/", http.HandlerFunc(m.debugAuthenticate))
	rt.Handle("/history-ui", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, settingsPage)
	}))
	rt.AddMenu(module.MenuEntry{ID: "history", Group: "core", Label: "Istoric", Path: "/history", Order: 20})
	rt.AddMenu(module.MenuEntry{ID: "history-settings", Group: "settings", Label: "SIUI History", Path: "/settings/history", Order: 10})
	return nil
}
func (m *Module) Start(ctx context.Context) error {
	<-ctx.Done()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}
func output(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, status int, e error) {
	output(w, status, map[string]any{"success": false, "error": e.Error()})
}
func parse(w http.ResponseWriter, r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("application/json required")
	}
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v)
}
func (m *Module) authenticate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	r = r.WithContext(ctx)
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "GET" && r.Header.Get("Origin") != "" {
		u, e := url.Parse(r.Header.Get("Origin"))
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if e != nil || u.Host != r.Host || u.Scheme != scheme {
			failure(w, 403, errors.New("origin rejected"))
			return
		}
	}
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		sum := sha256.Sum256([]byte(token))
		hash := hex.EncodeToString(sum[:])
		m.mu.Lock()
		keys := append([]bridgeKey(nil), m.cfg.Keys...)
		m.mu.Unlock()
		for _, k := range keys {
			if subtle.ConstantTimeCompare([]byte(hash), []byte(k.Hash)) == 1 {
				debugIdentity(r, k.Tenant, k.Bridge)
				m.serve(w, r, k.Tenant, k.Bridge, false)
				return
			}
		}
		failure(w, 401, errors.New("invalid history credential"))
		return
	}
	if m.guard == nil {
		failure(w, 503, errors.New("session service unavailable"))
		return
	}
	m.guard.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, admin, ok := m.guard.SessionIdentity(r)
		if !ok || !admin {
			failure(w, 403, errors.New("history administrator required"))
			return
		}
		m.mu.Lock()
		tenant := m.cfg.Tenant
		m.mu.Unlock()
		if p, yes := apibridge.PrincipalFrom(r.Context()); yes && p.TenantID != tenant {
			failure(w, 403, errors.New("tenant mismatch"))
			return
		}
		m.serve(w, r, tenant, "", true)
	})).ServeHTTP(w, r)
}
func (m *Module) save(c settings) error {
	cfg, e := config.Load(m.rt.ConfigPath())
	if e != nil {
		return e
	}
	b, _ := json.Marshal(c)
	var v map[string]interface{}
	_ = json.Unmarshal(b, &v)
	cfg.Modules[m.ID()] = v
	if e = cfg.Save(); e != nil {
		return e
	}
	if e = os.Chmod(m.rt.ConfigPath(), 0600); e != nil {
		return e
	}
	if m.db != nil {
		m.db.Close()
		m.db = nil
	}
	m.cfg = c
	return nil
}
func (m *Module) database(ctx context.Context) (*sql.DB, error) {
	if m.db != nil {
		return m.db, nil
	}
	c := m.cfg
	if c.MySQLDatabase == "" || c.MySQLUser == "" {
		return nil, errors.New("configure MySQL database and user")
	}
	mc := mysql.NewConfig()
	mc.User = c.MySQLUser
	mc.Passwd = c.MySQLPassword
	mc.Net = "tcp"
	mc.Addr = net.JoinHostPort(c.MySQLHost, c.MySQLPort)
	mc.DBName = c.MySQLDatabase
	mc.TLSConfig = c.MySQLTLS
	mc.Timeout = 5 * time.Second
	mc.ReadTimeout = 30 * time.Second
	mc.WriteTimeout = 30 * time.Second
	db, e := sql.Open("mysql", mc.FormatDSN())
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)
	if e = db.PingContext(ctx); e != nil {
		db.Close()
		return nil, e
	}
	_, e = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS siui_history_calls (tenant VARCHAR(128) NOT NULL, bridge VARCHAR(128) NOT NULL, call_id VARCHAR(128) NOT NULL, operation_id VARCHAR(128) NOT NULL, operation VARCHAR(128) NOT NULL, created_at VARCHAR(40) NOT NULL, received_at VARCHAR(40) NOT NULL, status VARCHAR(20) NOT NULL, digest CHAR(64) NOT NULL, record LONGTEXT NOT NULL, PRIMARY KEY(tenant,bridge,call_id), INDEX history_operation(tenant,operation,created_at), INDEX history_operation_id(tenant,operation_id)) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin`)
	if e != nil {
		db.Close()
		return nil, e
	}
	_, e = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS siui_history_details (tenant VARCHAR(128) NOT NULL,bridge VARCHAR(128) NOT NULL,call_id VARCHAR(128) NOT NULL,pid VARCHAR(64) NOT NULL,cid VARCHAR(64) NOT NULL,details LONGTEXT NOT NULL,parse_error TEXT NOT NULL,PRIMARY KEY(tenant,bridge,call_id),INDEX history_pid(tenant,pid),INDEX history_cid(tenant,cid)) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin`)
	if e != nil {
		db.Close()
		return nil, e
	}
	_, e = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS siui_history_storage(tenant VARCHAR(128) NOT NULL,bridge VARCHAR(128) NOT NULL,call_id VARCHAR(128) NOT NULL,location TEXT NOT NULL,PRIMARY KEY(tenant,bridge,call_id)) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin`)
	if e != nil {
		db.Close()
		return nil, e
	}
	m.db = db
	return db, nil
}
func (m *Module) serve(w http.ResponseWriter, r *http.Request, tenant, bridge string, admin bool) {
	if strings.TrimPrefix(r.URL.Path, "/api/history/") == "dashboard" {
		if !admin {
			failure(w, 403, errors.New("history administrator required"))
			return
		}
		m.dashboardHTTP(w, r, tenant)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	route := strings.TrimPrefix(r.URL.Path, "/api/history/")
	if route == "debug/requests" {
		if !admin {
			failure(w, 403, errors.New("history administrator required"))
			return
		}
		if r.Method != "GET" {
			failure(w, 405, errors.New("GET required"))
			return
		}
		m.debugRequests(w, r, tenant)
		return
	}
	if route == "identity" && r.Method == "GET" {
		output(w, 200, map[string]any{"success": true, "tenant": tenant, "bridge": bridge})
		return
	}
	if route == "settings" && admin {
		if r.Method == "GET" {
			c := m.cfg
			c.MySQLPassword = ""
			c.SecretKey = ""
			c.SessionToken = ""
			c.Keys = nil
			output(w, 200, c)
			return
		}
		if r.Method == "PUT" {
			var c settings
			if e := parse(w, r, &c); e != nil {
				failure(w, 400, e)
				return
			}
			if c.MySQLPassword == "" {
				c.MySQLPassword = m.cfg.MySQLPassword
			}
			if c.SecretKey == "" {
				c.SecretKey = m.cfg.SecretKey
			}
			if c.SessionToken == "" {
				c.SessionToken = m.cfg.SessionToken
			}
			c.Keys = m.cfg.Keys
			if c.Tenant == "" || c.StagingPath == "" || !validName(c.Tenant) {
				failure(w, 400, errors.New("tenant and staging path required"))
				return
			}
			if e := m.save(c); e != nil {
				failure(w, 500, e)
				return
			}
			output(w, 200, map[string]bool{"success": true})
			return
		}
	}
	if route == "keys" && admin && r.Method == "GET" {
		rows := []map[string]string{}
		for _, k := range m.cfg.Keys {
			rows = append(rows, map[string]string{"tenant": k.Tenant, "bridge": k.Bridge})
		}
		output(w, 200, map[string]any{"success": true, "keys": rows})
		return
	}
	if route == "keys" && admin && r.Method == "DELETE" {
		id := r.URL.Query().Get("bridge")
		if !validName(id) {
			failure(w, 400, errors.New("bridge required"))
			return
		}
		c := m.cfg
		c.Keys = nil
		for _, k := range m.cfg.Keys {
			if k.Bridge != id || k.Tenant != tenant {
				c.Keys = append(c.Keys, k)
			}
		}
		if e := m.save(c); e != nil {
			failure(w, 500, e)
			return
		}
		output(w, 200, map[string]bool{"success": true})
		return
	}
	if route == "keys" && admin && r.Method == "POST" {
		var in struct {
			Bridge string `json:"bridge"`
		}
		if e := parse(w, r, &in); e != nil || !validName(in.Bridge) || !validName(tenant) {
			failure(w, 400, errors.New("configure tenant and valid bridge ID first"))
			return
		}
		b := make([]byte, 32)
		if _, e := rand.Read(b); e != nil {
			failure(w, 500, e)
			return
		}
		token := hex.EncodeToString(b)
		h := sha256.Sum256([]byte(token))
		c := m.cfg
		c.Keys = append(append([]bridgeKey(nil), c.Keys...), bridgeKey{tenant, in.Bridge, hex.EncodeToString(h[:])})
		if e := m.save(c); e != nil {
			failure(w, 500, e)
			return
		}
		output(w, 201, map[string]any{"success": true, "token": token, "bridge_id": in.Bridge, "tenant": tenant})
		return
	}
	if route == "test-storage" && admin && r.Method == "POST" {
		s, e := m.store()
		if e != nil {
			failure(w, 400, e)
			return
		}
		key := fmt.Sprintf("_tests/%d.txt", time.Now().UnixNano())
		e = s.Put(r.Context(), key, strings.NewReader("SIUI History storage test\n"), int64(len("SIUI History storage test\n")))
		if e != nil {
			failure(w, 502, e)
			return
		}
		e = s.Delete(r.Context(), key)
		result := map[string]any{"success": true, "write": true, "deleted": e == nil, "key": key}
		if e != nil {
			result["delete_error"] = e.Error()
		}
		output(w, 200, result)
		return
	}
	db, e := m.database(r.Context())
	if e != nil {
		failure(w, 503, e)
		return
	}
	if route == "test-db" && admin && r.Method == "POST" {
		output(w, 200, map[string]bool{"success": true})
		return
	}
	m.calls(w, r, db, route, tenant, bridge, admin)
}
