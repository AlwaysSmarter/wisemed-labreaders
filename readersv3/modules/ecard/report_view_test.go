package ecard

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadersReportUsesConfirmedSimpleFormat(t *testing.T) {
	m := testModule(t)
	req := testRequest()
	req.Operation = "readers"
	req.Fields = []string{"A1", "B1"}
	seed(t, m.store, "readers", req, 0)
	m.factory = func(context.Context, string, Settings) (backend, error) {
		return &fakeBackend{open: Reply{OK: true, Data: json.RawMessage(`["Gemalto USB Smart Card Reader 0"]`)}}, nil
	}
	m.run(context.Background(), "readers")
	j, _ := m.store.job("readers", "owner")
	w := httptest.NewRecorder()
	m.reportHTTP(w, httptest.NewRequest("GET", "/api/ecard/jobs/readers/report", nil), "owner", j)
	var out map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || out["used_sdk"] != "2.0.0.4" || out["state"] != "completed" {
		t.Fatal(w.Body.String())
	}
	for _, key := range []string{"job", "final_response", "final_response_events", "sdk_responses", "submitted_items"} {
		if _, ok := out[key]; ok {
			t.Fatalf("duplicate verbose property %s", key)
		}
	}
	if len(out["other_sdks"].([]interface{})) != 0 {
		t.Fatal("final SDK duplicated in other_sdks")
	}
	if len(out["request"].(map[string]interface{})) != 1 {
		t.Fatal("reader list includes irrelevant card/contract fields")
	}
	response := out["response"].(map[string]interface{})
	if response["ok"] != true || response["error"] != nil || len(response) != 3 || response["data"].([]interface{})[0] != "Gemalto USB Smart Card Reader 0" {
		t.Fatal(response)
	}
	if strings.Count(w.Body.String(), "Gemalto USB Smart Card Reader 0") != 1 {
		t.Fatal("reader data duplicated")
	}
	if strings.Contains(w.Body.String(), "Operația nu a reușit: configured_sdk_order") {
		t.Fatal("informational event reported as failure")
	}
}

func TestCompactReportUsesLastSDKOnSuccessAndFailure(t *testing.T) {
	for _, success := range []bool{true, false} {
		m := testModule(t)
		req := testRequest()
		req.Operation = "read"
		seed(t, m.store, "read", req, 0)
		calls := 0
		m.factory = func(context.Context, string, Settings) (backend, error) {
			calls++
			return &fakeBackend{open: Reply{OK: success && calls == 2, Code: "sdk_error", SDKCode: -29, Data: json.RawMessage(`{"A1":"last"}`)}}, nil
		}
		m.run(context.Background(), "read")
		j, _ := m.store.job("read", "owner")
		raw, err := m.operationReport("owner", j)
		if err != nil {
			t.Fatal(err)
		}
		out := compactOperationReport(raw)
		if out["used_sdk"] != "2.0.0.3" {
			t.Fatal(out)
		}
		others := out["other_sdks"].([]interface{})
		if len(others) != 1 || others[0].(map[string]interface{})["sdk"] != "2.0.0.4" {
			t.Fatal(others)
		}
		response := out["response"].(map[string]interface{})
		if response["ok"] != success {
			t.Fatal(response)
		}
		if !success && response["error"].(map[string]interface{})["sdk_code"] != float64(-29) {
			t.Fatal("lost last SDK numeric error", response)
		}
		encoded, _ := json.Marshal(out["events"])
		if strings.Contains(string(encoded), `"source":"2.0.0.4"`) {
			t.Fatal("earlier SDK events leaked into final SDK")
		}
	}
}

func TestInformationalReasonDoesNotProduceFailureMessage(t *testing.T) {
	v := map[string]interface{}{"reason": "configured_sdk_order"}
	explainErrors(v)
	if _, ok := v["message"]; ok {
		t.Fatal(v)
	}
	v = map[string]interface{}{"reason": "certificate_outside_validity_period", "valid": false}
	explainErrors(v)
	if v["message"] == nil {
		t.Fatal("real verification failure lost explanation")
	}
}

func TestWrapperWindowsDiagnosticsSurviveReplyAndCompactReport(t *testing.T) {
	var reply Reply
	input := `{"ok":false,"code":"sdk_exception","exception_type":"System.ComponentModel.Win32Exception","stage":"cnas_wrapper_initialize","message":"Windows initialization failed","diagnostics":{"native_error_code":5,"native_message":"Access is denied","initialization_step":"construct_cnas_context","apartment_state":"STA","stack_frames":["Vendor.Context..ctor"]}}`
	if err := json.Unmarshal([]byte(input), &reply); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]interface{}
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	response := compactSDKResponse(value)
	failure := response["error"].(map[string]interface{})
	diagnostics := failure["diagnostics"].(map[string]interface{})
	if failure["message"] != "Windows initialization failed" || diagnostics["native_error_code"] != float64(5) || diagnostics["initialization_step"] != "construct_cnas_context" || diagnostics["apartment_state"] != "STA" {
		t.Fatal(response)
	}
}

func TestCallEventPreservesOriginalWindowsMessage(t *testing.T) {
	items := []interface{}{map[string]interface{}{"event": "call_finished", "data": map[string]interface{}{"action": "open", "response": map[string]interface{}{"ok": false, "code": "sdk_exception", "message": "No process is on the other end of the pipe"}}}}
	compact := compactEvents(items)
	explainErrors(compact)
	data := compact[0].(map[string]interface{})["data"].(map[string]interface{})
	if data["detail"] != "No process is on the other end of the pipe" || data["message"] != errorDescription("sdk_exception", 0) || data["error_code"] != "ECARD_SDK_EXCEPTION" {
		t.Fatal(data)
	}
}
