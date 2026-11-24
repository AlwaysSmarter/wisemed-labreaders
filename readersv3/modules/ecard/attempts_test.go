package ecard

import (
	"context"
	"testing"
)

func TestReadSDKErrorAttemptsArePersisted(t *testing.T) {
	for _, succeed := range []bool{false, true} {
		m := testModule(t)
		req := testRequest()
		req.Operation = "read"
		req.Fields = []string{"A1"}
		seed(t, m.store, "read", req, 0)
		count := 0
		m.factory = func(context.Context, string, Settings) (backend, error) {
			count++
			return &fakeBackend{open: Reply{OK: succeed && count == 2, Code: "sdk_error", SDKCode: -999}}, nil
		}
		m.run(context.Background(), "read")
		if count != 2 {
			t.Fatalf("expected both SDKs, got %d", count)
		}
		attempts, e := m.sdkAttempts("read")
		if e != nil || len(attempts) != 2 {
			t.Fatalf("attempts: %v %v", attempts, e)
		}
		var logs int
		m.store.db.QueryRow(`SELECT count(*) FROM call_history WHERE owner='owner'`).Scan(&logs)
		if logs != 2 {
			t.Fatalf("missing SDK logs: %d", logs)
		}
		j, _ := m.store.job("read", "owner")
		if succeed && j.State != "completed" {
			t.Fatal(j)
		}
		if !succeed && j.State != "failed" {
			t.Fatal(j)
		}
	}
}
func TestReadTriesAllSDKsAfterPINFailure(t *testing.T) {
	m := testModule(t)
	req := testRequest()
	req.Operation = "read"
	seed(t, m.store, "read", req, 0)
	count := 0
	m.factory = func(context.Context, string, Settings) (backend, error) {
		count++
		return &fakeBackend{open: Reply{Code: "sdk_error", SDKCode: -999, PINAttempted: true}}, nil
	}
	m.run(context.Background(), "read")
	if count != 2 {
		t.Fatal("read did not try all SDKs")
	}
	attempts, e := m.sdkAttempts("read")
	if e != nil || len(attempts) != 2 {
		t.Fatal("missing stopped attempt")
	}
}
