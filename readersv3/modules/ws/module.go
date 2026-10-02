package ws

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/shared/apibridge"
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
	EquipmentID  string `json:"equipment_id,omitempty"`
	Mode         string `json:"mode,omitempty"`
	ConnectionID string `json:"connection_id,omitempty"`
	ClientType   string `json:"client_type,omitempty"`
	ReaderID     string `json:"reader_id,omitempty"`
	Topic        string `json:"topic,omitempty"`
}

type equipmentAPI interface {
	Settings() map[string]string
	EnsureEquipmentInitialized() (map[string]interface{}, error)
}
type tokenAPI interface {
	WSSAccessToken(context.Context, string, map[string]interface{}) (string, error)
}
type Trace struct {
	ID        uint64    `json:"id"`
	At        time.Time `json:"at"`
	Direction string    `json:"direction"`
	Message   Envelope  `json:"message"`
}
type Module struct {
	rt            module.Runtime
	dispatcher    *ActionDispatcher
	mu            sync.RWMutex
	settings      map[string]interface{}
	connected     bool
	phase         string
	connID        string
	equipmentID   string
	connectedAt   time.Time
	nextRetry     time.Time
	lastError     string
	paused        bool
	sendCh        chan Envelope
	sessionDone   <-chan struct{}
	sessionCancel context.CancelFunc
	wake          chan struct{}
	traces        []Trace
	traceSeq      uint64
	pendingMu     sync.Mutex
	pending       map[string]chan Envelope
	seq           uint64
	nonce         string
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "wisemed-ws" }
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	m.settings = cloneMap(rt.ModuleSettings(m.ID()))
	m.wake = make(chan struct{}, 1)
	m.pending = map[string]chan Envelope{}
	m.phase = "disconnected"
	b := make([]byte, 8)
	if _, e := rand.Read(b); e != nil {
		return e
	}
	m.nonce = base64.RawURLEncoding.EncodeToString(b)
	m.dispatcher = &ActionDispatcher{}
	m.dispatcher.Register(m)
	rt.RegisterService("ws-action-dispatcher", m.dispatcher)
	rt.RegisterService("wisemed-ws-status", m)
	rt.RegisterService("wisemed-ws-client", m)
	return nil
}
func (m *Module) settingsSnapshot() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneMap(m.settings)
}

