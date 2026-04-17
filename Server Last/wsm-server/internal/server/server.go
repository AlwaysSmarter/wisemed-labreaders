package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"

	"wisemed-labreaders/serverlast/wsm-server/internal/config"
)

type Server struct {
	cfg      *config.Config
	hub      *Hub
	upgrader websocket.Upgrader
}

func New(cfg *config.Config) *Server {
	return &Server{
		cfg: cfg,
		hub: NewHub(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
	}
}

func (s *Server) Run(ctx context.Context) error {
	httpServer := &http.Server{
		Addr:              s.cfg.Server.Address + ":" + itoa(s.cfg.Server.Port),
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	log.Printf("wsm-server listening on %s", httpServer.Addr)
	return httpServer.ListenAndServe()
}

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(s.httpLogger)
	r.Use(cors)
	r.Get("/", s.handleRoot)
	r.Get("/healthz", s.healthz)
	r.Get("/api/connections", s.listConnections)
	r.Get("/api/debug/state", s.debugState)
	r.Get("/api/test-token", s.handleTestToken)
	r.Get("/ws", s.handleWS)
	r.Handle("/test/*", s.withNoCache(http.StripPrefix("/test/", http.FileServer(http.Dir("web")))))
	return r
}

func (s *Server) httpLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lrw, r)
		log.Printf("http %s %s status=%d remote=%s duration=%s ua=%q", r.Method, r.URL.RequestURI(), lrw.status, r.RemoteAddr, time.Since(start).Round(time.Millisecond), r.UserAgent())
	})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/test/", http.StatusFound)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "ok",
		"service":  "wsm-server",
		"wisemed":  s.cfg.WiseMed.BaseURL,
		"now_utc":  time.Now().UTC(),
		"ws_route": "/ws",
	})
}

func (s *Server) listConnections(w http.ResponseWriter, _ *http.Request) {
	snapshot := s.hub.Snapshot()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"connections": snapshot,
		"count":       len(snapshot),
		"stats":       s.hub.Stats(),
	})
}

func (s *Server) debugState(w http.ResponseWriter, _ *http.Request) {
	snapshot := s.hub.Snapshot()
	readers := 0
	browsers := 0
	for _, conn := range snapshot {
		switch conn.ClientType {
		case "reader":
			readers++
		case "browser":
			browsers++
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"summary": map[string]interface{}{
			"total_connections":   len(snapshot),
			"reader_connections":  readers,
			"browser_connections": browsers,
		},
		"connections": snapshot,
		"hub_stats":   s.hub.Stats(),
	})
}

