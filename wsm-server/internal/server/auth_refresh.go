package server

import (
	"errors"
	"net/http"
	"strings"
	"wisemed-labreaders/serverlast/wsm-server/internal/config"
)

func refreshClaims(cfg *config.Config, current *AuthClaims, msg Envelope) (*AuthClaims, error) {
	denied := errors.New("authentication renewal denied")
	if current.ControlEquipmentID != "" || msg.RequestID == "" || len(msg.RequestID) > 128 || msg.Target != nil || msg.Broadcast {
		return nil, denied
	}
	token := asString(msg.Payload["token"])
	if token == "" || len(token) > 8192 || strings.ContainsAny(token, "\r\n") {
		return nil, denied
	}
	req := &http.Request{Header: http.Header{"Authorization": []string{"Bearer " + token}}}
	next, err := authenticate(cfg, req, false)
	if err != nil {
		return nil, denied
	}
	if next.TenantID != current.TenantID || next.Subject != current.Subject || next.Role != current.Role || next.ClientID != current.ClientID || next.ReaderID != current.ReaderID || next.EquipmentID != current.EquipmentID || !next.ExpiresAt.After(current.ExpiresAt.Time) || len(next.Scopes) != len(current.Scopes) {
		return nil, denied
	}
	for _, scope := range current.Scopes {
		if !next.Has(scope) {
			return nil, denied
		}
	}
	return next, nil
}
