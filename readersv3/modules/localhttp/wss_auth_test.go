package localhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wisemed-labreaders/readersv3/shared/apibridge"
)

func TestBridgeUsesExistingSessionAndActor(t *testing.T) {
	store := &identityStoreStub{}
	m := &Module{rt: identityRuntimeStub{store: store}}
	mux := http.NewServeMux()
	mux.Handle("/api/orders/id", m.requireSession(http.HandlerFunc(m.handleOrderIDChange)))
	body := json.RawMessage(`{"order_id":1,"expected_id":"NAME","new_id":"123","confirmation":"deacord","actor":"forged"}`)
	reply, e := apibridge.Invoke(context.Background(), mux, apibridge.Principal{Subject: "remote-operator", TenantID: "clinic-a", ConnectionID: "session-1"}, apibridge.Request{Method: "PUT", Path: "/api/orders/id", Body: body})
	if e != nil || reply.Status != 200 || store.actor != "remote-operator" {
		t.Fatal(reply, e, store.actor)
	}
	req := httptest.NewRequest("PUT", "/api/orders/id", strings.NewReader(string(body)))
	req.Header.Set("X-WSS-Subject", "remote-operator")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatal("HTTP header manufactured remote session")
	}
}
func TestOrdinaryLocalUserCannotDelegateRuntimeWSSPrivileges(t *testing.T) {
	m := &Module{rt: identityRuntimeStub{store: &identityStoreStub{}}, sessions: map[string]session{"ordinary": {Username: "ordinary", UserType: 1, ExpiresAt: time.Now().Add(time.Hour)}}}
	for _, path := range []string{"/api/wss/open", "/api/wss/bridge", "/api/wss/debug", "/api/wss/settings", "/api/wss/control"} {
		req := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: legacySessionCookieName, Value: "ordinary"})
		w := httptest.NewRecorder()
		m.handleWSS(w, req)
		if w.Code != 403 {
			t.Fatalf("%s got %d", path, w.Code)
		}
	}
}
