package server

import (
	"log"
	"net/http"
	"slices"
	"sort"

	"github.com/go-chi/chi/v5"
	"wisemed-labreaders/serverlast/wsm-server/internal/config"
)

func maskCredential(secret string) string {
	chars := []rune(secret)
	if len(chars) < 7 {
		if secret == "" {
			return ""
		}
		return "***"
	}
	return string(chars[:3]) + "***" + string(chars[len(chars)-3:])
}
func (s *Server) adminTenantInfo(w http.ResponseWriter, r *http.Request) {
	c, sess, ok := s.requireAdmin(w, r, false)
	if !ok {
		return
	}
	tenant := c.Tenants[sess.TenantID]
	writeJSON(w, 200, map[string]interface{}{
		"tenant_id": sess.TenantID, "issuer": tenant.Issuer,
		"allowed_origins": tenant.AllowedOrigins,
		"wisemed":         map[string]string{"base_url": tenant.WiseMed.BaseURL, "api_key_masked": maskCredential(tenant.WiseMed.APIKey)},
	})
}
func browserView(c *config.Config, id string) map[string]interface{} {
	key := c.Security.Keys[id]
	browser, managed := c.Browsers[id]
	return map[string]interface{}{"key_id": id, "tenant_id": key.TenantID,
		"label": browser.Label, "origin": browser.Origin, "managed": managed,
		"issuer": c.Tenants[key.TenantID].Issuer, "audience": c.Security.Audience,
		"allowed_origins": c.Tenants[key.TenantID].AllowedOrigins,
		"roles":           key.Roles, "scopes": key.Scopes, "secret_masked": maskCredential(key.Secret), "disabled": key.Disabled}
}
func (s *Server) adminBrowsers(w http.ResponseWriter, r *http.Request) {
	c, sess, ok := s.requireAdmin(w, r, false)
	if !ok {
		return
	}
	list := []map[string]interface{}{}
	for id, key := range c.Security.Keys {
		if key.TenantID == sess.TenantID && slices.Contains(key.Roles, "browser") {
			list = append(list, browserView(c, id))
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i]["key_id"].(string) < list[j]["key_id"].(string) })
	writeJSON(w, 200, map[string]interface{}{"browsers": list, "permissions": config.DevicePermissionCatalog()})
}

