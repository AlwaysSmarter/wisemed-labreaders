package server

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"wisemed-labreaders/serverlast/wsm-server/internal/config"
)

type controlTicket struct {
	claims     AuthClaims
	generation uint64
	expires    time.Time
}

func (s *Server) issueControlTicket(cfg *config.Config, parent *AuthClaims, equipment string) (map[string]interface{}, error) {
	if !parent.Has("api:invoke") || !parent.Has("route:command") {
		return nil, errors.New("api:invoke and route:command required")
	}
	if !config.ValidID(equipment) {
		return nil, errors.New("valid equipment_id required")
	}
	if parent.ControlEquipmentID != "" {
		return nil, errors.New("control sessions cannot delegate")
	}
	if s.equipmentStatus(parent.TenantID, equipment)["online"] != true {
		return nil, errors.New("equipment is offline")
	}
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return nil, e
	}
	ticket := base64.RawURLEncoding.EncodeToString(b)
	now := time.Now()
	expiry := now.Add(5 * time.Minute)
	if parent.ExpiresAt.Time.Before(expiry) {
		expiry = parent.ExpiresAt.Time
	}
	claims := *parent
	claims.Role = "browser"
	claims.ReaderID = ""
	claims.EquipmentID = ""
	claims.ClientID = "control"
	claims.ControlEquipmentID = equipment
	claims.ExpiresAt = jwt.NewNumericDate(expiry)
	claims.Scopes = []string{"api:invoke", "route:command"}
	if parent.Has("api:admin") {
		claims.Scopes = append(claims.Scopes, "api:admin")
	}
	s.ticketMu.Lock()
	defer s.ticketMu.Unlock()
	for key, t := range s.tickets {
		if now.After(t.expires) || t.generation != s.authEpoch {
			delete(s.tickets, key)
		}
	}
	if len(s.tickets) >= 1024 {
		return nil, errors.New("too many pending control tickets")
	}
	s.tickets[ticket] = controlTicket{claims: claims, generation: s.authEpoch, expires: now.Add(time.Minute)}
	return map[string]interface{}{"ticket": ticket, "equipment_id": equipment, "expires_at": expiry}, nil
}
func (s *Server) authenticateSocket(cfg *config.Config, r *http.Request) (*AuthClaims, error) {
	for _, p := range websocket.Subprotocols(r) {
		if strings.HasPrefix(p, "wsm.ticket.") {
			key := strings.TrimPrefix(p, "wsm.ticket.")
			s.ticketMu.Lock()
			t, ok := s.tickets[key]
			delete(s.tickets, key)
			s.ticketMu.Unlock()
			if !ok || t.generation != s.authEpoch || time.Now().After(t.expires) || time.Now().After(t.claims.ExpiresAt.Time) {
				return nil, errors.New("invalid control ticket")
			}
			return &t.claims, nil
		}
	}
	return authenticate(cfg, r, true)
}
func socketOriginAllowed(r *http.Request, t config.Tenant, c *AuthClaims) bool {
	if c.ControlEquipmentID != "" {
		return r.Header.Get("Origin") == "https://"+r.Host
	}
	return allowedOrigin(r, t)
}
func (s *Server) controlAsset(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/control" {
		destination := "/control/"
		if r.URL.RawQuery != "" {
			destination += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, destination, http.StatusTemporaryRedirect)
		return
	}
	cfg := s.current()
	root := cfg.Server.ControlUIDir
	if root == "" {
		http.Error(w, "control UI is not installed", 503)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/control/")
	if r.URL.Path == "/app.js" || r.URL.Path == "/styles.css" || r.URL.Path == "/wss-remote.js" {
		path = strings.TrimPrefix(r.URL.Path, "/")
	}
	if r.URL.Path == "/control" || path == "" {
		path = "index.html"
	}
	// Only bundled frontend files; no directory listing/config/secrets.
	if path != "app.js" && path != "styles.css" && path != "wss-remote.js" {
		path = "index.html"
	}
	file := filepath.Join(root, path)
	if _, e := os.Stat(file); e != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.ServeFile(w, r, file)
}
