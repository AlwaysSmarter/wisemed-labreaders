package ecard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompletedJobsDoNotConsumeCapacity(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(fmt.Sprint(batch), func(t *testing.T) {
			m := testModule(t)
			m.guard = guard{user: "user"}
			m.settings.MaxJobs = 1
			for i, state := range []string{"completed", "failed", "cancelled", "unknown", "interrupted"} {
				id := fmt.Sprint(i)
				seed(t, m.store, id, testRequest(), 0)
				if e := m.store.state(id, state, ""); e != nil {
					t.Fatal(e)
				}
			}
			var body interface{} = testRequest()
			path := "/api/ecard/jobs"
			want := 201
			if batch {
				path = "/api/ecard/sign/batch?async=true"
				want = 202
				body = map[string]interface{}{"operation_id": "ed6eae56-069d-464e-bfa8-0dafb760e10a", "request": testRequest(), "items": []Item{{ID: "a", Text: "hello"}}}
			}
			b, _ := json.Marshal(body)
			r := httptest.NewRequest("POST", path, bytes.NewReader(b))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			m.handle(w, r)
			if w.Code != want {
				t.Fatalf("completed jobs blocked admission: %d %s", w.Code, w.Body.String())
			}
		})
	}
}
func TestHistoryEncryptedRedactedAndOwnerScoped(t *testing.T) {
	m := testModule(t)
	r := httptest.NewRequest("POST", "/api/ecard/jobs/example/pin", strings.NewReader(`{"value":"123456","challenge":"private-challenge"}`))
	w, finish := m.recordCall(httptest.NewRecorder(), r, "local:user")
	io.Copy(io.Discard, r.Body)
	respond(w, 200, map[string]interface{}{"ok": true, "token": "private-token"})
	finish()
	var raw []byte
	if e := m.store.db.QueryRow(`SELECT payload FROM call_history`).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(raw, []byte("response")) {
		t.Fatal("history stored unencrypted")
	}
	for _, owner := range []string{"local:user", "local:other"} {
		out := httptest.NewRecorder()
		m.history(out, httptest.NewRequest("GET", "/api/ecard/history", nil), owner)
		s := out.Body.String()
		if strings.Contains(s, "123456") || strings.Contains(s, "private-token") || strings.Contains(s, "private-challenge") {
			t.Fatal("history leaked credentials")
		}
		var data struct {
			Items []interface{} `json:"items"`
		}
		json.Unmarshal(out.Body.Bytes(), &data)
		if (owner == "local:user" && len(data.Items) != 1) || (owner == "local:other" && len(data.Items) != 0) {
			t.Fatal("owner isolation failed", s)
		}
	}
}
func TestHistoryOversizeIsOmitted(t *testing.T) {
	b := &callCapture{}
	b.Write([]byte(strings.Repeat("x", 65537)))
	if capturedJSON(b) != "[payload exceeds 64 KiB; omitted]" {
		t.Fatal("oversize payload retained")
	}
}

func TestHistoryPreservesDiagnosticFlagsWithoutSecrets(t *testing.T) {
	value := map[string]interface{}{"pin_attempted": true, "_offline_token": false, "pin": "1234", "token": "secret", "nested": map[string]interface{}{"pin_attempted": "not a flag"}}
	out := cleanHistory(value).(map[string]interface{})
	if out["pin_attempted"] != true || out["_offline_token"] != false {
		t.Fatalf("diagnostic flags lost: %v", out)
	}
	if out["pin"] != "[redacted]" || out["token"] != "[redacted]" || out["nested"].(map[string]interface{})["pin_attempted"] != "[redacted]" {
		t.Fatalf("secrets not redacted: %v", out)
	}
}