// Called under the gate; only publish credentials after durable registry persistence.
func (s *Server) persistAccess(w http.ResponseWriter, c *config.Config, registry config.Registry) *config.Config {
	next, err := c.WithAccess(registry)
	if err != nil {
		adminError(w, 400, err.Error())
		return nil
	}
	if err = config.WriteRegistry(c.Security.DeviceKeysFile, registry); err != nil {
		adminError(w, 500, "cannot persist access registry")
		return nil
	}
	s.cfg = next
	return next
}
func (s *Server) adminCreateBrowser(w http.ResponseWriter, r *http.Request) {
	c, sess, ok := s.requireAdmin(w, r, true)
	if !ok {
		return
	}
	var in struct {
		KeyID  string   `json:"key_id"`
		Origin string   `json:"origin"`
		Scopes []string `json:"scopes"`
	}
	if !decodeAdmin(w, r, &in) {
		return
	}
	if !config.ValidID(in.KeyID) || !config.ValidOrigin(in.Origin) {
		adminError(w, 400, "valid key_id and exact HTTPS origin required")
		return
	}
	if in.Scopes == nil {
		in.Scopes = config.BrowserScopes()
	}
	if err := config.ValidateDeviceScopes(in.Scopes); err != nil {
		adminError(w, 400, err.Error())
		return
	}
	secret, err := randomAdminToken()
	if err != nil {
		adminError(w, 500, "cannot generate credential")
		return
	}
	s.gate.Lock()
	defer s.gate.Unlock()
	if !s.adminMutationSession(w, r, c) {
		return
	}
	if _, exists := c.Security.Keys[in.KeyID]; exists {
		adminError(w, 409, "key_id already exists")
		return
	}
	registry, err := config.ReadRegistry(c.Security.DeviceKeysFile)
	if err != nil {
		adminError(w, 500, "cannot read access registry")
		return
	}
	registry.Browsers[in.KeyID] = config.Browser{TenantID: sess.TenantID, Label: in.KeyID, Origin: in.Origin, Secret: secret, Scopes: in.Scopes}
	origins := slices.Clone(c.Tenants[sess.TenantID].AllowedOrigins)
	if !slices.Contains(origins, in.Origin) {
		origins = append(origins, in.Origin)
	}
	registry.Origins[sess.TenantID] = origins
	next := s.persistAccess(w, c, registry)
	if next == nil {
		return
	}
	log.Printf("admin browser created tenant=%s key=%s", sess.TenantID, in.KeyID)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 201, map[string]interface{}{"browser": browserView(next, in.KeyID), "secret": secret})
}
func (s *Server) adminRevealBrowser(w http.ResponseWriter, r *http.Request) {
	c, sess, ok := s.requireAdmin(w, r, true)
	if !ok {
		return
	}
	var in struct {
		KeyID string `json:"key_id"`
	}
	if !decodeAdmin(w, r, &in) {
		return
	}
	s.gate.RLock()
	defer s.gate.RUnlock()
	if !s.adminMutationSession(w, r, c) {
		return
	}
	key, exists := c.Security.Keys[in.KeyID]
	if !exists || key.TenantID != sess.TenantID || !slices.Contains(key.Roles, "browser") {
		adminError(w, 404, "cheie browser inexistenta in tenantul curent; foloseste ID-ul exact din browsers")
		return
	}
	if key.Secret == "" {
		adminError(w, 409, "secretul cheii nu este incarcat; verifica daca cheia si tenantul sunt active")
		return
	}
	log.Printf("admin browser key revealed tenant=%s key=%s", sess.TenantID, in.KeyID)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]interface{}{"browser": browserView(c, in.KeyID), "secret": key.Secret})
}
func (s *Server) adminChangeBrowser(w http.ResponseWriter, r *http.Request) {
	c, sess, ok := s.requireAdmin(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Scopes *[]string `json:"scopes"`
		Origin *string   `json:"origin"`
		Rotate bool      `json:"rotate"`
	}
	if !decodeAdmin(w, r, &in) {
		return
	}
	if r.Method == "PATCH" {
		if in.Scopes == nil && in.Origin == nil && !in.Rotate {
			adminError(w, 400, "origin, scopes or rotate required")
			return
		}
		if in.Scopes != nil {
			if err := config.ValidateDeviceScopes(*in.Scopes); err != nil {
				adminError(w, 400, err.Error())
				return
			}
		}
		if in.Origin != nil && !config.ValidOrigin(*in.Origin) {
			adminError(w, 400, "exact HTTPS origin required")
			return
		}
	}
	s.gate.Lock()
	defer s.gate.Unlock()
	if !s.adminMutationSession(w, r, c) {
		return
	}
	id := chi.URLParam(r, "keyID")
	browser, exists := c.Browsers[id]
	if !exists || browser.TenantID != sess.TenantID {
		adminError(w, 404, "managed browser not found; configured keys are read-only")
		return
	}
	registry, err := config.ReadRegistry(c.Security.DeviceKeysFile)
	if err != nil {
		adminError(w, 500, "cannot read access registry")
		return
	}
	secret := ""
	if r.Method == "DELETE" {
		delete(registry.Browsers, id)
	} else {
		if in.Scopes != nil {
			browser.Scopes = *in.Scopes
		}
		if in.Origin != nil {
			browser.Origin = *in.Origin
			origins := slices.Clone(c.Tenants[sess.TenantID].AllowedOrigins)
			if !slices.Contains(origins, browser.Origin) {
				origins = append(origins, browser.Origin)
			}
			registry.Origins[sess.TenantID] = origins
		}
		if in.Rotate {
			secret, err = randomAdminToken()
			if err != nil {
				adminError(w, 500, "cannot generate credential")
				return
			}
			browser.Secret = secret
		}
		registry.Browsers[id] = browser
	}
	next := s.persistAccess(w, c, registry)
	if next == nil {
		return
	}
	s.revokeTenantAccess(sess.TenantID)
	log.Printf("admin browser changed tenant=%s key=%s method=%s", sess.TenantID, id, r.Method)
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == "DELETE" {
		writeJSON(w, 200, map[string]bool{"ok": true})
	} else {
		out := map[string]interface{}{"browser": browserView(next, id)}
		if secret != "" {
			out["secret"] = secret
		}
		writeJSON(w, 200, out)
	}
}
func (s *Server) adminUpdateTenant(w http.ResponseWriter, r *http.Request) {
	c, sess, ok := s.requireAdmin(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Issuer  *string `json:"issuer"`
		BaseURL *string `json:"base_url"`
		APIKey  *string `json:"api_key"`
	}
	if !decodeAdmin(w, r, &in) {
		return
	}
	if in.Issuer == nil && in.BaseURL == nil && in.APIKey == nil {
		adminError(w, 400, "issuer, base_url or api_key required")
		return
	}
	s.gate.Lock()
	defer s.gate.Unlock()
	if !s.adminMutationSession(w, r, c) {
		return
	}
	registry, err := config.ReadRegistry(c.Security.DeviceKeysFile)
	if err != nil {
		adminError(w, 500, "cannot read access registry")
		return
	}
	settings := registry.Tenants[sess.TenantID]
	if in.Issuer != nil {
		settings.Issuer = in.Issuer
	}
	if in.BaseURL != nil {
		settings.BaseURL = in.BaseURL
	}
	if in.APIKey != nil {
		settings.APIKey = in.APIKey
	}
	registry.Tenants[sess.TenantID] = settings
	if s.persistAccess(w, c, registry) == nil {
		return
	}
	s.revokeTenantAccess(sess.TenantID)
	// Subsequent administration must authenticate against the updated WiseMED API.
	s.admin.mu.Lock()
	for id, session := range s.admin.sessions {
		if session.TenantID == sess.TenantID {
			delete(s.admin.sessions, id)
		}
	}
	s.admin.mu.Unlock()
	log.Printf("admin tenant settings changed tenant=%s", sess.TenantID)
	writeJSON(w, 200, map[string]bool{"ok": true, "reauthenticate": true})
}
func (s *Server) adminOrigins(w http.ResponseWriter, r *http.Request) {
	c, sess, ok := s.requireAdmin(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Origin string `json:"origin"`
		Action string `json:"action"`
	}
	if !decodeAdmin(w, r, &in) {
		return
	}
	if !config.ValidOrigin(in.Origin) || (in.Action != "add" && in.Action != "delete") {
		adminError(w, 400, "action add/delete and exact HTTPS origin required")
		return
	}
	s.gate.Lock()
	defer s.gate.Unlock()
	if !s.adminMutationSession(w, r, c) {
		return
	}
	origins := slices.Clone(c.Tenants[sess.TenantID].AllowedOrigins)
	if in.Action == "add" {
		if !slices.Contains(origins, in.Origin) {
			origins = append(origins, in.Origin)
		}
	} else {
		origins = slices.DeleteFunc(origins, func(value string) bool { return value == in.Origin })
	}
	registry, err := config.ReadRegistry(c.Security.DeviceKeysFile)
	if err != nil {
		adminError(w, 500, "cannot read access registry")
		return
	}
	registry.Origins[sess.TenantID] = origins
	next := s.persistAccess(w, c, registry)
	if next == nil {
		return
	}
	if in.Action == "delete" {
		s.revokeTenantAccess(sess.TenantID)
	}
	log.Printf("admin origin changed tenant=%s action=%s", sess.TenantID, in.Action)
	writeJSON(w, 200, map[string]interface{}{"allowed_origins": next.Tenants[sess.TenantID].AllowedOrigins})
}

// A browser key can sign different subjects. Reauthenticate the tenant after
// revocation, including pending sockets and delegated tickets.
func (s *Server) revokeTenantAccess(tenant string) {
	s.authEpoch++
	s.ticketMu.Lock()
	for id, ticket := range s.tickets {
		if ticket.claims.TenantID == tenant {
			delete(s.tickets, id)
		}
	}
	s.ticketMu.Unlock()
	s.activeMu.Lock()
	for conn := range s.active {
		if conn.TenantID == tenant {
			conn.Close()
		}
	}
	s.activeMu.Unlock()
}
