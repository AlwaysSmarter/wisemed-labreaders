package siui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testPIN(t *testing.T) (*pinVault, settings) {
	t.Helper()
	v := &pinVault{path: filepath.Join(t.TempDir(), "pins.json")}
	return v, settings{Store: "CurrentUser", Thumbprint: "CERT-A", pin: v}
}
func TestPINEncryptedBoundAndSuspendedAcrossRestart(t *testing.T) {
	v, s := testPIN(t)
	const secret = "test-only-PIN-1234"
	if e := v.save(s, secret, false, false); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(v.path)
	if e != nil || bytes.Contains(b, []byte(secret)) {
		t.Fatal("plaintext leaked", e)
	}
	set, blocked, e := v.status(s)
	if e != nil || !set || blocked {
		t.Fatal(set, blocked, e)
	}
	other := s
	other.Thumbprint = "CERT-B"
	p, _, e := v.begin(other)
	if e != nil || p != nil {
		t.Fatal("wrong certificate exposed PIN")
	}
	p, finish, e := v.begin(s)
	if e != nil || string(p) != secret {
		t.Fatal("cannot decrypt", e)
	}
	finish(pinRejected)
	for i := 0; i < 2; i++ {
		_, next, _ := v.begin(s)
		next(pinRejected)
	} // Third rejection persists across restart.
	restarted := &pinVault{path: v.path}
	p2, _, e := restarted.begin(s)
	if e != nil || p2 != nil {
		t.Fatal("attempt replayed after restart", e)
	}
	finish(pinRejected)
	if !bytes.Equal(p, make([]byte, len(p))) {
		t.Fatal("PIN buffer not cleared")
	}
	if e = v.save(s, secret, false, false); e != nil {
		t.Fatal(e)
	}
	p, finish, e = v.begin(s)
	if e != nil {
		t.Fatal(e)
	}
	finish(pinSuccess)
	_, blocked, e = v.status(s)
	if e != nil || blocked {
		t.Fatal("successful signing not restored", e)
	}
	if e = v.save(s, "", true, false); e != nil {
		t.Fatal(e)
	}
	set, _, _ = v.status(s)
	if set {
		t.Fatal("PIN not removed")
	}
}
func TestPINOnlyOneConcurrentAttempt(t *testing.T) {
	v, s := testPIN(t)
	if e := v.save(s, "123456", false, false); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	count := 0
	var complete func(pinOutcome)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, finish, e := v.begin(s)
			if e != nil {
				t.Error(e)
			}
			if len(p) > 0 {
				mu.Lock()
				count++
				complete = finish
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if count != 1 {
		t.Fatal(count)
	}
	complete(pinRejected)
}
func TestPINManualNeverRearmsSavedSecret(t *testing.T) {
	v, s := testPIN(t)
	_ = v.save(s, "old-value", false, false)
	_ = v.save(s, "new-manual-value", false, true)
	p, finish, e := v.begin(s)
	if e != nil || string(p) != "new-manual-value" {
		t.Fatal(e)
	}
	finish(pinSuccess)
	p, finish, e = v.begin(s)
	if e != nil || string(p) != "new-manual-value" {
		t.Fatal("manual session not reused", e)
	}
	finish(pinRejected)
	p, _, _ = v.begin(s)
	if len(p) > 0 {
		t.Fatal("wrong manual PIN replayed or saved PIN reactivated")
	}
	_ = v.save(s, "expired", false, true)
	v.until = time.Now().Add(-time.Second)
	p, _, _ = v.begin(s)
	if len(p) > 0 {
		t.Fatal("expired manual credential returned")
	}
}
func TestPINResponseNeverContainsSecretOrCiphertext(t *testing.T) {
	v, s := testPIN(t)
	m := &Module{cfg: s, rt: &testRuntime{}}
	_ = v.save(s, "sensitive-test-value", false, false)
	w := httptest.NewRecorder()
	m.tokenPINHTTP(w, httptest.NewRequest("GET", "/api/siui/token-pin", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "sensitive-test-value") || strings.Contains(w.Body.String(), "sealed") {
		t.Fatal(w.Code, w.Body.String())
	}
	var data map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &data); e != nil {
		t.Fatal(e)
	}
	if data["hint"] != "<hidden>" || data["configured"] != true {
		t.Fatal(data)
	}
	m.pending = 1
	w = httptest.NewRecorder()
	m.tokenPINHTTP(w, httptest.NewRequest(http.MethodPut, "/api/siui/token-pin", strings.NewReader(`{"pin":"replacement"}`)))
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
}
func TestPINCannotDecryptForAnotherCertificate(t *testing.T) {
	v, s := testPIN(t)
	_ = v.save(s, "test-pin", false, false)
	v.mu.Lock()
	rows, _ := v.read()
	other := s
	other.Thumbprint = "B"
	rows[pinIdentity(other)] = rows[pinIdentity(s)]
	_ = v.write(rows)
	v.mu.Unlock()
	if _, _, e := v.begin(other); e == nil {
		t.Fatal("ciphertext not bound to identity")
	}
}
func TestPINOldAttemptCannotReenableReplacement(t *testing.T) {
	v, s := testPIN(t)
	_ = v.save(s, "old-pin", false, false)
	_, oldDone, _ := v.begin(s)
	_ = v.save(s, "new-pin", false, false)
	_, newDone, _ := v.begin(s)
	newDone(pinRejected)
	for i := 0; i < 2; i++ {
		_, next, _ := v.begin(s)
		next(pinRejected)
	}
	oldDone(pinSuccess)
	_, blocked, _ := v.status(s)
	if !blocked {
		t.Fatal("old attempt reenabled replacement")
	}
}

func TestPINUnrelatedFailureAndInterruptedAttemptRemainEnabled(t *testing.T) {
	v, s := testPIN(t)
	if err := v.save(s, "test-pin", false, false); err != nil {
		t.Fatal(err)
	}
	_, done, err := v.begin(s)
	if err != nil {
		t.Fatal(err)
	}
	if p, _, err := v.begin(s); err != nil || len(p) != 0 {
		t.Fatal("concurrent attempt allowed", err)
	}
	// Simulate restart before completion: no credential rejection was received.
	restarted := &pinVault{path: v.path}
	p, restartedDone, err := restarted.begin(s)
	if err != nil || string(p) != "test-pin" {
		t.Fatal("interruption suspended PIN", err)
	}
	restartedDone(pinSuccess)
	done(pinUnrelated) // Missing token / cancellation / TLS error, not a rejected PIN.
	p, done, err = v.begin(s)
	if err != nil || string(p) != "test-pin" {
		t.Fatal("unrelated error suspended PIN", err)
	}
	done(pinSuccess)
}

func TestNativePINErrorClassification(t *testing.T) {
	for _, code := range []uint32{0x8010006b, 0x8010006c} {
		if !windowsPINRejected(code) {
			t.Fatalf("PIN error missed: %x", code)
		}
	}
	for _, code := range []uint32{0, 0x8010000c, 0x80100069, 0x8010006e, 12175} {
		if windowsPINRejected(code) {
			t.Fatalf("unrelated Windows error: %x", code)
		}
	}
	for _, code := range []uint64{0xa0, 0xa1, 0xa2, 0xa4} {
		if !pkcs11PINRejected(code) {
			t.Fatalf("PIN error missed: %x", code)
		}
	}
	for _, code := range []uint64{0, 5, 0x30, 0x32, 0xe0, 0xe1, 0x101} {
		if pkcs11PINRejected(code) {
			t.Fatalf("unrelated PKCS11 error: %x", code)
		}
	}
}

func TestPINThreeRejectionsResetOnlyOnSuccess(t *testing.T) {
	v, s := testPIN(t)
	if err := v.save(s, "pin", false, false); err != nil {
		t.Fatal(err)
	}
	attempt := func(outcome pinOutcome, want int, blocked bool) {
		t.Helper()
		p, done, err := v.begin(s)
		if err != nil || len(p) == 0 {
			t.Fatal("attempt unavailable", err)
		}
		done(outcome)
		count, err := v.failureCount(s)
		_, gotBlocked, _ := v.status(s)
		if err != nil || count != want || gotBlocked != blocked {
			t.Fatalf("count=%d blocked=%v err=%v", count, gotBlocked, err)
		}
	}
	attempt(pinRejected, 1, false)
	attempt(pinRejected, 2, false)
	v = &pinVault{path: v.path}
	attempt(pinUnrelated, 2, false)
	attempt(pinSuccess, 0, false)
	attempt(pinRejected, 1, false)
	attempt(pinRejected, 2, false)
	attempt(pinRejected, 3, true)
	v = &pinVault{path: v.path}
	if p, _, err := v.begin(s); err != nil || len(p) > 0 {
		t.Fatal("fourth automatic attempt allowed", err)
	}
}
