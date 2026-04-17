package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

type ReaderSession struct {
	ReaderID       string
	AnalyzerCode   string
	AnalyzerName   string
	AnalyzerType   string
	LicenseCode    string
	Conn           *websocket.Conn
	ConnectedAt    time.Time
	LastHeartbeat  time.Time
	LastPong       time.Time
	SendMu         sync.Mutex
	BufferedResult []ResultOutboxItemWire
	pingWaiters    map[string]chan time.Time
	pingMu         sync.Mutex
}

type Server struct {
	cfg       *Config
	cfgMu     sync.RWMutex
	upgrader  websocket.Upgrader
	adminJWT  string
	mu        sync.RWMutex
	sessions  map[string]*ReaderSession
	waitersMu sync.Mutex
	waiters   map[string]chan CommandResultMessage
}

type SendCommandRequest struct {
	Command   string                 `json:"command"`
	Args      map[string]interface{} `json:"args"`
	TimeoutMS int                    `json:"timeout_ms"`
}

type ReaderSetupRequest struct {
	MedicalUnitID string   `json:"medical_unit_id"`
	DepartmentID  string   `json:"department_id"`
	DeviceLabel   string   `json:"device_label"`
	AllowedTags   []string `json:"allowed_tags"`
}

type WorklistResolveRequest struct {
	ReaderID  string   `json:"reader_id"`
	SampleID  string   `json:"sample_id"`
	PatientID string   `json:"patient_id"`
	Tags      []string `json:"tags"`
}

func New(cfg *Config) (*Server, error) {
	secret := cfg.Security.AdminJWTSecret
	if secret == "" && cfg.Security.AdminJWTSecretRef != "" {
		secret = os.Getenv(cfg.Security.AdminJWTSecretRef)
	}
	if secret == "" {
		return nil, fmt.Errorf("admin jwt secret missing: set security.admin_jwt_secret or env var %s", cfg.Security.AdminJWTSecretRef)
	}
	return &Server{
		cfg:      cfg,
		adminJWT: secret,
		upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		sessions: map[string]*ReaderSession{},
		waiters:  map[string]chan CommandResultMessage{},
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	r := chi.NewRouter()
	r.Use(corsMiddleware)
	r.Get("/healthz", s.healthz)
	r.Get("/ws/readers", s.readerWS)
	r.Get("/api/readers", s.adminAuth(s.listReaders))
	r.Get("/api/readers/{readerID}/status", s.readerOrAdminAuth(s.readerStatus))
	r.Get("/api/readers/{readerID}/logs", s.readerOrAdminAuth(s.readerLogs))
	r.Put("/api/readers/{readerID}/setup", s.adminAuth(s.readerSetup))
	r.Post("/api/readers/{readerID}/commands", s.adminAuth(s.sendCommand))
	r.Get("/api/readers/{readerID}/results", s.adminAuth(s.getBufferedResults))
	r.Post("/api/readers/{readerID}/ping", s.pingReader)
	r.Post("/api/readers/pingall", s.pingAll)
	r.Post("/api/worklist/resolve", s.readerOrAdminAuth(s.resolveWorklist))

	h := &http.Server{Addr: s.cfg.ListenAddr(), Handler: r}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.Shutdown(shutdownCtx)
	}()
	log.Printf("wisemedws listening on %s", s.cfg.ListenAddr())
	err := h.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok", "service": "wisemedws"})
}

