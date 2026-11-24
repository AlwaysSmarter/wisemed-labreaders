package ecard

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

type verbosityRuntime struct {
	eventRuntime
	changed int
}

func (r *verbosityRuntime) ChangeDebugVerbosity(level int) error {
	r.changed = level
	r.level = level
	return nil
}
func TestRemoteVerbosityChangeRequiresAdminAndValidLevel(t *testing.T) {
	m := testModule(t)
	m.guard = wsmGuard{}
	rt := &verbosityRuntime{eventRuntime: eventRuntime{publisher: &eventPublisher{}, level: 1}}
	m.rt = rt
	for _, tc := range []struct {
		admin  bool
		body   string
		status int
	}{
		{false, `{"verbose_level":5}`, 403}, {true, `{"verbose_level":9}`, 400}, {true, `{"verbose_level":1.5}`, 400},
		{true, `{"verbose_level":5}`, 200}, {true, `{"verbose_level":1}`, 200},
	} {
		before := rt.changed
		response, err := apibridge.Invoke(context.Background(), http.HandlerFunc(m.handle), apibridge.Principal{Subject: "user", TenantID: "tenant", ConnectionID: "browser", Admin: tc.admin}, apibridge.Request{Method: "PUT", Path: "/api/ecard/debug/verbosity", Body: json.RawMessage(tc.body)})
		if err != nil || response.Status != tc.status {
			t.Fatalf("%+v: %v %+v", tc, err, response)
		}
		if tc.status != 200 && rt.changed != before {
			t.Fatal("invalid update applied")
		}
	}
	if rt.changed != 1 || m.verboseEvents() {
		t.Fatal("verbosity not reset")
	}
}
