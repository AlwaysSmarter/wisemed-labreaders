package ws

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"testing"
)

func TestConnectionErrorDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&net.DNSError{Err: "private", Name: "private"}, "DNS"},
		{x509.UnknownAuthorityError{}, "authority"},
		{x509.HostnameError{}, "hostname"},
		{x509.CertificateInvalidError{}, "expired"},
		{syscall.ECONNREFUSED, "refused"},
		{context.DeadlineExceeded, "timed out"},
		{errors.New("private credential"), "before HTTP upgrade"},
	} {
		got := connectionError(fmt.Errorf("private wrapper: %w", tc.err)).Error()
		if !strings.Contains(got, tc.want) || strings.Contains(got, "private") {
			t.Fatal(got)
		}
	}
	if !errors.Is(connectionError(context.Canceled), context.Canceled) {
		t.Fatal("lost cancellation")
	}
}
func TestReconnectClearsPausedPhase(t *testing.T) {
	m := &Module{paused: true, phase: "paused", wake: make(chan struct{}, 1)}
	m.Reconnect()
	if m.paused || m.phase != "reconnecting" {
		t.Fatal("inconsistent reconnect state")
	}
}
