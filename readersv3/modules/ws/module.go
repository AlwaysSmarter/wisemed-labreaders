package ws

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"wisemed-labreaders/readersv3/core/module"
)

type WSActionHandler interface {
	HandleWSAction(action string, payload map[string]interface{}) (map[string]interface{}, bool, error)
}

type WSRequester interface {
	Connected() bool
	Request(ctx context.Context, action string, payload map[string]interface{}) (map[string]interface{}, error)
}

type ActionDispatcher struct {
	mu       sync.RWMutex
	handlers []WSActionHandler
}

func (d *ActionDispatcher) Register(handler WSActionHandler) {
	if handler == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers = append(d.handlers, handler)
}

func (d *ActionDispatcher) Dispatch(action string, payload map[string]interface{}) (map[string]interface{}, bool, error) {
	d.mu.RLock()
	list := append([]WSActionHandler(nil), d.handlers...)
	d.mu.RUnlock()
	for _, handler := range list {
		resp, ok, err := handler.HandleWSAction(action, payload)
		if ok {
			return resp, true, err
		}
	}
	return nil, false, nil
}

type Envelope struct {
	Type          string                 `json:"type"`
	RequestID     string                 `json:"request_id,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	ConnectionID  string                 `json:"connection_id,omitempty"`
	Target        *Target                `json:"target,omitempty"`
	Broadcast     bool                   `json:"broadcast,omitempty"`
	Payload       map[string]interface{} `json:"payload,omitempty"`
	Timestamp     time.Time              `json:"timestamp,omitempty"`
}

type Target struct {
	Mode         string `json:"mode,omitempty"`
	ConnectionID string `json:"connection_id,omitempty"`
	ClientType   string `json:"client_type,omitempty"`
	ReaderID     string `json:"reader_id,omitempty"`
	Topic        string `json:"topic,omitempty"`
}

type helloPayload struct {
	ClientType string `json:"client_type"`
	ClientID   string `json:"client_id"`
	UserID     string `json:"user_id,omitempty"`
	ReaderID   string `json:"reader_id"`
	Label      string `json:"label"`
}

type Module struct {
	rt         module.Runtime
	dispatcher *ActionDispatcher

	mu        sync.RWMutex
	connected bool
	connID    string
	sendCh    chan Envelope

	pendingMu sync.Mutex
	pending   map[string]chan Envelope
	seq       uint64
}

type tokenClaims struct {
	Role     string `json:"role"`
	ClientID string `json:"client_id"`
	ReaderID string `json:"reader_id"`
	Label    string `json:"label"`
	Sub      string `json:"sub"`
	Iat      int64  `json:"iat"`
	Nbf      int64  `json:"nbf"`
	Exp      int64  `json:"exp"`
}

type wisemedAPISettings interface {
	Settings() map[string]string
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "wisemed-ws" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	m.dispatcher = &ActionDispatcher{}
	m.dispatcher.Register(m)
	m.pending = map[string]chan Envelope{}
	m.rt.RegisterService("ws-action-dispatcher", m.dispatcher)
	m.rt.RegisterService("wisemed-ws-status", m)
	m.rt.RegisterService("wisemed-ws-client", m)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	settings := m.rt.ModuleSettings(m.ID())
	if enabled, ok := settings["enabled"].(bool); ok && !enabled {
		<-ctx.Done()
		return nil
	}
	serverURL := strings.TrimSpace(asString(settings["url"]))
	if serverURL == "" {
		serverURL = "wss://wslocal.wisemed.eu/ws"
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err := m.runSession(ctx, serverURL); err != nil {
			m.setSession(nil, "", false)
			m.rt.Logf("wisemed-ws disconnected: %v", err)
		}
		delayMS := intFromSettings(settings, "reconnect_delay_ms", 5000)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Duration(delayMS) * time.Millisecond):
		}
	}
}

func (m *Module) Connected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

func (m *Module) Request(ctx context.Context, action string, payload map[string]interface{}) (map[string]interface{}, error) {
	if strings.TrimSpace(action) == "" {
		return nil, errors.New("action is required")
	}
	requestID := m.nextRequestID()
	replyCh := make(chan Envelope, 1)
	m.pendingMu.Lock()
	m.pending[requestID] = replyCh
	m.pendingMu.Unlock()
	defer func() {
		m.pendingMu.Lock()
		delete(m.pending, requestID)
		m.pendingMu.Unlock()
	}()

	msg := Envelope{
		Type:      "command",
		RequestID: requestID,
		Target:    &Target{Mode: "server"},
		Payload: map[string]interface{}{
			"command": action,
			"args":    cloneMap(payload),
		},
	}
	if err := m.send(msg); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-replyCh:
		if resp.Type == "error" {
			return nil, errors.New(strings.TrimSpace(asString(resp.Payload["message"])))
		}
		if resp.Payload == nil {
			return map[string]interface{}{}, nil
		}
		return cloneMap(resp.Payload), nil
	}
}

func (m *Module) HandleWSAction(action string, payload map[string]interface{}) (map[string]interface{}, bool, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "reader.status":
		return map[string]interface{}{
			"reader":         m.readerPayload(),
			"ws_connected":   m.Connected(),
			"connection_id":  m.connectionID(),
			"wisemed_online": m.wsReadyPayload(),
		}, true, nil
	case "ws.status", "wisemed.ws.status":
		return map[string]interface{}{
			"connected":     m.Connected(),
			"connection_id": m.connectionID(),
			"reader":        m.readerPayload(),
		}, true, nil
	default:
		return nil, false, nil
	}
}

func (m *Module) runSession(ctx context.Context, serverURL string) error {
	token, err := m.createJWT()
	if err != nil {
		m.setSession(nil, "", false)
		return err
	}
	dialURL, err := appendToken(serverURL, token)
	if err != nil {
		m.setSession(nil, "", false)
		return err
	}

	conn, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
	if err != nil {
		m.setSession(nil, "", false)
		return err
	}
	defer conn.Close()

	sendCh := make(chan Envelope, 32)
	defer close(sendCh)
	m.setSession(sendCh, "", true)
	defer m.setSession(nil, "", false)

	go m.writeLoop(ctx, conn, sendCh)

	if err := m.send(Envelope{
		Type:      "hello",
		RequestID: m.nextRequestID(),
		Payload: map[string]interface{}{
			"client_type": "reader",
			"client_id":   m.clientID(),
			"reader_id":   m.rt.ReaderID(),
			"label":       m.readerLabel(),
		},
	}); err != nil {
		return err
	}

	heartbeatMS := intFromSettings(m.rt.ModuleSettings(m.ID()), "heartbeat_ms", 15000)
	if heartbeatMS > 0 {
		ticker := time.NewTicker(time.Duration(heartbeatMS) * time.Millisecond)
		defer ticker.Stop()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = m.send(Envelope{Type: "ping", RequestID: m.nextRequestID()})
				}
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		var msg Envelope
		if err := conn.ReadJSON(&msg); err != nil {
			return err
		}
		if err := m.handleIncoming(msg); err != nil {
			m.rt.Logf("wisemed-ws incoming message error: %v", err)
		}
	}
}

func (m *Module) writeLoop(ctx context.Context, conn *websocket.Conn, sendCh <-chan Envelope) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-sendCh:
			if !ok {
				return
			}
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		}
	}
}

func (m *Module) handleIncoming(msg Envelope) error {
	switch msg.Type {
	case "hello_ack":
		m.setConnectionID(asString(msg.Payload["connection_id"]))
		m.rt.Logf("wisemed-ws hello accepted connection_id=%s", m.connectionID())
		return nil
	case "pong", "presence", "subscribe_ack", "unsubscribe_ack", "command_ack":
		return nil
	case "reply", "error":
		return m.resolvePending(msg)
	case "ping":
		return m.send(Envelope{
			Type:          "pong",
			RequestID:     m.nextRequestID(),
			CorrelationID: firstNonEmpty(msg.RequestID, msg.CorrelationID),
		})
	case "command":
		return m.handleCommand(msg)
	default:
		return nil
	}
}

func (m *Module) handleCommand(msg Envelope) error {
	if msg.Payload == nil {
		msg.Payload = map[string]interface{}{}
	}
	action := strings.TrimSpace(asString(msg.Payload["command"]))
	args := mapValue(msg.Payload["args"])
	if action == "" {
		return m.replyTo(msg, nil, errors.New("missing command"))
	}
	resp, handled, err := m.dispatcher.Dispatch(action, args)
	if !handled {
		err = errors.New("unknown action")
	}
	return m.replyTo(msg, resp, err)
}

func (m *Module) replyTo(msg Envelope, payload map[string]interface{}, err error) error {
	replyType := "reply"
	if err != nil {
		replyType = "error"
		payload = map[string]interface{}{"message": err.Error()}
	} else if payload == nil {
		payload = map[string]interface{}{}
	}

	target := &Target{Mode: "connection", ConnectionID: strings.TrimSpace(asString(msg.Payload["sender_connection_id"]))}
	if target.ConnectionID == "" {
		target = nil
	}

	return m.send(Envelope{
		Type:          replyType,
		RequestID:     firstNonEmpty(msg.RequestID, m.nextRequestID()),
		CorrelationID: firstNonEmpty(msg.RequestID, msg.CorrelationID),
		Target:        target,
		Payload:       payload,
	})
}

func (m *Module) resolvePending(msg Envelope) error {
	key := firstNonEmpty(msg.CorrelationID, msg.RequestID)
	if key == "" {
		return nil
	}
	m.pendingMu.Lock()
	ch, ok := m.pending[key]
	m.pendingMu.Unlock()
	if !ok {
		return nil
	}
	select {
	case ch <- msg:
	default:
	}
	return nil
}

func (m *Module) createJWT() (string, error) {
	subject := m.readerSubject()
	secret := m.authSecret()
	if subject == "" {
		return "", errors.New("wisemed-ws subject is missing")
	}
	if secret == "" {
		return "", errors.New("wisemed-ws secret is missing")
	}
	now := time.Now().UTC()
	headerJSON, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claimsJSON, _ := json.Marshal(tokenClaims{
		Role:     "reader",
		ClientID: m.clientID(),
		ReaderID: m.rt.ReaderID(),
		Label:    m.readerLabel(),
		Sub:      subject,
		Iat:      now.Unix(),
		Nbf:      now.Add(-1 * time.Minute).Unix(),
		Exp:      now.Add(2 * time.Hour).Unix(),
	})
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	claims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	unsigned := header + "." + claims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + signature, nil
}

func appendToken(rawURL, token string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("token", token)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (m *Module) send(msg Envelope) error {
	m.mu.RLock()
	sendCh := m.sendCh
	connected := m.connected
	m.mu.RUnlock()
	if !connected || sendCh == nil {
		return errors.New("wisemed-ws is not connected")
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	select {
	case sendCh <- msg:
		return nil
	default:
		return errors.New("wisemed-ws send queue is full")
	}
}

func (m *Module) setSession(sendCh chan Envelope, connID string, connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendCh = sendCh
	m.connID = connID
	m.connected = connected
}

func (m *Module) setConnectionID(connID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connID = strings.TrimSpace(connID)
}

func (m *Module) connectionID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connID
}

func (m *Module) nextRequestID() string {
	id := atomic.AddUint64(&m.seq, 1)
	return fmt.Sprintf("reader-%s-%d", sanitizeToken(m.rt.ReaderID()), id)
}

func (m *Module) clientID() string {
	settings := m.rt.ModuleSettings(m.ID())
	if value := strings.TrimSpace(asString(settings["client_id"])); value != "" {
		return value
	}
	return m.rt.ReaderID()
}

func (m *Module) readerLabel() string {
	if svc, ok := m.rt.Service("reader-config"); ok {
		if cfg, ok := svc.(map[string]interface{}); ok {
			if value := strings.TrimSpace(asString(cfg["label"])); value != "" {
				return value
			}
		}
	}
	return m.rt.ReaderID()
}

func (m *Module) readerSubject() string {
	settings := m.rt.ModuleSettings(m.ID())
	if value := strings.TrimSpace(asString(settings["subject"])); value != "" {
		return value
	}
	return m.rt.ReaderID()
}

func (m *Module) authSecret() string {
	settings := m.rt.ModuleSettings(m.ID())
	for _, key := range []string{"api_key", "secret", "token_secret"} {
		if value := strings.TrimSpace(asString(settings[key])); value != "" {
			return value
		}
	}
	if svc, ok := m.rt.Service("wisemed-api"); ok {
		if api, ok := svc.(wisemedAPISettings); ok {
			apiSettings := api.Settings()
			for _, key := range []string{"api_key_echipament", "cfg_wisemed_key"} {
				if value := strings.TrimSpace(apiSettings[key]); value != "" && value != "-" {
					return value
				}
			}
		}
	}
	return ""
}

func (m *Module) readerPayload() map[string]interface{} {
	reader := map[string]interface{}{
		"id":         m.rt.ReaderID(),
		"client_id":  m.clientID(),
		"label":      m.readerLabel(),
		"ws_subject": m.readerSubject(),
	}
	if svc, ok := m.rt.Service("reader-config"); ok {
		if cfg, ok := svc.(map[string]interface{}); ok {
			for _, key := range []string{"analyzer_name", "analyzer_code", "db_name"} {
				if value := cfg[key]; value != nil {
					reader[key] = value
				}
			}
		}
	}
	if svc, ok := m.rt.Service("analyzer-config"); ok {
		if cfg, ok := svc.(map[string]interface{}); ok {
			for _, key := range []string{"comm_type", "protocol"} {
				if value := cfg[key]; value != nil {
					reader[key] = value
				}
			}
		}
	}
	return reader
}

func (m *Module) wsReadyPayload() map[string]interface{} {
	return map[string]interface{}{
		"connected":     m.Connected(),
		"connection_id": m.connectionID(),
		"url":           strings.TrimSpace(asString(m.rt.ModuleSettings(m.ID())["url"])),
	}
}

func intFromSettings(settings map[string]interface{}, key string, def int) int {
	if settings == nil {
		return def
	}
	v, ok := settings[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	default:
		return def
	}
}

func asString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func mapValue(value interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	if item, ok := value.(map[string]interface{}); ok {
		return cloneMap(item)
	}
	return map[string]interface{}{}
}

func cloneMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return strings.TrimSpace(item)
		}
	}
	return ""
}

func sanitizeToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ".", "-", ":", "-", ";", "-", ",", "-")
	value = replacer.Replace(value)
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	value = strings.Trim(value, "-")
	if value == "" {
		return "reader"
	}
	return value
}
