package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"wisemed-labreaders/serverlast/wsm-server/internal/config"
)

func TestHandshakeAuthDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		change     func(*AuthClaims)
	}{
		{"role", "wsm_role_denied", func(c *AuthClaims) { c.Role = "browser" }},
		{"scope", "wsm_scope_denied", func(c *AuthClaims) { c.Scopes = append(c.Scopes, "api:invoke") }},
		{"tenant", "wsm_tenant_mismatch", func(c *AuthClaims) { c.TenantID = "b" }},
		{"issuer", "wsm_issuer_mismatch", func(c *AuthClaims) { c.Issuer = "other" }},
		{"subject", "wsm_subject_mismatch", func(c *AuthClaims) { c.Subject = "other" }},
		{"reader", "wsm_reader_id_mismatch", func(c *AuthClaims) { c.ReaderID = "other" }},
		{"equipment", "wsm_equipment_id_mismatch", func(c *AuthClaims) { c.EquipmentID = "19" }},
		{"expiry", "wsm_token_expired", func(c *AuthClaims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute)) }},
		{"audience", "wsm_audience_mismatch", func(c *AuthClaims) { c.Audience = jwt.ClaimStrings{"other"} }},
		{"future", "wsm_token_not_yet_valid", func(c *AuthClaims) { c.IssuedAt = jwt.NewNumericDate(time.Now().Add(time.Minute)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := testConfig(t).WithDevices(map[string]config.Device{"device": {TenantID: "a", EquipmentID: "18", ReaderID: "siui-bridge", Subject: "device", Secret: strings.Repeat("private-secret", 4)}})
			if err != nil {
				t.Fatal(err)
			}
			c := claimsFor("a", "service", "siui-bridge")
			c.Subject = "device"
			c.ReaderID = "siui-bridge"
			c.EquipmentID = "18"
			c.Scopes = config.DeviceScopes()
			tc.change(&c)
			token := signed(t, cfg, c, "device")
			s := New(cfg)
			defer s.Close()
			w := httptest.NewRecorder()
			s.handleWS(w, authRequest(token))
			var body map[string]string
			if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != 401 || body["error"] != tc.code || body["message"] == "" {
				t.Fatalf("status=%d, body=%s", w.Code, w.Body)
			}
			if w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), token) || strings.Contains(w.Body.String(), cfg.Security.Keys["device"].Secret) {
				t.Fatal("credential exposure or cacheable error")
			}
			if tc.name == "scope" && (!strings.Contains(body["message"], "api:invoke") || !strings.Contains(body["message"], "rights")) {
				t.Fatal("missing actionable scope diagnostic")
			}
			// Forged tokens must not disclose verified-identity diagnostics.
			bad := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
			bad.Header["kid"] = "device"
			forged, err := bad.SignedString([]byte("wrong-secret"))
			if err != nil {
				t.Fatal(err)
			}
			w = httptest.NewRecorder()
			s.handleWS(w, authRequest(forged))
			json.Unmarshal(w.Body.Bytes(), &body)
			if body["error"] != "wsm_invalid_credentials" {
				t.Fatalf("forged token diagnostic: %s", w.Body)
			}
		})
	}
}
