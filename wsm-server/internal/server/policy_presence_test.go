package server

import (
	"github.com/gorilla/websocket"
	"testing"
	"time"
)

func TestPolicyClosedReaderPublishesDisconnectedPresence(t *testing.T) {
	f := newFixture(t)
	f.cfg.Server.MessagesPerSecond = 1
	f.cfg.Server.MessageBurst = 3
	browser, _ := f.connect(t, "a", "browser", "observer")
	reader, id := f.connect(t, "a", "reader", "terminal")
	_ = reader.WriteJSON(Envelope{Type: "hello"})
	reader.SetReadDeadline(time.Now().Add(time.Second))
	for {
		_, _, err := reader.ReadMessage()
		if err == nil {
			continue
		}
		if !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
			t.Fatal(err)
		}
		break
	}
	browser.SetReadDeadline(time.Now().Add(time.Second))
	for {
		var msg Envelope
		if err := browser.ReadJSON(&msg); err != nil {
			t.Fatal("missing disconnection presence:", err)
		}
		if msg.Type == "presence" && msg.Payload["connection_id"] == id && msg.Payload["event"] == "disconnected" {
			break
		}
	}
	for _, c := range f.s.hub.Snapshot("a") {
		if c.ID == id {
			t.Fatal("disconnected reader still listed")
		}
	}
}
