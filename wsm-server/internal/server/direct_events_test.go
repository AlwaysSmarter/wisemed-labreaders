package server

import "testing"

func TestReaderDirectEvents(t *testing.T) {
	reader := claimsFor("a", "reader", "utility")
	event := Envelope{Type: "event", Target: &Target{Mode: "connection", ConnectionID: "browser-connection"}, Payload: map[string]interface{}{"event": "utility.event"}}
	if err := authorizeMessage(&reader, event); err != nil {
		t.Fatalf("direct utility event rejected: %v", err)
	}
	for _, mode := range []string{"all", "connections", "equipment"} {
		other := event
		other.Target = &Target{Mode: mode, ConnectionIDs: []string{"browser-connection"}, EquipmentID: "42"}
		if authorizeMessage(&reader, other) == nil {
			t.Fatalf("unexpected reader fanout allowed: %s", mode)
		}
	}
	reader.Scopes = []string{"route:reply"}
	if authorizeMessage(&reader, event) == nil {
		t.Fatal("route:event required")
	}
}

func TestDirectEventsReachBrowserAndCLI(t *testing.T) {
	f := newFixture(t)
	browser, bid := f.connect(t, "a", "browser", "operator")
	reader, rid := f.connect(t, "a", "reader", "utility")
	observer, ok := f.s.hub.observe("a", []string{"event"})
	if !ok {
		t.Fatal("observer")
	}
	defer f.s.hub.unobserve(observer)
	for _, kind := range []string{"card_inserted", "card_removed"} {
		send(t, reader, Envelope{Type: "event", RequestID: kind, Target: &Target{Mode: "connection", ConnectionID: bid}, Payload: map[string]interface{}{"event": "ecard.event", "subtype": kind}})
		got := readType(t, browser, "event")
		if got.ConnectionID != rid || got.Payload["subtype"] != kind {
			t.Fatalf("wrong notification: %+v", got)
		}
		select {
		case <-observer.messages:
		default:
			t.Fatal("CLI did not observe routed event")
		}
	}
	_, foreign := f.connect(t, "b", "browser", "other-tenant")
	send(t, reader, Envelope{Type: "event", RequestID: "foreign", Target: &Target{Mode: "connection", ConnectionID: foreign}, Payload: map[string]interface{}{"event": "utility.event"}})
	for {
		ack := readType(t, reader, "command_ack")
		if ack.CorrelationID == "foreign" || ack.RequestID == "foreign" {
			if ack.Payload["recipients"] != float64(0) {
				t.Fatal("cross-tenant delivery")
			}
			break
		}
	}
}
