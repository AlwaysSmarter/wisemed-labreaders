package signingpad

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

type remoteRequest struct {
	command padCommand
	reply   chan map[string]interface{}
}
type remoteSession struct {
	requests chan remoteRequest
	done     chan struct{}
}

func (m *Module) sessionTimeout() time.Duration {
	timeout := time.Duration(intSetting(m.rt.ModuleSettings(m.ID()), "session_timeout_seconds", 180)) * time.Second
	if timeout < time.Second || timeout > 10*time.Minute {
		return 180 * time.Second
	}
	return timeout
}

// HandleWSAction plugs into the same WiseMED dispatcher as barcodeprinter.
// The caller supplies a fresh, unguessable session_id for each capture.
func (m *Module) HandleWSAction(action string, payload map[string]interface{}) (map[string]interface{}, bool, error) {
	if !strings.HasPrefix(action, "esignature.") {
		return nil, false, nil
	}
	command := strings.TrimPrefix(action, "esignature.")
	switch command {
	case "health", "devices", "start", "retry", "confirm", "cancel":
	default:
		return nil, false, nil
	}
	if command == "health" || command == "devices" {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		session := &padSession{m: m}
		defer session.close()
		return dispatcherResult(session.execute(padCommand{Action: command}))
	}
	id := strings.TrimSpace(asString(payload["session_id"]))
	if len(id) < 16 || len(id) > 128 {
		return nil, true, fmt.Errorf("session_id must contain 16–128 characters; use a fresh random ID per capture")
	}
	m.remoteMu.Lock()
	remote := m.remoteSessions[id]
	if remote == nil && command == "start" {
		if len(m.remoteSessions) >= 32 {
			m.remoteMu.Unlock()
			return nil, true, fmt.Errorf("too many sessions")
		}
		remote = &remoteSession{requests: make(chan remoteRequest), done: make(chan struct{})}
		m.remoteSessions[id] = remote
		go m.runRemote(id, remote)
	}
	m.remoteMu.Unlock()
	if remote == nil {
		return nil, true, fmt.Errorf("session not found; start a capture first")
	}
	request := remoteRequest{command: padCommand{ID: asString(payload["id"]), Action: command}, reply: make(chan map[string]interface{}, 1)}
	timer := time.NewTimer(m.sessionTimeout())
	defer timer.Stop()
	select {
	case remote.requests <- request:
	case <-remote.done:
		return nil, true, fmt.Errorf("session closed")
	case <-timer.C:
		return nil, true, fmt.Errorf("session timeout")
	}
	select {
	case reply := <-request.reply:
		return dispatcherResult(reply)
	case <-timer.C:
		return nil, true, fmt.Errorf("session timeout")
	}
}
func (m *Module) runRemote(id string, remote *remoteSession) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	session := &padSession{m: m}
	defer func() {
		session.close()
		m.remoteMu.Lock()
		delete(m.remoteSessions, id)
		m.remoteMu.Unlock()
		close(remote.done)
	}()
	timer := time.NewTimer(m.sessionTimeout())
	defer timer.Stop()
	for {
		select {
		case request := <-remote.requests:
			result := session.execute(request.command)
			request.reply <- result
			if !session.active {
				return
			}
		case <-timer.C:
			return
		case <-m.stopping:
			return
		}
	}
}

func dispatcherResult(response map[string]interface{}) (map[string]interface{}, bool, error) {
	if response["ok"] != true {
		return nil, true, fmt.Errorf("%s", asString(response["message"]))
	}
	return response, true, nil
}
