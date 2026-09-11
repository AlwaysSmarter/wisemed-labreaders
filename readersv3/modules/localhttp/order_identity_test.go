package localhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"wisemed-labreaders/readersv3/core/module"
	model "wisemed-labreaders/readersv3/modules/core/model"
)

type identityStoreStub struct {
	actor  string
	change model.OrderIDChange
}

func (s *identityStoreStub) CorrectOrderID(c model.OrderIDChange, a string) (model.Order, error) {
	s.actor = a
	s.change = c
	return model.Order{ID: c.OrderID, SampleID: c.NewID}, nil
}

type identityRuntimeStub struct {
	module.Runtime
	store *identityStoreStub
}

func (r identityRuntimeStub) Service(string) (interface{}, bool)         { return r.store, true }
func (identityRuntimeStub) ReaderID() string                             { return "test-reader" }
func (identityRuntimeStub) ModuleSettings(string) map[string]interface{} { return nil }
func TestIDChangeUsesAuthenticatedActor(t *testing.T) {
	store := &identityStoreStub{}
	m := &Module{rt: identityRuntimeStub{store: store}, sessions: map[string]session{"logged": {Username: "ana", FirstName: "Ana", LastName: "Pop", ExpiresAt: time.Now().Add(time.Hour)}}}
	body := `{"order_id":1,"expected_id":"NAME","new_id":"123","confirmation":"deacord","actor":"forged"}`
	req := httptest.NewRequest(http.MethodPut, "/api/orders/id", strings.NewReader(body))
	w := httptest.NewRecorder()
	m.handleOrderIDChange(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatal(w.Code)
	}
	req = httptest.NewRequest(http.MethodPut, "/api/orders/id", strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: legacySessionCookieName, Value: "logged"})
	w = httptest.NewRecorder()
	m.handleOrderIDChange(w, req)
	if w.Code != http.StatusOK || store.actor != "Ana Pop" {
		t.Fatal(w.Code, store.actor, w.Body.String())
	}
}
