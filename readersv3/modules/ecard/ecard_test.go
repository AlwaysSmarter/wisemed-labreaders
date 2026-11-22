package ecard

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/des"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

func TestPatientBinding(t *testing.T) {
	for _, tt := range []struct {
		name string
		e, a Identity
		ok   bool
	}{
		{"same CNP", Identity{CNP: "123"}, Identity{CNP: "123"}, true},
		{"CID only", Identity{CID: "cid"}, Identity{CNP: "123", CID: "cid"}, true},
		{"missing CNP with CID", Identity{CNP: "123", CID: "cid"}, Identity{CID: "cid"}, true},
		{"anonymous with CID", Identity{CNP: "123", CID: "cid"}, Identity{CNP: "0000000000000", CID: "cid"}, true},
		{"missing CNP no CID", Identity{CNP: "123"}, Identity{}, false},
		{"anonymous no CID", Identity{CNP: "123"}, Identity{CNP: "0000000000000"}, false},
		{"different CNP matching CID", Identity{CNP: "123", CID: "cid"}, Identity{CNP: "456", CID: "cid"}, false},
		{"matching CNP different CID", Identity{CNP: "123", CID: "cid"}, Identity{CNP: "123", CID: "other"}, false},
		{"empty expectation", Identity{}, Identity{CNP: "123"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchIdentity(tt.e, tt.a) == nil; got != tt.ok {
				t.Fatalf("match=%v want %v", got, tt.ok)
			}
		})
	}
}
func TestFallbackNeverReplaysPINOrMutations(t *testing.T) {
	for _, op := range []string{"read", "certificates", "sign"} {
		if !canFallback(op, Reply{SDKCode: -6}, false) {
			t.Fatal("read preparation fallback missing")
		}
	}
	for _, op := range []string{"write", "activate", "change-pin", "reset-pin", "enroll"} {
		if canFallback(op, Reply{SDKCode: -6}, false) {
			t.Fatal("mutation replay", op)
		}
	}
	for _, code := range []int{-14, -15, -16, -29, -61, -57, -58, -71} {
		if canFallback("read", Reply{SDKCode: code}, false) {
			t.Fatal("unsafe fallback", code)
		}
	}
	if canFallback("read", Reply{SDKCode: -6}, true) || canFallback("read", Reply{SDKCode: -6, PINAttempted: true}, false) {
		t.Fatal("PIN retried")
	}
}
func testStore(t *testing.T) *store {
	t.Helper()
	s, e := openStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.db.Close() })
	return s
}
func testRequest() Request {
	return Request{Operation: "sign", Expected: Identity{CID: "cid"}, Encoding: "ascii", Certificate: "insured", Contract: Contract{CIF: "1", CUI: "1", Number: "1", Date: "2026-01-01", House: "CAS", ProviderType: "PARA"}}
}
func seed(t *testing.T, s *store, id string, req Request, n int) {
	t.Helper()
	b, e := s.seal(req, id)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.db.Exec(`INSERT INTO jobs(id,owner,operation,state,total,created,request) VALUES(?,'owner',?,'queued',?,?,?)`, id, req.Operation, n, time.Now().Unix(), b)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < n; i++ {
		b, _ = s.seal(Item{ID: strconv.Itoa(i), Text: "private clinical text"}, id+":"+strconv.Itoa(i))
		_, e = s.db.Exec(`INSERT INTO items(job,n,id,payload) VALUES(?,?,?,?)`, id, i, strconv.Itoa(i), b)
		if e != nil {
			t.Fatal(e)
		}
	}
}

type fakeBackend struct {
	open    Reply
	signs   int
	failAt  int
	unknown bool
	closed  bool
}