func (s *Server) readerWS(w http.ResponseWriter, r *http.Request) {
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		h := r.Header.Get("Authorization")
		if len(h) >= 8 && h[:7] == "Bearer " {
			tokenStr = h[7:]
		}
	}
	readerIDFromToken, err := s.validateReaderToken(tokenStr)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var currentReaderID string
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if currentReaderID != "" {
				s.removeSession(currentReaderID)
			}
			return
		}
		var env Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue
		}

		switch env.Type {
		case MsgTypeReaderHello:
			var hello ReaderHelloMessage
			if err := json.Unmarshal(raw, &hello); err != nil {
				continue
			}
			if hello.ReaderID == "" || hello.ReaderID != readerIDFromToken {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "reader_id mismatch with token"})
				return
			}
			if hello.ReaderID == "" {
				continue
			}
			currentReaderID = hello.ReaderID
			sess := &ReaderSession{
				ReaderID:      hello.ReaderID,
				AnalyzerCode:  hello.AnalyzerCode,
				AnalyzerName:  hello.AnalyzerName,
				AnalyzerType:  hello.AnalyzerType,
				LicenseCode:   hello.LicenseCode,
				Conn:          conn,
				ConnectedAt:   time.Now().UTC(),
				LastHeartbeat: time.Now().UTC(),
				LastPong:      time.Now().UTC(),
				pingWaiters:   map[string]chan time.Time{},
			}
			conn.SetPongHandler(func(appData string) error {
				sess.pingMu.Lock()
				sess.LastPong = time.Now().UTC()
				if ch, ok := sess.pingWaiters[appData]; ok {
					select {
					case ch <- sess.LastPong:
					default:
					}
				}
				sess.pingMu.Unlock()
				return nil
			})
			s.upsertSession(sess)
			profile, registered := s.getReaderProfile(hello.ReaderID)
			state := RegistrationStateMessage{
				Type:          MsgTypeRegisterState,
				ReaderID:      hello.ReaderID,
				Registered:    registered,
				SetupComplete: profile.MedicalUnitID != "",
				CreatedAt:     time.Now().UTC(),
			}
			if registered {
				state.Profile = profileToMap(profile)
			}
			sess.SendMu.Lock()
			_ = sess.Conn.WriteJSON(state)
			sess.SendMu.Unlock()
		case MsgTypeHeartbeat:
			if currentReaderID != "" {
				s.touchHeartbeat(currentReaderID)
			}
		case MsgTypeCommandResult:
			var res CommandResultMessage
			if err := json.Unmarshal(raw, &res); err != nil {
				continue
			}
			s.waitersMu.Lock()
			ch, ok := s.waiters[res.CorrelationID]
			s.waitersMu.Unlock()
			if ok {
				select {
				case ch <- res:
				default:
				}
			}
		case MsgTypeResultBatch:
			var batch ResultBatchMessage
			if err := json.Unmarshal(raw, &batch); err != nil {
				continue
			}
			s.appendResults(batch.ReaderID, batch.Items)
			ackRefs := make([]string, 0, len(batch.Items))
			for _, it := range batch.Items {
				ackRefs = append(ackRefs, it.RefID)
			}
			_ = conn.WriteJSON(ResultBatchAckMessage{Type: MsgTypeResultAck, ReaderID: batch.ReaderID, AcceptedRefs: ackRefs})
		}
	}
}

func (s *Server) listReaders(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]map[string]interface{}, 0, len(s.sessions))
	for _, x := range s.sessions {
		profile, _ := s.getReaderProfile(x.ReaderID)
		res = append(res, map[string]interface{}{
			"reader_id":       x.ReaderID,
			"analyzer_code":   x.AnalyzerCode,
			"analyzer_name":   x.AnalyzerName,
			"analyzer_type":   x.AnalyzerType,
			"license_code":    x.LicenseCode,
			"connected_at":    x.ConnectedAt,
			"last_heartbeat":  x.LastHeartbeat,
			"last_pong":       x.LastPong,
			"pending_results": len(x.BufferedResult),
			"setup_complete":  profile.MedicalUnitID != "",
			"profile":         profileToMap(profile),
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"readers": res})
}

