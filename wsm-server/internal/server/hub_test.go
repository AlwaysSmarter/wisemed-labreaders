package server

import (
	"fmt"
	"testing"
)

func hubClient(h *Hub, tenant, id, kind, reader string) *Connection {
	c := h.NewConnection(nil, tenant, 128)
	c.ID = id
	h.Register(c, HelloPayload{ClientType: kind, ClientID: id, ReaderID: reader})
	return c
}
func TestHubTenantIsolationEveryRoute(t *testing.T) {
	h := NewHub()
	a := hubClient(h, "a", "a-client", "browser", "")
	b := hubClient(h, "a", "a-reader", "reader", "reader-1")
	x := hubClient(h, "b", "b-reader", "reader", "reader-1")
	h.Subscribe(b.ID, "results:reader-1", 64)
	h.Subscribe(x.ID, "results:reader-1", 64)
	cases := []struct {
		target *Target
		want   int
	}{
		{&Target{Mode: "all"}, 2}, {&Target{Mode: "connection", ConnectionID: x.ID}, 0},
		{&Target{Mode: "connection", ConnectionID: b.ID}, 1}, {&Target{Mode: "reader", ReaderID: "reader-1"}, 1},
		{&Target{Mode: "readers", ReaderIDs: []string{"reader-1", "reader-1"}}, 1},
		{&Target{Mode: "connections", ConnectionIDs: []string{b.ID, x.ID, b.ID}}, 1},
		{&Target{Mode: "topic", Topic: "results:reader-1"}, 1}, {&Target{Mode: "client_type", ClientType: "reader"}, 1},
		{&Target{Mode: "self"}, 1}, {nil, 0},
	}
	for _, tc := range cases {
		if n := h.Route(Envelope{Type: "command", Target: tc.target}, a); n != tc.want {
			t.Errorf("target=%+v got %d want %d", tc.target, n, tc.want)
		}
	}
	if len(x.send) != 0 {
		t.Fatal("cross-tenant message leaked")
	}
	if len(h.Snapshot("a")) != 2 || len(h.Snapshot("b")) != 1 {
		t.Fatal("snapshot leaked")
	}
	if h.Stats("a").TotalAccepted != 2 || h.Stats("b").TotalAccepted != 1 {
		t.Fatal("stats leaked")
	}
}
func TestHubBroadcastTo50Connections(t *testing.T) {
	h := NewHub()
	for i := 0; i < 50; i++ {
		hubClient(h, "a", fmt.Sprint(i), "browser", "")
	}
	hubClient(h, "b", "foreign", "browser", "")
	if n := h.Broadcast("a", Envelope{Type: "event"}); n != 50 {
		t.Fatal(n)
	}
}
func TestHubDuplicateAndBackpressure(t *testing.T) {
	h := NewHub()
	c := hubClient(h, "a", "first", "reader", "r")
	other := h.NewConnection(nil, "a", 1)
	if h.Register(other, HelloPayload{ClientType: "reader", ClientID: "second", ReaderID: "r"}) {
		t.Fatal("duplicate reader")
	}
	if h.Register(c, HelloPayload{}) {
		t.Fatal("duplicate hello")
	}
	if !h.Subscribe(c.ID, "one", 1) || h.Subscribe(c.ID, "two", 1) {
		t.Fatal("topic bound")
	}
	h.Unsubscribe(c.ID, "one")
	if !h.Subscribe(c.ID, "two", 1) {
		t.Fatal("unsubscribe")
	}
	for i := 0; i < 128; i++ {
		if !h.send(c, Envelope{}) {
			t.Fatal(i)
		}
	}
	if h.send(c, Envelope{}) || h.Stats("a").TotalDropped != 1 {
		t.Fatal("overflow not counted")
	}
	select {
	case <-c.closed:
	default:
		t.Fatal("slow consumer not disconnected")
	}
	h.Remove(c.ID)
	if len(h.Snapshot("a")) != 0 {
		t.Fatal("not removed")
	}
}
