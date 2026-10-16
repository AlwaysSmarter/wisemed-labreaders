package server

import (
	"errors"
	"fmt"
	"log"
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

// authFailure carries only operator-safe diagnostics, never JWTs or signing secrets.
type authFailure struct {
	Code    string
	Message string
}

func (e *authFailure) Error() string { return e.Code + ": " + e.Message }
func authDenied(code, message string) error {
	return &authFailure{Code: "wsm_" + code, Message: message}
}
func writeAuthFailure(w http.ResponseWriter, err error) {
	failure := &authFailure{Code: "wsm_invalid_credentials", Message: "Autentificare WSM refuzată. Verifică cheia/tokenul sau tichetul de control; pentru chei de echipament folosește key <equipment-id> în consola WSM."}
	var detail *authFailure
	if errors.As(err, &detail) {
		failure = detail
	}
	log.Printf("WSS authentication rejected: %s", failure)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": failure.Code, "message": failure.Message})
}

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
					return nil, authDenied("ambiguous_credentials", "Mai multe JWT-uri în subprotocoale. Trimite un singur token.")
				}
				tokenString = strings.TrimPrefix(p, "wsm.jwt.")
			}
		}
	}
	if tokenString == "" && queryAllowed && cfg.Security.AllowQueryToken {
		tokenString = r.URL.Query().Get("token")
	}
	if tokenString == "" || len(tokenString) > 8192 {
		return nil, authDenied("invalid_credentials", "JWT absent, invalid, expirat sau semnat cu o cheie necunoscută/inactivă. Verifică data/ora, key_id și secretul; recopiază datele din key <equipment-id> în consola WSM. Pentru token_file/token_endpoint obține un JWT nou.")
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
	if err != nil && !errors.Is(err, jwt.ErrTokenSignatureInvalid) && !errors.Is(err, jwt.ErrTokenUnverifiable) {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, authDenied("token_expired", "JWT expirat. Reînnoiește token_file/token_endpoint; pentru device_key verifică data/ora sistemului și reconectează.")
		}
		if errors.Is(err, jwt.ErrTokenInvalidAudience) {
			return nil, authDenied("audience_mismatch", "Audiența JWT nu corespunde WSM. Copiază Audiență JWT din key <equipment-id> în bridge/emitentul JWT.")
		}
		if errors.Is(err, jwt.ErrTokenUsedBeforeIssued) || errors.Is(err, jwt.ErrTokenNotValidYet) {
			return nil, authDenied("token_not_yet_valid", "JWT emis în viitor sau încă nevalid. Sincronizează data/ora clientului și serverului; verifică iat/nbf.")
		}
	}
	if err != nil || token == nil || !token.Valid {
		return nil, authDenied("invalid_credentials", "JWT absent, invalid, expirat sau semnat cu o cheie necunoscută/inactivă. Verifică data/ora, key_id și secretul; recopiază datele din key <equipment-id> în consola WSM. Pentru token_file/token_endpoint obține un JWT nou.")
	}
	tenant := cfg.Tenants[selected.TenantID]
	if claims.TenantID != selected.TenantID {
		return nil, authDenied("tenant_mismatch", "tenant_id nu corespunde cheii. Copiază Tenant din key <equipment-id>; după redenumirea tenantului actualizează bridge-ul/emitentul JWT.")
	}
	if claims.Issuer != tenant.Issuer {
		return nil, authDenied("issuer_mismatch", "Emitentul JWT (issuer/iss) nu corespunde tenantului. Copiază Emitent JWT din key <equipment-id>.")
	}
	if !config.ValidID(claims.Subject) || !config.ValidID(claims.ClientID) || claims.IssuedAt == nil || claims.ExpiresAt == nil {
		return nil, authDenied("invalid_identity", "JWT necesită sub și client_id valide, iat și exp. Verifică identitatea din key <equipment-id> și emitentul JWT.")
	}
	if claims.ExpiresAt.Sub(claims.IssuedAt.Time) <= 0 || claims.ExpiresAt.Sub(claims.IssuedAt.Time) > time.Duration(cfg.Security.MaxTokenTTLSeconds)*time.Second {
		return nil, authDenied("invalid_token_lifetime", "Durata JWT trebuie să fie pozitivă și cel mult max_token_ttl_seconds din WSM. Corectează emitentul JWT.")
	}
	if !slices.Contains(selected.Roles, claims.Role) {
		return nil, authDenied("role_denied", "Rolul JWT nu este permis de cheie. SIUI bridge necesită service; cheile generate necesită WSM actualizat cu suport service. Pentru chei statice verifică security.keys.<key-id>.roles.")
	}
	if len(claims.Scopes) == 0 {
		return nil, authDenied("scopes_required", "JWT nu conține permisiuni. Citește rights <equipment-id> și configurează scopes în bridge/emitentul JWT.")
	}
	if selected.Subject != "" && claims.Subject != selected.Subject {
		return nil, authDenied("subject_mismatch", "Subiectul JWT (sub) nu corespunde cheii. Copiază Subiect JWT din key <equipment-id> în bridge/emitentul JWT.")
	}
	if selected.ReaderID != "" && claims.ReaderID != selected.ReaderID {
		return nil, authDenied("reader_id_mismatch", "reader_id nu corespunde restricției cheii. Verifică ID-ul intern al bridge-ului și key <equipment-id>; corectează restricția cu edit <equipment-id> <reader-id> dacă este necesar.")
	}
	if selected.EquipmentID != "" && claims.EquipmentID != selected.EquipmentID {
		return nil, authDenied("equipment_id_mismatch", "equipment_id nu corespunde cheii. Verifică ID-ul inițializat în WiseMED și folosește cheia acelui echipament: key <equipment-id>.")
	}
	if claims.Role == "reader" || claims.Role == "service" && claims.ReaderID != "" {
		if !config.ValidID(claims.EquipmentID) {
			return nil, authDenied("equipment_id_required", "JWT necesită equipment_id valid pentru reader/service de echipament. Verifică inițializarea WiseMED.")
		}
		if !config.ValidID(claims.ReaderID) {
			return nil, authDenied("reader_id_required", "JWT necesită reader_id valid pentru această identitate de echipament.")
		}
	} else if claims.ReaderID != "" {
		return nil, authDenied("reader_role_required", "reader_id este permis doar pentru rolurile reader sau service de echipament. Actualizează rolul JWT și WSM.")
	}
	for _, scope := range claims.Scopes {
		if !config.ValidScope(scope) {
			return nil, authDenied("invalid_scope", "JWT conține o permisiune invalidă. Verifică lista scopes și rights <equipment-id>.")
		}
		if !slices.Contains(selected.Scopes, scope) {
			return nil, authDenied("scope_denied", fmt.Sprintf("Permisiunea solicitată %q nu este acordată cheii. Verifică rights <equipment-id> în consola WSM; elimin-o din scopes în bridge sau acord-o prin toggle <număr> și save, apoi copiază lista salvată în bridge.", scope))
		}
	}
	return claims, nil
}
func validateHelloAgainstClaims(h HelloPayload, c *AuthClaims) error {
	if c == nil || !(h.ClientType == c.Role || c.Role == "service" && h.ClientType == "siui-bridge") || h.ClientID != c.ClientID || h.ReaderID != c.ReaderID || h.EquipmentID != "" && h.EquipmentID != c.EquipmentID || h.UserID != "" && h.UserID != c.Subject || h.Label != "" && h.Label != c.Label {
		return errors.New("hello must match token identity")
	}
	return nil
}
func allowedOrigin(r *http.Request, t config.Tenant) bool {
	origin := r.Header.Get("Origin")
	return origin == "" || slices.Contains(t.AllowedOrigins, origin)
}
