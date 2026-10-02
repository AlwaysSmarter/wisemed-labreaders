package apibridge

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

func TestSameHandlerAndIdentity(t *testing.T) {
	calls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		p, ok := PrincipalFrom(r.Context())
		if !ok || p.Subject != "operator" || !p.Admin {
			t.Error("principal missing")
		}
		if r.Method != "PATCH" || r.URL.Query().Get("date") != "2026-09-22" {
			t.Error("request not preserved")
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "sensitive=secret")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(body)
	})
	out, e := Invoke(context.Background(), handler, Principal{Subject: "operator", TenantID: "a", Admin: true}, Request{Method: "PATCH", Path: "/api/items?date=2026-09-22", Body: json.RawMessage(`{"value":42}`)})
	if e != nil || calls != 1 || out.Status != 201 || out.Kind != "api.response" || out.Body.(map[string]interface{})["value"] != json.Number("42") {
		t.Fatal(out, e)
	}
	if out.Headers["Set-Cookie"] != "" {
		t.Fatal("cookie escaped")
	}
}
func TestExplicitJSONNullDiffersFromEmptyResponse(t *testing.T) {
	for _, body := range []string{"", "null"} {
		h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, body)
		})
		out, err := Invoke(context.Background(), h, Principal{Subject: "u", TenantID: "t"}, Request{Method: "GET", Path: "/api/example"})
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]json.RawMessage
		if err = json.Unmarshal(encoded, &wire); err != nil {
			t.Fatal(err)
		}
		v, present := wire["body"]
		if present != (body == "null") || (present && string(v) != "null") {
			t.Fatalf("body %q became %s", body, encoded)
		}
	}
}
func TestRejectedRequestsNeverInvoke(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("invalid bridge request reached handler") })
	p := Principal{Subject: "u", TenantID: "t"}
	for _, path := range []string{"https://evil/api/test", "//evil/api/test", "/", "/app.js", "/api/../app.js", "/api/wss/bridge", "/api/wss/open", "/api/wss/debug", "/api/session/login"} {
		if _, e := Invoke(context.Background(), handler, p, Request{Method: "GET", Path: path}); e == nil {
			t.Error(path)
		}
	}
	if _, e := Invoke(context.Background(), handler, Principal{}, Request{Method: "GET", Path: "/api/test"}); e == nil {
		t.Fatal("unverified principal")
	}
	if _, e := Invoke(context.Background(), handler, p, Request{Method: "CONNECT", Path: "/api/test"}); e == nil {
		t.Fatal("CONNECT")
	}
}
func TestBinaryResponseAndBodyLimit(t *testing.T) {
	p := Principal{Subject: "u", TenantID: "a"}
	raw := []byte{0, 1, 2, 3}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if string(b) != string(raw) {
			t.Error("binary request changed")
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(b)
	})
	out, e := Invoke(context.Background(), handler, p, Request{Method: "POST", Path: "/api/upload", BodyBase64: base64.StdEncoding.EncodeToString(raw), ContentType: "application/octet-stream"})
	if e != nil || out.BodyBase64 != base64.StdEncoding.EncodeToString(raw) {
		t.Fatal(out, e)
	}
	for _, h := range []http.HandlerFunc{func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html>UI</html>")
	}, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, strings.Repeat("x", MaxBody+1)) }} {
		if _, e := Invoke(context.Background(), h, p, Request{Method: "GET", Path: "/api/test"}); e == nil {
			t.Fatal("UI/oversize response accepted")
		}
	}
	// A local HTTP header cannot manufacture a trusted bridge context.
	r := httptest.NewRequest("GET", "/api/test", nil)
	r.Header.Set("X-WSS-Subject", "admin")
	if _, ok := PrincipalFrom(r.Context()); ok {
		t.Fatal("header bypass")
	}
}
