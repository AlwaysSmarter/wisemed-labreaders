package ecard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

func TestOperationReportContainsFailedAndFinalSDK(t *testing.T) {
	m := testModule(t)
	req := testRequest()
	req.Operation = "read"
	seed(t, m.store, "report", req, 0)
	count := 0
	m.factory = func(context.Context, string, Settings) (backend, error) {
		count++
		if count == 1 {
			return &fakeBackend{open: Reply{Code: "sdk_error", SDKCode: -29, HostVersion: "new-host"}}, nil
		}
		return &fakeBackend{open: Reply{OK: true, Data: json.RawMessage(`{"A1":"card"}`), HostVersion: "new-host"}}, nil
	}
	m.run(context.Background(), "report")
	j, _ := m.store.job("report", "owner")
	out, err := m.operationReport("owner", j)
	if err != nil {
		t.Fatal(err)
	}
	groups := out["sdk_responses"].([]interface{})
	if len(groups) != 2 || out["complete"] != true || out["events_complete"] != true {
		t.Fatalf("incomplete report: %+v", out)
	}
	final := out["final_response"].(map[string]interface{})
	if final["sdk_used"] != "2.0.0.3" || final["ok"] != true || len(final["results"].([]interface{})) != 1 || final["data"].(map[string]interface{})["A1"] != "card" {
		t.Fatal(final)
	}
	first := groups[0].(map[string]interface{})
	if len(first["errors"].([]interface{})) == 0 || first["request"] == nil {
		t.Fatal(first)
	}
	changed := false
	for _, v := range out["events"].([]interface{}) {
		e := v.(map[string]interface{})
		if e["event"] == "sdk_changed" {
			changed = true
		}
	}
	if !changed {
		t.Fatal("missing sdk_changed")
	}
	if _, err = m.operationReport("other", j); err == nil {
		t.Fatal("report leaked to other owner")
	}
	// The global ring may expire; the complete job report must survive it.
	m.store.db.Exec(`DELETE FROM events`)
	out, err = m.operationReport("owner", j)
	if err != nil || out["events_complete"] != true {
		t.Fatalf("job events lost with global ring: %v %v", out, err)
	}
}

func TestCreateOperationQueuesOnceAndReturnsReportPath(t *testing.T) {
	m := testModule(t)
	req := testRequest()
	req.Operation = "read"
	request, _ := json.Marshal(map[string]interface{}{"operation_id": "ccfd04dc-9255-4ef5-ae0b-d9d680d9ad77", "request": req})
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		m.createOperation(w, httptest.NewRequest("POST", "/api/ecard/operations", bytes.NewReader(request)), "owner", false)
		if w.Code != 202 || !strings.Contains(w.Body.String(), "report_path") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if len(m.queue) != 1 {
		t.Fatal("operation replayed")
	}
	w := httptest.NewRecorder()
	m.createOperation(w, httptest.NewRequest("POST", "/api/ecard/operations", bytes.NewReader(request)), "other", false)
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
}

type eventPublisher struct {
	connections []string
	payloads    []map[string]interface{}
}

func (p *eventPublisher) PublishEventToConnection(c string, v map[string]interface{}) error {
	p.connections = append(p.connections, c)
	p.payloads = append(p.payloads, v)
	return nil
}

type eventRuntime struct {
	module.Runtime
	publisher *eventPublisher
	level     int
}

func (r eventRuntime) Service(name string) (interface{}, bool) {
	return r.publisher, name == "wisemed-ws-client"
}
func (r eventRuntime) ModuleSettings(string) map[string]interface{} {
	return map[string]interface{}{"verbose_level": r.level}
}
func (r eventRuntime) ReaderID() string            { return "reader" }
func (r eventRuntime) Logf(string, ...interface{}) {}

func TestWSMEventSubscriptionIsAttestedAndOwnerScoped(t *testing.T) {
	m := testModule(t)
	p := &eventPublisher{}
	m.rt = eventRuntime{publisher: p, level: 5}
	call := func(subject, connection string) {
		result, err := apibridge.Invoke(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.subscribeEvents(w, r, "wsm:tenant:"+subject) }), apibridge.Principal{Subject: subject, TenantID: "tenant", ConnectionID: connection}, apibridge.Request{Method: "POST", Path: "/api/ecard/events/subscribe", Body: json.RawMessage(`{"hardware":true}`)})
		if err != nil || result.Status != 200 {
			t.Fatalf("subscribe: %v %+v", err, result)
		}
	}
	call("a", "connection-a")
	call("b", "connection-b")
	m.emitEvent("wsm:tenant:a", "job", "2.0.0.4", "sdk_selected", nil)
	if len(p.connections) != 1 || p.connections[0] != "connection-a" {
		t.Fatal(p.connections)
	}
	m.emitEvent("", "", "pcsc", "reader_connected", nil)
	if len(p.connections) != 3 {
		t.Fatal("hardware not delivered to subscribed connections", p.connections)
	}
	w := httptest.NewRecorder()
	m.subscribeEvents(w, httptest.NewRequest("POST", "/api/ecard/events/subscribe", strings.NewReader(`{}`)), "local:a")
	if w.Code != 400 {
		t.Fatal("unattested subscription accepted")
	}
}

