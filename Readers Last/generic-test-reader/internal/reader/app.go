package reader

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"

	"wisemed-labreaders/readerslast/generic-test-reader/internal/config"
	"wisemed-labreaders/readerslast/generic-test-reader/internal/storage"
)

type App struct {
	cfg   *config.Config
	store *storage.Store

	sendMu sync.RWMutex
	sendCh chan Envelope

	rtLogsMu sync.RWMutex
	rtLogs   bool

	stateMu            sync.RWMutex
	wiseMedWSConnected bool
	analyzerConnected  bool

	importMu       sync.Mutex
	importInFlight map[string]struct{}
}

func New(cfg *config.Config, store *storage.Store) *App {
	return &App{
		cfg:            cfg,
		store:          store,
		sendCh:         make(chan Envelope, 256),
		importInFlight: map[string]struct{}{},
	}
}

func (a *App) Run(ctx context.Context) error {
	a.logEvent("info", "reader_starting", "reader starting", map[string]interface{}{
		"comm_type": a.cfg.Comm.Type,
		"protocol":  a.cfg.Comm.Protocol,
		"layout":    a.cfg.Layout.Kind,
	})

	go a.runCommLoop(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		log.Printf("connecting to WiseMedWS at %s ...", a.cfg.WiseMedWS.WSURL)
		if err := a.runWS(ctx); err != nil {
			a.setWiseMedWSConnected(false)
			a.logEvent("error", "ws_disconnected", "WiseMedWS connection failed", map[string]interface{}{"error": err.Error()})
			log.Printf("WiseMedWS connection failed: %v", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Duration(a.cfg.WiseMedWS.ReconnectDelayMS) * time.Millisecond):
			log.Printf("retrying WiseMedWS connection in %d ms", a.cfg.WiseMedWS.ReconnectDelayMS)
		}
	}
}

func (a *App) runCommLoop(ctx context.Context) {
	switch a.cfg.Comm.Type {
	case config.CommTypeFile:
		a.setAnalyzerConnected(true)
		defer a.setAnalyzerConnected(false)
		log.Printf("starting file communication loop import_dir=%s pattern=%s", a.cfg.Comm.File.ImportDir, a.cfg.Comm.File.Pattern)
		a.logEvent("info", "comm_started", "file communication loop started", map[string]interface{}{
			"import_dir": a.cfg.Comm.File.ImportDir,
			"pattern":    a.cfg.Comm.File.Pattern,
		})
		a.fileLoop(ctx)
	case config.CommTypeSerial:
		a.setAnalyzerConnected(false)
		a.logEvent("warn", "comm_not_implemented", "serial runtime is configured but not implemented yet", map[string]interface{}{
			"port":      a.cfg.Comm.Serial.Port,
			"baud":      a.cfg.Comm.Serial.Baud,
			"parity":    a.cfg.Comm.Serial.Parity,
			"data_bits": a.cfg.Comm.Serial.DataBits,
			"stop_bits": a.cfg.Comm.Serial.StopBits,
		})
		<-ctx.Done()
	case config.CommTypeNetwork:
		a.setAnalyzerConnected(false)
		a.logEvent("warn", "comm_not_implemented", "network runtime is configured but not implemented yet", map[string]interface{}{
			"host": a.cfg.Comm.Network.Host,
			"port": a.cfg.Comm.Network.Port,
			"mode": a.cfg.Comm.Network.Mode,
		})
		<-ctx.Done()
	}
}

func (a *App) runWS(ctx context.Context) error {
	token, err := a.signJWT()
	if err != nil {
		return fmt.Errorf("sign ws jwt: %w", err)
	}
	wsURL, err := url.Parse(a.cfg.WiseMedWS.WSURL)
	if err != nil {
		return err
	}
	query := wsURL.Query()
	query.Set("token", token)
	wsURL.RawQuery = query.Encode()

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	header.Set("X-Reader-ID", a.cfg.Reader.ID)

	dialer := websocket.Dialer{
		HandshakeTimeout: time.Duration(a.cfg.WiseMedWS.ConnectTimeoutMS) * time.Millisecond,
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: a.cfg.WiseMedWS.InsecureSkipVerify,
		},
	}
	conn, resp, err := dialer.DialContext(ctx, wsURL.String(), header)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("dial ws status=%d: %w", resp.StatusCode, err)
		}
		return err
	}
	defer conn.Close()
	a.setWiseMedWSConnected(true)
	defer a.setWiseMedWSConnected(false)

	log.Printf("connected to WiseMedWS at %s", a.cfg.WiseMedWS.WSURL)
	a.logEvent("info", "ws_connected", "connected to WiseMedWS", map[string]interface{}{"ws_url": a.cfg.WiseMedWS.WSURL})

	localSend := make(chan Envelope, 256)
	a.setSendChannel(localSend)
	defer a.setSendChannel(nil)

	done := make(chan error, 1)
	go a.writeLoop(conn, localSend, done)
	go a.readLoop(conn, done)

	a.enqueue(Envelope{
		Type:      "hello",
		RequestID: newRequestID(),
		Timestamp: time.Now().UTC(),
		Payload: map[string]interface{}{
			"client_type": "reader",
			"client_id":   a.cfg.Reader.ClientID,
			"reader_id":   a.cfg.Reader.ID,
			"label":       a.cfg.Reader.Label,
		},
	})
	log.Printf("reader hello queued for reader_id=%s", a.cfg.Reader.ID)

	heartbeatTicker := time.NewTicker(time.Duration(a.cfg.WiseMedWS.HeartbeatMS) * time.Millisecond)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-done:
			return err
		case <-heartbeatTicker.C:
			a.enqueue(Envelope{
				Type:      "ping",
				RequestID: newRequestID(),
				Timestamp: time.Now().UTC(),
				Payload: map[string]interface{}{
					"reader_id": a.cfg.Reader.ID,
					"kind":      "heartbeat",
					"stats":     a.store.Stats(),
				},
			})
			a.sendLogEvent("tick", map[string]interface{}{
				"reader_id": a.cfg.Reader.ID,
				"stats":     a.store.Stats(),
			})
		}
	}
}

