package signingpad

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const legacyIdleDelay = 3 * time.Second

type padEvent struct{ Action string }
type legacyDriver interface {
	padDriver
	BeginLegacy(name string) (<-chan padEvent, error)
	SigString() (string, error)
	LastActivity() time.Time
	ConfirmLegacy(imageType string) ([]byte, error)
}

type legacyResponse struct {
	Success  bool                   `json:"success"`
	ForEvent json.RawMessage        `json:"forevent"`
	Data     map[string]interface{} `json:"data"`
	Error    string                 `json:"error,omitempty"`
}

func (m *Module) ServeWebSocket(w http.ResponseWriter, r *http.Request) { m.handleWebSocket(w, r) }

func parseLegacyRequest(raw json.RawMessage) (string, string, error) {
	var request map[string]json.RawMessage
	if err := json.Unmarshal(raw, &request); err != nil || request == nil {
		return "", "", fmt.Errorf("invalid request")
	}
	var command, name string
	if err := json.Unmarshal(request["cmd"], &command); err != nil {
		return "", "", fmt.Errorf("cmd is required")
	}
	if command == "signpatient" {
		for _, key := range []string{"sig_type", "pacient_id"} {
			value, ok := request[key]
			if !ok || string(value) == "null" || string(value) == `""` {
				return command, "", fmt.Errorf("%s is required", key)
			}
			var text string
			var number json.Number
			if json.Unmarshal(value, &text) != nil && json.Unmarshal(value, &number) != nil {
				return command, "", fmt.Errorf("%s must be a string or number", key)
			}
		}
		if err := json.Unmarshal(request["nume_pacient"], &name); err != nil || len(name) > 512 {
			return command, "", fmt.Errorf("invalid nume_pacient")
		}
	}
	return command, name, nil
}

// The client saves any successful signpatient reply immediately. Never send a
// success acknowledgement until both signature exports are complete.
func (m *Module) handleLegacyConnection(conn *websocket.Conn, first json.RawMessage) {
	conn.SetReadDeadline(time.Time{}) // The old WiseMED client keeps init connections open.
	incoming := make(chan json.RawMessage)
	disconnected := make(chan struct{})
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		defer close(disconnected)
		for {
			var raw json.RawMessage
			if err := conn.ReadJSON(&raw); err != nil {
				return
			}
			select {
			case incoming <- raw:
			case <-finished:
				return
			}
		}
	}()
	var driver legacyDriver
	var events <-chan padEvent
	var pending json.RawMessage
	var timer *time.Timer
	var timeout <-chan time.Time
	var idleTimer *time.Timer
	var idle <-chan time.Time
	stopIdle := func() {
		if idleTimer != nil {
			idleTimer.Stop()
		}
		idleTimer = nil
		idle = nil
	}
	cleanup := func() {
		stopIdle()
		if timer != nil {
			timer.Stop()
		}
		timer = nil
		timeout = nil
		events = nil
		pending = nil
		if driver != nil {
			driver.Close()
			driver = nil
			m.deviceMu.Unlock()
		}
	}
	defer cleanup()
	send := func(raw json.RawMessage, data map[string]interface{}, err error) bool {
		response := legacyResponse{Success: err == nil, ForEvent: raw, Data: data}
		if response.Data == nil {
			response.Data = map[string]interface{}{}
		}
		if err != nil {
			response.Error = err.Error()
		}
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(response) == nil
	}
	process := func(raw json.RawMessage) bool {
		command, name, err := parseLegacyRequest(raw)
		if err != nil {
			return send(raw, nil, err)
		}
		switch command {
		case "init":
			return send(raw, map[string]interface{}{"service": "eSignature server"}, nil)
		case "signpatient":
			if driver != nil {
				return send(raw, nil, fmt.Errorf("a signature is already in progress"))
			}
			if !m.deviceMu.TryLock() {
				return send(raw, nil, fmt.Errorf("pad is busy in another session"))
			}
			native, err := m.driverFactory(m.rt.ResolvePath(firstNonEmpty(asString(m.rt.ModuleSettings(m.ID())["dll_path"]), "./signotec/STPadLib.dll")))
			if err != nil {
				m.deviceMu.Unlock()
				return send(raw, nil, err)
			}
			var ok bool
			driver, ok = native.(legacyDriver)
			if !ok {
				native.Close()
				m.deviceMu.Unlock()
				return send(raw, nil, fmt.Errorf("driver does not support pad confirmation"))
			}
			if configurable, ok := driver.(interface{ SetDeviceIndex(int) }); ok {
				configurable.SetDeviceIndex(m.padSettingsSnapshot().DeviceIndex)
			}
			events, err = driver.BeginLegacy(name)
			if err != nil {
				cleanup()
				return send(raw, nil, err)
			}
			pending = append(json.RawMessage(nil), raw...)
			timer = time.NewTimer(m.sessionTimeout())
			timeout = timer.C
			return true
		default:
			return send(raw, nil, fmt.Errorf("unknown command: %s", command))
		}
	}
	if !process(first) {
		return
	}
	finishCapture := func() bool {
		image, err := driver.ConfirmLegacy(legacyImageType(pending))
		var sigString string
		if err == nil {
			sigString, err = driver.SigString()
		}
		if err == nil && sigString == "" {
			err = fmt.Errorf("empty signature verification data")
		}
		var data map[string]interface{}
		if err == nil {
			data = map[string]interface{}{"id": 1, "sigbase64": base64.StdEncoding.EncodeToString(image), "sigenc": sigString}
		}
		raw := pending
		cleanup()
		m.recordAction(padCommand{Action: "signpatient"}, map[string]interface{}{"ok": err == nil})
		return send(raw, data, err)
	}
	for {
		select {
		case <-idle:
			last := driver.LastActivity()
			if last.IsZero() {
				stopIdle()
				continue
			}
			// Callback notifications are coalesced. A queued timer must consult the
			// actual last point before confirming, even if the activity event is pending.
			if remaining := legacyIdleDelay - time.Since(last); remaining > 0 {
				stopIdle()
				idleTimer = time.NewTimer(remaining)
				idle = idleTimer.C
				continue
			}
			if !finishCapture() {
				return
			}

		case raw := <-incoming:
			if !process(raw) {
				return
			}
		case event := <-events:
			var data map[string]interface{}
			var err error
			switch event.Action {
			case "retry":
				stopIdle()
				err = driver.Retry()
				if err == nil {
					continue
				}
			case "activity":
				stopIdle()
				idleTimer = time.NewTimer(legacyIdleDelay)
				idle = idleTimer.C
				continue
			case "confirm":
				if !finishCapture() {
					return
				}
				continue
			case "cancel":
				err = fmt.Errorf("signature cancelled")
			case "disconnect":
				err = fmt.Errorf("signature pad disconnected")
			default:
				err = fmt.Errorf("signature capture failed")
			}
			raw := pending
			cleanup()
			m.recordAction(padCommand{Action: "signpatient"}, map[string]interface{}{"ok": err == nil})
			if !send(raw, data, err) {
				return
			}
		case <-timeout:
			raw := pending
			cleanup()
			if !send(raw, nil, fmt.Errorf("signature capture timed out")) {
				return
			}
		case <-disconnected:
			return
		case <-m.stopping:
			return
		}
	}
}

// Keep patient names short enough for the pad; identifiers and CNP never enter logs.
func padDisplayName(name string) string {
	r := []rune(strings.ReplaceAll(strings.ReplaceAll(name, "\r", " "), "\n", " "))
	if len(r) > 45 {
		r = r[:45]
	}
	return string(r)
}
