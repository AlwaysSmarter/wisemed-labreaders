package apibridge

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestFormatterDeadline(t *testing.T) {
	for _, tc := range []struct {
		path, method string
		max          time.Duration
	}{
		{"/api/html-print-formatter/render", "POST", 610 * time.Second},
		{"/api/html-print-formatter/render", "GET", 30 * time.Second},
		{"/api/html-print-formatter/pdf", "POST", 610 * time.Second},
		{"/api/html-print-formatter/pdf", "GET", 30 * time.Second},
		{"/api/status", "POST", 30 * time.Second},
	} {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			deadline, ok := r.Context().Deadline()
			remaining := time.Until(deadline)
			if !ok || remaining > tc.max || remaining < tc.max-time.Second {
				t.Errorf("%s %s: %s", tc.method, tc.path, remaining)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true}`))
		})
		_, err := Invoke(context.Background(), handler, Principal{Subject: "test", TenantID: "test"}, Request{Method: tc.method, Path: tc.path})
		if err != nil {
			t.Fatal(err)
		}
	}
}
