package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"wisemed-labreaders/serverlast/wsm-server/internal/config"
)

type Server struct {
	admin          adminState
	authEpoch      uint64
	ticketMu       sync.Mutex
	tickets        map[string]controlTicket
	gate           sync.RWMutex // Serializes reload against admission and message dispatch.
	cfg            *config.Config
	hub            *Hub
	activeMu       sync.Mutex
	active         map[*Connection]bool // Includes sockets awaiting hello.
	reserved       map[string]int
	stopping       bool
	upstreamClient *http.Client
}

func New(cfg *config.Config) *Server {
	return &Server{tickets: map[string]controlTicket{}, cfg: cfg, hub: NewHub(), active: map[*Connection]bool{}, reserved: map[string]int{}, upstreamClient: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (s *Server) current() *config.Config { s.gate.RLock(); defer s.gate.RUnlock(); return s.cfg }

// Reload atomically replaces validated credentials/origins/upstreams/certificate.
// All sockets are disconnected so revoked credentials cannot retain access.
func (s *Server) Reload(cfg *config.Config) error {
	s.gate.Lock()
	defer s.gate.Unlock()
	a, b := s.cfg.Server, cfg.Server
	a.TLS = config.TLS{}
	b.TLS = config.TLS{}
	if !reflect.DeepEqual(a, b) || (s.cfg.Certificate == nil) != (cfg.Certificate == nil) {
		return errors.New("server limits/listener/TLS mode changes require restart")
	}
	candidate := *cfg
	cfg = &candidate
	if err := cfg.RefreshDevices(); err != nil {
		return err
	}
	s.admin.mu.Lock()
	s.admin.epoch++
	s.admin.sessions = nil
	s.admin.mu.Unlock()
	s.authEpoch++
	s.cfg = cfg
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	for c := range s.active {
		c.Close()
	}
	log.Printf("configuration reloaded; existing connections closed")
	return nil
}
func (s *Server) Close() {
	s.gate.Lock()
	defer s.gate.Unlock()
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	s.stopping = true
	for c := range s.active {
		c.Close()
	}
}
func (s *Server) Run(ctx context.Context) error {
	cfg := s.current()
	srv := &http.Server{Addr: net.JoinHostPort(cfg.Server.Address, fmt.Sprint(cfg.Server.Port)), Handler: s.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return s.current().Certificate, nil }}
	return s.serve(ctx, srv, nil)
}

// A supplied listener is used by integration tests with the same production TLS path.
func (s *Server) serve(ctx context.Context, srv *http.Server, listener net.Listener) error {
	done := make(chan struct{})
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-ctx.Done():
			s.Close()
			c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(c)
		case <-done:
		}
	}()
	log.Printf("wsm-server listening address=%s tls=%t", srv.Addr, s.current().Certificate != nil)
	var err error
	if listener != nil {
		if s.current().Certificate != nil {
			err = srv.ServeTLS(listener, "", "")
		} else {
			err = srv.Serve(listener)
		}
	} else if s.current().Certificate != nil {
		err = srv.ListenAndServeTLS("", "")
	} else {
		err = srv.ListenAndServe()
	}
	close(done)
	<-shutdownDone
	s.Close()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	// Never log query strings, headers or message payloads (JWT/medical data).
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "no-store")
			if r.TLS != nil {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	})
	s.adminRoutes(r)
	r.Group(func(adminUI chi.Router) {
		adminUI.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if s.adminConfig(w, r, false) != nil {
					next.ServeHTTP(w, r)
				}
			})
		})
		registerAdminUI(adminUI)
	})
	r.Get("/app.js", s.controlAsset)
	r.Get("/styles.css", s.controlAsset)
	r.Get("/wss-remote.js", s.controlAsset)
	r.Get("/control", s.controlAsset)
	r.Get("/control/*", s.controlAsset)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]interface{}{"status": "ok", "service": "wsm-server"})
	})
	r.Get("/api/connections", s.connections)
	r.Get("/api/debug/state", s.connections)
	r.Get("/api/equipment/{id}", s.equipmentHTTP)
	r.Get("/ws", s.handleWS)
	return r
}
func (s *Server) connections(w http.ResponseWriter, r *http.Request) {
	cfg := s.current()
	claims, err := authenticate(cfg, r, false)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	if !claims.Has("connections:read") || !allowedOrigin(r, cfg.Tenants[claims.TenantID]) {
		http.Error(w, "forbidden", 403)
		return
	}
	v := s.hub.Snapshot(claims.TenantID)
	writeJSON(w, 200, map[string]interface{}{"tenant_id": claims.TenantID, "connections": v, "count": len(v), "stats": s.hub.Stats(claims.TenantID)})
}
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	// Holding the admission gate prevents a token validated before reload from
	// opening a socket after revocation has closed the old generation.
	s.gate.RLock()
	cfg := s.cfg
	epoch := s.authEpoch
	claims, err := s.authenticateSocket(cfg, r)
	if err != nil {
		s.gate.RUnlock()
		http.Error(w, "unauthorized", 401)
		return
	}
	tenant := cfg.Tenants[claims.TenantID]
	if !socketOriginAllowed(r, tenant, claims) {
		s.gate.RUnlock()
		http.Error(w, "origin denied", 403)
		return
	}
	s.activeMu.Lock()
	total := 0
	for _, n := range s.reserved {
		total += n
	}
	if s.stopping || total >= cfg.Server.MaxConnections || s.reserved[claims.TenantID] >= cfg.Server.MaxConnectionsPerTenant {
		s.activeMu.Unlock()
		s.gate.RUnlock()
		http.Error(w, "connection limit", 503)
		return
	}
	s.reserved[claims.TenantID]++
	s.activeMu.Unlock()
	release := func() {
		s.activeMu.Lock()
		s.reserved[claims.TenantID]--
		if s.reserved[claims.TenantID] == 0 {
			delete(s.reserved, claims.TenantID)
		}
		s.activeMu.Unlock()
	}
	upgrader := websocket.Upgrader{HandshakeTimeout: 5 * time.Second, Subprotocols: []string{"wsm.v1"}, CheckOrigin: func(r *http.Request) bool { return socketOriginAllowed(r, tenant, claims) }}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		release()
		s.gate.RUnlock()
		return
	}
	conn := s.hub.NewConnection(ws, claims.TenantID, cfg.Server.SendQueueSize)
	conn.Subject = claims.Subject
	conn.EquipmentID = claims.EquipmentID
	conn.RemoteIP, _, _ = net.SplitHostPort(r.RemoteAddr)
	ctx, cancel := context.WithCancel(r.Context())
	conn.cancel = cancel
	s.activeMu.Lock()
	s.active[conn] = true
	s.activeMu.Unlock()
	s.gate.RUnlock()
	defer func() {
		conn.Close()
		s.hub.Remove(conn.ID)
		s.activeMu.Lock()
		delete(s.active, conn)
		s.activeMu.Unlock()
		release()
	}()
	// Authentication is a lease; even an idle connection closes at JWT expiry.
	expiry := time.AfterFunc(time.Until(claims.ExpiresAt.Time), conn.Close)
	defer expiry.Stop()
	writerDone := make(chan struct{})
	go func(writerConfig *config.Config) { defer close(writerDone); s.writePump(conn, writerConfig) }(cfg)
	defer func() { conn.Close(); <-writerDone }()
	ws.SetReadLimit(cfg.Server.MaxMessageBytes)
	helloDeadline := time.Now().Add(time.Duration(cfg.Server.HelloTimeoutMS) * time.Millisecond)
	_ = ws.SetReadDeadline(helloDeadline)
	registered := false
	ws.SetPongHandler(func(string) error {
		s.hub.Touch(conn.ID)
		if !registered {
			return ws.SetReadDeadline(helloDeadline)
		}
		return ws.SetReadDeadline(time.Now().Add(time.Duration(cfg.Server.ReadTimeoutMS) * time.Millisecond))
	})
	tokens := float64(cfg.Server.MessageBurst)
	last := time.Now()
	for {
		var msg Envelope
		if err := ws.ReadJSON(&msg); err != nil {
			if registered {
				s.hub.Remove(conn.ID)
				s.hub.Broadcast(conn.TenantID, presence(conn, "disconnected"))
			}
			return
		}
		now := time.Now()
		tokens = min(float64(cfg.Server.MessageBurst), tokens+now.Sub(last).Seconds()*float64(cfg.Server.MessagesPerSecond))
		last = now
		if tokens < 1 {
			policyClose(ws, "message rate exceeded")
			return
		}
		tokens--
		s.gate.RLock()
		if s.authEpoch != epoch || now.After(claims.ExpiresAt.Time) {
			s.gate.RUnlock()
			return
		}
		cfg = s.cfg // Device provisioning changes keys without invalidating unrelated leases.
		if !registered {
			if msg.Type != "hello" {
				s.gate.RUnlock()
				policyClose(ws, "hello required")
				return
			}
			hello, e := decodeHello(msg)
			if e != nil || validateHelloAgainstClaims(hello, claims) != nil {
				s.gate.RUnlock()
				policyClose(ws, "hello identity denied")
				return
			}
			hello.Label = claims.Label
			if !s.hub.Register(conn, hello) {
				s.gate.RUnlock()
				policyClose(ws, "reader identity already connected")
				return
			}
			registered = true
			_ = ws.SetReadDeadline(now.Add(time.Duration(cfg.Server.ReadTimeoutMS) * time.Millisecond))
			s.hub.send(conn, Envelope{Type: "hello_ack", Payload: map[string]interface{}{"connection_id": conn.ID, "tenant_id": conn.TenantID, "equipment_id": conn.EquipmentID, "client_type": conn.ClientType, "client_id": conn.ClientID, "reader_id": conn.ReaderID, "subject": conn.Subject, "expires_at": claims.ExpiresAt.Time}})
			s.hub.Broadcast(conn.TenantID, presence(conn, "connected"))
			s.gate.RUnlock()
			log.Printf("ws connected tenant=%s connection=%s role=%s", conn.TenantID, conn.ID, conn.ClientType)
			continue
		}
		s.hub.Touch(conn.ID)
		if msg.Type == "hello" {
			s.gate.RUnlock()
			policyClose(ws, "hello already completed")
			return
		}
		if err := authorizeMessage(claims, msg); err != nil {
			s.hub.send(conn, errorReply(msg, "forbidden", err.Error()))
			s.gate.RUnlock()
			continue
		}
		// Upstream requests use the authenticated tenant snapshot. They are cancellable
		// on disconnect/reload, and do not hold the reload gate while waiting on HTTP.
		if msg.Type == "command" && msg.Target != nil && msg.Target.Mode == "server" {
			s.gate.RUnlock()
			var payload map[string]interface{}
			var e error
			if asString(msg.Payload["command"]) == "control.ticket" {
				payload, e = s.issueControlTicket(cfg, claims, asString(mapPayload(msg.Payload["args"])["equipment_id"]))
			} else {
				payload, e = s.handleServerCommand(ctx, tenant, conn, msg)
			}
			if e != nil {
				s.hub.send(conn, errorReply(msg, "command_failed", e.Error()))
			} else {
				s.hub.send(conn, Envelope{Type: "reply", RequestID: msg.RequestID, CorrelationID: msg.RequestID, Payload: payload})
			}
			continue
		}
		s.dispatch(conn, claims, msg, cfg)
		s.gate.RUnlock()
	}
}
func presence(c *Connection, event string) Envelope {
	return Envelope{Type: "presence", Payload: map[string]interface{}{"event": event, "connection_id": c.ID, "client_type": c.ClientType, "client_id": c.ClientID, "reader_id": c.ReaderID, "equipment_id": c.EquipmentID}}
}
func policyClose(ws *websocket.Conn, reason string) {
	_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, reason), time.Now().Add(time.Second))
}
func errorReply(m Envelope, code, message string) Envelope {
	return Envelope{Type: "error", RequestID: m.RequestID, CorrelationID: m.RequestID, Payload: map[string]interface{}{"code": code, "message": message}}
}
func authorizeMessage(c *AuthClaims, m Envelope) error {
	denied := errors.New("operation not permitted or invalid message")
	if c.ControlEquipmentID != "" && m.Type != "ping" && !(m.Type == "command" && m.Target != nil && m.Target.Mode == "equipment" && m.Target.EquipmentID == c.ControlEquipmentID && asString(m.Payload["command"]) == "api.request") {
		return denied
	}
	if len(m.RequestID) > 128 || len(m.CorrelationID) > 128 {
		return denied
	}
	switch m.Type {
	case "ping":
		return nil
	case "list_connections":
		if c.Has("connections:read") {
			return nil
		}
	case "subscribe", "unsubscribe":
		topic, _ := m.Payload["topic"].(string)
		if c.Has("topics:subscribe") && config.ValidID(topic) {
			return nil
		}
	case "command", "reply", "event":
		if m.Broadcast || m.Target == nil {
			return errors.New("explicit target required; broadcast flag is unsupported")
		}
		t := m.Target
		if m.Type == "command" && m.RequestID == "" {
			return errors.New("command request_id required")
		}
		if m.Type == "reply" && m.CorrelationID == "" {
			return errors.New("reply correlation_id required")
		}
		if t.Mode == "server" {
			if m.Type != "command" {
				return denied
			}
			command := asString(m.Payload["command"])
			if command == "control.ticket" && c.Has("api:invoke") && c.Has("route:command") {
				return nil
			}
			if (command == "equipment.status" || command == "equipment.list") && c.Has("connections:read") {
				return nil
			}
			if command == "server.status" && c.Has("server:status") {
				return nil
			}
			if strings.HasPrefix(command, "wisemed.") && c.Has("wisemed:proxy") {
				return nil
			}
			return denied
		}
		if m.Type == "command" && asString(m.Payload["command"]) == "api.request" && !c.Has("api:invoke") {
			return denied
		}
		if !c.Has("route:" + m.Type) {
			return denied
		}
		if c.Role == "reader" {
			if m.Type == "command" && asString(m.Payload["command"]) == "api.request" && c.Has("api:invoke") && (t.Mode == "connection" || t.Mode == "equipment" || t.Mode == "reader") {
				id := t.ConnectionID
				if t.Mode == "equipment" {
					id = t.EquipmentID
				}
				if t.Mode == "reader" {
					id = t.ReaderID
				}
				if config.ValidID(id) {
					return nil
				}
			}
			if m.Type == "command" && c.Has("devices:debug") && (asString(m.Payload["command"]) == "device.ping" || asString(m.Payload["command"]) == "ws.reconnect" || asString(m.Payload["command"]) == "debug.message") && (t.Mode == "connection" || t.Mode == "equipment" || t.Mode == "reader") {
				id := t.ConnectionID
				if t.Mode == "equipment" {
					id = t.EquipmentID
				}
				if t.Mode == "reader" {
					id = t.ReaderID
				}
				if config.ValidID(id) {
					return nil
				}
			}
			// Readers respond directly and publish only their own telemetry topic.
			if m.Type == "command" {
				return denied
			}
			if m.Type == "reply" && t.Mode != "connection" {
				return denied
			}
			if m.Type == "event" && (t.Mode != "topic" || (t.Topic != "results:"+c.ReaderID && t.Topic != "logs:"+c.ReaderID)) {
				return denied
			}
		}
		switch t.Mode {
		case "connection":
			if config.ValidID(t.ConnectionID) {
				return nil
			}
		case "equipment":
			if config.ValidID(t.EquipmentID) {
				return nil
			}
		case "reader":
			if config.ValidID(t.ReaderID) {
				return nil
			}
		case "self":
			return nil
		case "connections", "readers", "equipments":
			ids := t.ConnectionIDs
			if t.Mode == "equipments" {
				ids = t.EquipmentIDs
			}
			if t.Mode == "readers" {
				ids = t.ReaderIDs
			}
			if len(ids) == 0 || len(ids) > 100 {
				return denied
			}
			for _, id := range ids {
				if !config.ValidID(id) {
					return denied
				}
			}
			if c.Has("route:broadcast") {
				return nil
			}
		case "all":
			if c.Has("route:broadcast") {
				return nil
			}
		case "client_type":
			if c.Has("route:broadcast") && (t.ClientType == "reader" || t.ClientType == "browser" || t.ClientType == "service") {
				return nil
			}
		case "topic":
			if config.ValidID(t.Topic) && (c.Role == "reader" || c.Has("route:broadcast")) {
				return nil
			}
		}
	}
	return denied
}
func (s *Server) dispatch(c *Connection, claims *AuthClaims, m Envelope, cfg *config.Config) {
	response := Envelope{RequestID: m.RequestID, CorrelationID: m.RequestID}
	switch m.Type {
	case "ping":
		response.Type = "pong"
		response.Payload = map[string]interface{}{"server_time": time.Now().UTC()}
	case "list_connections":
		response.Type = "connections"
		response.Payload = map[string]interface{}{"connections": s.hub.Snapshot(c.TenantID)}
	case "subscribe", "unsubscribe":
		topic := m.Payload["topic"].(string)
		ok := false
		if m.Type == "subscribe" {
			ok = s.hub.Subscribe(c.ID, topic, cfg.Server.MaxTopicsPerConnection)
		} else {
			ok = s.hub.Unsubscribe(c.ID, topic)
		}
		response.Type = m.Type + "_ack"
		response.Payload = map[string]interface{}{"topic": topic, "ok": ok, m.Type + "d": ok}
	case "command", "reply", "event":
		deliver := m
		deliver.ConnectionID = c.ID
		deliver.Timestamp = time.Now().UTC()
		deliver.Payload = clonePayload(m.Payload)
		deliver.Payload["sender_connection_id"] = c.ID
		deliver.Payload["sender_client_type"] = c.ClientType
		deliver.Payload["sender_client_id"] = c.ClientID
		deliver.Payload["sender_reader_id"] = c.ReaderID
		deliver.Payload["sender_tenant_id"] = c.TenantID
		deliver.Payload["sender_subject"] = claims.Subject
		deliver.Payload["sender_scopes"] = claims.Scopes
		n := s.hub.Route(deliver, c)
		response.Type = "command_ack"
		response.Payload = map[string]interface{}{"routed_type": m.Type, "recipients": n, "target": m.Target, "delivery": "queued"}
	}
	s.hub.send(c, response)
}
func (s *Server) writePump(c *Connection, cfg *config.Config) {
	defer c.Close()
	ticker := time.NewTicker(time.Duration(cfg.Server.PingIntervalMS) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.closed:
			return
		case msg := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(time.Duration(cfg.Server.WriteTimeoutMS) * time.Millisecond))
			if c.conn.WriteJSON(msg) != nil {
				return
			}
		case <-ticker.C:
			if c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Duration(cfg.Server.WriteTimeoutMS)*time.Millisecond)) != nil {
				return
			}
		}
	}
}
func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
func clonePayload(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func decodeHello(m Envelope) (HelloPayload, error) {
	var h HelloPayload
	b, e := json.Marshal(m.Payload)
	if e != nil {
		return h, e
	}
	e = json.Unmarshal(b, &h)
	return h, e
}
