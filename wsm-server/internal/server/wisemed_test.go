package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"wisemed-labreaders/serverlast/wsm-server/internal/config"
)

func TestWiseMedTenantUpstreams(t *testing.T) {
	roots := x509.NewCertPool()
	upstreams := map[string]*httptest.Server{}
	for _, id := range []string{"a", "b"} {
		id := id
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			token, e := jwt.Parse(raw, func(*jwt.Token) (interface{}, error) { return []byte("api-secret-" + id), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
			if e != nil || !token.Valid {
				t.Error("wrong tenant API credential")
				http.Error(w, "unauthorized", 401)
				return
			}
			if r.URL.RawQuery != "" {
				t.Error("debug query still enabled")
			}
			if r.URL.Path != "/fileforanalyzer/f1/e1/" {
				t.Error(r.URL.Path)
			}
			writeJSON(w, 200, map[string]interface{}{"tenant": id})
		}))
		t.Cleanup(ts.Close)
		roots.AddCert(ts.Certificate())
		upstreams[id] = ts
	}
	s := New(testConfig(t))
	s.upstreamClient = &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}}
	defer s.upstreamClient.CloseIdleConnections()
	for _, id := range []string{"a", "b"} {
		tenant := config.Tenant{WiseMed: config.WiseMed{BaseURL: upstreams[id].URL, APIKey: "api-secret-" + id}}
		c := &Connection{ConnectionInfo: ConnectionInfo{TenantID: id}}
		data, e := s.handleServerCommand(context.Background(), tenant, c, Envelope{Payload: map[string]interface{}{"command": "wisemed.fetch_file_for_analyzer", "args": map[string]interface{}{"file_id": "f1", "equipment_id": "e1"}}})
		if e != nil || data["tenant"] != id {
			t.Fatal(data, e)
		}
	}
}
func TestWiseMedRedirectBodyBoundAndCancellation(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/destination", 302)
		case "/large":
			io.WriteString(w, strings.Repeat("x", 4*1024*1024+1))
		case "/error":
			http.Error(w, "PRIVATE UPSTREAM CONTENT", 500)
		case "/destination":
			t.Error("redirect followed")
		default:
			writeJSON(w, 200, map[string]interface{}{})
		}
	}))
	defer ts.Close()
	s := New(testConfig(t))
	s.upstreamClient.Transport = ts.Client().Transport
	defer s.upstreamClient.CloseIdleConnections()
	up := config.WiseMed{BaseURL: ts.URL, APIKey: "api-secret"}
	for _, path := range []string{"/redirect", "/large", "/error"} {
		_, e := s.doWiseMedJSON(context.Background(), up, http.MethodGet, path, nil)
		if e == nil || strings.Contains(e.Error(), "PRIVATE") {
			t.Fatal(path, e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.doWiseMedJSON(ctx, up, http.MethodGet, "/", nil); e == nil {
		t.Fatal("cancel ignored")
	}
}
