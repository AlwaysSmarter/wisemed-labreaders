package ecard

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEventsOwnershipRedactionAndCursor(t *testing.T) {
	m := testModule(t)
	m.rt = eventRuntime{publisher: &eventPublisher{}, level: 5}
	m.emitEvent("", "", "pcsc", "card_inserted", map[string]string{"reader": "Reader"})
	m.emitEvent("local:a", "job-a", "sdk", "authentication", map[string]string{"token": "secret-token", "pin": "secret-pin", "state": "offline"})
	m.emitEvent("local:b", "job-b", "sdk", "read", map[string]string{"patient": "other-user"})
	fetch := func(path string) map[string]interface{} {
		w := httptest.NewRecorder()
		m.events(w, httptest.NewRequest("GET", path, nil), "local:a")
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var out map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &out)
		if strings.Contains(w.Body.String(), "secret-") || strings.Contains(w.Body.String(), "other-user") {
			t.Fatal("event data leaked")
		}
		return out
	}
	out := fetch("/api/ecard/events")
	if len(out["items"].([]interface{})) != 2 {
		t.Fatal(out)
	}
	out = fetch("/api/ecard/events?after=2")
	if len(out["items"].([]interface{})) != 0 {
		t.Fatal(out)
	}
	var raw []byte
	m.store.db.QueryRow(`SELECT payload FROM events WHERE id=2`).Scan(&raw)
	if strings.Contains(string(raw), "offline") {
		t.Fatal("event is not encrypted")
	}
}
func TestEventPayloadBound(t *testing.T) {
	m := testModule(t)
	m.rt = eventRuntime{publisher: &eventPublisher{}, level: 5}
	m.emitEvent("", "", "sdk", "read", strings.Repeat("x", 70000))
	w := httptest.NewRecorder()
	m.events(w, httptest.NewRequest("GET", "/api/ecard/events", nil), "local:a")
	if w.Body.Len() > 1000 || !strings.Contains(w.Body.String(), "exceeds 64 KiB") {
		t.Fatal("event cap failed")
	}
}

func TestEventCategories(t *testing.T) {
	for _, tc := range []struct{ source, kind, want string }{
		{"pcsc", "card_inserted", "pcsc"},
		{"worker", "operation_finished", "operation"},
		{"2.0.0.4", "pin_required", "authentication"},
		{"2.0.0.4", "StareAutentificareSchimbata", "authentication"},
		{"2.0.0.4", "StareComunicatieCuUMSchimbata", "um"},
		{"2.0.0.4", "um_token_request_finished", "um"},
		{"2.0.0.4", "call_started", "sdk"},
	} {
		if got := eventType(tc.source, tc.kind); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.kind, got, tc.want)
		}
	}
}

func TestLiveEventLogFollowsUtilityVerbosityAndAdvancesCursor(t *testing.T) {
	m := testModule(t)
	m.rt = eventRuntime{publisher: &eventPublisher{}, level: 1}
	m.emitEvent("", "", "pcsc", "card_inserted", nil)
	m.emitEvent("local:a", "job", "sdk", "sdk_selected", nil)
	m.emitEvent("local:a", "job", "sdk", "sdk_attempt_finished", nil)
	fetch := func() map[string]interface{} {
		w := httptest.NewRecorder()
		m.events(w, httptest.NewRequest("GET", "/api/ecard/events", nil), "local:a")
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var out map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	out := fetch()
	if len(out["items"].([]interface{})) != 1 || out["cursor"] != float64(3) {
		t.Fatal(out)
	}
	m.rt = eventRuntime{publisher: &eventPublisher{}, level: 5}
	out = fetch()
	if len(out["items"].([]interface{})) != 3 {
		t.Fatal(out)
	}
}
