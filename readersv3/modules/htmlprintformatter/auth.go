package htmlprintformatter

import (
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

const jwtAudience = "html-print-formatter"

// WSS identities are installed by the authenticated bridge, never by HTTP headers.
func (m *Module) authorize(req *http.Request) error {
	if _, ok := apibridge.PrincipalFrom(req.Context()); ok {
		return nil
	}
	if len(m.settings.JWTKeys) == 0 {
		return failure("auth_not_configured", "Configure modules.html-print-formatter.jwt_keys in config.yaml", 503)
	}
	headers := req.Header.Values("Authorization")
	if len(headers) != 1 || len(headers[0]) > 8192 {
		return unauthorized()
	}
	parts := strings.Fields(headers[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return unauthorized()
	}
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(parts[1], claims, func(token *jwt.Token) (interface{}, error) {
		id, ok := token.Header["kid"].(string)
		secret := m.settings.JWTKeys[id]
		if !ok || id == "" || secret == "" {
			return nil, errors.New("unknown key")
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithAudience(jwtAudience), jwt.WithIssuer(jwtAudience), jwt.WithIssuedAt())
	if err != nil || !token.Valid || strings.TrimSpace(claims.Subject) == "" || claims.IssuedAt == nil {
		return unauthorized()
	}
	return nil
}

func unauthorized() error {
	return failure("unauthorized", "A valid HS256 Bearer JWT with kid, iss, aud, sub, iat and exp is required", 401)
}

func (m *Module) signToken(subject string) (string, error) {
	id := m.settings.JWTSigningKeyID
	if id == "" || m.settings.JWTKeys[id] == "" {
		return "", failure("auth_not_configured", "Configure jwt_signing_key_id for browser access", 503)
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer: jwtAudience, Audience: jwt.ClaimStrings{jwtAudience}, Subject: subject,
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
	})
	token.Header["kid"] = id
	return token.SignedString([]byte(m.settings.JWTKeys[id]))
}

func (m *Module) handleToken(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, failure("method_not_allowed", "Use POST", 405), "")
		return
	}
	ct, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil || ct != "application/json" {
		writeError(w, failure("unsupported_media_type", "Use application/json", 415), "")
		return
	}
	if origin := req.Header.Get("Origin"); origin != "" {
		u, e := url.Parse(origin)
		if e != nil || u.Host != req.Host || (u.Scheme != "http" && u.Scheme != "https") {
			writeError(w, failure("forbidden", "Same-origin request required", 403), "")
			return
		}
	}
	var username string
	var authenticated bool
	if m.rt != nil {
		if svc, ok := m.rt.Service("local-http-control"); ok {
			if session, ok := svc.(interface {
				SessionIdentity(*http.Request) (string, bool, bool)
			}); ok {
				username, _, authenticated = session.SessionIdentity(req)
			}
		}
	}
	if !authenticated || strings.TrimSpace(username) == "" {
		writeError(w, failure("unauthorized", "Login required to obtain a browser token", 401), "")
		return
	}
	token, err := m.signToken(username)
	if err != nil {
		writeError(w, err, "")
		return
	}
	writeJSON(w, 200, map[string]interface{}{"ok": true, "token": token, "expires_in": 300})
}