func (a *App) setWiseMedWSConnected(v bool) {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	a.wiseMedWSConnected = v
}

func (a *App) setAnalyzerConnected(v bool) {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	a.analyzerConnected = v
}

func (a *App) connectionState() (bool, bool) {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	return a.wiseMedWSConnected, a.analyzerConnected
}

func (a *App) readLoop(conn *websocket.Conn, done chan<- error) {
	for {
		var msg Envelope
		if err := conn.ReadJSON(&msg); err != nil {
			done <- err
			return
		}
		a.logEvent("debug", "ws_rx", "received ws message", summarizeWSEnvelope(msg))
		log.Printf("received ws message type=%s", msg.Type)
		switch msg.Type {
		case "command":
			a.handleCommand(msg)
		case "hello_ack", "command_ack", "presence", "connections", "pong", "error":
		default:
		}
	}
}

func summarizeWSEnvelope(msg Envelope) map[string]interface{} {
	out := map[string]interface{}{
		"type": msg.Type,
	}
	if msg.RequestID != "" {
		out["request_id"] = msg.RequestID
	}
	if msg.CorrelationID != "" {
		out["correlation_id"] = msg.CorrelationID
	}
	if msg.Target != nil {
		out["target"] = map[string]interface{}{
			"mode":  msg.Target.Mode,
			"topic": msg.Target.Topic,
		}
	}
	if len(msg.Payload) == 0 {
		return out
	}
	out["payload_keys"] = mapKeys(msg.Payload)
	out["payload_size"] = len(msg.Payload)
	return out
}

func mapKeys(in map[string]interface{}) []string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (a *App) writeLoop(conn *websocket.Conn, send <-chan Envelope, done chan<- error) {
	for msg := range send {
		if msg.Timestamp.IsZero() {
			msg.Timestamp = time.Now().UTC()
		}
		if err := conn.WriteJSON(msg); err != nil {
			done <- err
			return
		}
	}
}

func (a *App) setSendChannel(ch chan Envelope) {
	a.sendMu.Lock()
	defer a.sendMu.Unlock()
	a.sendCh = ch
}

func (a *App) enqueue(msg Envelope) {
	a.sendMu.RLock()
	ch := a.sendCh
	a.sendMu.RUnlock()
	if ch == nil {
		return
	}
	select {
	case ch <- msg:
	default:
		log.Printf("dropping outbound ws message type=%s", msg.Type)
	}
}

func (a *App) signJWT() (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"sub":             a.cfg.Reader.ID,
		"role":            "reader",
		"client_id":       a.cfg.Reader.ClientID,
		"reader_id":       a.cfg.Reader.ID,
		"label":           a.cfg.Reader.Label,
		"medical_unit_id": a.cfg.Reader.MedicalUnitID,
		"iat":             now.Unix(),
		"exp":             now.Add(5 * time.Minute).Unix(),
		"jti":             newRequestID(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(a.cfg.Reader.APIKey))
}

func (a *App) logEvent(level, eventType, message string, payload map[string]interface{}) {
	if err := a.store.AppendEvent(level, eventType, message, payload); err != nil {
		log.Printf("append event failed: %v", err)
	}
	a.rtLogsMu.RLock()
	active := a.rtLogs
	a.rtLogsMu.RUnlock()
	if active && shouldBroadcastRealtimeLog(eventType, message) {
		a.sendLogEvent("log", map[string]interface{}{
			"level":      level,
			"event_type": eventType,
			"message":    message,
			"payload":    payload,
			"created_at": time.Now().UTC(),
		})
	}
}

func shouldBroadcastRealtimeLog(eventType, message string) bool {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "ws_rx", "ws_tx", "ws_ping", "ws_pong":
		return false
	}
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "received ws message", "sent ws message":
		return false
	}
	return true
}

func (a *App) sendLogEvent(eventType string, payload map[string]interface{}) {
	a.sendTopicEvent(a.logTopic(), eventType, payload)
}

func (a *App) sendResultEvent(eventType string, payload map[string]interface{}) {
	a.sendTopicEvent(a.resultsTopic(), eventType, payload)
}

func (a *App) sendTopicEvent(topic, eventType string, payload map[string]interface{}) {
	a.enqueue(Envelope{
		Type:      "event",
		RequestID: newRequestID(),
		Target: &Target{
			Mode:  "topic",
			Topic: topic,
		},
		Payload: map[string]interface{}{
			"event_type": eventType,
			"reader_id":  a.cfg.Reader.ID,
			"payload":    payload,
		},
		Timestamp: time.Now().UTC(),
	})
}

func (a *App) logTopic() string {
	return "logs:" + a.cfg.Reader.ID
}

func (a *App) resultsTopic() string {
	return "results:" + a.cfg.Reader.ID
}

func (a *App) respond(correlationID string, success bool, data map[string]interface{}, errText string) {
	a.enqueue(Envelope{
		Type:          "reply",
		RequestID:     newRequestID(),
		CorrelationID: correlationID,
		Timestamp:     time.Now().UTC(),
		Payload: map[string]interface{}{
			"success": success,
			"data":    data,
			"error":   errText,
		},
	})
}

func asJSON(v interface{}) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
