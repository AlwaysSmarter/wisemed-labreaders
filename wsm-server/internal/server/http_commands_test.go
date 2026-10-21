package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type httpCommandResult struct {
	status int
	body   map[string]interface{}
	err    error
}

func httpCommandCall(f *fixture, token, id, body string) httpCommandResult {
	r, err := http.NewRequest("POST", f.http.URL+"/api/connections/"+id+"/commands", strings.NewReader(body))
	if err != nil {
		return httpCommandResult{err: err}
	}
	r.Header.Set("Authorization", "Bearer "+token)
	resp, err := f.http.Client().Do(r)
	if err != nil {
		return httpCommandResult{err: err}
	}
	defer resp.Body.Close()
	out := httpCommandResult{status: resp.StatusCode}
	out.err = json.NewDecoder(resp.Body).Decode(&out.body)
	return out
}

func TestHTTPCommandsConcurrentReplies(t *testing.T) {
	f := newFixture(t)
	peer, peerID := f.connect(t, "a", "reader", "reader-http")
	claims := claimsFor("a", "service", "backend")
	token := signed(t, f.cfg, claims, "a-key")
	results := make(chan httpCommandResult, 2)
	for range 2 {
		go func() {
			results <- httpCommandCall(f, token, peerID, `{"command":"reader.status","args":{"test":true},"timeout_ms":2000}`)
		}()
	}
	first := readType(t, peer, "command")
	second := readType(t, peer, "command")
	if first.RequestID == second.RequestID || first.Payload["sender_subject"] != "backend" || first.Payload["sender_client_type"] != "service" {
		t.Fatalf("incorrect command identities: %+v %+v", first, second)
	}
	for _, m := range []Envelope{second, first} {
		send(t, peer, Envelope{Type: "reply", CorrelationID: m.RequestID, Target: &Target{Mode: "connection", ConnectionID: asString(m.Payload["sender_connection_id"])}, Payload: map[string]interface{}{"result_for": m.RequestID}})
	}
	for range 2 {
		r := <-results
		if r.err != nil || r.status != 200 {
			t.Fatalf("result: %+v", r)
		}
		payload := r.body["response"].(map[string]interface{})
		if r.body["request_id"] != payload["result_for"] || r.body["connection_id"] != peerID {
			t.Fatalf("uncorrelated reply: %+v", r.body)
		}
	}
	if n := len(f.s.hub.Snapshot("a")); n != 1 {
		t.Fatalf("HTTP connections leaked: %d", n)
	}
}

func TestHTTPCommandsIsolationScopesAndValidation(t *testing.T) {
	f := newFixture(t)
	_, foreignID := f.connect(t, "b", "reader", "foreign")
	_, peerID := f.connect(t, "a", "reader", "local")
	c := claimsFor("a", "service", "backend")
	token := signed(t, f.cfg, c, "a-key")
	for _, tc := range []struct {
		name, target, body string
		status             int
	}{
		{"foreign", foreignID, `{"command":"reader.status"}`, 404},
		{"offline", "missing", `{"command":"reader.status"}`, 404},
		{"missing api scope", peerID, `{"command":"api.request","args":{"method":"GET","path":"/api/status"}}`, 403},
		{"timeout", peerID, `{"command":"reader.status","timeout_ms":300001}`, 400},
		{"empty", peerID, `{}`, 400},
		{"trailing", peerID, `{"command":"reader.status"}{}`, 400},
		{"unknown", peerID, `{"command":"reader.status","tenant_id":"b"}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httpCommandCall(f, token, tc.target, tc.body)
			if r.err != nil || r.status != tc.status {
				t.Fatalf("%+v", r)
			}
		})
	}
	c.Scopes = []string{"connections:read"}
	r := httpCommandCall(f, signed(t, f.cfg, c, "a-key"), peerID, `{"command":"reader.status"}`)
	if r.status != 403 {
		t.Fatalf("missing command scope: %+v", r)
	}
	r = httpCommandCall(f, "invalid", peerID, `{"command":"reader.status"}`)
	if r.status != 401 {
		t.Fatalf("bad auth: %+v", r)
	}
}

func TestHTTPCommandsTimeoutDisconnectAndReload(t *testing.T) {
	for _, mode := range []string{"timeout", "disconnect", "reload", "spoof"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			peer, id := f.connect(t, "a", "reader", "target")
			other, _ := f.connect(t, "a", "reader", "other")
			token := signed(t, f.cfg, claimsFor("a", "service", "backend"), "a-key")
			results := make(chan httpCommandResult, 1)
			go func() { results <- httpCommandCall(f, token, id, `{"command":"reader.status","timeout_ms":150}`) }()
			m := readType(t, peer, "command")
			expected := 504
			switch mode {
			case "disconnect":
				peer.Close()
				expected = 502
			case "reload":
				if err := f.s.Reload(f.cfg); err != nil {
					t.Fatal(err)
				}
				expected = 503
			case "spoof":
				send(t, other, Envelope{Type: "reply", CorrelationID: m.RequestID, Target: &Target{Mode: "connection", ConnectionID: asString(m.Payload["sender_connection_id"])}, Payload: map[string]interface{}{"ok": true}})
			}
			select {
			case r := <-results:
				if r.err != nil || r.status != expected {
					t.Fatalf("%+v", r)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("HTTP command did not finish")
			}
			for _, c := range f.s.hub.Snapshot("a") {
				if c.ClientType == "service" {
					t.Fatal("HTTP connection leaked")
				}
			}
		})
	}
}

func TestHTTPConnectionsTypeFilter(t *testing.T) {
	f := newFixture(t)
	f.connect(t, "a", "reader", "reader")
	f.connect(t, "a", "service", "service")
	f.connect(t, "b", "service", "foreign-service")
	token := signed(t, f.cfg, claimsFor("a", "service", "backend"), "a-key")
	for _, kind := range []string{"service", "reader", "siui-bridge", "anything"} {
		r, _ := http.NewRequest("GET", f.http.URL+"/api/connections?client_type="+kind, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		resp, err := f.http.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Connections []ConnectionInfo `json:"connections"`
			Count       int              `json:"count"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		want := 0
		if kind == "service" || kind == "reader" {
			want = 1
		}
		if result.Count != want || len(result.Connections) != want {
			t.Fatalf("%s: %s", kind, data)
		}
		for _, c := range result.Connections {
			if c.ClientType != kind || c.TenantID != "a" {
				t.Fatalf("wrong result: %+v", c)
			}
		}
	}
}
