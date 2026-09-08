package signingpad

import (
	"encoding/base64"
	"fmt"
	"runtime"
	"wisemed-labreaders/readersv3/shared/appmeta"
)

type padSession struct {
	m      *Module
	driver padDriver
	active bool
}

func (session *padSession) close() {
	if session.driver != nil {
		session.driver.Close()
		session.driver = nil
		session.active = false
		session.m.deviceMu.Unlock()
	}
}
func (session *padSession) execute(cmd padCommand) map[string]interface{} {
	m := session.m
	response := map[string]interface{}{"id": cmd.ID, "action": cmd.Action, "ok": true}
	var commandErr error
	switch cmd.Action {
	case "health":
		response["service"] = "eSignature server"
		response["version"] = appmeta.CurrentVersion()
		response["platform"] = runtime.GOOS
		response["native_supported"] = runtime.GOOS == "windows"
	case "devices", "start", "retry", "confirm", "cancel":
		if session.driver == nil {
			if !m.deviceMu.TryLock() {
				commandErr = fmt.Errorf("pad is busy in another session")
			} else {
				path := m.rt.ResolvePath(firstNonEmpty(asString(m.rt.ModuleSettings(m.ID())["dll_path"]), "./signotec/STPadLib.dll"))
				session.driver, commandErr = m.driverFactory(path)
				if commandErr != nil {
					m.deviceMu.Unlock()
				}
			}
		}
		if commandErr == nil {
			switch cmd.Action {
			case "devices":
				if session.active {
					commandErr = fmt.Errorf("capture already active")
					break
				}
				var count int32
				count, commandErr = session.driver.Count()
				response["count"] = count
			case "start":
				if session.active {
					commandErr = fmt.Errorf("capture already active")
					break
				}
				commandErr = session.driver.Start()
				session.active = commandErr == nil
			case "retry", "confirm", "cancel":
				if !session.active {
					commandErr = fmt.Errorf("start a capture first")
					break
				}
				switch cmd.Action {
				case "retry":
					commandErr = session.driver.Retry()
				case "cancel":
					commandErr = session.driver.Cancel()
					session.active = false
				case "confirm":
					var data []byte
					data, commandErr = session.driver.Confirm()
					session.active = false
					if commandErr == nil {
						response["imageBase64"] = base64.StdEncoding.EncodeToString(data)
						response["mimeType"] = "image/png"
					}
				}
			}
		}
	default:
		commandErr = fmt.Errorf("unknown action: %s", cmd.Action)
	}
	if session.driver != nil && !session.active {
		session.driver.Close()
		session.driver = nil
		m.deviceMu.Unlock()
	}
	if commandErr != nil {
		response["ok"] = false
		response["message"] = commandErr.Error()
	}

	m.recordAction(cmd, response)
	return response
}