func (s *Server) readerSetup(w http.ResponseWriter, r *http.Request) {
	readerID := chi.URLParam(r, "readerID")
	var req ReaderSetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	req.MedicalUnitID = strings.TrimSpace(req.MedicalUnitID)
	req.DepartmentID = strings.TrimSpace(req.DepartmentID)
	req.DeviceLabel = strings.TrimSpace(req.DeviceLabel)
	if req.MedicalUnitID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "medical_unit_id is required"})
		return
	}

	s.cfgMu.Lock()
	if s.cfg.Readers.Profiles == nil {
		s.cfg.Readers.Profiles = map[string]ReaderProfile{}
	}
	prof := s.cfg.Readers.Profiles[readerID]
	prof.ReaderID = readerID
	prof.MedicalUnitID = req.MedicalUnitID
	prof.DepartmentID = req.DepartmentID
	prof.DeviceLabel = req.DeviceLabel
	prof.AllowedTags = sanitizeTags(req.AllowedTags)
	s.cfg.Readers.Profiles[readerID] = prof
	saveErr := s.cfg.Save()
	s.cfgMu.Unlock()
	if saveErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": saveErr.Error()})
		return
	}

	s.notifyRegistrationState(readerID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"reader_id":      readerID,
		"setup_complete": prof.MedicalUnitID != "",
		"profile":        profileToMap(prof),
	})
}

func (s *Server) readerStatus(w http.ResponseWriter, r *http.Request) {
	role, sub, _ := roleFromContext(r.Context())
	readerID := chi.URLParam(r, "readerID")
	if role == "reader" && sub != readerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "reader token can access only itself"})
		return
	}

	profile, registered := s.getReaderProfile(readerID)
	status := map[string]interface{}{
		"reader_id":      readerID,
		"registered":     registered,
		"setup_complete": profile.MedicalUnitID != "",
		"profile":        profileToMap(profile),
	}

	s.mu.RLock()
	sess, connected := s.sessions[readerID]
	s.mu.RUnlock()
	status["connected_to_wisemedws"] = connected
	if connected {
		status["connected_at"] = sess.ConnectedAt
		status["last_heartbeat"] = sess.LastHeartbeat
		status["last_pong"] = sess.LastPong
	}

	if connected {
		res, err := s.sendCommandToReader(readerID, "get_status", nil, time.Duration(s.cfg.Control.DefaultTimeoutMS)*time.Millisecond)
		if err == nil {
			status["analyzer_status"] = res.Data
			status["connected_to_analyzer"] = extractCommunicationState(res.Data)
		} else {
			status["analyzer_status_error"] = err.Error()
			status["connected_to_analyzer"] = false
		}
	} else {
		status["connected_to_analyzer"] = false
	}

	writeJSON(w, http.StatusOK, status)
}

func (s *Server) readerLogs(w http.ResponseWriter, r *http.Request) {
	role, sub, _ := roleFromContext(r.Context())
	readerID := chi.URLParam(r, "readerID")
	if role == "reader" && sub != readerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "reader token can access only itself"})
		return
	}
	limit := 200
	if q := r.URL.Query().Get("limit"); q != "" {
		if _, err := fmt.Sscanf(q, "%d", &limit); err != nil || limit <= 0 {
			limit = 200
		}
	}
	data := map[string]interface{}{"limit": limit}
	res, err := s.sendCommandToReader(readerID, "get_comm_logs", data, time.Duration(s.cfg.Control.DefaultTimeoutMS)*time.Millisecond)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"reader_id": readerID,
		"logs":      res.Data["logs"],
		"count":     res.Data["count"],
	})
}

func (s *Server) resolveWorklist(w http.ResponseWriter, r *http.Request) {
	role, sub, _ := roleFromContext(r.Context())
	var req WorklistResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	req.ReaderID = strings.TrimSpace(req.ReaderID)
	req.SampleID = strings.TrimSpace(req.SampleID)
	req.PatientID = strings.TrimSpace(req.PatientID)
	req.Tags = sanitizeTags(req.Tags)
	if req.ReaderID == "" {
		req.ReaderID = sub
	}
	if req.ReaderID == "" || req.SampleID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reader_id and sample_id are required"})
		return
	}
	if role == "reader" && sub != req.ReaderID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "reader token can resolve only itself"})
		return
	}
	approved := s.resolveTagsForReader(req.ReaderID, req.SampleID, req.Tags)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"reader_id":      req.ReaderID,
		"sample_id":      req.SampleID,
		"patient_id":     req.PatientID,
		"requested_tags": req.Tags,
		"approved_tags":  approved,
	})
}

