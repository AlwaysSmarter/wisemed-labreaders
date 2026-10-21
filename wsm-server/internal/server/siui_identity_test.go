package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"wisemed-labreaders/serverlast/wsm-server/internal/config"
)

func TestSIUIHelloAuthorization(t *testing.T) {
	for _, role := range []string{"reader", "browser", "service"} {
		c := claimsFor("a", role, "siui-client")
		h := HelloPayload{ClientType: "siui-bridge", ClientID: c.ClientID, ReaderID: c.ReaderID, EquipmentID: c.EquipmentID}
		err := validateHelloAgainstClaims(h, &c)
		if (err == nil) != (role == "service") {
			t.Fatalf("role %s: %v", role, err)
		}
		if role == "service" {
			h.EquipmentID = "different-equipment"
			if validateHelloAgainstClaims(h, &c) == nil {
				t.Fatal("equipment impersonation accepted")
			}
		}
	}
}

func TestSIUIServiceToken(t *testing.T) {
	cfg := testConfig(t)
	c := claimsFor("a", "service", "siui-client")
	c.ReaderID = "siui-bridge"
	c.EquipmentID = "equipment-siui"
	if _, err := authenticate(cfg, authRequest(signed(t, cfg, c, "a-key")), false); err != nil {
		t.Fatal(err)
	}
	c.EquipmentID = ""
	if _, err := authenticate(cfg, authRequest(signed(t, cfg, c, "a-key")), false); err == nil {
		t.Fatal("equipment service accepted without equipment identity")
	}
}

func TestSIUIGeneratedDeviceAuthentication(t *testing.T) {
	cfg, err := testConfig(t).WithDevices(map[string]config.Device{
		"siui-key": {TenantID: "a", EquipmentID: "18", ReaderID: "siui-bridge", Subject: "siui-device", Secret: strings.Repeat("s", 40)},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := claimsFor("a", "service", "siui-bridge")
	c.Subject = "siui-device"
	c.ReaderID = "siui-bridge"
	c.EquipmentID = "18"
	c.Scopes = config.DeviceScopes()
	check := func(wantOK bool) {
		t.Helper()
		_, err := authenticate(cfg, authRequest(signed(t, cfg, c, "siui-key")), false)
		if (err == nil) != wantOK {
			t.Fatalf("authentication success=%v, want %v: %v", err == nil, wantOK, err)
		}
	}
	check(true)
	s := New(cfg)
	defer s.Close()
	host := httptest.NewTLSServer(s.routes())
	defer host.Close()
	dialer := websocket.Dialer{TLSClientConfig: host.Client().Transport.(*http.Transport).TLSClientConfig.Clone()}
	dialer.TLSClientConfig.MinVersion = tls.VersionTLS12
	ws, _, err := dialer.Dial("wss"+strings.TrimPrefix(host.URL, "https")+"/ws", http.Header{"Authorization": {"Bearer " + signed(t, cfg, c, "siui-key")}})
	if err != nil {
		t.Fatal("generated SIUI key handshake:", err)
	}
	defer ws.Close()
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := ws.WriteJSON(Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "siui-bridge", "client_id": c.ClientID, "reader_id": c.ReaderID, "equipment_id": c.EquipmentID}}); err != nil {
		t.Fatal(err)
	}
	var ack Envelope
	if err := ws.ReadJSON(&ack); err != nil || ack.Type != "hello_ack" {
		t.Fatalf("generated SIUI key hello: type=%s, error=%v", ack.Type, err)
	}
	c.Role = "reader"
	check(true)
	c.Role = "browser"
	check(false)
	c.Role = "service"
	c.EquipmentID = "19"
	check(false)
	c.EquipmentID = "18"
	c.ReaderID = "other-reader"
	check(false)
	c.ReaderID = "siui-bridge"
	c.Scopes = append(c.Scopes, "api:invoke")
	check(false)
}

func TestHistoryGeneratedDeviceAuthentication(t *testing.T) {
	cfg, err := testConfig(t).WithDevices(map[string]config.Device{
		"siui-key": {TenantID: "a", EquipmentID: "18", ReaderID: "siui-bridge-history", Subject: "siui-device", Secret: strings.Repeat("s", 40)},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := claimsFor("a", "service", "siui-bridge-history")
	c.Subject = "siui-device"
	c.ReaderID = "siui-bridge-history"
	c.EquipmentID = "18"
	c.Scopes = config.DeviceScopes()
	check := func(wantOK bool) {
		t.Helper()
		_, err := authenticate(cfg, authRequest(signed(t, cfg, c, "siui-key")), false)
		if (err == nil) != wantOK {
			t.Fatalf("authentication success=%v, want %v: %v", err == nil, wantOK, err)
		}
	}
	check(true)
	s := New(cfg)
	defer s.Close()
	host := httptest.NewTLSServer(s.routes())
	defer host.Close()
	dialer := websocket.Dialer{TLSClientConfig: host.Client().Transport.(*http.Transport).TLSClientConfig.Clone()}
	dialer.TLSClientConfig.MinVersion = tls.VersionTLS12
	ws, _, err := dialer.Dial("wss"+strings.TrimPrefix(host.URL, "https")+"/ws", http.Header{"Authorization": {"Bearer " + signed(t, cfg, c, "siui-key")}})
	if err != nil {
		t.Fatal("generated SIUI key handshake:", err)
	}
	defer ws.Close()
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := ws.WriteJSON(Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "siui-bridge-history", "client_id": c.ClientID, "reader_id": c.ReaderID, "equipment_id": c.EquipmentID}}); err != nil {
		t.Fatal(err)
	}
	var ack Envelope
	if err := ws.ReadJSON(&ack); err != nil || ack.Type != "hello_ack" {
		t.Fatalf("generated SIUI key hello: type=%s, error=%v", ack.Type, err)
	}
	c.Role = "reader"
	check(true)
	c.Role = "browser"
	check(false)
	c.Role = "service"
	c.EquipmentID = "19"
	check(false)
	c.EquipmentID = "18"
	c.ReaderID = "other-reader"
	check(false)
	c.ReaderID = "siui-bridge-history"
	c.Scopes = append(c.Scopes, "api:invoke")
	check(false)
}

func TestHistoryHelloRequiresServiceAndExactIdentity(t *testing.T) {
	for _, role := range []string{"reader", "browser", "service"} {
		c := claimsFor("a", role, "history")
		c.ReaderID = "siui-bridge-history"
		c.EquipmentID = "19"
		h := HelloPayload{ClientType: "siui-bridge-history", ClientID: c.ClientID, ReaderID: c.ReaderID, EquipmentID: c.EquipmentID}
		err := validateHelloAgainstClaims(h, &c)
		if (err == nil) != (role == "service") {
			t.Fatalf("role=%s error=%v", role, err)
		}
		h.EquipmentID = "20"
		if validateHelloAgainstClaims(h, &c) == nil {
			t.Fatal("equipment impersonation accepted")
		}
	}
}
