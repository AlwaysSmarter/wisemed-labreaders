package server

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"wisemed-labreaders/serverlast/wsm-server/internal/config"
)

type AuthClaims struct {
	ControlEquipmentID string   `json:"-"`
	EquipmentID        string   `json:"equipment_id,omitempty"`
	TenantID           string   `json:"tenant_id"`
	Role               string   `json:"role"`
	ClientID           string   `json:"client_id"`
	ReaderID           string   `json:"reader_id,omitempty"`
	Label              string   `json:"label,omitempty"`
	Scopes             []string `json:"scopes"`
	jwt.RegisteredClaims
}

func (c *AuthClaims) Has(scope string) bool { return slices.Contains(c.Scopes, scope) }
func authenticate(cfg *config.Config, r *http.Request, queryAllowed bool) (*AuthClaims, error) {
	tokenString := ""
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		tokenString = strings.TrimSpace(auth[7:])
	}
	if tokenString == "" && queryAllowed {
		for _, p := range websocket.Subprotocols(r) {
			if strings.HasPrefix(p, "wsm.jwt.") {
				if tokenString != "" {
					return nil, errors.New("ambiguous credentials")
				}
				tokenString = strings.TrimPrefix(p, "wsm.jwt.")
			}
		}
	}
	if tokenString == "" && queryAllowed && cfg.Security.AllowQueryToken {
		tokenString = r.URL.Query().Get("token")
	}
	if tokenString == "" || len(tokenString) > 8192 {
		return nil, errors.New("invalid credentials")
	}
	claims := &AuthClaims{}
	var selected config.Key
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok {
			return nil, errors.New("kid required")
		}
		k, ok := cfg.Security.Keys[kid]
		tenant, found := cfg.Tenants[k.TenantID]
		if !ok || !found || k.Disabled || tenant.Disabled || k.Secret == "" {
			return nil, errors.New("inactive key")
		}
		selected = k
		return []byte(k.Secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithAudience(cfg.Security.Audience))
	if err != nil || !token.Valid {
		return nil, errors.New("invalid credentials")
	}
	tenant := cfg.Tenants[selected.TenantID]
	if claims.TenantID != selected.TenantID || claims.Issuer != tenant.Issuer || !config.ValidID(claims.Subject) || !config.ValidID(claims.ClientID) || claims.IssuedAt == nil || claims.ExpiresAt == nil {
		return nil, errors.New("invalid identity")
	}
	if claims.ExpiresAt.Sub(claims.IssuedAt.Time) <= 0 || claims.ExpiresAt.Sub(claims.IssuedAt.Time) > time.Duration(cfg.Security.MaxTokenTTLSeconds)*time.Second {
		return nil, errors.New("invalid token lifetime")
	}
	if !slices.Contains(selected.Roles, claims.Role) || len(claims.Scopes) == 0 {
		return nil, errors.New("role/scopes denied")
	}
	if selected.Subject != "" && claims.Subject != selected.Subject || selected.ReaderID != "" && claims.ReaderID != selected.ReaderID || selected.EquipmentID != "" && claims.EquipmentID != selected.EquipmentID {
		return nil, errors.New("device identity denied")
	}
	if claims.Role == "reader" {
		if !config.ValidID(claims.EquipmentID) {
			return nil, errors.New("equipment_id required")
		}
		if !config.ValidID(claims.ReaderID) {
			return nil, errors.New("reader_id required")
		}
	} else if claims.ReaderID != "" {
		return nil, errors.New("reader_id only valid for reader")
	}
	for _, scope := range claims.Scopes {
		if !config.ValidScope(scope) || !slices.Contains(selected.Scopes, scope) {
			return nil, errors.New("scope denied")
		}
	}
	return claims, nil
}
func validateHelloAgainstClaims(h HelloPayload, c *AuthClaims) error {
	if c == nil || h.ClientType != c.Role || h.ClientID != c.ClientID || h.ReaderID != c.ReaderID || h.EquipmentID != "" && h.EquipmentID != c.EquipmentID || h.UserID != "" && h.UserID != c.Subject || h.Label != "" && h.Label != c.Label {
		return errors.New("hello must match token identity")
	}
	return nil
}
func allowedOrigin(r *http.Request, t config.Tenant) bool {
	origin := r.Header.Get("Origin")
	return origin == "" || slices.Contains(t.AllowedOrigins, origin)
}
