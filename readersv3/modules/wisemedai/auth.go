package wisemedai

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"wisemed-labreaders/readersv3/modules/wisemedapi"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

type principal struct {
	UserType      int    `json:"user_type"`
	InstanceAdmin bool   `json:"instance_admin"`
	User          string `json:"user_id"`
	Tenant        string `json:"tenant_id"`
	Role          string `json:"role"`
	Email         string `json:"email"`
	Name          string `json:"name"`
}
type loginAttempt struct {
	Count int
	Until time.Time
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		fail(w, 400, "invalid_json")
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		fail(w, 400, "invalid_json")
		return false
	}
	return true
}
func (m *Module) identity(r *http.Request) (p principal, e error) {
	defer func() { p.UserType = userType(p.InstanceAdmin, p.Role) }()
	if x, ok := apibridge.PrincipalFrom(r.Context()); ok && m.cfg.WSMIdentity {
		e = m.db.QueryRow(r.Context(), `SELECT u.id,m.tenant_id,m.role,u.email,u.name,u.instance_admin FROM wai_memberships m JOIN wai_users u ON u.id=m.user_id WHERE m.tenant_id::text=$1 AND m.external_subject=$2 AND NOT u.disabled AND NOT m.disabled`, x.TenantID, x.Subject).Scan(&p.User, &p.Tenant, &p.Role, &p.Email, &p.Name, &p.InstanceAdmin)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		if c, err := r.Cookie("wai_session"); err == nil {
			token = c.Value
		}
	}
	if len(token) != 64 {
		return p, pgx.ErrNoRows
	}
	e = m.db.QueryRow(r.Context(), `SELECT u.id,s.tenant_id,m.role,u.email,u.name,u.instance_admin FROM wai_auth_sessions s JOIN wai_users u ON u.id=s.user_id JOIN wai_memberships m ON m.user_id=s.user_id AND m.tenant_id=s.tenant_id WHERE s.token_hash=$1 AND s.expires_at>now() AND NOT u.disabled AND NOT m.disabled`, digest(token)).Scan(&p.User, &p.Tenant, &p.Role, &p.Email, &p.Name, &p.InstanceAdmin)
	return
}
func (m *Module) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host == "" {
		host = r.RemoteAddr
	}
	m.loginMu.Lock()
	a := m.attempts[host]
	if time.Now().After(a.Until) {
		a = loginAttempt{Until: time.Now().Add(10 * time.Minute)}
	}
	a.Count++
	m.attempts[host] = a
	for k, v := range m.attempts {
		if time.Now().After(v.Until) {
			delete(m.attempts, k)
		}
	}
	m.loginMu.Unlock()
	if a.Count > 15 {
		fail(w, 429, "login_rate_limited")
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Tenant   string `json:"tenant_id"`
		WiseMED  bool   `json:"wisemed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Password) > 200 || len(in.Email) > 254 {
		fail(w, 400, "invalid_credentials")
		return
	}
	var p principal
	var hash string
	var err error
	if in.WiseMED {
		if !m.cfg.WiseMEDLogin {
			fail(w, 503, "wisemed_login_disabled")
			return
		}
		svc, ok := m.rt.Service("wisemed-api")
		api, yes := svc.(interface {
			Login(wisemedapi.LoginRequest) (wisemedapi.LoginResponse, error)
		})
		if !ok || !yes {
			fail(w, 503, "wisemed_unavailable")
			return
		}
		// The shared client performs the real server-to-server protocol. Its login token is never accepted as an AI bearer token.
		info, e := api.Login(wisemedapi.LoginRequest{Username: in.Email, Password: in.Password, DeviceID: m.rt.ReaderID(), DeviceName: "WiseMED AI"})
		if e != nil {
			fail(w, 401, "invalid_credentials")
			return
		}
		err = m.db.QueryRow(r.Context(), `SELECT u.id,m.tenant_id,m.role,u.email,u.name,u.password_hash,u.instance_admin FROM wai_users u JOIN wai_memberships m ON m.user_id=u.id WHERE m.external_subject=$1 AND m.tenant_id::text=$2 AND NOT u.disabled AND NOT m.disabled`, "wisemed:"+info.UserID.String(), in.Tenant).Scan(&p.User, &p.Tenant, &p.Role, &p.Email, &p.Name, &hash, &p.InstanceAdmin)
	} else {
		err = m.db.QueryRow(r.Context(), `SELECT u.id,m.tenant_id,m.role,u.email,u.name,u.password_hash,u.instance_admin FROM wai_users u JOIN wai_memberships m ON m.user_id=u.id WHERE u.email=$1 AND NOT u.disabled AND NOT m.disabled AND ($2='' OR m.tenant_id::text=$2) ORDER BY m.tenant_id LIMIT 1`, strings.ToLower(in.Email), in.Tenant).Scan(&p.User, &p.Tenant, &p.Role, &p.Email, &p.Name, &hash, &p.InstanceAdmin)
		if err == nil {
			err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password))
		} else {
			_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$7EqJtq98hPqEX7fNZaFWoO5u.WFV3/u/l8n6eKhFMuERzStDeM3jm"), []byte(in.Password))
		}
	}
	if err != nil {
		fail(w, 401, "invalid_credentials")
		return
	}
	p.UserType = userType(p.InstanceAdmin, p.Role)
	token := randomToken()
	tx, e := m.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), `INSERT INTO wai_auth_sessions(token_hash,tenant_id,user_id,expires_at)VALUES($1,$2,$3,now()+interval '8 hours')`, digest(token), p.Tenant, p.User); e == nil {
		e = audit(r.Context(), tx, p, "login", p.User)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "wai_session", Value: token, Path: "/", HttpOnly: true, Secure: m.cfg.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: 28800})
	respond(w, 200, map[string]any{"token": token, "profile": p, "expires_in": 28800})
}
func sameOrigin(r *http.Request) bool {
	if o := r.Header.Get("Origin"); o != "" {
		u, e := url.Parse(o)
		return e == nil && u.Host == r.Host && (u.Scheme == "https" || u.Scheme == "http")
	}
	return r.Header.Get("Sec-Fetch-Site") != "cross-site"
}
func (m *Module) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !sameOrigin(r) {
		fail(w, 403, "cross_origin_forbidden")
		return
	}
	if m.db == nil {
		fail(w, 503, "database_not_configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	r = r.WithContext(ctx)
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/"), "/")
	if path == "auth/providers" && r.Method == "GET" {
		respond(w, 200, map[string]bool{"standalone": true, "wisemed": m.cfg.WiseMEDLogin})
		return
	}
	if path == "auth/login" {
		m.login(w, r)
		return
	}
	if path == "browser-speech/bridge" {
		m.browserSpeechBridge(w, r)
		return
	}
	p, e := m.identity(r)
	if e != nil {
		fail(w, 401, "authentication_required")
		return
	}
	if path == "auth/logout" && r.Method == "POST" {
		if c, e := r.Cookie("wai_session"); e == nil {
			_, _ = m.db.Exec(ctx, `DELETE FROM wai_auth_sessions WHERE token_hash=$1`, digest(c.Value))
		}
		if t := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); len(t) == 64 {
			_, _ = m.db.Exec(ctx, `DELETE FROM wai_auth_sessions WHERE token_hash=$1`, digest(t))
		}
		http.SetCookie(w, &http.Cookie{Name: "wai_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: m.cfg.SecureCookies})
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if path == "profile/speech-consent" {
		m.speechConsent(w, r, p)
		return
	}
	if path == "profile" {
		m.profile(w, r, p)
		return
	}
	if strings.HasPrefix(path, "community/speech-contributions") {
		if r.Method != "GET" && r.Method != "DELETE" && p.Role == "viewer" {
			fail(w, 403, "doctor_required")
			return
		}
		m.communitySpeech(w, r, p, strings.Split(path, "/"))
		return
	}
	if path == "speech-models" || strings.HasPrefix(path, "speech-models/") {
		m.speechModels(w, r, p, strings.Split(path, "/"))
		return
	}
	// A financial suspension keeps authentication, statements and payment recovery accessible.
	if !p.InstanceAdmin && !strings.HasPrefix(path, "billing/") {
		state, e := m.subscriptionState(ctx, p.User)
		if e != nil {
			fail(w, 503, "billing_unavailable")
			return
		}
		if state != nil && state.Suspended {
			respond(w, 402, map[string]any{"error": state.Reason, "subscription": state, "account_url": "/v1/billing/account"})
			return
		}
	}
	if r.Method != "GET" && p.Role == "viewer" {
		fail(w, 403, "doctor_required")
		return
	}
	switch {
	case path == "admin/sessions" || strings.HasPrefix(path, "admin/sessions/"):
		m.adminSessions(w, r, p, strings.TrimPrefix(path, "admin/sessions"))
	case path == "browser-speech":
		m.createBrowserSpeech(w, r, p)
	case strings.HasPrefix(path, "templates/") && (strings.Contains(path, "/speech-adaptation") || strings.Contains(path, "/speech-terms") || strings.Contains(path, "/speech-feedback")):
		m.speechAdaptation(w, r, p, strings.Split(path, "/"))
	case strings.HasPrefix(path, "community/speech-contributions"):
		m.communitySpeech(w, r, p, strings.Split(path, "/"))
	case strings.HasPrefix(path, "billing/template-addons/"):
		m.cancelTemplateAddon(w, r, p, strings.TrimPrefix(path, "billing/template-addons/"))
	case strings.HasPrefix(path, "billing/"):
		m.billing(w, r, p, path)
	case path == "templates/proposals":
		m.proposals(w, r, p)
	case path == "organizations" && r.Method != "GET":
		m.organization(w, r, p)
	case path == "runtime-settings":
		m.runtimeSettings(w, r, p, "")
	case path == "updates/check" || path == "wsm/reconnect":
		m.runtimeSettings(w, r, p, path)
	case path == "memory/search":
		m.memorySearch(w, r, p)
	case strings.HasPrefix(path, "memory/examples/") && strings.HasSuffix(path, "/embedding"):
		m.memoryEmbedding(w, r, p, strings.TrimSuffix(strings.TrimPrefix(path, "memory/examples/"), "/embedding"))
	case path == "outbox":
		m.outbox(w, r, p)
	case path == "dashboard":
		m.dashboard(w, r, p)
	case path == "settings":
		m.settings(w, r, p)
	case path == "templates" || strings.HasPrefix(path, "templates/"):
		m.templates(w, r, p, strings.TrimPrefix(path, "templates"))
	case path == "sessions" || strings.HasPrefix(path, "sessions/"):
		m.sessions(w, r, p, strings.TrimPrefix(path, "sessions"))
	case path == "pairing/redeem":
		m.redeem(w, r, p)
	case path == "users":
		m.users(w, r, p)
	case path == "audit":
		m.auditHTTP(w, r, p)
	case path == "usage":
		m.usage(w, r, p)
	default:
		m.resources(w, r, p, path)
	}
}
