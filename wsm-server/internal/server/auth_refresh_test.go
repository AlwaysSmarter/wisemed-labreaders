package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuthenticationRefreshIdentityAndExpiry(t *testing.T) {
	cfg := testConfig(t)
	current := claimsFor("a", "browser", "operator")
	for _, tc := range []struct {
		name    string
		change  func(*AuthClaims)
		allowed bool
	}{
		{"same identity", func(c *AuthClaims) {}, true},
		{"changed subject", func(c *AuthClaims) { c.Subject = "other" }, false},
		{"changed client", func(c *AuthClaims) { c.ClientID = "other" }, false},
		{"changed scopes", func(c *AuthClaims) { c.Scopes = c.Scopes[:1] }, false},
		{"no extension", func(c *AuthClaims) { c.ExpiresAt = current.ExpiresAt }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := current
			next.ExpiresAt = jwt.NewNumericDate(time.Now().Add(10 * time.Minute))
			tc.change(&next)
			_, err := refreshClaims(cfg, &current, Envelope{Type: "auth.refresh", RequestID: "renew", Payload: map[string]interface{}{"token": signed(t, cfg, next, "a-key")}})
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%t err=%v", tc.allowed, err)
			}
		})
	}
	current.ControlEquipmentID = "42"
	next := claimsFor("a", "browser", "operator")
	next.ExpiresAt = jwt.NewNumericDate(time.Now().Add(10 * time.Minute))
	if _, err := refreshClaims(cfg, &current, Envelope{RequestID: "renew", Payload: map[string]interface{}{"token": signed(t, cfg, next, "a-key")}}); err == nil {
		t.Fatal("control ticket renewed")
	}
}
func TestAuthenticationRefreshKeepsConnectionAndSubscriptions(t *testing.T) {
	f := newFixture(t)
	browser, bid := f.connect(t, "a", "browser", "operator")
	reader, _ := f.connect(t, "a", "reader", "utility")
	send(t, browser, Envelope{Type: "subscribe", RequestID: "topic", Payload: map[string]interface{}{"topic": "logs:utility"}})
	readType(t, browser, "subscribe_ack")
	observer, _ := f.s.hub.observe("a", []string{"all"})
	defer f.s.hub.unobserve(observer)
	next := claimsFor("a", "browser", "operator")
	next.ExpiresAt = jwt.NewNumericDate(time.Now().Add(10 * time.Minute))
	send(t, browser, Envelope{Type: "auth.refresh", RequestID: "renew", Payload: map[string]interface{}{"token": signed(t, f.cfg, next, "a-key")}})
	if readType(t, browser, "auth_refreshed").CorrelationID != "renew" {
		t.Fatal("renewal correlation")
	}
	select {
	case <-observer.messages:
		t.Fatal("authentication leaked into traffic feed")
	default:
	}
	send(t, reader, Envelope{Type: "event", RequestID: "direct", Target: &Target{Mode: "connection", ConnectionID: bid}, Payload: map[string]interface{}{"event": "ecard.event"}})
	if readType(t, browser, "event").RequestID != "direct" {
		t.Fatal("connection changed")
	}
	send(t, reader, Envelope{Type: "event", RequestID: "topic-event", Target: &Target{Mode: "topic", Topic: "logs:utility"}, Payload: map[string]interface{}{"event": "utility.status"}})
	if readType(t, browser, "event").RequestID != "topic-event" {
		t.Fatal("topic subscription lost")
	}
}

func TestAuthenticationRefreshSurvivesOriginalExpiry(t *testing.T) {
	f := newFixture(t)
	short := claimsFor("a", "browser", "operator")
	short.ExpiresAt = jwt.NewNumericDate(time.Now().Add(3 * time.Second))
	socket, _, err := f.dialer.Dial("wss"+strings.TrimPrefix(f.http.URL, "https")+"/ws", http.Header{"Authorization": []string{"Bearer " + signed(t, f.cfg, short, "a-key")}})
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	send(t, socket, Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "browser", "client_id": "operator"}})
	before := readType(t, socket, "hello_ack")
	if before.Payload["auth_refresh"] != true {
		t.Fatal("renewal capability missing")
	}
	next := claimsFor("a", "browser", "operator")
	send(t, socket, Envelope{Type: "auth.refresh", RequestID: "renew", Payload: map[string]interface{}{"token": signed(t, f.cfg, next, "a-key")}})
	readType(t, socket, "auth_refreshed")
	time.Sleep(time.Until(short.ExpiresAt.Time) + 100*time.Millisecond)
	send(t, socket, Envelope{Type: "ping", RequestID: "still-connected"})
	readType(t, socket, "pong")
}