func (s *Server) handleTestToken(w http.ResponseWriter, r *http.Request) {
	subject := strings.TrimSpace(r.URL.Query().Get("subject"))
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	clientID := strings.TrimSpace(r.URL.Query().Get("client_id"))
	readerID := strings.TrimSpace(r.URL.Query().Get("reader_id"))
	label := strings.TrimSpace(r.URL.Query().Get("label"))
	if subject == "" {
		log.Printf("test-token denied: missing subject remote=%s", r.RemoteAddr)
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "subject is required"})
		return
	}
	secret, ok := s.cfg.Security.AcceptedKeys[subject]
	if !ok || strings.TrimSpace(secret) == "" {
		log.Printf("test-token denied: subject=%s not configured remote=%s", subject, r.RemoteAddr)
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "subject is not configured in accepted_keys"})
		return
	}
	if role == "" {
		role = "browser"
	}
	log.Printf("test-token issued: subject=%s role=%s client_id=%s reader_id=%s label=%q remote=%s", subject, role, clientID, readerID, label, r.RemoteAddr)
	now := time.Now().UTC()
	claims := AuthClaims{
		Role:     role,
		ClientID: clientID,
		ReaderID: readerID,
		Label:    label,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-1 * time.Minute)),
			ExpiresAt: jwt.NewNumericDate(now.Add(2 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"token":   signed,
		"subject": subject,
		"role":    role,
	})
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	log.Printf("ws upgrade requested remote=%s path=%s", r.RemoteAddr, r.URL.RequestURI())
	claims, err := s.authenticateWS(r)
	if err != nil {
		log.Printf("ws auth denied remote=%s error=%v", r.RemoteAddr, err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	log.Printf("ws auth ok remote=%s subject=%s role=%s client_id=%s reader_id=%s label=%q", r.RemoteAddr, claims.Subject, claims.Role, claims.ClientID, claims.ReaderID, claims.Label)

	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade failed remote=%s error=%v", r.RemoteAddr, err)
		return
	}
	defer ws.Close()

	conn := s.hub.NewConnection(ws, r.RemoteAddr, s.cfg.Server.SendQueueSize)
	defer s.hub.Remove(conn.ID)
	go s.writePump(conn)

	ws.SetReadLimit(s.cfg.Server.MaxMessageBytes)
	_ = ws.SetReadDeadline(time.Now().Add(time.Duration(s.cfg.Server.ReadTimeoutMS) * time.Millisecond))
	ws.SetPongHandler(func(string) error {
		s.hub.Touch(conn.ID)
		return ws.SetReadDeadline(time.Now().Add(time.Duration(s.cfg.Server.ReadTimeoutMS) * time.Millisecond))
	})

	registered := false
	for {
		var msg Envelope
		if err := ws.ReadJSON(&msg); err != nil {
			log.Printf("ws read closed connection_id=%s remote=%s registered=%t error=%v", conn.ID, r.RemoteAddr, registered, err)
			if registered {
				s.hub.Broadcast(Envelope{
					Type: "presence",
					Payload: map[string]interface{}{
						"event":         "disconnected",
						"connection_id": conn.ID,
						"client_type":   conn.ClientType,
						"client_id":     conn.ClientID,
						"reader_id":     conn.ReaderID,
						"label":         conn.Label,
					},
				})
			}
			return
		}
		s.hub.Touch(conn.ID)

		switch msg.Type {
		case "hello":
			log.Printf("ws rx hello connection_id=%s payload=%s", conn.ID, mustJSON(msg.Payload))
			hello, err := decodeHello(msg)
			if err != nil || hello.ClientType == "" || hello.ClientID == "" {
				log.Printf("ws hello invalid connection_id=%s error=%v payload=%s", conn.ID, err, mustJSON(msg.Payload))
				s.hub.send(conn, Envelope{
					Type: "error",
					Payload: map[string]interface{}{
						"message": "invalid hello payload",
					},
				})
				continue
			}
			if err := validateHelloAgainstClaims(hello, claims); err != nil {
				log.Printf("ws hello denied connection_id=%s subject=%s role=%s error=%v", conn.ID, claims.Subject, claims.Role, err)
				s.hub.send(conn, Envelope{
					Type: "error",
					Payload: map[string]interface{}{
						"message": err.Error(),
					},
				})
				return
			}
			conn.Subject = claims.Subject
			conn.Role = claims.Role
			s.hub.Register(conn, hello)
			registered = true
			log.Printf("ws hello accepted connection_id=%s client_type=%s client_id=%s reader_id=%s subject=%s role=%s label=%q", conn.ID, conn.ClientType, conn.ClientID, conn.ReaderID, conn.Subject, conn.Role, conn.Label)
			s.hub.send(conn, Envelope{
				Type: "hello_ack",
				Payload: map[string]interface{}{
					"connection_id": conn.ID,
					"client_type":   conn.ClientType,
					"client_id":     conn.ClientID,
					"subject":       conn.Subject,
					"role":          conn.Role,
					"reader_id":     conn.ReaderID,
					"label":         conn.Label,
				},
			})
			s.hub.Broadcast(Envelope{
				Type: "presence",
				Payload: map[string]interface{}{
					"event":         "connected",
					"connection_id": conn.ID,
					"client_type":   conn.ClientType,
					"client_id":     conn.ClientID,
					"reader_id":     conn.ReaderID,
					"label":         conn.Label,
				},
			})
		case "ping":
			log.Printf("ws rx ping connection_id=%s request_id=%s", conn.ID, msg.RequestID)
			s.hub.send(conn, Envelope{
				Type:          "pong",
				RequestID:     msg.RequestID,
				CorrelationID: msg.RequestID,
				Payload: map[string]interface{}{
					"server_time": time.Now().UTC(),
				},
			})
		case "subscribe":
			topic, _ := msg.Payload["topic"].(string)
			ok := s.hub.Subscribe(conn.ID, topic)
			log.Printf("ws subscribe connection_id=%s topic=%s ok=%t", conn.ID, topic, ok)
			s.hub.send(conn, Envelope{
				Type:          "subscribe_ack",
				RequestID:     msg.RequestID,
				CorrelationID: msg.RequestID,
				Payload: map[string]interface{}{
					"topic":      topic,
					"subscribed": ok,
				},
			})
		case "unsubscribe":
			topic, _ := msg.Payload["topic"].(string)
			ok := s.hub.Unsubscribe(conn.ID, topic)
			log.Printf("ws unsubscribe connection_id=%s topic=%s ok=%t", conn.ID, topic, ok)
			s.hub.send(conn, Envelope{
				Type:          "unsubscribe_ack",
				RequestID:     msg.RequestID,
				CorrelationID: msg.RequestID,
				Payload: map[string]interface{}{
					"topic":        topic,
					"unsubscribed": ok,
				},
			})
		case "command", "reply", "event":
			if !registered {
				log.Printf("ws %s denied connection_id=%s reason=hello_required", msg.Type, conn.ID)
				s.hub.send(conn, Envelope{
					Type: "error",
					Payload: map[string]interface{}{
						"message": "send hello before other messages",
					},
				})
				continue
			}
			deliver := msg
			deliver.Payload = clonePayload(msg.Payload)
			deliver.Payload["sender_connection_id"] = conn.ID
			deliver.Payload["sender_client_type"] = conn.ClientType
			deliver.Payload["sender_client_id"] = conn.ClientID
			deliver.Payload["sender_reader_id"] = conn.ReaderID

			recipients := s.hub.Route(deliver, conn)
			log.Printf("ws route type=%s request_id=%s sender_connection_id=%s sender_client_type=%s sender_client_id=%s sender_reader_id=%s recipients=%d target=%s broadcast=%t payload=%s", msg.Type, msg.RequestID, conn.ID, conn.ClientType, conn.ClientID, conn.ReaderID, recipients, mustJSON(msg.Target), msg.Broadcast, mustJSON(msg.Payload))
			s.hub.send(conn, Envelope{
				Type:          "command_ack",
				RequestID:     msg.RequestID,
				CorrelationID: msg.RequestID,
				Payload: map[string]interface{}{
					"routed_type": msg.Type,
					"recipients":  recipients,
					"target":      msg.Target,
					"broadcast":   msg.Broadcast,
				},
			})
		case "list_connections":
			log.Printf("ws list_connections connection_id=%s", conn.ID)
			s.hub.send(conn, Envelope{
				Type:          "connections",
				RequestID:     msg.RequestID,
				CorrelationID: msg.RequestID,
				Payload: map[string]interface{}{
					"connections": s.hub.Snapshot(),
				},
			})
		default:
			log.Printf("ws unsupported type=%s connection_id=%s payload=%s", msg.Type, conn.ID, mustJSON(msg.Payload))
			s.hub.send(conn, Envelope{
				Type: "error",
				Payload: map[string]interface{}{
					"message": "unsupported message type",
					"type":    msg.Type,
				},
			})
		}
	}
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying response writer does not implement http.Hijacker")
	}
	return hijacker.Hijack()
}

