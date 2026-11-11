package ws

import "errors"

// PublishEventToConnection uses normal WSM routing and the device's provisioned
// route:event permission. Callers must derive connectionID from an attested
// principal; the method never broadcasts or modifies authentication/scopes.
func (m *Module) PublishEventToConnection(connectionID string, payload map[string]interface{}) error {
	if connectionID == "" {
		return errors.New("event target connection is required")
	}
	return m.send(Envelope{Type: "event", RequestID: m.nextRequestID(), Target: &Target{Mode: "connection", ConnectionID: connectionID}, Payload: payload})
}