func (s *Server) sendCommand(w http.ResponseWriter, r *http.Request) {
	readerID := chi.URLParam(r, "readerID")
	var req SendCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if req.Command == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command required"})
		return
	}
	timeout := time.Duration(s.cfg.Control.DefaultTimeoutMS) * time.Millisecond
	if req.TimeoutMS > 0 {
		timeout = time.Duration(req.TimeoutMS) * time.Millisecond
	}

	res, err := s.sendCommandToReader(readerID, req.Command, req.Args, timeout)
	if err != nil {
		if err.Error() == "reader not connected" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		if err.Error() == "timeout" {
			writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) getBufferedResults(w http.ResponseWriter, r *http.Request) {
	readerID := chi.URLParam(r, "readerID")
	s.mu.RLock()
	sess, ok := s.sessions[readerID]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "reader not connected"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"reader_id": readerID, "results": sess.BufferedResult})
}

func (s *Server) pingReader(w http.ResponseWriter, r *http.Request) {
	role, sub, ok := s.authAny(w, r)
	if !ok {
		return
	}
	readerID := chi.URLParam(r, "readerID")
	if role == "reader" && sub != readerID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "reader token can ping only itself"})
		return
	}
	timeoutMS := s.cfg.Control.DefaultTimeoutMS
	if q := r.URL.Query().Get("timeout_ms"); q != "" {
		var parsed int
		if _, err := fmt.Sscanf(q, "%d", &parsed); err == nil && parsed > 0 {
			timeoutMS = parsed
		}
	}
	timeout := time.Duration(timeoutMS) * time.Millisecond

	s.mu.RLock()
	sess, ok := s.sessions[readerID]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "reader not connected"})
		return
	}

	latency, ponged, errText := pingSession(sess, timeout)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"reader_id":   readerID,
		"ponged":      ponged,
		"latency_ms":  latency.Milliseconds(),
		"timeout_ms":  timeoutMS,
		"error":       errText,
		"last_pong":   sess.LastPong,
		"ping_target": "single",
	})
}

func (s *Server) pingAll(w http.ResponseWriter, r *http.Request) {
	role, sub, ok := s.authAny(w, r)
	if !ok {
		return
	}
	timeoutMS := s.cfg.Control.DefaultTimeoutMS
	if q := r.URL.Query().Get("timeout_ms"); q != "" {
		var parsed int
		if _, err := fmt.Sscanf(q, "%d", &parsed); err == nil && parsed > 0 {
			timeoutMS = parsed
		}
	}
	timeout := time.Duration(timeoutMS) * time.Millisecond

	s.mu.RLock()
	snapshot := make([]*ReaderSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		snapshot = append(snapshot, sess)
	}
	s.mu.RUnlock()

	results := make([]map[string]interface{}, 0, len(snapshot))
	for _, sess := range snapshot {
		latency, ponged, errText := pingSession(sess, timeout)
		results = append(results, map[string]interface{}{
			"reader_id":  sess.ReaderID,
			"ponged":     ponged,
			"latency_ms": latency.Milliseconds(),
			"error":      errText,
			"last_pong":  sess.LastPong,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ping_target": "all",
		"timeout_ms":  timeoutMS,
		"results":     results,
	})

	// Also broadcast pongall result to all connected WS readers.
	s.broadcastToAll(map[string]interface{}{
		"type":       "pongall",
		"initiator":  map[string]interface{}{"role": role, "sub": sub},
		"timeout_ms": timeoutMS,
		"results":    results,
		"created_at": time.Now().UTC(),
	})
}

type authCtxKey string

const (
	authRoleKey authCtxKey = "auth_role"
	authSubKey  authCtxKey = "auth_sub"
)

func roleFromContext(ctx context.Context) (string, string, bool) {
	role, rok := ctx.Value(authRoleKey).(string)
	sub, _ := ctx.Value(authSubKey).(string)
	return role, sub, rok
}

func (s *Server) readerOrAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role, sub, ok := s.authAny(w, r)
		if !ok {
			return
		}
		ctx := context.WithValue(r.Context(), authRoleKey, role)
		ctx = context.WithValue(ctx, authSubKey, sub)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) authAny(w http.ResponseWriter, r *http.Request) (role string, sub string, ok bool) {
	h := r.Header.Get("Authorization")
	if len(h) < 8 || h[:7] != "Bearer " {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
		return "", "", false
	}
	tokenStr := h[7:]

	claims := jwt.MapClaims{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.adminJWT), nil
	})
	if err == nil && tok != nil && tok.Valid {
		if role, _ := claims["role"].(string); role == "admin" {
			return "admin", "", true
		}
	}

	readerSub, err := s.validateReaderToken(tokenStr)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return "", "", false
	}
	return "reader", readerSub, true
}