func (f *fakeBackend) Call(_ context.Context, p map[string]interface{}, _ func(string) (string, error)) (Reply, error) {
	if p["action"] == "open" {
		return f.open, nil
	}
	f.signs++
	if f.failAt > 0 && f.signs == f.failAt {
		if f.unknown {
			return Reply{}, errors.New("pipe lost")
		}
		return Reply{Code: "card_changed", SDKCode: -61}, nil
	}
	return Reply{OK: true, Data: json.RawMessage(`"signed-cms"`)}, nil
}
func (f *fakeBackend) Close() { f.closed = true }
func testModule(t *testing.T) *Module {
	s, _ := readSettings(nil)
	s.SDKOrder = []string{"2.0.0.4", "2.0.0.3"}
	return &Module{settings: s, store: testStore(t), active: map[string]*activeJob{}, queue: make(chan string, 32), ctx: context.Background()}
}
func TestBatchPinsIdentityAndStopsWithoutReplay(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		m := testModule(t)
		seed(t, m.store, "job", testRequest(), 5)
		f := &fakeBackend{open: Reply{OK: true, Data: json.RawMessage(`{"cnp":"","cid":"cid"}`)}, failAt: 3, unknown: unknown}
		starts := 0
		m.factory = func(context.Context, string, Settings) (backend, error) { starts++; return f, nil }
		m.run(context.Background(), "job")
		j, e := m.store.job("job", "owner")
		if e != nil {
			t.Fatal(e)
		}
		want := "failed"
		if unknown {
			want = "unknown"
		}
		if j.State != want || j.Completed != 2 || f.signs != 3 || starts != 1 || !f.closed {
			t.Fatalf("bad result %+v signs=%d starts=%d", j, f.signs, starts)
		}
	}
}
func TestMismatchNeverSigns(t *testing.T) {
	m := testModule(t)
	seed(t, m.store, "job", testRequest(), 2)
	f := &fakeBackend{open: Reply{OK: true, Data: json.RawMessage(`{"cnp":"123","cid":"wrong"}`)}}
	m.factory = func(context.Context, string, Settings) (backend, error) { return f, nil }
	m.run(context.Background(), "job")
	if f.signs != 0 {
		t.Fatal("signed wrong patient's data")
	}
}
func TestFallbackOrderThenOneSessionForBatch(t *testing.T) {
	m := testModule(t)
	seed(t, m.store, "job", testRequest(), 20)
	order := []string{}
	good := &fakeBackend{open: Reply{OK: true, Data: json.RawMessage(`{"cid":"cid"}`)}}
	m.factory = func(_ context.Context, sdk string, _ Settings) (backend, error) {
		order = append(order, sdk)
		if len(order) == 1 {
			return &fakeBackend{open: Reply{SDKCode: -6, Code: "sdk_error"}}, nil
		}
		return good, nil
	}
	m.run(context.Background(), "job")
	j, _ := m.store.job("job", "owner")
	if j.State != "completed" || good.signs != 20 || len(order) != 2 || order[0] != "2.0.0.4" || order[1] != "2.0.0.3" {
		t.Fatalf("%+v %v", j, order)
	}
}
func TestEncryptedPayloadAndAAD(t *testing.T) {
	s := testStore(t)
	b, e := s.seal(Item{Text: "private clinical text"}, "job:0")
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(b, []byte("private")) {
		t.Fatal("plaintext stored")
	}
	var v Item
	if s.open(b, "other:0", &v) == nil {
		t.Fatal("cross-job ciphertext accepted")
	}
	if e = s.open(b, "job:0", &v); e != nil || v.Text != "private clinical text" {
		t.Fatal(e)
	}
}
func TestRestartMarksPendingNoReplay(t *testing.T) {
	dir := t.TempDir()
	s, e := openStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	seed(t, s, "job", testRequest(), 1)
	s.db.Close()
	s, e = openStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.db.Close()
	j, _ := s.job("job", "owner")
	if j.State != "interrupted" {
		t.Fatal(j)
	}
}
func TestMigrationHashAndParser(t *testing.T) {
	id := TDXIdentity{Contract: testRequest().Contract, Host: "old", UMEndpoint: "127.0.0.1:443"}
	old, e := id.filename()
	if e != nil {
		t.Fatal(e)
	}
	id.Host = "new"
	next, _ := id.filename()
	if old == next {
		t.Fatal("hostname not bound")
	}
	var b bytes.Buffer
	for _, s := range []string{"terminal", "serial", "00112233445566778899AABBCCDDEEFF"} {
		b.WriteByte(byte(len(s)))
		b.WriteString(s)
	}
	b.WriteByte(0)
	binary.Write(&b, binary.LittleEndian, int32(1))
	binary.Write(&b, binary.LittleEndian, int32(2))
	binary.Write(&b, binary.LittleEndian, int32(16))
	b.Write(make([]byte, 16))
	pad := 8 - b.Len()%8
	b.Write(bytes.Repeat([]byte{byte(pad)}, pad))
	plain := b.Bytes()
	raw := make([]byte, len(plain))
	block, _ := des.NewCipher([]byte("lka@xx1-"))
	cipher.NewCBCEncrypter(block, make([]byte, 8)).CryptBlocks(raw, plain)
	meta, e := inspectTDX(raw)
	if e != nil || meta.TerminalID != "terminal" || len(meta.Profiles) != 1 {
		t.Fatalf("%+v %v", meta, e)
	}
	raw[len(raw)-1] ^= 1
	if _, e = inspectTDX(raw); e == nil {
		t.Fatal("bad padding accepted")
	}
}
func TestSourceTDXReadOnlyCompatibility(t *testing.T) {
	// Optional local fixture; never print or copy patient/operator data into tests.
	paths, _ := filepath.Glob("../../../old-src/HITMED/Novensys.eCard.SDK/TerminalData/*.tdx")
	for _, p := range paths {
		raw, e := os.ReadFile(p)
		if e != nil {
			t.Fatal("fixture unavailable")
		}
		if _, e = inspectTDX(raw); e != nil {
			t.Fatal("legacy TDX incompatible")
		}
	}
}

