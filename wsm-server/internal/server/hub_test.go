package server

import (
	"testing"
	"time"
)

func TestHubBroadcastTo50Connections(t *testing.T) {
	hub := NewHub()
	totalClients := 50
	connections := make([]*Connection, 0, totalClients)

	for i := 0; i < totalClients; i++ {
		conn := &Connection{
			ID:          "conn-" + itoa(i),
			ClientType:  "browser",
			ClientID:    "client-" + itoa(i),
			ConnectedAt: time.Now().UTC(),
			LastSeenAt:  time.Now().UTC(),
			send:        make(chan Envelope, 8),
			closed:      make(chan struct{}),
		}
		hub.Register(conn, HelloPayload{
			ClientType: "browser",
			ClientID:   conn.ClientID,
			Label:      "test",
		})
		connections = append(connections, conn)
	}

	sent := hub.Broadcast(Envelope{
		Type:      "command",
		RequestID: "broadcast-1",
		Broadcast: true,
		Payload: map[string]interface{}{
			"text": "enterprise-broadcast",
		},
	})
	if sent != totalClients {
		t.Fatalf("expected %d routed messages, got %d", totalClients, sent)
	}

	for _, conn := range connections {
		select {
		case msg := <-conn.send:
			if msg.Type != "command" {
				t.Fatalf("expected command message for %s, got %s", conn.ID, msg.Type)
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout waiting for message on %s", conn.ID)
		}
	}
}

func TestHubTargetSingleConnection(t *testing.T) {
	hub := NewHub()

	left := &Connection{
		ID:          "conn-left",
		ClientType:  "browser",
		ClientID:    "left",
		ConnectedAt: time.Now().UTC(),
		LastSeenAt:  time.Now().UTC(),
		send:        make(chan Envelope, 4),
		closed:      make(chan struct{}),
	}
	right := &Connection{
		ID:          "conn-right",
		ClientType:  "browser",
		ClientID:    "right",
		ConnectedAt: time.Now().UTC(),
		LastSeenAt:  time.Now().UTC(),
		send:        make(chan Envelope, 4),
		closed:      make(chan struct{}),
	}

	hub.Register(left, HelloPayload{ClientType: "browser", ClientID: "left"})
	hub.Register(right, HelloPayload{ClientType: "browser", ClientID: "right"})

	sent := hub.Route(Envelope{
		Type: "command",
		Target: &Target{
			Mode:         "connection",
			ConnectionID: "conn-right",
		},
		Payload: map[string]interface{}{"text": "hello"},
	}, left)
	if sent != 1 {
		t.Fatalf("expected one routed message, got %d", sent)
	}

	select {
	case <-left.send:
		t.Fatalf("left should not receive the targeted message")
	default:
	}

	select {
	case msg := <-right.send:
		if msg.Type != "command" {
			t.Fatalf("expected command message, got %s", msg.Type)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("right did not receive the targeted message")
	}
}