func (s *Server) adminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if len(h) < 8 || h[:7] != "Bearer " {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
			return
		}
		tokenStr := h[7:]
		claims := jwt.MapClaims{}
		tok, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(s.adminJWT), nil
		})
		if err != nil || !tok.Valid {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}
		if role, _ := claims["role"].(string); role != "admin" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
			return
		}
		next(w, r)
	}
}

func (s *Server) validateReaderToken(tokenStr string) (string, error) {
	if tokenStr == "" {
		return "", errors.New("missing reader token")
	}
	claims := jwt.MapClaims{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		sub, _ := claims["sub"].(string)
		if sub == "" {
			return nil, errors.New("missing sub claim")
		}
		apiKey, ok := s.cfg.Readers.APIKeys[sub]
		if !ok || apiKey == "" {
			return nil, errors.New("unknown reader")
		}
		return []byte(apiKey), nil
	})
	if err != nil || !tok.Valid {
		return "", errors.New("invalid reader token")
	}
	role, _ := claims["role"].(string)
	if role != "reader" {
		return "", errors.New("invalid reader role")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", errors.New("missing sub claim")
	}
	return sub, nil
}

func (s *Server) upsertSession(x *ReaderSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.sessions[x.ReaderID]; ok {
		x.BufferedResult = old.BufferedResult
	}
	s.sessions[x.ReaderID] = x
}

func (s *Server) removeSession(readerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, readerID)
}

func (s *Server) touchHeartbeat(readerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x, ok := s.sessions[readerID]; ok {
		x.LastHeartbeat = time.Now().UTC()
	}
}

func (s *Server) appendResults(readerID string, items []ResultOutboxItemWire) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x, ok := s.sessions[readerID]; ok {
		x.BufferedResult = append(x.BufferedResult, items...)
	}
}

func (s *Server) sendCommandToReader(readerID, command string, args map[string]interface{}, timeout time.Duration) (CommandResultMessage, error) {
	s.mu.RLock()
	sess, ok := s.sessions[readerID]
	s.mu.RUnlock()
	if !ok {
		return CommandResultMessage{}, fmt.Errorf("reader not connected")
	}

	corr := fmt.Sprintf("corr-%d", time.Now().UnixNano())
	cmd := CommandMessage{
		Type:          MsgTypeCommand,
		CommandID:     fmt.Sprintf("cmd-%d", time.Now().UnixNano()),
		CorrelationID: corr,
		Command:       command,
		Args:          args,
		IssuedAt:      time.Now().UTC(),
	}

	ch := make(chan CommandResultMessage, 1)
	s.waitersMu.Lock()
	s.waiters[corr] = ch
	s.waitersMu.Unlock()
	defer func() {
		s.waitersMu.Lock()
		delete(s.waiters, corr)
		s.waitersMu.Unlock()
	}()

	sess.SendMu.Lock()
	err := sess.Conn.WriteJSON(cmd)
	sess.SendMu.Unlock()
	if err != nil {
		return CommandResultMessage{}, err
	}

	select {
	case res := <-ch:
		return res, nil
	case <-time.After(timeout):
		return CommandResultMessage{}, fmt.Errorf("timeout")
	}
}

func (s *Server) getReaderProfile(readerID string) (ReaderProfile, bool) {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	prof, ok := s.cfg.Readers.Profiles[readerID]
	if ok && prof.ReaderID == "" {
		prof.ReaderID = readerID
	}
	return prof, ok
}