type guard struct {
	user  string
	admin bool
}

func (g guard) RequireSession(h http.Handler) http.Handler { return h }
func (g guard) SessionIdentity(*http.Request) (string, bool, bool) {
	return g.user, g.admin, g.user != ""
}
func TestAPIAuthOwnershipAndPINChallenge(t *testing.T) {
	m := testModule(t)
	seed(t, m.store, "secret", testRequest(), 1)
	m.guard = guard{user: "intruder"}
	w := httptest.NewRecorder()
	m.handle(w, httptest.NewRequest("GET", "/api/ecard/jobs/secret", nil))
	if w.Code != 404 {
		t.Fatal("cross-owner job leak", w.Code)
	}
	m.guard = guard{}
	w = httptest.NewRecorder()
	m.handle(w, httptest.NewRequest("GET", "/api/ecard/settings", nil))
	if w.Code != 401 {
		t.Fatal("missing authentication")
	}
}
func TestBodyTrailingJSONAndCSRF(t *testing.T) {
	m := testModule(t)
	m.guard = guard{user: "user"}
	r := httptest.NewRequest("POST", "http://local/api/ecard/jobs", bytes.NewBufferString(`{} {}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	m.handle(w, r)
	if w.Code != 403 {
		t.Fatal("cross origin accepted")
	}
}

func TestBatchAdmissionIdempotencyAndChangedContent(t *testing.T) {
	m := testModule(t)
	m.guard = guard{user: "user"}
	id := "ed6eae56-069d-464e-bfa8-0dafb760e10a"
	body := map[string]interface{}{"operation_id": id, "request": testRequest(), "items": []Item{{ID: "a", Text: "hello"}}}
	send := func() int {
		b, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "/api/ecard/sign/batch?async=true", bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		m.handle(w, r)
		return w.Code
	}
	if code := send(); code != 202 {
		t.Fatal(code)
	}
	if code := send(); code != 202 {
		t.Fatal(code)
	}
	if len(m.queue) != 1 {
		t.Fatal("duplicate batch queued")
	}
	body["items"] = []Item{{ID: "a", Text: "changed"}}
	if code := send(); code != 409 {
		t.Fatal("changed content accepted", code)
	}
}
func TestUploadOffsetDuplicateAndResultPagination(t *testing.T) {
	m := testModule(t)
	req := testRequest()
	seed(t, m.store, "j", req, 0)
	m.store.db.Exec(`UPDATE jobs SET state='uploading',owner='local:user' WHERE id='j'`)
	m.guard = guard{user: "user"}
	send := func(body string) int {
		r := httptest.NewRequest("POST", "/api/ecard/jobs/j/items", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		m.handle(w, r)
		return w.Code
	}
	if send(`{"offset":0,"items":[{"id":"a","text":"abc"}]}`) != 200 {
		t.Fatal("upload failed")
	}
	if send(`{"offset":0,"items":[{"id":"b","text":"abc"}]}`) != 409 {
		t.Fatal("offset ignored")
	}
	if send(`{"offset":1,"items":[{"id":"a","text":"abc"}]}`) != 409 {
		t.Fatal("duplicate accepted")
	}
	j, _ := m.store.job("j", "local:user")
	if j.Total != 1 {
		t.Fatal("non-atomic upload")
	}
}
func TestPINChallengeOneUse(t *testing.T) {
	m := testModule(t)
	seed(t, m.store, "j", testRequest(), 1)
	m.store.db.Exec(`UPDATE jobs SET owner='local:user' WHERE id='j'`)
	m.guard = guard{user: "user"}
	m.active["j"] = &activeJob{pin: make(chan string, 1), challenge: "challenge", prompt: "PIN"}
	send := func() int {
		r := httptest.NewRequest("POST", "/api/ecard/jobs/j/pin", bytes.NewBufferString(`{"challenge":"challenge","value":"1234"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		m.handle(w, r)
		return w.Code
	}
	if send() != 200 || send() != 409 {
		t.Fatal("PIN challenge replay")
	}
}

type wsmGuard struct{}

func (wsmGuard) RequireSession(h http.Handler) http.Handler { return h }
func (wsmGuard) SessionIdentity(r *http.Request) (string, bool, bool) {
	p, ok := apibridge.PrincipalFrom(r.Context())
	return p.Subject, p.Admin, ok
}
func TestWSMUsesSameAPIAndIsolatesTenant(t *testing.T) {
	m := testModule(t)
	m.guard = wsmGuard{}
	req := testRequest()
	b, _ := json.Marshal(map[string]interface{}{"request": req, "items": []Item{{ID: "1", Text: "test"}}})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ecard/", m.handle)
	p := apibridge.Principal{Subject: "alice", TenantID: "tenant1"}
	res, e := apibridge.Invoke(context.Background(), mux, p, apibridge.Request{Method: "POST", Path: "/api/ecard/sign/batch?async=true", Body: b})
	if e != nil || res.Status != 202 {
		t.Fatalf("%+v %v", res, e)
	}
	id := <-m.queue
	p.TenantID = "tenant2"
	res, e = apibridge.Invoke(context.Background(), mux, p, apibridge.Request{Method: "GET", Path: "/api/ecard/jobs/" + id})
	if e != nil || res.Status != 404 {
		t.Fatal("cross-tenant job access")
	}
}
func TestSingleCallReturnsAllBatchResults(t *testing.T) {
	m := testModule(t)
	m.guard = guard{user: "user"}
	f := &fakeBackend{open: Reply{OK: true, Data: json.RawMessage(`{"cid":"cid"}`)}}
	m.factory = func(context.Context, string, Settings) (backend, error) { return f, nil }
	done := make(chan struct{})
	go func() { defer close(done); id := <-m.queue; m.run(context.Background(), id) }()
	b, _ := json.Marshal(map[string]interface{}{"request": testRequest(), "items": []Item{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}, {ID: "3", Text: "three"}}})
	r := httptest.NewRequest("POST", "/api/ecard/sign/batch", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	m.handle(w, r)
	<-done
	var out struct {
		Job   Job           `json:"job"`
		Items []interface{} `json:"items"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(e)
	}
	if w.Code != 200 || out.Job.State != "completed" || len(out.Items) != 3 || f.signs != 3 {
		t.Fatalf("bad single call response %+v", out)
	}
}
