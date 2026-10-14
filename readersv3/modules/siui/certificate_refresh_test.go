package siui

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
)

type refreshBackend struct {
	events []string
	err    error
}

func (b *refreshBackend) refreshCertificates() error {
	b.events = append(b.events, "reload")
	return b.err
}
func (b *refreshBackend) Validate(context.Context, string, []byte) error { return nil }
func (b *refreshBackend) Do(context.Context, string, string, http.Header, []byte) (int, http.Header, []byte, error) {
	b.events = append(b.events, "ocsp")
	return 200, http.Header{"Oscp_response": []string{"fresh-token"}}, nil, nil
}
func TestCertificatesReloadBeforeEveryOCSP(t *testing.T) {
	b := &refreshBackend{}
	c := client{cfg: settings{Username: "test", Licence: "test", BaseURL: "https://example.test"}, backend: b}
	for i := 0; i < 2; i++ {
		h, err := c.sessionHeaders(context.Background())
		if err != nil || h.Get("SessionID") != "fresh-token" {
			t.Fatal(h, err)
		}
	}
	if !reflect.DeepEqual(b.events, []string{"reload", "ocsp", "reload", "ocsp"}) {
		t.Fatal(b.events)
	}
	b.events = nil
	b.err = errors.New("token unavailable")
	if _, err := c.sessionHeaders(context.Background()); err == nil {
		t.Fatal("expected reload failure")
	}
	if !reflect.DeepEqual(b.events, []string{"reload"}) {
		t.Fatal(b.events)
	}
}

func TestAutomaticCertificateRefreshPreservesConfigurationAndReleasesToken(t *testing.T) {
	b := &refreshBackend{}
	cfg := settings{Store: "CurrentUser", Thumbprint: "saved-certificate"}
	m := &Module{cfg: cfg, rt: &testRuntime{}, factory: func(got settings) backend {
		if got.Store != cfg.Store || got.Thumbprint != cfg.Thumbprint {
			t.Fatal("configuration changed")
		}
		return b
	}}
	for _, reason := range []string{"startup", "login"} {
		if err := m.reloadCertificates(reason); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(b.events, []string{"reload", "reload"}) {
		t.Fatal(b.events)
	}
	b.err = errors.New("missing token")
	if err := m.reloadCertificates("login"); !errors.Is(err, b.err) {
		t.Fatal(err)
	}
	if m.pending != 0 || m.checking || m.cfg.Thumbprint != cfg.Thumbprint {
		t.Fatal("state not restored")
	}
	m.pending = 1
	before := len(b.events)
	if err := m.reloadCertificates("login"); err == nil {
		t.Fatal("busy token should not be enumerated")
	}
	if len(b.events) != before || m.pending != 1 {
		t.Fatal("active operation changed")
	}
}
