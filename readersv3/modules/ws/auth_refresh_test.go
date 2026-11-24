package ws

import (
	"context"
	"testing"
	"time"
)

func TestRenewAuthenticationKeepsTransportAndResolvesAck(t *testing.T) {
	m := secretModule(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.connected = true
	m.sendCh = make(chan Envelope, 1)
	m.sessionDone = ctx.Done()
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.renewAuthentication(ctx, time.Now().Add(100*time.Millisecond), func(context.Context) (string, error) { return "test-renewed-token", nil })
	}()
	select {
	case msg := <-m.sendCh:
		if msg.Type != "auth.refresh" || msg.Payload["token"] != "test-renewed-token" {
			t.Fatal("wrong renewal")
		}
		m.record("sent", msg)
		traces := m.Trace()
		if _, exists := traces[len(traces)-1].Message.Payload["token"]; exists {
			t.Fatal("token leaked in diagnostic trace")
		}
		m.handleIncoming(Envelope{Type: "auth_refreshed", CorrelationID: msg.RequestID, Payload: map[string]interface{}{"expires_at": time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)}})
	case <-time.After(time.Second):
		t.Fatal("no renewal")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("renewal worker did not stop")
	}
	if !m.connected {
		t.Fatal("renewal disconnected transport")
	}
}
