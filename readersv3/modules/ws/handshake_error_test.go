package ws

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type diagnosticBody struct {
	io.Reader
	closed bool
}

func (b *diagnosticBody) Close() error { b.closed = true; return nil }
func TestHandshakeErrorDiagnostics(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"WSM", `{"error":"wsm_scope_denied","message":"Permisiunea api:invoke lipsește; verifică rights 18."}`, "[wsm_scope_denied]: Permisiunea api:invoke"},
		{"proxy", "<html>private proxy error</html>", "nu a furnizat un diagnostic"},
		{"old WSM", "unauthorized", "nu a furnizat un diagnostic"},
		{"large", strings.Repeat("x", 4097), "nu a furnizat un diagnostic"},
		{"newline", `{"error":"wsm_scope_denied","message":"private\nlog injection"}`, "nu a furnizat un diagnostic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &diagnosticBody{Reader: strings.NewReader(tc.body)}
			got := handshakeError(&http.Response{StatusCode: 401, Body: b}).Error()
			if !strings.Contains(got, "HTTP 401") || !strings.Contains(got, tc.want) || strings.Contains(got, "private") || !b.closed {
				t.Fatal(got, "closed", b.closed)
			}
		})
	}
	if got := handshakeError(&http.Response{StatusCode: 502}).Error(); got != "WSS handshake rejected: HTTP 502" {
		t.Fatal(got)
	}
}
