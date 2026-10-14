package siui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type ocspBudgetBackend struct {
	operationBackend
	remaining time.Duration
}

func (b *ocspBudgetBackend) Do(ctx context.Context, method, address string, h http.Header, data []byte) (int, http.Header, []byte, error) {
	if d, ok := ctx.Deadline(); ok {
		b.remaining = time.Until(d)
	}
	return b.operationBackend.Do(ctx, method, address, h, data)
}
func TestExplicitOCSPRoutesAllowNativeTransportBudget(t *testing.T) {
	for _, path := range []string{"/api/siui/general/ocsp", "/api/siui/ocsp-test"} {
		b := &ocspBudgetBackend{operationBackend: operationBackend{t: t}}
		_, rt := serviceTestModule(t, b)
		req := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "test-local-session")
		w := httptest.NewRecorder()
		rt.mux.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
		if b.remaining < 110*time.Second || b.remaining > ocspTimeout {
			t.Fatal(path, b.remaining)
		}
	}
}
