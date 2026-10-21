package wisemedapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCallbackAuthenticatedAndNoRedirect(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		parts := strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), ".")
		if len(parts) == 3 {
			payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
			var claims map[string]any
			json.Unmarshal(payload, &claims)
			expected := any(nil)
			if count == 3 {
				expected = "per-call-token"
			}
			if claims["lt"] != expected || claims["login_user_id"] != nil {
				t.Error("background callback used a user session")
			}
		}

		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || r.Header.Get("Idempotency-Key") != "siui-id" {
			t.Error("missing authentication or idempotency")
		}
		if r.URL.Path == "/api/redirect" {
			http.Redirect(w, r, "/unexpected", 302)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/api/pacient/1000000000000" || r.Method != "PUT" || string(raw) != `{"state":1}` {
			t.Error("wrong callback")
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	m, _ := newEquipmentTestModule(t, server.URL)
	m.settings["login_token"] = "session-not-for-background"
	m.settings["login_user_id"] = "123"
	destination, err := m.CallbackDestination()
	if err != nil {
		t.Fatal(err)
	}
	code, err := m.SendCallback(context.Background(), destination, "PUT", "/pacient/1000000000000", "siui-id", "", []byte(`{"state":1}`))
	if err != nil || code != 204 {
		t.Fatal(code, err)
	}
	code, err = m.SendCallback(context.Background(), destination, "POST", "/redirect", "siui-id", "", nil)
	if err != nil || code != 302 || count != 2 {
		t.Fatal("redirect followed", code, err, count)
	}
	code, err = m.SendCallback(context.Background(), destination, "PUT", "/pacient/1000000000000", "siui-id", "per-call-token", []byte(`{"state":1}`))
	if err != nil || code != 204 {
		t.Fatal(code, err)
	}
	for _, path := range []string{"//evil.example/x", "https://evil.example/x", "/../x", "/%2e%2e/x", "/x?secret=y"} {
		if _, err = m.SendCallback(context.Background(), destination, "POST", path, "siui-id", "", nil); err == nil {
			t.Fatal("unsafe path", path)
		}
	}
	if _, err = m.SendCallback(context.Background(), "https://different.example/api", "POST", "/test", "siui-id", "", nil); err == nil {
		t.Fatal("destination binding lost")
	}
}
