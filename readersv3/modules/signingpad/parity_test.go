package signingpad

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRemoteCaptureOwnsPadAndExpires(t *testing.T) {
	rt := &testRuntime{dir: t.TempDir(), mux: http.NewServeMux(), settings: map[string]interface{}{"session_timeout_seconds": 1, "log_db_path": "./history.db"}}
	m := &Module{}
	if err := m.Init(rt); err != nil {
		t.Fatal(err)
	}
	defer m.history.Close()
	closed := make(chan struct{})
	m.driverFactory = func(string) (padDriver, error) { return &fakePad{closed: closed}, nil }
	response, handled, err := m.HandleWSAction("esignature.start", map[string]interface{}{"session_id": "1234567890123456"})
	if err != nil || !handled || response["ok"] != true {
		t.Fatal(response, handled, err)
	}
	local := &padSession{m: m}
	if r := local.execute(padCommand{Action: "start"}); r["ok"] != false {
		t.Fatal("local client stole remote pad")
	}
	if _, _, err := m.HandleWSAction("esignature.confirm", map[string]interface{}{"session_id": "another-session-id"}); err == nil {
		t.Fatal("different session confirmed capture")
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("remote timeout did not close device")
	}
	recorder := httptest.NewRecorder()
	m.handleHistory(recorder, httptest.NewRequest("GET", "http://localhost/api/esignature/jobs", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "start") {
		t.Fatal(recorder.Body.String())
	}
}
