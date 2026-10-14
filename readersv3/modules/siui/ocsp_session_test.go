package siui

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type cachedOCSPBackend struct {
	calls int
	fail  bool
}

func (b *cachedOCSPBackend) Validate(context.Context, string, []byte) error { return nil }
func (b *cachedOCSPBackend) Do(context.Context, string, string, http.Header, []byte) (int, http.Header, []byte, error) {
	b.calls++
	if b.fail {
		return 0, nil, nil, errors.New("offline")
	}
	return 200, http.Header{"Oscp_response": []string{"test-token"}}, nil, nil
}
func TestOCSPReusesSessionOnlyForMatchingIdentityAndAge(t *testing.T) {
	b := &cachedOCSPBackend{}
	cache := &ocspSession{}
	cfg := settings{Username: "7991962_CAS-DJ", Licence: "test", BaseURL: "https://www.siui.ro", Store: "CurrentUser", Thumbprint: "A"}
	first := client{cfg: cfg, backend: b, session: cache}
	if _, err := first.sessionHeaders(context.Background()); err != nil {
		t.Fatal(err)
	}
	cache.acquired = time.Now().Add(-149 * time.Second)
	var message string
	next := client{cfg: cfg, backend: b, session: cache, logf: func(f string, a ...interface{}) { message = f }}
	if _, err := next.sessionHeaders(context.Background()); err != nil || b.calls != 1 || !next.sessionReused || next.sessionAge != 149 || !strings.Contains(message, "vechime") {
		t.Fatal(err, b.calls, next.sessionAge, message)
	}
	cache.acquired = time.Now().Add(-151 * time.Second)
	if _, err := next.sessionHeaders(context.Background()); err != nil || b.calls != 2 || next.sessionReused {
		t.Fatal(err, b.calls)
	}
	for _, change := range []func(*settings){func(s *settings) { s.Username = "123_CAS-DJ" }, func(s *settings) { s.Username = "7991962_CAS-B" }, func(s *settings) { s.Thumbprint = "B" }, func(s *settings) { s.BaseURL = "https://www.siui.ro:444" }, func(s *settings) { s.Licence = "other" }} {
		before := b.calls
		change(&next.cfg)
		if _, err := next.sessionHeaders(context.Background()); err != nil || b.calls != before+1 || next.sessionReused {
			t.Fatal("identity reused", err)
		}
	}
	cache.acquired = time.Now().Add(-151 * time.Second)
	b.fail = true
	if _, err := next.sessionHeaders(context.Background()); err == nil || cache.token != "" {
		t.Fatal("stale token retained")
	}
}
