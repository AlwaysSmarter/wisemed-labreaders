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
	events    chan padEvent
	started   chan struct{}
	imageType string
	points    signaturePoints
}

func (p *fakeLegacyPad) BeginLegacy(string) (<-chan padEvent, error) {
	close(p.started)
	return p.events, nil
}
func (p *fakeLegacyPad) SigString() (string, error) { return "310D0A310D0A31302032300D0A300D0A", nil }
func (p *fakeLegacyPad) ConfirmLegacy(imageType string) ([]byte, error) {
	p.imageType = imageType
	return p.Confirm()
}

func TestLegacyWiseMEDProtocol(t *testing.T) {
	rt := &testRuntime{dir: t.TempDir(), mux: http.NewServeMux(), settings: map[string]interface{}{"session_timeout_seconds": 2}}
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
		if reply.Data["id"] != float64(1) {
			t.Fatal("missing legacy id", reply)
		}
		if reply.Data["sigbase64"] != base64.StdEncoding.EncodeToString([]byte("PNG bytes")) || reply.Data["sigenc"] != "310D0A310D0A31302032300D0A300D0A" {
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

func (p *fakeLegacyPad) LastActivity() time.Time { return p.points.lastActivity() }

func TestLegacyAutomaticConfirmAfterLastPoint(t *testing.T) {
	rt := &testRuntime{dir: t.TempDir(), mux: http.NewServeMux(), settings: map[string]interface{}{"session_timeout_seconds": 20}}
	m := &Module{}
	if err := m.Init(rt); err != nil {
		t.Fatal(err)
	}
	pad := &fakeLegacyPad{fakePad: fakePad{closed: make(chan struct{})}, events: make(chan padEvent, 8), started: make(chan struct{})}
	m.driverFactory = func(string) (padDriver, error) { return pad, nil }
	server := httptest.NewServer(rt.mux)
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	conn.WriteMessage(1, []byte(`{"cmd":"signpatient","sig_type":1,"pacient_id":"p1","nume_pacient":"Test","img_type":"jpg"}`))
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
		} else {
			replies <- reply
		}
	}()
	noReply := func(delay time.Duration) {
		t.Helper()
		select {
		case reply := <-replies:
			t.Fatal("premature completion", reply)
		case err := <-failures:
			t.Fatal(err)
		case <-time.After(delay):
		}
	}
	// An empty pad is never automatically confirmed.
	noReply(legacyIdleDelay + 50*time.Millisecond)
	pad.points.add(10, 20, 0)
	pad.events <- padEvent{Action: "activity"}
	noReply(1500 * time.Millisecond)
	// Simulate a coalesced callback: the timer must check the latest point even
	// without a second notification, rather than confirming after the first one.
	pad.points.add(30, 40, 100)
	noReply(1700 * time.Millisecond)
	select {
	case reply := <-replies:
		if !reply.Success {
			t.Fatal(reply)
		}
	case err := <-failures:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("automatic confirmation did not occur")
	}
	<-pad.closed
	if pad.imageType != "jpg" {
		t.Fatal("img_type ignored", pad.imageType)
	}
}
