package siui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type failedPostBackend struct{ err error }

func (b failedPostBackend) Validate(context.Context, string, []byte) error { return nil }
func (b failedPostBackend) Do(_ context.Context, _ string, address string, _ http.Header, _ []byte) (int, http.Header, []byte, error) {
	if strings.Contains(address, "/OCSP/") {
		return 200, http.Header{"Oscp_response": []string{"token"}}, nil, nil
	}
	return 0, nil, nil, b.err
}
func TestDialFailureIsNotAnUncertainSubmission(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		uncertain bool
	}{
		{"dial", fmt.Errorf("macOS CNAS request failed: %w", &url.Error{Op: "Post", URL: "https://www.siui.ro", Err: &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded}}), false},
		{"read", &net.OpError{Op: "read", Net: "tcp", Err: context.DeadlineExceeded}, true},
		{"write", &net.OpError{Op: "write", Net: "tcp", Err: context.DeadlineExceeded}, true},
		{"untyped", errors.New("dial tcp: connect timeout"), true},
		{"deadline", context.DeadlineExceeded, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := client{cfg: settings{Username: "test", Licence: "test", BaseURL: "https://www.siui.ro"}, backend: failedPostBackend{tc.err}}
			op, _ := findOperation("facturare", "sendEInvoice")
			_, submitted, err := c.invokeOperation(context.Background(), op, map[string]string{"invoiceFile": "test"})
			if err == nil || submitted != tc.uncertain {
				t.Fatal(submitted, err)
			}
		})
	}
}