func TestWSMCompletionCarriesUnifiedReport(t *testing.T) {
	m := testModule(t)
	p := &eventPublisher{}
	m.rt = eventRuntime{publisher: p, level: 5}
	m.bus.subscriptions = map[string]eventSubscription{"sub": {Owner: "owner", Connection: "attested-connection", Expires: time.Now().Add(time.Minute)}}
	req := testRequest()
	req.Operation = "read"
	seed(t, m.store, "complete", req, 0)
	m.factory = func(context.Context, string, Settings) (backend, error) {
		return &fakeBackend{open: Reply{OK: true, Data: json.RawMessage(`{"A1":"test"}`)}}, nil
	}
	m.run(context.Background(), "complete")
	var report map[string]interface{}
	for _, payload := range p.payloads {
		if v, ok := payload["operation_report"].(map[string]interface{}); ok {
			report = v
		}
	}
	if report == nil || report["state"] != "completed" || report["response"] == nil || report["used_sdk"] == nil {
		t.Fatalf("missing final report: %+v", p.payloads)
	}
}

func TestReportRecoversLegacyEventsAndMarksIncomplete(t *testing.T) {
	m := testModule(t)
	seed(t, m.store, "old", testRequest(), 0)
	m.store.state("old", "failed", "sdk_error")
	blob, _ := m.store.seal(map[string]interface{}{"job": "old", "event": "read_result", "source": "2.0.0.4"}, "ecard-event:owner")
	m.store.db.Exec(`INSERT INTO events(owner,payload) VALUES('owner',?)`, blob)
	j, _ := m.store.job("old", "owner")
	out, err := m.operationReport("owner", j)
	if err != nil || len(out["events"].([]interface{})) != 1 || out["events_complete"] != false || out["diagnostic_warning"] == nil {
		t.Fatalf("legacy recovery: %+v %v", out, err)
	}
}

func TestLargeReportIsReassembledWithoutLossOverWSM(t *testing.T) {
	m := testModule(t)
	req := testRequest()
	req.Operation = "read"
	seed(t, m.store, "large", req, 0)
	data, _ := json.Marshal(map[string]string{"A1": strings.Repeat("x", 3400000)})
	m.factory = func(context.Context, string, Settings) (backend, error) {
		return &fakeBackend{open: Reply{OK: true, Data: data}}, nil
	}
	m.run(context.Background(), "large")
	j, _ := m.store.job("large", "owner")
	invoke := func(path string) apibridge.Response {
		out, err := apibridge.Invoke(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.reportHTTP(w, r, "owner", j) }), apibridge.Principal{Subject: "owner", TenantID: "tenant"}, apibridge.Request{Method: "GET", Path: path})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	root := "/api/ecard/jobs/large/report"
	if out := invoke(root); out.Status != 413 {
		t.Fatalf("missing explicit WSM size error: %d", out.Status)
	}
	var assembled []byte
	var checksum string
	for part := 0; ; part++ {
		out := invoke(root + "?part=" + strconv.Itoa(part))
		if out.Status != 200 {
			t.Fatal(out.Status)
		}
		raw, _ := json.Marshal(out.Body)
		var chunk struct {
			Parts int    `json:"parts"`
			Data  string `json:"data_base64"`
			SHA   string `json:"sha256"`
		}
		if err := json.Unmarshal(raw, &chunk); err != nil {
			t.Fatal(err)
		}
		b, err := base64.StdEncoding.DecodeString(chunk.Data)
		if err != nil {
			t.Fatal(err)
		}
		assembled = append(assembled, b...)
		if checksum != "" && checksum != chunk.SHA {
			t.Fatal("report changed between parts")
		}
		checksum = chunk.SHA
		if part+1 == chunk.Parts {
			break
		}
	}
	sum := sha256.Sum256(assembled)
	if checksum != hex.EncodeToString(sum[:]) {
		t.Fatal("report checksum mismatch")
	}
	var report map[string]interface{}
	if err := json.Unmarshal(assembled, &report); err != nil {
		t.Fatal(err)
	}
	value := report["response"].(map[string]interface{})["data"].(map[string]interface{})["A1"]
	if value != strings.Repeat("x", 3400000) {
		t.Fatal("report data lost")
	}
}

func TestWSMMinimalEventsUnlessVerbose(t *testing.T) {
	for _, level := range []int{0, 1, 4, 5, 6} {
		m := testModule(t)
		p := &eventPublisher{}
		m.rt = eventRuntime{publisher: p, level: level}
		m.bus.subscriptions = map[string]eventSubscription{"sub": {Owner: "owner", Connection: "browser", Hardware: true, Expires: time.Now().Add(time.Minute)}}
		for _, kind := range []string{"sdk_selected", "sdk_attempt_finished", "operation_progress"} {
			m.publishEvent("owner", map[string]interface{}{"event": kind, "source": "sdk", "job": "job"})
		}
		expected := 0
		if level >= 5 {
			expected = 3
		}
		if len(p.payloads) != expected {
			t.Fatalf("level %d diagnostic events: %d", level, len(p.payloads))
		}
		m.publishEvent("owner", map[string]interface{}{"event": "pin_required", "source": "sdk", "job": "job"})
		m.publishEvent("owner", map[string]interface{}{"event": "operation_finished", "source": "worker"})
		for _, kind := range []string{"card_inserted", "card_removed", "reader_connected", "reader_disconnected", "monitor_unavailable"} {
			m.publishEvent("", map[string]interface{}{"event": kind, "source": "pcsc"})
		}
		if len(p.payloads) != expected+7 {
			t.Fatalf("level %d missing essential event", level)
		}
	}
}
