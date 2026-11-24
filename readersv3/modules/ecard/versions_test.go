package ecard

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"wisemed-labreaders/readersv3/core/module"
)

type versionRuntime struct {
	module.Runtime
	level int
}

func (r versionRuntime) ModuleSettings(string) map[string]interface{} {
	return map[string]interface{}{"verbose_level": r.level}
}
func TestVersionsOnlyOnVerboseFive(t *testing.T) {
	for _, level := range []int{0, 4, 5} {
		m := testModule(t)
		m.rt = versionRuntime{level: level}
		m.guard = guard{user: "user"}
		w := httptest.NewRecorder()
		m.handle(w, httptest.NewRequest("GET", "/api/ecard/unknown", nil))
		var out map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &out)
		_, present := out["versions"]
		if present != (level == 5) {
			t.Fatalf("level %d: %s", level, w.Body.String())
		}
		if (w.Header().Get("X-ECard-App-Version") != "") != (level == 5) {
			t.Fatal("headers disagree")
		}
		if out["message"] == nil {
			t.Fatal("missing error explanation")
		}
	}
}
func TestSDKErrorExplanation(t *testing.T) {
	w := httptest.NewRecorder()
	respond(w, 200, map[string]interface{}{"response": Reply{Code: "sdk_error", SDKCode: -29}})
	var out struct {
		Response struct {
			Message string `json:"message"`
		} `json:"response"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Response.Message != sdkErrorDescriptions[-29] || out.Response.Message == "" {
		t.Fatal(w.Body.String())
	}
}

func TestFailedJobWithoutSDKCodeNeverReportsSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	respond(w, 200, map[string]interface{}{"job": Job{State: "failed", Error: "sdk_error"}})
	var out struct {
		Job struct {
			Message      string `json:"message"`
			ErrorMessage string `json:"error_message"`
		} `json:"job"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Job.Message == "" || out.Job.Message == sdkErrorDescriptions[0] || out.Job.ErrorMessage == sdkErrorDescriptions[0] {
		t.Fatal(w.Body.String())
	}
}
