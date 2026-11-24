package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAdminClientActionsIsolation(t *testing.T) {
	s, _ := adminFixture(t, `{"user_type":-1,"token":"TOKEN-PRIVATE"}`)
	cookie, csrf := loginAdmin(t, s, "a")
	a := s.hub.NewConnection(nil, "a", 10)
	b := s.hub.NewConnection(nil, "b", 10)
	s.hub.Register(a, HelloPayload{ClientType: "reader", ClientID: "reader-a", ReaderID: "reader-a"})
	s.hub.Register(b, HelloPayload{ClientType: "browser", ClientID: "browser-b"})
	for _, path := range []string{"clients", "messages", "messages/stream?filters=event"} {
		if w := adminCall(s, "GET", "/admin/api/"+path, "", nil, ""); w.Code != 401 {
			t.Fatalf("anonymous %s: %d", path, w.Code)
		}
	}
	w := adminCall(s, "GET", "/admin/api/clients", "", cookie, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), a.ID) || strings.Contains(w.Body.String(), b.ID) {
		t.Fatal(w.Body.String())
	}
	for _, action := range []string{"message", "reconnect", "kick"} {
		body := fmt.Sprintf(`{"id":%q,"action":%q,"message":"hello"}`, b.ID, action)
		if w := adminCall(s, "POST", "/admin/api/clients/action", body, cookie, csrf); w.Code != 404 {
			t.Fatalf("cross tenant %s %d", action, w.Code)
		}
	}
	body := fmt.Sprintf(`{"id":%q,"action":"message","message":"hello"}`, a.ID)
	if w := adminCall(s, "POST", "/admin/api/clients/action", body, cookie, ""); w.Code != 403 {
		t.Fatal("CSRF")
	}
	if w := adminCall(s, "POST", "/admin/api/clients/action", body, cookie, csrf); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	m := <-a.send
	if m.Payload["command"] != "debug.message" || m.Payload["sender_subject"] != "operator" {
		t.Fatalf("%+v", m)
	}
	body = fmt.Sprintf(`{"id":%q,"action":"reconnect"}`, a.ID)
	if w := adminCall(s, "POST", "/admin/api/clients/action", body, cookie, csrf); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if m := <-a.send; m.Payload["command"] != "ws.reconnect" {
		t.Fatal(m)
	}
	body = fmt.Sprintf(`{"id":%q,"action":"kick"}`, a.ID)
	adminCall(s, "POST", "/admin/api/clients/action", body, cookie, csrf)
	select {
	case <-a.closed:
	default:
		t.Fatal("not kicked")
	}
	select {
	case <-b.closed:
		t.Fatal("other tenant kicked")
	default:
	}
}
func TestTrafficFiltersBoundsAndConcurrency(t *testing.T) {
	h := NewHub()
	o, _ := h.observe("a", []string{"event", "topic:test"})
	defer h.unobserve(o)
	h.publish("b", Envelope{Type: "event"})
	h.publish("a", Envelope{Type: "command"})
	select {
	case <-o.messages:
		t.Fatal("wrong tenant/type")
	default:
	}
	h.publish("a", Envelope{Type: "reply", ConnectionID: "verified", Target: &Target{Mode: "topic", Topic: "test"}, Payload: map[string]interface{}{"text": strings.Repeat("x", 70000), "sender_client_id": "client"}})
	var m Envelope
	json.Unmarshal(<-o.messages, &m)
	if m.ConnectionID != "verified" || m.Payload["truncated"] != true || m.Payload["sender_client_id"] != "client" {
		t.Fatal(m)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				h.publish("a", Envelope{Type: "event"})
			}
		}()
	}
	wg.Wait()
	select {
	case <-o.overflow:
	default:
		t.Fatal("missing overflow signal")
	}
	for i := 0; i < 7; i++ {
		if _, ok := h.observe("a", []string{"all"}); !ok {
			t.Fatal("early limit")
		}
	}
	if _, ok := h.observe("a", []string{"all"}); ok {
		t.Fatal("unbounded observers")
	}
}

func TestAdminLiveStreamAndLogout(t *testing.T) {
	s, _ := adminFixture(t, `{"user_type":-1,"token":"TOKEN-PRIVATE"}`)
	cookie, csrf := loginAdmin(t, s, "a")
	ts := httptest.NewServer(s.routes())
	defer ts.Close()
	req, _ := http.NewRequest("GET", ts.URL+"/admin/api/messages/stream?filters=event", nil)
	req.Host = "admin.example"
	req.AddCookie(cookie)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal(resp.Status)
	}
	reader := bufio.NewReader(resp.Body)
	readEvent := func(want string) {
		t.Helper()
		line, err := reader.ReadString('\n')
		if err != nil || line != "event: "+want+"\n" {
			t.Fatalf("event %q %v", line, err)
		}
		reader.ReadString('\n')
		reader.ReadString('\n')
	}
	readEvent("ready")
	s.hub.publish("b", Envelope{Type: "event"})
	s.hub.publish("a", Envelope{Type: "event", ConnectionID: "actual-client"})
	readEvent("traffic")
	adminCall(s, "POST", "/admin/api/logout", `{}`, cookie, csrf)
	readEvent("expired")
}
