package ws

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"wisemed-labreaders/readersv3/core/module"
)

type WSActionHandler interface {
	HandleWSAction(action string, payload map[string]interface{}) (map[string]interface{}, bool, error)
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

type Module struct {
	rt         module.Runtime
	dispatcher *ActionDispatcher
	mu         sync.RWMutex
	connected  bool
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "wisemed-ws" }
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	m.dispatcher = &ActionDispatcher{}
	m.rt.RegisterService("ws-action-dispatcher", m.dispatcher)
	m.rt.RegisterService("wisemed-ws-status", m)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	settings := m.rt.ModuleSettings(m.ID())
	if enabled, ok := settings["enabled"].(bool); ok && !enabled {
		<-ctx.Done()
		return nil
	}
	url, _ := settings["url"].(string)
	if url == "" {
		url = "wss://wslocal.wisemed.eu/ws"
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err := m.runSession(ctx, url); err != nil {
			m.setConnected(false)
			m.rt.Logf("ws disconnected: %v", err)
		}
		delayMS := intFromSettings(settings, "reconnect_delay_ms", 5000)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Duration(delayMS) * time.Millisecond):
		}
	}
}

func (m *Module) runSession(ctx context.Context, url string) error {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		m.setConnected(false)
		return err
	}
	defer conn.Close()
	m.setConnected(true)
	defer m.setConnected(false)
	m.rt.Logf("ws connected to %s", url)
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
					_ = conn.WriteJSON(map[string]interface{}{
						"action": "ping",
						"ts":     time.Now().UTC().Format(time.RFC3339Nano),
					})
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
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		resp, err := m.processIncoming(raw)
		if err != nil {
			resp = map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			}
		}
		if err := conn.WriteJSON(resp); err != nil {
			return err
		}
	}
}

func (m *Module) Connected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

func (m *Module) setConnected(connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = connected
}

func (m *Module) processIncoming(raw []byte) (map[string]interface{}, error) {
	msg := map[string]interface{}{}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	action, _ := msg["action"].(string)
	payload, _ := msg["payload"].(map[string]interface{})
	if payload == nil {
		payload, _ = msg["data"].(map[string]interface{})
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	switch action {
	case "ping":
		return map[string]interface{}{"action": "pong", "success": true}, nil
	default:
		if m.dispatcher == nil {
			return nil, errors.New("ws dispatcher unavailable")
		}
		resp, handled, err := m.dispatcher.Dispatch(action, payload)
		if !handled {
			return map[string]interface{}{
				"success": false,
				"error":   "unknown action",
				"action":  action,
			}, nil
		}
		if resp == nil {
			resp = map[string]interface{}{}
		}
		resp["action"] = action
		resp["success"] = err == nil
		if err != nil {
			resp["error"] = err.Error()
		}
		if reqID, ok := msg["request_id"]; ok {
			resp["request_id"] = reqID
		}
		return resp, nil
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
