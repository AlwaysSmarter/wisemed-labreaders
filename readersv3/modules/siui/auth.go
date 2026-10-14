package siui

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

type apiClaims struct {
	Subject  string `json:"sub"`
	Issuer   string `json:"iss"`
	Audience string `json:"aud"`
	Expires  int64  `json:"exp"`
	Issued   int64  `json:"iat"`
	Admin    bool   `json:"admin"`
	Owner    string `json:"owner,omitempty"`
}
type apiIdentityKey struct{}

func (m *Module) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := apibridge.PrincipalFrom(r.Context()); ok {
			m.guard.RequireSession(next).ServeHTTP(w, r)
			return
		}
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			// The shared browser session remains cookie-based. JWT is scoped to SIUI routes.
			if !strings.HasPrefix(h, "Bearer ") || r.TLS == nil {
				fail(w, 401, "SIUI Bearer JWT requires HTTPS")
				return
			}
			claims, err := m.verifyJWT(strings.TrimPrefix(h, "Bearer "))
			if err != nil {
				fail(w, 401, "invalid or expired SIUI JWT")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), apiIdentityKey{}, claims)))
			return
		}
		m.guard.RequireSession(next).ServeHTTP(w, r)
	})
}
func (m *Module) signJWT(c apiClaims) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	b, _ := json.Marshal(c)
	v := header + "." + base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, m.jwtKey)
	mac.Write([]byte(v))
	return v + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (m *Module) verifyJWT(token string) (apiClaims, error) {
	var c apiClaims
	bad := errors.New("invalid JWT")
	p := strings.Split(token, ".")
	if len(p) != 3 || len(token) > 8192 {
		return c, bad
	}
	header, e := base64.RawURLEncoding.DecodeString(p[0])
	if e != nil {
		return c, bad
	}
	var h struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if json.Unmarshal(header, &h) != nil || h.Alg != "HS256" || h.Typ != "JWT" {
		return c, bad
	}
	mac := hmac.New(sha256.New, m.jwtKey)
	mac.Write([]byte(p[0] + "." + p[1]))
	sig, e := base64.RawURLEncoding.DecodeString(p[2])
	if e != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return c, bad
	}
	b, e := base64.RawURLEncoding.DecodeString(p[1])
	if e != nil || json.Unmarshal(b, &c) != nil {
		return c, bad
	}
	now := time.Now().Unix()
	if c.Subject == "" || c.Issuer != "siui-bridge" || c.Audience != "siui-api" || c.Expires <= now || c.Issued > now || c.Expires-c.Issued > 900 {
		return c, bad
	}
	return c, nil
}
func (m *Module) tokenHTTP(w http.ResponseWriter, r *http.Request, subject, owner string, admin bool) {
	if _, ok := r.Context().Value(apiIdentityKey{}).(apiClaims); ok {
		fail(w, 403, "use a browser login or WSM identity to issue a new JWT")
		return
	}
	if _, remote := apibridge.PrincipalFrom(r.Context()); !remote && r.TLS == nil {
		fail(w, 400, "JWT issuance requires HTTPS")
		return
	}
	var in struct{}
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, e.Error())
		return
	}
	now := time.Now().Unix()
	c := apiClaims{Subject: subject, Issuer: "siui-bridge", Audience: "siui-api", Expires: now + 900, Issued: now, Admin: admin, Owner: owner}
	respond(w, 200, map[string]any{"ok": true, "access_token": m.signJWT(c), "token_type": "Bearer", "expires_in": 900})
}
