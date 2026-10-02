package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"wisemed-labreaders/serverlast/wsm-server/internal/config"
)

const adminCookie = "wsm_admin"

type adminSession struct {
	TenantID  string    `json:"tenant_id"`
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expires_at"`
	CSRF      string    `json:"-"`
	Epoch     uint64    `json:"-"`
}
type loginWindow struct {
	Count int
	Until time.Time
}
type adminState struct {
	mu       sync.Mutex
	sessions map[[32]byte]adminSession
	attempts map[string]loginWindow
	global   loginWindow
	epoch    uint64
}

func (s *Server) adminRoutes(r chi.Router) {
	r.Get("/admin/api/tenants", s.adminTenants)
	r.Post("/admin/api/login", s.adminLogin)
	r.Get("/admin/api/session", s.adminSessionStatus)
	r.Post("/admin/api/logout", s.adminLogout)
	r.Get("/admin/api/devices", s.adminDevices)
	r.Post("/admin/api/devices", s.adminCreateDevice)
	r.Post("/admin/api/devices/key", s.adminRevealDevice)
	r.Delete("/admin/api/devices/{keyID}", s.adminDeleteDevice)
	r.Patch("/admin/api/devices/{keyID}", s.adminUpdateDevice)
}
func adminError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{"error": message})
}
func (s *Server) adminConfig(w http.ResponseWriter, r *http.Request, mutation bool) *config.Config {
	c := s.current()
	if !c.Admin.Enabled {
		http.NotFound(w, r)
		return nil
	}
	origin, _ := url.Parse(c.Admin.PublicOrigin)
	if origin == nil || r.Host != origin.Host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != c.Admin.PublicOrigin) {
		adminError(w, 403, "origin not allowed")
		return nil
	}
	if mutation {
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if r.Header.Get("Origin") != c.Admin.PublicOrigin || err != nil || media != "application/json" {
			adminError(w, 403, "same-origin JSON required")
			return nil
		}
	}
	return c
}
func decodeAdmin(w http.ResponseWriter, r *http.Request, out interface{}) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		adminError(w, 400, "invalid JSON request")
		return false
	}
	var extra interface{}
	if d.Decode(&extra) != io.EOF {
		adminError(w, 400, "invalid JSON request")
		return false
	}
	return true
}
func randomAdminToken() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), e
}
func (s *Server) adminTenants(w http.ResponseWriter, r *http.Request) {
	c := s.adminConfig(w, r, false)
	if c == nil {
		return
	}
	list := []map[string]string{}
	for id, t := range c.Tenants {
		if !t.Disabled && t.WiseMed.BaseURL != "" && t.WiseMed.APIKey != "" {
			list = append(list, map[string]string{"id": id, "label": id})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i]["id"] < list[j]["id"] })
	writeJSON(w, 200, map[string]interface{}{"tenants": list})
}
func (s *Server) allowAdminLogin(ip string) bool {
	s.admin.mu.Lock()
	defer s.admin.mu.Unlock()
	now := time.Now()
	if s.admin.attempts == nil {
		s.admin.attempts = map[string]loginWindow{}
	}
	for id, v := range s.admin.attempts {
		if !now.Before(v.Until) {
			delete(s.admin.attempts, id)
		}
	}
	if !now.Before(s.admin.global.Until) {
		s.admin.global = loginWindow{Until: now.Add(time.Minute)}
	}
	v, exists := s.admin.attempts[ip]
	if !exists {
		if len(s.admin.attempts) >= 1024 {
			return false
		}
		v.Until = now.Add(5 * time.Minute)
	}
	if v.Count >= 10 || s.admin.global.Count >= 60 {
		return false
	}
	v.Count++
	s.admin.global.Count++
	s.admin.attempts[ip] = v
	return true
}
func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	c := s.adminConfig(w, r, true)
	if c == nil {
		return
	}
	ip, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		ip = r.RemoteAddr
	}
	if !s.allowAdminLogin(ip) {
		adminError(w, 429, "too many login attempts")
		return
	}
	var in struct {
		TenantID string `json:"tenant_id"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeAdmin(w, r, &in) {
		return
	}
	if !config.ValidID(in.TenantID) || strings.TrimSpace(in.Username) == "" || len(in.Username) > 256 || in.Password == "" || len(in.Password) > 4096 {
		adminError(w, 400, "tenant, username and password required")
		return
	}
	t, ok := c.Tenants[in.TenantID]
	if !ok || t.Disabled || t.WiseMed.BaseURL == "" || t.WiseMed.APIKey == "" {
		adminError(w, 401, "authentication failed")
		return
	}
	// No locks or stored passwords/tokens while calling the configured HTTPS backend.
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if err := s.authenticateAdmin(ctx, t.WiseMed, in.Username, in.Password); err != nil {
		adminError(w, 401, "authentication failed: WiseMED administrator user_type -1 required")
		return
	}
	in.Password = ""
	token, err := randomAdminToken()
	if err != nil {
		adminError(w, 500, "cannot create session")
		return
	}
	csrf, err := randomAdminToken()
	if err != nil {
		adminError(w, 500, "cannot create session")
		return
	}
	// Reload may revoke access while the upstream request was running.
	s.gate.RLock()
	defer s.gate.RUnlock()
	if s.cfg != c {
		adminError(w, 409, "configuration changed; sign in again")
		return
	}
	s.admin.mu.Lock()
	defer s.admin.mu.Unlock()
	now := time.Now()
	if s.admin.sessions == nil {
		s.admin.sessions = map[[32]byte]adminSession{}
	}
	for id, v := range s.admin.sessions {
		if !now.Before(v.ExpiresAt) {
			delete(s.admin.sessions, id)
		}
	}
	if len(s.admin.sessions) >= 1024 {
		adminError(w, 503, "too many admin sessions")
		return
	}
	if cookie, e := r.Cookie(adminCookie); e == nil {
		delete(s.admin.sessions, sha256.Sum256([]byte(cookie.Value)))
	}
	sess := adminSession{TenantID: in.TenantID, Username: in.Username, ExpiresAt: now.Add(time.Duration(c.Admin.SessionTTLSeconds) * time.Second), CSRF: csrf, Epoch: s.admin.epoch}
	s.admin.sessions[sha256.Sum256([]byte(token))] = sess
	s.setAdminCookie(w, c, token, c.Admin.SessionTTLSeconds)
	writeJSON(w, 200, map[string]interface{}{"authenticated": true, "session": sess, "csrf_token": csrf})
}
func (s *Server) authenticateAdmin(ctx context.Context, upstream config.WiseMed, username, password string) error {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password, "device_id": "wsm-server", "device_name": "WSM administration"})
	req, e := http.NewRequestWithContext(ctx, http.MethodPut, strings.TrimRight(upstream.BaseURL, "/")+"/administrative/login", bytes.NewReader(body))
	if e != nil {
		return e
	}
	token, e := createWiseMedJWT(upstream.APIKey)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	response, e := s.upstreamClient.Do(req)
	if e != nil {
		return errors.New("login upstream failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("login rejected")
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, 65537))
	if e != nil || len(data) > 65536 {
		return errors.New("invalid login response")
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return errors.New("invalid login response")
	}
	// Presence is mandatory. Accept only the integer -1 or canonical string "-1";
	// never a default zero, less-than-zero comparison, decimal or boolean coercion.
	v := strings.TrimSpace(string(raw["user_type"]))
	if v != "-1" && v != `"-1"` {
		return errors.New("not a WiseMED administrator")
	}
	for _, name := range []string{"error", "errors"} {
		value := strings.TrimSpace(string(raw[name]))
		if value != "" && value != "null" && value != "false" && value != `""` && value != "[]" && value != "{}" {
			return errors.New("login rejected")
		}
	}
	switch strings.ToLower(strings.TrimSpace(string(raw["status"]))) {
	case "false", "0", `"false"`, `"failure"`, `"failed"`, `"error"`, `"denied"`:
		return errors.New("login rejected")
	}
	for _, name := range []string{"ok", "success"} {
		if value := strings.TrimSpace(string(raw[name])); value != "" && value != "true" {
			return errors.New("login rejected")
		}
	}
	for _, name := range []string{"login_token", "token", "lt"} {
		var value string
		if json.Unmarshal(raw[name], &value) == nil && strings.TrimSpace(value) != "" {
			return nil
		}
	}
	return errors.New("missing login token")
}
func (s *Server) setAdminCookie(w http.ResponseWriter, c *config.Config, value string, maxAge int) {
	cookie := &http.Cookie{Name: adminCookie, Value: value, Path: "/admin", HttpOnly: true, Secure: strings.HasPrefix(c.Admin.PublicOrigin, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: maxAge}
	if maxAge < 0 {
		cookie.Expires = time.Unix(1, 0)
	}
	http.SetCookie(w, cookie)
}
func (s *Server) findAdminSession(r *http.Request) (adminSession, bool) {
	cookie, e := r.Cookie(adminCookie)
	if e != nil || len(cookie.Value) != 43 {
		return adminSession{}, false
	}
	id := sha256.Sum256([]byte(cookie.Value))
	s.admin.mu.Lock()
	defer s.admin.mu.Unlock()
	sess, ok := s.admin.sessions[id]
	if !ok {
		return sess, false
	}
	if !time.Now().Before(sess.ExpiresAt) || sess.Epoch != s.admin.epoch {
		delete(s.admin.sessions, id)
		return adminSession{}, false
	}
	return sess, true
}
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request, mutation bool) (*config.Config, adminSession, bool) {
	c := s.adminConfig(w, r, mutation)
	if c == nil {
		return nil, adminSession{}, false
	}
	sess, ok := s.findAdminSession(r)
	t, found := c.Tenants[sess.TenantID]
	if !ok || !found || t.Disabled {
		adminError(w, 401, "sign in required")
		return nil, adminSession{}, false
	}
	if mutation && subtle.ConstantTimeCompare([]byte(sess.CSRF), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
		adminError(w, 403, "invalid CSRF token")
		return nil, adminSession{}, false
	}
	return c, sess, true
}
func (s *Server) adminSessionStatus(w http.ResponseWriter, r *http.Request) {
	if s.adminConfig(w, r, false) == nil {
		return
	}
	sess, ok := s.findAdminSession(r)
	if !ok {
		writeJSON(w, 200, map[string]interface{}{"authenticated": false})
		return
	}
	writeJSON(w, 200, map[string]interface{}{"authenticated": true, "session": sess, "csrf_token": sess.CSRF})
}
func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	c, _, ok := s.requireAdmin(w, r, true)
	if !ok {
		return
	}
	cookie, _ := r.Cookie(adminCookie)
	s.admin.mu.Lock()
	delete(s.admin.sessions, sha256.Sum256([]byte(cookie.Value)))
	s.admin.mu.Unlock()
	s.setAdminCookie(w, c, "", -1)
	writeJSON(w, 200, map[string]interface{}{"ok": true})
}
func (s *Server) deviceView(c *config.Config, id string, d config.Device) map[string]interface{} {
	online := s.equipmentStatus(d.TenantID, d.EquipmentID)["online"] == true
	return map[string]interface{}{"key_id": id, "equipment_id": d.EquipmentID, "reader_id": d.ReaderID, "subject": d.Subject, "tenant_id": d.TenantID, "issuer": c.Tenants[d.TenantID].Issuer, "audience": c.Security.Audience, "scopes": d.EffectiveScopes(), "online": online}
}
func (s *Server) adminDevices(w http.ResponseWriter, r *http.Request) {
	c, sess, ok := s.requireAdmin(w, r, false)
	if !ok {
		return
	}
	list := []map[string]interface{}{}
	for id, d := range c.Devices {
		if d.TenantID == sess.TenantID {
			list = append(list, s.deviceView(c, id, d))
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i]["equipment_id"].(string) < list[j]["equipment_id"].(string) })
	writeJSON(w, 200, map[string]interface{}{"devices": list, "permissions": config.DevicePermissionCatalog()})
}

// Mutations revalidate the session while holding the same gate as reload and WSS
// admissions. Persist before publishing; no caller can observe an uncommitted key.
func (s *Server) adminMutationSession(w http.ResponseWriter, r *http.Request, c *config.Config) bool {
	if s.cfg != c {
		adminError(w, 409, "configuration changed; retry")
		return false
	}
	if _, ok := s.findAdminSession(r); !ok {
		adminError(w, 401, "sign in required")
		return false
	}
	return true
}
func (s *Server) adminCreateDevice(w http.ResponseWriter, r *http.Request) {
	c, sess, ok := s.requireAdmin(w, r, true)
	if !ok {
		return
	}
	var in struct {
		EquipmentID string `json:"equipment_id"`
		ReaderID    string `json:"reader_id"`
	}
	if !decodeAdmin(w, r, &in) {
		return
	}
	if !config.ValidID(in.EquipmentID) || (in.ReaderID != "" && !config.ValidID(in.ReaderID)) {
		adminError(w, 400, "valid equipment_id and optional reader_id required")
		return
	}
	secret, e := randomAdminToken()
	if e != nil {
		adminError(w, 500, "cannot generate credential")
		return
	}
	suffix, e := randomAdminToken()
	if e != nil {
		adminError(w, 500, "cannot generate credential")
		return
	}
	id := "device-" + suffix
	s.gate.Lock()
	defer s.gate.Unlock()
	if !s.adminMutationSession(w, r, c) {
		return
	}
	if s.equipmentStatus(sess.TenantID, in.EquipmentID)["online"] == true {
		adminError(w, 409, "equipment is already connected; provision while offline")
		return
	}
	for _, k := range c.Security.Keys {
		if k.TenantID == sess.TenantID && k.EquipmentID == in.EquipmentID {
			adminError(w, 409, "equipment already has a key")
			return
		}
	}
	devices := map[string]config.Device{}
	for k, v := range c.Devices {
		devices[k] = v
	}
	device := config.Device{TenantID: sess.TenantID, EquipmentID: in.EquipmentID, ReaderID: in.ReaderID, Subject: id, Secret: secret}
	devices[id] = device
	next, e := c.WithDevices(devices)
	if e != nil {
		adminError(w, 409, "device identity conflict")
		return
	}
	if config.WriteDevices(c.Security.DeviceKeysFile, devices) != nil {
		adminError(w, 500, "cannot persist device registry")
		return
	}
	s.cfg = next
	log.Printf("admin device created tenant=%s equipment=%s key=%s", sess.TenantID, in.EquipmentID, id)
	writeJSON(w, 201, map[string]interface{}{"device": s.deviceView(next, id, device), "secret": secret})
}
func (s *Server) adminRevealDevice(w http.ResponseWriter, r *http.Request) {
	c, sess, ok := s.requireAdmin(w, r, true)
	if !ok {
		return
	}
	var in struct {
		EquipmentID string `json:"equipment_id"`
	}
	if !decodeAdmin(w, r, &in) {
		return
	}
	s.gate.RLock()
	defer s.gate.RUnlock()
	if !s.adminMutationSession(w, r, c) {
		return
	}
	for id, d := range c.Devices {
		if d.TenantID == sess.TenantID && d.EquipmentID == in.EquipmentID {
			log.Printf("admin device key revealed tenant=%s equipment=%s key=%s", sess.TenantID, d.EquipmentID, id)
			writeJSON(w, 200, map[string]interface{}{"device": s.deviceView(c, id, d), "secret": d.Secret})
			return
		}
	}
	adminError(w, 404, "managed equipment not found")
}
func (s *Server) adminDeleteDevice(w http.ResponseWriter, r *http.Request) {
	c, sess, ok := s.requireAdmin(w, r, true)
	if !ok {
		return
	}
	id := chi.URLParam(r, "keyID")
	s.gate.Lock()
	defer s.gate.Unlock()
	if !s.adminMutationSession(w, r, c) {
		return
	}
	d, exists := c.Devices[id]
	if !exists || d.TenantID != sess.TenantID {
		adminError(w, 404, "managed equipment not found")
		return
	}
	devices := map[string]config.Device{}
	for k, v := range c.Devices {
		if k != id {
			devices[k] = v
		}
	}
	next, e := c.WithDevices(devices)
	if e != nil {
		adminError(w, 500, "cannot update devices")
		return
	}
	if config.WriteDevices(c.Security.DeviceKeysFile, devices) != nil {
		adminError(w, 500, "cannot persist device registry")
		return
	}
	s.cfg = next
	s.revokeDeviceTickets(d)
	s.activeMu.Lock()
	for conn := range s.active {
		if conn.TenantID == d.TenantID && conn.Subject == d.Subject {
			conn.Close()
		}
	}
	s.activeMu.Unlock()
	log.Printf("admin device revoked tenant=%s equipment=%s key=%s", sess.TenantID, d.EquipmentID, id)
	writeJSON(w, 200, map[string]interface{}{"ok": true})
}

// adminUpdateDevice changes reader binding or permissions; credentials remain stable.
func (s *Server) adminUpdateDevice(w http.ResponseWriter, r *http.Request) {
	c, sess, ok := s.requireAdmin(w, r, true)
	if !ok {
		return
	}
	var in struct {
		ReaderID *string   `json:"reader_id"`
		Scopes   *[]string `json:"scopes"`
	}
	if !decodeAdmin(w, r, &in) {
		return
	}
	if (in.ReaderID == nil && in.Scopes == nil) || (in.ReaderID != nil && *in.ReaderID != "" && !config.ValidID(*in.ReaderID)) {
		adminError(w, 400, "reader_id or scopes is required; empty reader_id removes the binding")
		return
	}
	id := chi.URLParam(r, "keyID")
	s.gate.Lock()
	defer s.gate.Unlock()
	if !s.adminMutationSession(w, r, c) {
		return
	}
	d, exists := c.Devices[id]
	if !exists || d.TenantID != sess.TenantID {
		adminError(w, 404, "managed equipment not found")
		return
	}
	if in.Scopes != nil {
		if err := config.ValidateDeviceScopes(*in.Scopes); err != nil {
			adminError(w, 400, err.Error())
			return
		}
	}
	if (in.ReaderID == nil || d.ReaderID == *in.ReaderID) && (in.Scopes == nil || slices.Equal(d.EffectiveScopes(), *in.Scopes)) {
		writeJSON(w, 200, map[string]interface{}{"device": s.deviceView(c, id, d)})
		return
	}
	devices := map[string]config.Device{}
	for k, v := range c.Devices {
		devices[k] = v
	}
	if in.ReaderID != nil {
		d.ReaderID = *in.ReaderID
	}
	if in.Scopes != nil {
		scopes := slices.Clone(*in.Scopes)
		d.Scopes = &scopes
	}
	devices[id] = d
	next, err := c.WithDevices(devices)
	if err != nil {
		adminError(w, 409, "device identity conflict")
		return
	}
	if config.WriteDevices(c.Security.DeviceKeysFile, devices) != nil {
		adminError(w, 500, "cannot persist device registry")
		return
	}
	s.cfg = next
	s.revokeDeviceTickets(d)
	// Reauthenticate this device under its new policy, including pre-hello sockets.
	s.activeMu.Lock()
	for conn := range s.active {
		if conn.TenantID == d.TenantID && conn.Subject == d.Subject {
			conn.Close()
		}
	}
	s.activeMu.Unlock()
	log.Printf("admin device updated tenant=%s equipment=%s key=%s", sess.TenantID, d.EquipmentID, id)
	writeJSON(w, 200, map[string]interface{}{"device": s.deviceView(next, id, d)})
}

// Called under the admission gate: stale tickets must not retain removed permissions.
func (s *Server) revokeDeviceTickets(d config.Device) {
	s.ticketMu.Lock()
	defer s.ticketMu.Unlock()
	for id, t := range s.tickets {
		if t.claims.TenantID == d.TenantID && t.claims.Subject == d.Subject {
			delete(s.tickets, id)
		}
	}
}
