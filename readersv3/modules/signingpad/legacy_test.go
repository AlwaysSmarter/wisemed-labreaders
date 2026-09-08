package signingpad

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeLegacyPad struct {
	fakePad
	events  chan padEvent
	started chan struct{}
}

func (p *fakeLegacyPad) BeginLegacy(string) (<-chan padEvent, error) {
	close(p.started)
	return p.events, nil
}
func (p *fakeLegacyPad) SignData() ([]byte, error) { return []byte("vendor signature data"), nil }

func TestLegacyWiseMEDProtocol(t *testing.T) {
	rt := &testRuntime{dir: t.TempDir(), mux: http.NewServeMux(), settings: map[string]interface{}{"sigenc_format": "signotec-sign-data-base64", "session_timeout_seconds": 2}}
	m := &Module{}
	if err := m.Init(rt); err != nil {
		t.Fatal(err)
	}
	pad := &fakeLegacyPad{fakePad: fakePad{closed: make(chan struct{})}, events: make(chan padEvent, 2), started: make(chan struct{})}
	m.driverFactory = func(string) (padDriver, error) { return pad, nil }
	server := httptest.NewServer(rt.mux)
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.WriteMessage(1, []byte(`{"cmd":"init"}`))
	var init legacyResponse
	if err := conn.ReadJSON(&init); err != nil || !init.Success {
		t.Fatal(init, err)
	}
	raw := `{"cmd":"signpatient","sig_type":"gdpr","pacient_id":9007199254740993,"nume_pacient":"Test Only","cnp_pacient":"NOT-A-REAL-CNP"}`
	conn.WriteMessage(1, []byte(raw))
	select {
	case <-pad.started:
	case <-time.After(time.Second):
		t.Fatal("capture did not start")
	}
	replies := make(chan legacyResponse, 1)
	failures := make(chan error, 1)
	go func() {
		var reply legacyResponse
		if err := conn.ReadJSON(&reply); err != nil {
			failures <- err
			return
		}
		replies <- reply
	}()
	pad.events <- padEvent{Action: "retry"}
	select {
	case reply := <-replies:
		t.Fatal("premature signpatient reply", reply)
	case err := <-failures:
		t.Fatal(err)
	case <-time.After(30 * time.Millisecond):
	}
	pad.events <- padEvent{Action: "confirm"}
	select {
	case reply := <-replies:
		if !reply.Success {
			t.Fatal(reply)
		}
		var actual, expected bytes.Buffer
		json.Compact(&actual, reply.ForEvent)
		json.Compact(&expected, []byte(raw))
		if actual.String() != expected.String() {
			t.Fatal("forevent changed", actual.String())
		}
		if reply.Data["sigbase64"] != base64.StdEncoding.EncodeToString([]byte("PNG bytes")) || reply.Data["sigenc"] != base64.StdEncoding.EncodeToString([]byte("vendor signature data")) {
			t.Fatal(reply)
		}
	case err := <-failures:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("missing completed response")
	}
	select {
	case <-pad.closed:
	case <-time.After(time.Second):
		t.Fatal("pad not released")
	}
	// Old clients keep their WebSocket open longer than one capture timeout.
	time.Sleep(2100 * time.Millisecond)
	conn.WriteMessage(1, []byte(`{"cmd":"init"}`))
	if err := conn.ReadJSON(&init); err != nil || !init.Success {
		t.Fatal("idle legacy connection closed", err)
	}
}

func TestLegacySigencRequiresKnownFormat(t *testing.T) {
	rt := &testRuntime{dir: t.TempDir(), mux: http.NewServeMux(), settings: map[string]interface{}{}}
	m := &Module{}
	m.Init(rt)
	m.driverFactory = func(string) (padDriver, error) {
		t.Error("must not start capture with unknown sigenc format")
		return nil, nil
	}
	server := httptest.NewServer(rt.mux)
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.WriteMessage(1, []byte(`{"cmd":"signpatient","sig_type":1,"pacient_id":"p1","nume_pacient":"Test"}`))
	conn.SetReadDeadline(time.Now().Add(time.Second))
	var response legacyResponse
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	if response.Success || !strings.Contains(response.Error, "sigenc") {
		t.Fatal(response)
	}
}
