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
	restarted := &pinVault{path: v.path}
	p2, _, e := restarted.begin(s)
	if e != nil || p2 != nil {
		t.Fatal("attempt replayed after restart", e)
	}
	finish(false)
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
	finish(true)
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
				mu.Unlock()
				finish(false)
			}
		}()
	}
	wg.Wait()
	if count != 1 {
		t.Fatal(count)
	}
}
func TestPINManualNeverRearmsSavedSecret(t *testing.T) {
	v, s := testPIN(t)
	_ = v.save(s, "old-value", false, false)
	_ = v.save(s, "new-manual-value", false, true)
	p, finish, e := v.begin(s)
	if e != nil || string(p) != "new-manual-value" {
		t.Fatal(e)
	}
	finish(true)
	p, finish, e = v.begin(s)
	if e != nil || string(p) != "new-manual-value" {
		t.Fatal("manual session not reused", e)
	}
	finish(false)
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
	newDone(false)
	oldDone(true)
	_, blocked, _ := v.status(s)
	if !blocked {
		t.Fatal("old attempt reenabled replacement")
	}
}
