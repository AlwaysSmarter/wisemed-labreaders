package htmlprintformatter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gopkg.in/yaml.v3"
	"wisemed-labreaders/readersv3/core/module"
)

func testAuthSettings(s Settings) Settings {
	s.JWTKeys = map[string]string{"one": strings.Repeat("a", 32), "two": strings.Repeat("b", 32)}
	s.JWTSigningKeyID = "one"
	return s
}
func testBearer(t *testing.T, s Settings) string {
	t.Helper()
	token, err := (&Module{settings: s}).signToken("test-client")
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + token
}

type sessionRuntime struct {
	module.Runtime
	authenticated bool
}

func (r sessionRuntime) Service(name string) (interface{}, bool) {
	return r, name == "local-http-control"
}
func (r sessionRuntime) SessionIdentity(*http.Request) (string, bool, bool) {
	return "test-user", false, r.authenticated
}

func TestJWTAuthorization(t *testing.T) {
	s := testAuthSettings(Settings{MaxBodyBytes: 4096, DefaultTimeoutMS: 1000})
	m := &Module{settings: s}
	for _, path := range []string{"/render", "/render/html", "/render/url", "/pdf", "/api/html-print-formatter/render", "/api/html-print-formatter/pdf"} {
		for _, id := range []string{"", "one", "two"} {
			req := httptest.NewRequest("POST", path, strings.NewReader(`{"html":"x"}`))
			req.Header.Set("Content-Type", "application/json")
			want := 401
			if id != "" {
				s.JWTSigningKeyID = id
				req.Header.Set("Authorization", testBearer(t, s))
				want = 503
				if path == "/render/url" {
					want = 400
				}
			}
			w := httptest.NewRecorder()
			m.handleRender(w, req)
			if w.Code != want {
				t.Fatalf("%s key=%s: %d %s", path, id, w.Code, w.Body)
			}
		}
	}
	for _, variant := range []string{"expired", "future", "wrong-secret", "unknown-kid", "missing-kid", "missing-exp", "missing-iat", "missing-sub", "wrong-issuer", "wrong-audience", "HS512", "none"} {
		t.Run(variant, func(t *testing.T) {
			now := time.Now()
			claims := jwt.RegisteredClaims{Issuer: jwtAudience, Audience: jwt.ClaimStrings{jwtAudience}, Subject: "client", IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))}
			method := jwt.SigningMethod(jwt.SigningMethodHS256)
			key := interface{}([]byte(s.JWTKeys["one"]))
			kid := "one"
			switch variant {
			case "expired":
				claims.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Second))
			case "future":
				claims.NotBefore = jwt.NewNumericDate(now.Add(time.Minute))
			case "wrong-secret":
				key = []byte(strings.Repeat("z", 32))
			case "unknown-kid":
				kid = "other"
			case "missing-kid":
				kid = ""
			case "missing-exp":
				claims.ExpiresAt = nil
			case "missing-iat":
				claims.IssuedAt = nil
			case "missing-sub":
				claims.Subject = ""
			case "wrong-issuer":
				claims.Issuer = "other"
			case "wrong-audience":
				claims.Audience = jwt.ClaimStrings{"other"}
			case "HS512":
				method = jwt.SigningMethodHS512
			case "none":
				method = jwt.SigningMethodNone
				key = jwt.UnsafeAllowNoneSignatureType
			}
			token := jwt.NewWithClaims(method, claims)
			token.Header["kid"] = kid
			signed, err := token.SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("POST", "/pdf", nil)
			req.Header.Set("Authorization", "Bearer "+signed)
			requireCode(t, m.authorize(req), "unauthorized")
		})
	}
	req := httptest.NewRequest("POST", "/pdf", nil)
	req.Header.Set("Authorization", testBearer(t, s))
	delete(m.settings.JWTKeys, "two")
	requireCode(t, m.authorize(req), "unauthorized") // removing key revokes its tokens
	requireCode(t, (&Module{}).authorize(req), "auth_not_configured")
}

func TestJWTConfigAndBrowserToken(t *testing.T) {
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte("jwt_keys:\n  one: "+strings.Repeat("a", 32)+"\n  two: "+strings.Repeat("b", 32)+"\njwt_signing_key_id: one\n"), &raw); err != nil {
		t.Fatal(err)
	}
	s, err := ReadSettings(raw)
	if err != nil || len(s.JWTKeys) != 2 {
		t.Fatalf("keys not loaded: %v", err)
	}
	for _, bad := range []map[string]interface{}{
		{"jwt_keys": map[string]string{"one": "short"}}, {"jwt_keys": []string{"bad"}}, {"jwt_keys": map[string]string{"": "12345678901234567890123456789012"}}, {"jwt_signing_key_id": "missing"},
	} {
		if _, err := ReadSettings(bad); err == nil {
			t.Fatal("bad JWT config accepted")
		}
	}
	for _, authenticated := range []bool{false, true} {
		m := &Module{settings: s, rt: sessionRuntime{authenticated: authenticated}}
		req := httptest.NewRequest("POST", "http://localhost/api/html-print-formatter/token", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		m.handleToken(w, req)
		if !authenticated {
			if w.Code != 401 {
				t.Fatal(w.Code)
			}
			continue
		}
		if w.Code != 200 || strings.Contains(w.Body.String(), s.JWTKeys["one"]) {
			t.Fatal("invalid token response")
		}
		var result struct{ Token string }
		json.Unmarshal(w.Body.Bytes(), &result)
		req.Header.Set("Authorization", "Bearer "+result.Token)
		if err := m.authorize(req); err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", "https://evil.example")
		w = httptest.NewRecorder()
		m.handleToken(w, req)
		if w.Code != 403 {
			t.Fatal("cross-origin token issuance allowed")
		}
	}
}