func (w *loggingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *loggingResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

type AuthClaims struct {
	Role     string `json:"role"`
	ClientID string `json:"client_id"`
	ReaderID string `json:"reader_id"`
	Label    string `json:"label"`
	jwt.RegisteredClaims
}

func (s *Server) authenticateWS(r *http.Request) (*AuthClaims, error) {
	tokenString := strings.TrimSpace(r.URL.Query().Get("token"))
	if tokenString == "" {
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			tokenString = strings.TrimSpace(authHeader[7:])
		}
	}
	if tokenString == "" {
		return nil, errors.New("missing bearer token")
	}
	if len(s.cfg.Security.AcceptedKeys) == 0 {
		return nil, errors.New("server has no accepted keys configured")
	}

	var lastErr error
	for subject, secret := range s.cfg.Security.AcceptedKeys {
		claims := &AuthClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unsupported signing method")
			}
			return []byte(secret), nil
		})
		if err != nil {
			lastErr = err
			continue
		}
		if !token.Valid {
			lastErr = errors.New("invalid token")
			continue
		}
		if claims.Subject == "" {
			lastErr = errors.New("missing subject")
			continue
		}
		if claims.Subject != subject {
			lastErr = errors.New("token subject does not match accepted key entry")
			continue
		}
		log.Printf("ws token matched accepted key subject=%s role=%s client_id=%s reader_id=%s", claims.Subject, claims.Role, claims.ClientID, claims.ReaderID)
		return claims, nil
	}
	if lastErr == nil {
		lastErr = errors.New("token validation failed")
	}
	return nil, lastErr
}

