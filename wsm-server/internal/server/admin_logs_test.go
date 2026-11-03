package server

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
	"wisemed-labreaders/serverlast/wsm-server/internal/logstore"
)

func TestAdminDailyLogAccessAndSessionRevocation(t *testing.T) {
	s, _ := adminFixture(t, `{"user_type":-1,"token":"LOGIN"}`)
	var stderr bytes.Buffer
	store, err := logstore.New(t.TempDir(), &stderr)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s.SetLogStore(store)
	store.Write([]byte("global diagnostic\nconnected tenant=a device=A\nconnected tenant=b device=PRIVATE\n"))
	if w := adminCall(s, "GET", "/admin/api/logs", "", nil, ""); w.Code != 401 {
		t.Fatal("anonymous read", w.Code)
	}
	cookie, csrf := loginAdmin(t, s, "a")
	w := adminCall(s, "GET", "/admin/api/logs", "", cookie, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "device=A") || strings.Contains(w.Body.String(), "PRIVATE") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("log caching")
	}
	if w := adminCall(s, "GET", "/admin/api/logs?date=../../secret", "", cookie, ""); w.Code != 400 {
		t.Fatal("path traversal")
	}
	if w := adminCall(s, "GET", "/admin/api/logs?cursor=-3", "", cookie, ""); w.Code != 400 {
		t.Fatal("negative cursor")
	}
	if w := adminCall(s, "GET", "/admin/api/logs/dates", "", cookie, ""); w.Code != 200 || !strings.Contains(w.Body.String(), store.Today()) {
		t.Fatal(w.Body.String())
	}
	r := httptest.NewRequest("GET", "https://other.example/admin/api/logs", nil)
	r.AddCookie(cookie)
	denied := httptest.NewRecorder()
	s.routes().ServeHTTP(denied, r)
	if denied.Code != 403 {
		t.Fatal("foreign origin")
	}
	if w := adminCall(s, "POST", "/admin/api/logout", `{}`, cookie, csrf); w.Code != 200 {
		t.Fatal("logout")
	}
	if w := adminCall(s, "GET", "/admin/api/logs", "", cookie, ""); w.Code != 401 {
		t.Fatal("live polling after logout")
	}
}
func TestLogLoginRequiresMinusOne(t *testing.T) {
	for _, kind := range []string{`0`, `1`, `-2`} {
		s, _ := adminFixture(t, `{"user_type":`+kind+`,"token":"LOGIN"}`)
		w := adminCall(s, "POST", "/admin/api/login", `{"tenant_id":"a","username":"operator","password":"PASSWORD-PRIVATE"}`, nil, "")
		if w.Code != 401 {
			t.Fatal("non-admin admitted", kind, w.Code)
		}
	}
}
