package siuidata

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gopkg.in/yaml.v3"
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

var cuiPattern = regexp.MustCompile(`^[0-9]{2,13}$`)

func normalizeCUI(raw string) (string, error) {
	s := strings.TrimSpace(strings.ToUpper(raw))
	s = strings.TrimSpace(strings.TrimPrefix(s, "RO"))
	s = strings.TrimLeft(s, "0")
	if !cuiPattern.MatchString(s) {
		return "", errors.New("invalid CUI; expected 2..13 digits (optional RO prefix)")
	}
	return s, nil
}
func validateClientKeys(keys map[string]string) error {
	if len(keys) > 10000 {
		return errors.New("too many client JWT keys")
	}
	seen := map[string]bool{}
	for cui, key := range keys {
		canonical, e := normalizeCUI(cui)
		if e != nil || canonical != cui {
			return errors.New("client JWT keys require canonical numeric CUI names")
		}
		if len(key) < 32 || len(key) > 4096 {
			return errors.New("each client JWT secret must contain 32..4096 bytes")
		}
		if seen[key] {
			return errors.New("use a different JWT secret for each CUI")
		}
		seen[key] = true
	}
	return nil
}
func (m *Module) jwtVerificationKey(t *jwt.Token) (any, error) {
	c, ok := t.Claims.(*claims)
	if !ok {
		return nil, errors.New("invalid JWT claims")
	}
	raw, exists := t.Header["kid"]
	if !exists || raw == "global" {
		if c.CUI != "" {
			return nil, errors.New("global key cannot sign tenant claims")
		}
		return m.key, nil
	}
	kid, ok := raw.(string)
	if !ok || c.CUI != kid || c.Admin {
		return nil, errors.New("invalid CUI token scope")
	}
	cui, e := normalizeCUI(kid)
	if e != nil || cui != kid {
		return nil, errors.New("invalid key ID")
	}
	m.cfgMu.RLock()
	key := m.cfg.ClientJWTSecrets[cui]
	m.cfgMu.RUnlock()
	if key == "" {
		return nil, errors.New("unknown CUI key")
	}
	return []byte(key), nil
}

// Confidential UI routes require a real local administrator session. Neither
// a bearer token nor a WSM principal can be upgraded to this trust boundary.
func (m *Module) internalBrowser(r *http.Request) bool {
	if r.Header.Get("Authorization") != "" {
		return false
	}
	if _, ok := apibridge.PrincipalFrom(r.Context()); ok {
		return false
	}
	if _, ok := r.Context().Value(identityKey{}).(*claims); ok {
		return false
	}
	_, admin, valid := m.guard.SessionIdentity(r)
	return valid && admin
}
func (m *Module) issueTokenHTTP(w http.ResponseWriter, r *http.Request, subject string, admin bool) {
	if !m.internalBrowser(r) {
		failure(w, 403, "local WiseMED administrator browser session required")
		return
	}
	if r.TLS == nil {
		failure(w, 400, "HTTPS required")
		return
	}
	var in struct {
		CUI string `json:"cui"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if e := d.Decode(&in); e != nil && e != io.EOF {
		failure(w, 400, "invalid token request")
		return
	}
	now := time.Now()
	c := claims{RegisteredClaims: jwt.RegisteredClaims{Subject: subject, Issuer: "wisemed-siui-data", Audience: jwt.ClaimStrings{"siui-data-api"}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute))}, Admin: admin}
	key := m.key
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, &c)
	t.Header["kid"] = "global"
	if in.CUI != "" {
		cui, e := normalizeCUI(in.CUI)
		if e != nil {
			failure(w, 400, e.Error())
			return
		}
		m.cfgMu.RLock()
		secret := m.cfg.ClientJWTSecrets[cui]
		m.cfgMu.RUnlock()
		if secret == "" {
			failure(w, 404, "no key configured for this CUI")
			return
		}
		c.CUI, c.Admin = cui, false
		key = []byte(secret)
		t.Header["kid"] = cui
	}
	token, e := t.SignedString(key)
	if e != nil {
		failure(w, 500, "token generation failed")
		return
	}
	output(w, 200, map[string]any{"access_token": token, "token_type": "Bearer", "expires_in": 900, "cui": c.CUI})
}

func (m *Module) clientKeysHTTP(w http.ResponseWriter, r *http.Request) {
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()
	m.cfgMu.RLock()
	c := m.cfg
	m.cfgMu.RUnlock()
	if r.Method == "GET" {
		cuis := []string{}
		for cui := range c.ClientJWTSecrets {
			cuis = append(cuis, cui)
		}
		sort.Strings(cuis)
		output(w, 200, map[string]any{"cuis": cuis, "global_secret_configured": len(m.key) > 0})
		return
	}
	if r.Method != "PUT" {
		failure(w, 405, "GET or PUT required")
		return
	}
	var in struct {
		CUI    string `json:"cui"`
		Secret string `json:"secret"`
		Delete bool   `json:"delete"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if e := d.Decode(&in); e != nil {
		failure(w, 400, "invalid key request")
		return
	}
	cui, e := normalizeCUI(in.CUI)
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	keys := map[string]string{}
	for k, v := range c.ClientJWTSecrets {
		keys[k] = v
	}
	if in.Delete {
		delete(keys, cui)
	} else {
		if subtle.ConstantTimeCompare([]byte(in.Secret), m.key) == 1 {
			failure(w, 400, "client key must differ from the global key")
			return
		}
		keys[cui] = in.Secret
	}
	if e = validateClientKeys(keys); e != nil {
		failure(w, 400, e.Error())
		return
	}
	c.ClientJWTSecrets = keys
	cfg, e := config.Load(m.rt.ConfigPath())
	if e != nil {
		failure(w, 500, "cannot load configuration")
		return
	}
	cfg.ModuleSettings(m.ID())["client_jwt_secrets"] = keys
	cfg.ApplyDefaults()
	b, e := yaml.Marshal(cfg)
	if e == nil {
		e = writePrivateConfig(m.rt.ConfigPath(), b)
	}
	if e != nil {
		failure(w, 500, "cannot save client keys")
		return
	}
	m.cfgMu.Lock()
	m.cfg.ClientJWTSecrets = keys
	m.cfgMu.Unlock()
	output(w, 200, map[string]any{"cui": cui, "configured": !in.Delete})
}
