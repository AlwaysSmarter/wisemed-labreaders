package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// commandHTTP uses a short-lived hub connection so the existing routing,
// identity stamping, scopes and reply transport also serve HTTP callers.
func (s *Server) commandHTTP(w http.ResponseWriter, r *http.Request) {
	fail := func(status int, code, requestID string) {
		writeJSON(w, status, map[string]interface{}{"error": code, "request_id": requestID})
	}
	s.gate.RLock()
	cfg := s.cfg
	epoch := s.authEpoch
	claims, err := authenticate(cfg, r, false)
	s.gate.RUnlock()
	if err != nil {
		writeAuthFailure(w, err)
		return
	}
	if (claims.Role != "service" && claims.Role != "browser") || !allowedOrigin(r, cfg.Tenants[claims.TenantID]) {
		fail(403, "forbidden", "")
		return
	}
	var in struct {
		Command   string                 `json:"command"`
		Args      map[string]interface{} `json:"args"`
		TimeoutMS int                    `json:"timeout_ms"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, cfg.Server.MaxMessageBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&in)
	if err == nil {
		var trailing interface{}
		if next := decoder.Decode(&trailing); next != io.EOF {
			err = next
			if err == nil {
				err = errors.New("trailing JSON")
			}
		}
	}
	if err != nil {
		var oversized *http.MaxBytesError
		if errors.As(err, &oversized) {
			fail(413, "request_too_large", "")
		} else {
			fail(400, "invalid_json", "")
		}
		return
	}
	if in.TimeoutMS == 0 {
		in.TimeoutMS = 135000
	}
	if in.Command == "" || in.TimeoutMS < 1 || in.TimeoutMS > 300000 {
		fail(400, "invalid_command_or_timeout", "")
		return
	}
	if in.Args == nil {
		in.Args = map[string]interface{}{}
	}
	targetID := chi.URLParam(r, "id")
	c := s.hub.NewConnection(nil, claims.TenantID, cfg.Server.SendQueueSize)
	c.RemoteIP, _, _ = net.SplitHostPort(r.RemoteAddr)
	c.Subject, c.EquipmentID = claims.Subject, claims.EquipmentID
	m := Envelope{Type: "command", RequestID: c.ID, Target: &Target{Mode: "connection", ConnectionID: targetID}, Payload: map[string]interface{}{"command": in.Command, "args": in.Args}}
	if authorizeMessage(claims, m) != nil {
		fail(403, "forbidden", m.RequestID)
		return
	}
	deadline := time.Now().Add(time.Duration(in.TimeoutMS) * time.Millisecond)
	if claims.ExpiresAt.Time.Before(deadline) {
		deadline = claims.ExpiresAt.Time
	}
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	c.cancel = cancel
	defer cancel()
	// The ordinary HTTP write deadline is 10s; command waits are explicitly bounded.
	if err := http.NewResponseController(w).SetWriteDeadline(deadline.Add(5 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		fail(500, "deadline_setup_failed", m.RequestID)
		return
	}
	s.gate.RLock()
	_, admissionErr := authenticate(s.cfg, r, false)
	if s.authEpoch != epoch || ctx.Err() != nil || admissionErr != nil {
		s.gate.RUnlock()
		fail(401, "credentials_expired_or_reloaded", m.RequestID)
		return
	}
	s.activeMu.Lock()
	total := 0
	for _, n := range s.reserved {
		total += n
	}
	if s.stopping || total >= cfg.Server.MaxConnections || s.reserved[c.TenantID] >= cfg.Server.MaxConnectionsPerTenant {
		s.activeMu.Unlock()
		s.gate.RUnlock()
		fail(503, "connection_limit", m.RequestID)
		return
	}
	s.reserved[c.TenantID]++
	s.active[c] = true
	s.activeMu.Unlock()
	defer func() {
		c.Close()
		s.hub.Remove(c.ID)
		s.activeMu.Lock()
		delete(s.active, c)
		s.reserved[c.TenantID]--
		if s.reserved[c.TenantID] == 0 {
			delete(s.reserved, c.TenantID)
		}
		s.activeMu.Unlock()
	}()
	if !s.hub.Register(c, HelloPayload{ClientType: claims.Role, ClientID: claims.ClientID, ReaderID: claims.ReaderID, Label: claims.Label}) {
		s.gate.RUnlock()
		fail(503, "registration_failed", m.RequestID)
		return
	}
	// Freeze the selected peer so disconnects/reconnects never redirect a command.
	s.hub.mu.RLock()
	peer := s.hub.connections[targetID]
	if peer != nil && peer.TenantID != claims.TenantID {
		peer = nil
	}
	s.hub.mu.RUnlock()
	if peer == nil || peer == c {
		s.gate.RUnlock()
		fail(404, "client_unavailable", m.RequestID)
		return
	}
	s.dispatch(c, claims, m, cfg)
	s.gate.RUnlock()
	for {
		select {
		case reply := <-c.send:
			if reply.CorrelationID != m.RequestID {
				continue
			}
			if reply.Type == "command_ack" && reply.ConnectionID == "" && reply.Payload["recipients"] == 0 {
				fail(503, "client_unavailable", m.RequestID)
				return
			}
			if reply.Type != "reply" || reply.ConnectionID != targetID {
				continue
			}
			// Revocation/reload must not release data from an old lease.
			s.gate.RLock()
			valid := s.authEpoch == epoch && ctx.Err() == nil
			select {
			case <-c.closed:
				valid = false
			default:
			}
			if valid {
				writeJSON(w, 200, map[string]interface{}{"request_id": m.RequestID, "connection_id": targetID, "response": reply.Payload})
			}
			s.gate.RUnlock()
			if !valid {
				fail(503, "request_interrupted_execution_unknown", m.RequestID)
			}
			return
		case <-peer.closed:
			s.gate.RLock()
			reloaded := s.authEpoch != epoch
			s.gate.RUnlock()
			if reloaded {
				fail(503, "request_interrupted_execution_unknown", m.RequestID)
			} else {
				fail(502, "client_disconnected_execution_unknown", m.RequestID)
			}
			return
		case <-c.closed:
			fail(503, "request_interrupted_execution_unknown", m.RequestID)
			return
		case <-ctx.Done():
			if r.Context().Err() == nil {
				fail(504, "timeout_execution_unknown", m.RequestID)
			}
			return
		}
	}
}