func profileToMap(p ReaderProfile) map[string]interface{} {
	return map[string]interface{}{
		"reader_id":       p.ReaderID,
		"medical_unit_id": p.MedicalUnitID,
		"department_id":   p.DepartmentID,
		"device_label":    p.DeviceLabel,
		"allowed_tags":    p.AllowedTags,
	}
}

func sanitizeTags(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	for _, tag := range in {
		tag = strings.ToUpper(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if !slices.Contains(out, tag) {
			out = append(out, tag)
		}
	}
	return out
}

func (s *Server) resolveTagsForReader(readerID, sampleID string, requested []string) []string {
	requested = sanitizeTags(requested)
	if len(requested) == 0 {
		return []string{}
	}
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()

	allowed := requested
	if profile, ok := s.cfg.Readers.Profiles[readerID]; ok && len(profile.AllowedTags) > 0 {
		allowedSet := sanitizeTags(profile.AllowedTags)
		filtered := make([]string, 0, len(requested))
		for _, t := range requested {
			if slices.Contains(allowedSet, t) {
				filtered = append(filtered, t)
			}
		}
		allowed = filtered
	}

	if override, ok := s.cfg.Worklist.SampleTagOverrides[sampleID]; ok && len(override) > 0 {
		overrideSet := sanitizeTags(override)
		filtered := make([]string, 0, len(allowed))
		for _, t := range allowed {
			if slices.Contains(overrideSet, t) {
				filtered = append(filtered, t)
			}
		}
		allowed = filtered
	}
	return allowed
}

func (s *Server) notifyRegistrationState(readerID string) {
	s.mu.RLock()
	sess, ok := s.sessions[readerID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	profile, registered := s.getReaderProfile(readerID)
	msg := RegistrationStateMessage{
		Type:          MsgTypeRegisterState,
		ReaderID:      readerID,
		Registered:    registered,
		SetupComplete: profile.MedicalUnitID != "",
		CreatedAt:     time.Now().UTC(),
	}
	if registered {
		msg.Profile = profileToMap(profile)
	}
	sess.SendMu.Lock()
	_ = sess.Conn.WriteJSON(msg)
	sess.SendMu.Unlock()
}

func extractCommunicationState(data map[string]interface{}) bool {
	if data == nil {
		return false
	}
	started, _ := data["communication_started"].(bool)
	if !started {
		return false
	}
	runtime, _ := data["communication_runtime"].(map[string]interface{})
	if runtime == nil {
		return started
	}
	running, ok := runtime["running"].(bool)
	if !ok {
		return started
	}
	return running
}

func pingSession(sess *ReaderSession, timeout time.Duration) (time.Duration, bool, string) {
	nonce := fmt.Sprintf("p-%d", time.Now().UnixNano())
	waiter := make(chan time.Time, 1)
	sess.pingMu.Lock()
	sess.pingWaiters[nonce] = waiter
	sess.pingMu.Unlock()
	defer func() {
		sess.pingMu.Lock()
		delete(sess.pingWaiters, nonce)
		sess.pingMu.Unlock()
	}()

	start := time.Now()
	sess.SendMu.Lock()
	err := sess.Conn.WriteControl(websocket.PingMessage, []byte(nonce), time.Now().Add(timeout))
	sess.SendMu.Unlock()
	if err != nil {
		return 0, false, err.Error()
	}

	select {
	case <-waiter:
		return time.Since(start), true, ""
	case <-time.After(timeout):
		return time.Since(start), false, "pong timeout"
	}
}

func (s *Server) broadcastToAll(msg interface{}) {
	s.mu.RLock()
	snapshot := make([]*ReaderSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		snapshot = append(snapshot, sess)
	}
	s.mu.RUnlock()

	for _, sess := range snapshot {
		sess.SendMu.Lock()
		_ = sess.Conn.WriteJSON(msg)
		sess.SendMu.Unlock()
	}
}

func writeJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Reader-API-Key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