// Settings returns only public configuration; the editable device key is write-only.
func (m *Module) Settings() map[string]interface{} {
	settings := m.settingsSnapshot()
	settings["device_secret_configured"] = strings.TrimSpace(asString(settings["device_secret"])) != ""
	delete(settings, "device_secret")
	return settings
}
func (m *Module) SaveSettings(next map[string]interface{}) error {
	allowed := map[string]bool{"enabled": true, "url": true, "auth_mode": true, "tenant_id": true, "key_id": true, "issuer": true, "audience": true, "subject": true, "client_id": true, "secret_file": true, "device_secret": true, "token_file": true, "token_path": true, "ca_file": true, "scopes": true}
	settings := m.settingsSnapshot()
	updates := map[string]interface{}{}
	for k, v := range next {
		if !allowed[k] {
			return fmt.Errorf("unsupported WSS setting: %s", k)
		}
		if k == "enabled" {
			if _, ok := v.(bool); !ok {
				return errors.New("enabled must be boolean")
			}
		} else if _, ok := v.(string); !ok {
			return errors.New("WSS settings must be strings")
		}
		if k == "device_secret" {
			secret := strings.TrimSpace(v.(string))
			// A blank password field preserves the stored key.
			if secret == "" {
				continue
			}
			if len(secret) < 32 {
				return errors.New("WSS device key must contain at least 32 bytes")
			}
			v = secret
		}
		settings[k] = v
		updates["modules.wisemed-ws."+k] = v
	}
	if u := strings.TrimSpace(asString(settings["url"])); u != "" {
		parsed, e := url.Parse(u)
		if e != nil || parsed.Scheme != "wss" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return errors.New("WSS URL must be wss:// without credentials, query or fragment")
		}
	}
	mode := firstNonEmpty(asString(settings["auth_mode"]), "device_key")
	if mode != "device_key" && mode != "token_file" && mode != "token_endpoint" {
		return errors.New("invalid WSS auth_mode")
	}
	for _, k := range []string{"enabled", "url"} {
		if v, ok := next[k]; ok {
			updates["wisemed_ws."+k] = v
		}
	}
	updates["wisemed_ws.reconnect_delay_ms"] = 30000
	updates["modules.wisemed-ws.reconnect_delay_ms"] = 30000
	if e := config.Update(m.rt.ConfigPath(), updates); e != nil {
		return e
	}
	m.mu.Lock()
	m.settings = settings
	m.mu.Unlock()
	m.Reconnect()
	return nil
}
func (m *Module) Connected() bool { m.mu.RLock(); defer m.mu.RUnlock(); return m.connected }
func (m *Module) Status() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	remaining := 0
	if !m.nextRetry.IsZero() {
		remaining = max(0, int(time.Until(m.nextRetry).Seconds()+0.999))
	}
	return map[string]interface{}{"connected": m.connected, "phase": m.phase, "connection_id": m.connID, "equipment_id": m.equipmentID, "connected_at": m.connectedAt, "next_retry_at": m.nextRetry, "retry_in_seconds": remaining, "retry_interval_seconds": 30, "last_error": m.lastError, "paused": m.paused, "url": asString(m.settings["url"]), "enabled": asString(m.settings["enabled"]) != "false" && strings.TrimSpace(asString(m.settings["url"])) != ""}
}
func (m *Module) Reconnect() {
	m.mu.Lock()
	m.paused = false
	if !m.connected {
		m.phase = "reconnecting"
	}
	m.nextRetry = time.Time{}
	cancel := m.sessionCancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.signal()
}
func (m *Module) Disconnect() {
	m.mu.Lock()
	m.paused = true
	m.phase = "paused"
	m.nextRetry = time.Time{}
	cancel := m.sessionCancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.signal()
}
func (m *Module) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func (m *Module) Start(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		settings := m.settingsSnapshot()
		m.mu.Lock()
		paused := m.paused
		enabled := asString(settings["enabled"]) != "false" && strings.TrimSpace(asString(settings["url"])) != ""
		if !enabled {
			m.phase = "disabled"
		}
		if paused {
			m.phase = "paused"
		}
		due := m.nextRetry
		m.mu.Unlock()
		if !enabled || paused {
			select {
			case <-ctx.Done():
				return nil
			case <-m.wake:
				continue
			}
		}
		if delay := time.Until(due); !due.IsZero() && delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-m.wake:
				timer.Stop()
				continue
			case <-timer.C:
			}
		}
		sessionCtx, cancel := context.WithCancel(ctx)
		m.mu.Lock()
		settings = cloneMap(m.settings)
		if m.paused || asString(settings["enabled"]) == "false" || strings.TrimSpace(asString(settings["url"])) == "" {
			m.mu.Unlock()
			cancel()
			continue
		}
		m.phase = "initializing"
		m.nextRetry = time.Time{}
		m.sessionCancel = cancel
		m.sessionDone = sessionCtx.Done()
		m.mu.Unlock()
		err := m.runSession(sessionCtx, settings)
		cancel()
		m.mu.Lock()
		m.connected = false
		m.connID = ""
		m.sendCh = nil
		m.sessionCancel = nil
		m.sessionDone = nil
		m.connectedAt = time.Time{}
		if m.paused {
			m.phase = "paused"
			m.nextRetry = time.Time{}
		} else {
			m.phase = "disconnected"
			m.nextRetry = time.Now().Add(30 * time.Second)
		}
		if err != nil && ctx.Err() == nil && !errors.Is(err, context.Canceled) {
			m.lastError = err.Error()
		}
		m.mu.Unlock()
		m.failPending()
		// A manual reconnect can interrupt both an active session and a retry wait.
		select {
		case <-m.wake:
			m.mu.Lock()
			m.nextRetry = time.Time{}
			m.mu.Unlock()
		default:
		}
	}
}
func (m *Module) runSession(ctx context.Context, settings map[string]interface{}) error {
	rawURL := strings.TrimSpace(asString(settings["url"]))
	u, e := url.Parse(rawURL)
	if e != nil || u.Scheme != "wss" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("configure a wss:// URL without query or credentials")
	}
	svc, ok := m.rt.Service("wisemed-api")
	if !ok {
		return errors.New("WiseMED API module unavailable")
	}
	api, ok := svc.(equipmentAPI)
	if !ok {
		return errors.New("WiseMED initialization unavailable")
	}
	if _, e = api.EnsureEquipmentInitialized(); e != nil {
		return fmt.Errorf("equipment initialization failed: %w", e)
	}
	equipment := strings.TrimSpace(api.Settings()["echipament_id"])
	if equipment == "" || equipment == "0" {
		return errors.New("WiseMED equipment ID unavailable")
	}
	m.mu.Lock()
	m.equipmentID = equipment
	m.phase = "connecting"
	m.mu.Unlock()
	token, e := m.accessToken(ctx, settings, api, equipment)
	if e != nil {
		return e
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if ca := asString(settings["ca_file"]); ca != "" {
		roots, e := x509.SystemCertPool()
		if e != nil {
			roots = x509.NewCertPool()
		}
		pem, e := os.ReadFile(m.rt.ResolvePath(ca))
		if e != nil {
			return errors.New("cannot read WSS CA file")
		}
		if !roots.AppendCertsFromPEM(pem) {
			return errors.New("invalid WSS CA file")
		}
		tlsConfig.RootCAs = roots
	}
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, TLSClientConfig: tlsConfig}
	conn, resp, e := dialer.DialContext(ctx, rawURL, http.Header{"Authorization": []string{"Bearer " + token}})
	if e != nil {
		if resp != nil {
			return fmt.Errorf("WSS handshake rejected: HTTP %d", resp.StatusCode)
		}
		return connectionError(e)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	conn.SetReadLimit(8 * 1024 * 1024)
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	hello := Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "reader", "client_id": m.clientID(), "reader_id": m.rt.ReaderID(), "equipment_id": equipment}}
	if e = conn.WriteJSON(hello); e != nil {
		return errors.New("WSS hello send failed")
	}
	var ack Envelope
	if e = conn.ReadJSON(&ack); e != nil || ack.Type != "hello_ack" {
		return errors.New("WSS hello not accepted")
	}
	if asString(ack.Payload["equipment_id"]) != equipment {
		return errors.New("WSS equipment identity mismatch")
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sendCh := make(chan Envelope, 128)
	m.mu.Lock()
	m.connected = true
	m.phase = "connected"
	m.connID = asString(ack.Payload["connection_id"])
	m.connectedAt = time.Now()
	m.lastError = ""
	m.sendCh = sendCh
	m.mu.Unlock()
	m.record("received", ack)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(60 * time.Second)) })
	writerDone := make(chan struct{})
	go func() { defer close(writerDone); defer conn.Close(); m.writeLoop(sessionCtx, conn, sendCh) }()
	defer func() { cancel(); conn.Close(); <-writerDone }()
	commands := make(chan Envelope, 16)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for {
			select {
			case <-sessionCtx.Done():
				return
			case command := <-commands:
				_ = m.handleCommandContext(sessionCtx, command)
			}
		}
	}()
	defer func() { cancel(); <-workerDone }()
	for {
		var msg Envelope
		if e = conn.ReadJSON(&msg); e != nil {
			return errors.New("WSS connection closed")
		}
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		m.record("received", msg)
		if msg.Type == "command" {
			select {
			case commands <- msg:
			default:
				return errors.New("WSS command queue is full")
			}
			continue
		}
		if e = m.handleIncoming(msg); e != nil {
			m.rt.Logf("WSS incoming command failed")
		}
	}
}
func (m *Module) writeLoop(ctx context.Context, conn *websocket.Conn, ch <-chan Envelope) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if conn.WriteJSON(msg) != nil {
				return
			}
			m.record("sent", msg)
			// Reconnect only after the command reply was written, never before it.
			if msg.Payload["wss_reconnect_scheduled"] == true {
				m.Reconnect()
				return
			}
		case <-ticker.C:
			if conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)) != nil {
				return
			}
		}
	}
}
func (m *Module) handleIncoming(msg Envelope) error {
	switch msg.Type {
	case "command_ack":
		if n, ok := msg.Payload["recipients"].(float64); ok && n == 0 {
			msg.Type = "error"
			msg.Payload = map[string]interface{}{"message": "No connected recipient (or insufficient routing permission)"}
			return m.resolvePending(msg)
		}
		return nil
	case "reply", "error", "connections", "pong":
		return m.resolvePending(msg)
	case "command":
		return m.handleCommand(msg)
	}
	return nil
}
func (m *Module) handleCommand(msg Envelope) error {
	return m.handleCommandContext(context.Background(), msg)
}
func (m *Module) handleCommandContext(ctx context.Context, msg Envelope) error {
	action := asString(msg.Payload["command"])
	args := mapValue(msg.Payload["args"])
	var resp map[string]interface{}
	var handled bool
	var e error
	if action == "api.request" {
		handled = true
		raw, _ := json.Marshal(args)
		var request apibridge.Request
		e = json.Unmarshal(raw, &request)
		scopes := map[string]bool{}
		if values, ok := msg.Payload["sender_scopes"].([]interface{}); ok {
			for _, v := range values {
				scopes[asString(v)] = true
			}
		}
		if !scopes["api:invoke"] {
			e = errors.New("api:invoke scope required")
		}
		if e == nil {
			var result apibridge.Response
			result, e = apibridge.Invoke(ctx, m.rt.Mux(), apibridge.Principal{Subject: asString(msg.Payload["sender_subject"]), TenantID: asString(msg.Payload["sender_tenant_id"]), ConnectionID: asString(msg.Payload["sender_connection_id"]), Admin: scopes["api:admin"]}, request)
			if e == nil {
				b, _ := json.Marshal(result)
				e = json.Unmarshal(b, &resp)
			}
		}
	} else {
		resp, handled, e = m.dispatcher.Dispatch(action, args)
	}
	if !handled {
		e = errors.New("unknown action")
	}
	target := asString(msg.Payload["sender_connection_id"])
	if target == "" {
		return errors.New("command sender missing")
	}
	if e != nil {
		resp = map[string]interface{}{"ok": false, "error": map[string]interface{}{"message": e.Error()}}
	}
	if resp == nil {
		resp = map[string]interface{}{}
	}
	return m.send(Envelope{Type: "reply", CorrelationID: msg.RequestID, Target: &Target{Mode: "connection", ConnectionID: target}, Payload: resp})
}
func (m *Module) HandleWSAction(action string, payload map[string]interface{}) (map[string]interface{}, bool, error) {
	switch action {
	case "reader.status", "ws.status", "wisemed.ws.status":
		return m.Status(), true, nil
	case "device.ping":
		return map[string]interface{}{"ok": true, "equipment_id": m.Status()["equipment_id"], "time": time.Now().UTC()}, true, nil
	case "ws.reconnect":
		return map[string]interface{}{"ok": true, "wss_reconnect_scheduled": true}, true, nil
	case "debug.message":
		return map[string]interface{}{"ok": true, "received": cloneMap(payload)}, true, nil
	}
	return nil, false, nil
}
func (m *Module) Request(ctx context.Context, action string, payload map[string]interface{}) (map[string]interface{}, error) {
	return m.Exchange(ctx, Envelope{Type: "command", Target: &Target{Mode: "server"}, Payload: map[string]interface{}{"command": action, "args": cloneMap(payload)}})
}
func (m *Module) Exchange(ctx context.Context, msg Envelope) (map[string]interface{}, error) {
	id := m.nextRequestID()
	msg.RequestID = id
	ch := make(chan Envelope, 1)
	m.pendingMu.Lock()
	m.pending[id] = ch
	m.pendingMu.Unlock()
	defer func() { m.pendingMu.Lock(); delete(m.pending, id); m.pendingMu.Unlock() }()
	if e := m.send(msg); e != nil {
		return nil, e
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.Type == "error" {
			return nil, errors.New(firstNonEmpty(asString(r.Payload["message"]), "WSS request failed"))
		}
		return cloneMap(r.Payload), nil
	}
}
func (m *Module) DebugRequest(ctx context.Context, action, targetID, text string) (map[string]interface{}, error) {
	switch action {
	case "list":
		return m.Exchange(ctx, Envelope{Type: "list_connections"})
	case "ping_server":
		return m.Exchange(ctx, Envelope{Type: "ping"})
	case "ping", "reconnect", "message":
		if targetID == "" {
			return nil, errors.New("select a connected equipment")
		}
		command := map[string]string{"ping": "device.ping", "reconnect": "ws.reconnect", "message": "debug.message"}[action]
		return m.Exchange(ctx, Envelope{Type: "command", Target: &Target{Mode: "connection", ConnectionID: targetID}, Payload: map[string]interface{}{"command": command, "args": map[string]interface{}{"text": text}}})
	}
	return nil, errors.New("unsupported debug action")
}
func (m *Module) resolvePending(msg Envelope) error {
	key := firstNonEmpty(msg.CorrelationID, msg.RequestID)
	m.pendingMu.Lock()
	ch := m.pending[key]
	m.pendingMu.Unlock()
	if ch != nil {
		select {
		case ch <- msg:
		default:
		}
	}
	return nil
}
func (m *Module) failPending() {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	for _, ch := range m.pending {
		select {
		case ch <- Envelope{Type: "error", Payload: map[string]interface{}{"message": "WSS disconnected"}}:
		default:
		}
	}
}
func (m *Module) send(msg Envelope) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.connected || m.sendCh == nil {
		return errors.New("WSS is disconnected")
	}
	msg.Timestamp = time.Now().UTC()
	select {
	case <-m.sessionDone:
		return errors.New("WSS session ended")
	default:
	}
	select {
	case m.sendCh <- msg:
		return nil
	default:
		return errors.New("WSS queue is full")
	}
}
func (m *Module) record(direction string, msg Envelope) {
	// Keep a bounded in-memory diagnostic trace; never retain tokens/headers.
	// Full payload is available only for explicit debug messages, not medical traffic.
	safe := Envelope{Type: msg.Type, RequestID: msg.RequestID, CorrelationID: msg.CorrelationID, Target: msg.Target, Timestamp: msg.Timestamp, Payload: map[string]interface{}{}}
	for _, k := range []string{"command", "event", "sender_connection_id", "sender_client_id", "sender_reader_id", "equipment_id", "connection_id", "message", "code"} {
		if v, ok := msg.Payload[k]; ok {
			safe.Payload[k] = v
		}
	}
	if msg.Payload["command"] == "debug.message" {
		safe.Payload["args"] = msg.Payload["args"]
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.traceSeq++
	m.traces = append(m.traces, Trace{ID: m.traceSeq, At: time.Now().UTC(), Direction: direction, Message: safe})
	if len(m.traces) > 200 {
		m.traces = append([]Trace(nil), m.traces[len(m.traces)-200:]...)
	}
}
func (m *Module) Trace() []Trace {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Trace{}, m.traces...)
}
func (m *Module) nextRequestID() string {
	return fmt.Sprintf("ws-%s-%d", m.nonce, atomic.AddUint64(&m.seq, 1))
}
func (m *Module) clientID() string {
	return firstNonEmpty(asString(m.settingsSnapshot()["client_id"]), m.rt.ReaderID())
}
func (m *Module) accessToken(ctx context.Context, settings map[string]interface{}, api equipmentAPI, equipment string) (string, error) {
	switch firstNonEmpty(asString(settings["auth_mode"]), "device_key") {
	case "token_file":
		b, e := os.ReadFile(m.rt.ResolvePath(asString(settings["token_file"])))
		if e != nil {
			return "", errors.New("cannot read WSS token file")
		}
		return strings.TrimSpace(string(b)), nil
	case "token_endpoint":
		provider, ok := api.(tokenAPI)
		if !ok {
			return "", errors.New("WSS token provider unavailable")
		}
		return provider.WSSAccessToken(ctx, asString(settings["token_path"]), map[string]interface{}{"reader_id": m.rt.ReaderID(), "client_id": m.clientID(), "equipment_id": equipment})
	case "device_key":
		secret := strings.TrimSpace(asString(settings["device_secret"]))
		if path := asString(settings["secret_file"]); secret == "" && path != "" {
			b, e := os.ReadFile(m.rt.ResolvePath(path))
			if e != nil {
				return "", errors.New("cannot read WSS device key")
			}
			secret = strings.TrimSpace(string(b))
		}
		if secret == "" {
			secret = strings.TrimSpace(api.Settings()["api_key_echipament"])
		}
		if len(secret) < 32 {
			return "", errors.New("WSS requires a dedicated device key of at least 32 bytes")
		}
		kid := asString(settings["key_id"])
		tenant := asString(settings["tenant_id"])
		issuer := asString(settings["issuer"])
		if kid == "" || tenant == "" || issuer == "" {
			return "", errors.New("configure WSS key_id, tenant_id and issuer")
		}
		now := time.Now()
		header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT", "kid": kid})
		claims, _ := json.Marshal(map[string]interface{}{"tenant_id": tenant, "iss": issuer, "aud": firstNonEmpty(asString(settings["audience"]), "wsm-server"), "sub": firstNonEmpty(asString(settings["subject"]), m.rt.ReaderID()), "client_id": m.clientID(), "reader_id": m.rt.ReaderID(), "equipment_id": equipment, "role": "reader", "scopes": strings.FieldsFunc(firstNonEmpty(asString(settings["scopes"]), "route:reply,route:event,route:command,devices:debug,connections:read,server:status"), func(r rune) bool { return r == ',' || r == ' ' }), "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix()})
		unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(unsigned))
		return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
	}
	return "", errors.New("invalid WSS authentication mode")
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

// Classify connection failures without exposing credentials or arbitrary remote text.
func connectionError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	var dns *net.DNSError
	var unknown x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	var timeout net.Error
	switch {
	case errors.As(err, &dns):
		return errors.New("WSS DNS lookup failed; check server hostname and DNS")
	case errors.As(err, &unknown):
		return errors.New("WSS TLS certificate authority is not trusted; check full certificate chain or CA file")
	case errors.As(err, &hostname):
		return errors.New("WSS TLS certificate does not match server hostname")
	case errors.As(err, &invalid):
		return errors.New("WSS TLS certificate is invalid or expired; check certificate and system clock")
	case errors.Is(err, syscall.ECONNREFUSED):
		return errors.New("WSS connection refused; check server/proxy listener and port")
	case errors.As(err, &timeout) && timeout.Timeout():
		return errors.New("WSS connection or handshake timed out; check network, Nginx upstream and whether WSM is paused in debugger")
	default:
		return errors.New("WSS connection failed before HTTP upgrade; check network and TLS")
	}
}