func validateHelloAgainstClaims(hello HelloPayload, claims *AuthClaims) error {
	if claims == nil {
		return errors.New("missing auth claims")
	}
	if claims.Role == "" {
		return errors.New("missing role in token")
	}
	if claims.ClientID != "" && hello.ClientID != claims.ClientID {
		return errors.New("hello client_id does not match token")
	}
	if claims.Label != "" && hello.Label != "" && hello.Label != claims.Label {
		return errors.New("hello label does not match token")
	}
	switch claims.Role {
	case "reader":
		if hello.ClientType != "reader" {
			return errors.New("reader token can only open reader connections")
		}
		expectedReaderID := claims.ReaderID
		if expectedReaderID == "" {
			expectedReaderID = claims.Subject
		}
		if hello.ReaderID != expectedReaderID {
			return errors.New("hello reader_id does not match token")
		}
	default:
		if hello.ClientType == "reader" && claims.ReaderID != "" && hello.ReaderID != claims.ReaderID {
			return errors.New("hello reader_id does not match token")
		}
	}
	return nil
}

func (s *Server) writePump(conn *Connection) {
	pingTicker := time.NewTicker(time.Duration(s.cfg.Server.PingIntervalMS) * time.Millisecond)
	defer pingTicker.Stop()

	for {
		select {
		case msg, ok := <-conn.send:
			if !ok {
				return
			}
			if err := conn.conn.SetWriteDeadline(time.Now().Add(time.Duration(s.cfg.Server.WriteTimeoutMS) * time.Millisecond)); err != nil {
				return
			}
			if err := conn.conn.WriteJSON(msg); err != nil {
				return
			}
		case <-pingTicker.C:
			if err := conn.conn.SetWriteDeadline(time.Now().Add(time.Duration(s.cfg.Server.WriteTimeoutMS) * time.Millisecond)); err != nil {
				return
			}
			if err := conn.conn.WriteMessage(websocket.PingMessage, []byte("ping")); err != nil {
				return
			}
		case <-conn.closed:
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) withNoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

func clonePayload(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mustJSON(v interface{}) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return `{"marshal_error":true}`
	}
	return string(raw)
}

func decodeHello(msg Envelope) (HelloPayload, error) {
	var hello HelloPayload
	raw, err := json.Marshal(msg.Payload)
	if err != nil {
		return hello, err
	}
	err = json.Unmarshal(raw, &hello)
	return hello, err
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
