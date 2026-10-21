package siui

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func exampleCallback(t *testing.T) *wmCallback {
	t.Helper()
	var c wmCallback
	err := json.Unmarshal([]byte(`{"apipath":"/pacient/{cnp_pacient}","method":"POST","transform_path":[{"from_field":"cnp","to_field":"cnp_pacient"}],"body":{"verificare_la_data":"2026-01-01 12:22:22","transform_result":[{"from_field":"cid","to_field":"cid_pacient"},{"from_field":"state","to_field":"stare_asigurare_cas"}]}}`), &c)
	if err != nil {
		t.Fatal(err)
	}
	return &c
}
func TestWMCallbackTransforms(t *testing.T) {
	c := exampleCallback(t)
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	path, err := c.path(map[string]string{"pid": "1000000000000"})
	if err != nil || path != "/pacient/1000000000000" {
		t.Fatal(path, err)
	}
	body, err := c.body(map[string]any{"insured": map[string]any{"cid": "40172426789613933080", "state": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	var actual map[string]any
	json.Unmarshal(body, &actual)
	if actual["cid_pacient"] != "40172426789613933080" || actual["stare_asigurare_cas"] != float64(1) || actual["verificare_la_data"] != "2026-01-01 12:22:22" || len(actual) != 3 {
		t.Fatal(string(body))
	}
	for _, state := range []string{"0", "3"} {
		if _, err = c.body(map[string]any{"insured": map[string]any{"cid": "123", "state": state}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = c.body(map[string]any{"insured": map[string]any{"state": "1"}}); err == nil {
		t.Fatal("missing CID silently accepted")
	}
	if _, err = c.body(map[string]any{"parse_error": "bad XML"}); err == nil {
		t.Fatal("invalid CNAS data accepted")
	}
	for _, bad := range []string{"https://evil.example/", "//evil.example/x", "/../patient", "/x?key=y", "/x/%2e%2e", "/x/{missing}"} {
		c.Path = bad
		if c.validate() == nil {
			t.Fatal("unsafe template accepted", bad)
		}
	}
	c = exampleCallback(t)
	if _, err = c.path(map[string]string{"pid": "../admin"}); err == nil {
		t.Fatal("path injection")
	}
}

type callbackTestSender struct {
	mu     sync.Mutex
	calls  int
	bodies [][]byte
	ids    []string
	code   int
	tokens []string
}

func (s *callbackTestSender) CallbackDestination() (string, error) {
	return "https://wisemed.example/api", nil
}
func (s *callbackTestSender) SendCallback(_ context.Context, destination, method, path, id, lt string, body []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.tokens = append(s.tokens, lt)
	s.bodies = append(s.bodies, append([]byte(nil), body...))
	s.ids = append(s.ids, id)
	return s.code, nil
}

type callbackRuntime struct {
	*testRuntime
	sender wmSender
}

func (r *callbackRuntime) Service(name string) (interface{}, bool) {
	if name == "wisemed-api" {
		return r.sender, true
	}
	return r.testRuntime.Service(name)
}
func TestWMDeliveryRestartAndStableID(t *testing.T) {
	db, err := openJobs(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sender := &callbackTestSender{code: 503}
	m := &Module{db: db, rt: &callbackRuntime{testRuntime: &testRuntime{}, sender: sender}}
	spec, _ := json.Marshal(exampleCallback(t))
	_, err = db.Exec(`INSERT INTO siui_calls(id,owner,operation_id,operation,digest,status,created_at) VALUES('call','owner','op','getInsured','hash','completed','now');
 INSERT INTO siui_wm_callbacks(call_id,spec,parameters,destination,status,path,body) VALUES('call',?,'{}','https://wisemed.example/api','pending','/pacient/1000000000000','{"state":1}')`, string(spec))
	if err != nil {
		t.Fatal(err)
	}
	m.deliverCallback(context.Background())
	call, err := readServiceCall(db, "owner", "call")
	if err != nil || call.Status != "completed" || call.Callback.Status != "pending" || call.Callback.Attempts != 1 {
		t.Fatal(call, err)
	}
	// Simulate process interruption after claiming delivery; recovery does not rerun CNAS.
	_, err = db.Exec("UPDATE siui_wm_callbacks SET status='sending',next_attempt=0")
	if err != nil {
		t.Fatal(err)
	}
	if err = initializeServiceJobs(db); err != nil {
		t.Fatal(err)
	}
	sender.code = 204
	m.deliverCallback(context.Background())
	call, err = readServiceCall(db, "owner", "call")
	if err != nil || call.Callback.Status != "delivered" || call.Callback.Attempts != 2 {
		t.Fatal(call, err)
	}
	if sender.ids[0] != sender.ids[1] || string(sender.bodies[0]) != string(sender.bodies[1]) {
		t.Fatal("retry changed payload or id")
	}
	m.deliverCallback(context.Background())
	if sender.calls != 2 {
		t.Fatal("delivered twice")
	}
	if _, err = readServiceCall(db, "other", "call"); err == nil {
		t.Fatal("owner isolation broken")
	}
}
func TestWMCallbackQueueAndIdempotency(t *testing.T) {
	cnasBackend := &operationBackend{t: t}
	dir := t.TempDir()
	sender := &callbackTestSender{code: 204}
	rt := &callbackRuntime{testRuntime: &testRuntime{mux: http.NewServeMux(), guard: testGuard{}, path: filepath.Join(dir, "config.yaml"), settings: map[string]interface{}{"username": "123_CAS-DJ", "licence": "test", "certificate_thumbprint": strings.Repeat("A", 40), "database": filepath.Join(dir, "jobs.db")}}, sender: sender}
	m := &Module{nativeSupported: true, factory: func(settings) backend { return cnasBackend }}
	if err := m.Init(rt); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()
	<-m.ready
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	op, _ := findOperation("general", "getPersonInfo")
	request := map[string]any{"operation_id": "callback-test", "parameters": sampleParameters(op), "callwmapi": map[string]any{"lt": "per-call-secret", "apipath": "/test", "method": "PUT", "body": map[string]any{"transform_result": []map[string]string{{"from_field": "values.0", "to_field": "value"}}}}}
	raw, _ := json.Marshal(request)
	submit := func(admin bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/siui/general/getPersonInfo", strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		m.serviceHTTP(w, r, "general/getPersonInfo", "owner", admin)
		return w
	}
	w := submit(false)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var response struct {
		Call serviceCall `json:"call"`
	}
	json.Unmarshal(w.Body.Bytes(), &response)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		call, err := readServiceCall(m.db, "owner", response.Call.ID)
		if err != nil {
			t.Fatal(err)
		}
		if call.Callback != nil && call.Callback.Status == "delivered" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	call, err := readServiceCall(m.db, "owner", response.Call.ID)
	if err != nil || call.Callback == nil || call.Callback.Status != "delivered" {
		t.Fatal(call, err)
	}
	if w = submit(false); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	sender.mu.Lock()
	token := sender.tokens[0]
	sender.mu.Unlock()
	if token != "per-call-secret" {
		t.Fatal("wrong token")
	}
	var stored string
	if err := m.db.QueryRow("SELECT spec FROM siui_wm_callbacks WHERE call_id=?", call.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, token) {
		t.Fatal("plaintext token persisted")
	}
	if cnasBackend.post.Load() != 1 {
		t.Fatal("CNAS repeated")
	}
	request["callwmapi"].(map[string]any)["apipath"] = "/different"
	raw, _ = json.Marshal(request)
	if w = submit(false); w.Code != 409 {
		t.Fatal("callback not bound to idempotency", w.Code)
	}
}

func TestWMCallbackProtectedRecovery(t *testing.T) {
	database := filepath.Join(t.TempDir(), "jobs.db")
	callback := exampleCallback(t)
	callback.LT = "persistent-login-token"
	spec, err := encodeWMCallback(database, "call", callback)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(spec, callback.LT) {
		t.Fatal("plaintext secret")
	}
	var recovered wmCallback
	if err = decodeWMCallback(database, "call", spec, &recovered); err != nil || recovered.LT != callback.LT {
		t.Fatal("recovery failed", err)
	}
	if err = decodeWMCallback(database, "other-call", spec, &recovered); err == nil {
		t.Fatal("missing call binding")
	}
	legacy, _ := json.Marshal(exampleCallback(t))
	if err = decodeWMCallback(database, "legacy", string(legacy), &recovered); err != nil || recovered.LT != "" {
		t.Fatal("legacy recovery failed", err)
	}
}
func TestWMCallbackAuthorizationFailure(t *testing.T) {
	for _, code := range []int{401, 403} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			db, err := openJobs(filepath.Join(t.TempDir(), "jobs.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			sender := &callbackTestSender{code: code}
			m := &Module{db: db, rt: &callbackRuntime{testRuntime: &testRuntime{}, sender: sender}}
			spec, _ := json.Marshal(exampleCallback(t))
			_, err = db.Exec(`INSERT INTO siui_calls(id,owner,operation_id,operation,digest,status,created_at) VALUES('call','owner','op','getInsured','hash','completed','now');
INSERT INTO siui_wm_callbacks(call_id,spec,parameters,destination,status,path,body) VALUES('call',?,'{}','https://wisemed.example/api','pending','/test','{}')`, string(spec))
			if err != nil {
				t.Fatal(err)
			}
			m.deliverCallback(context.Background())
			m.deliverCallback(context.Background())
			call, err := readServiceCall(db, "owner", "call")
			if err != nil || call.Callback.Status != "failed" || call.Callback.HTTPStatus != code || sender.calls != 1 {
				t.Fatal(call, err, sender.calls)
			}
		})
	}
}

func TestWMCallbackPersonalisationBody(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "siui-downloads"), 0700); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	w, _ := z.Create("personalisation.xml")
	w.Write([]byte("<provider code=\"000123\"><service code=\"A\"/><service code=\"B\"/></provider>"))
	z.Close()
	if err := os.WriteFile(filepath.Join(dir, "siui-downloads", "id"), archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	var callback wmCallback
	if err := json.Unmarshal([]byte(`{"apipath":"/cnas/personalisation","method":"POST","file":{"unzip":true,"validateXSD":false},"body":{"transform_result":[{"from_field":"file_json","to_field":"personalisation"}]}}`), &callback); err != nil {
		t.Fatal(err)
	}
	if err := callback.validate(); err != nil {
		t.Fatal(err)
	}
	result := map[string]any{"file": map[string]any{"filename": "id.zip"}}
	body, err := callback.prepareBody(context.Background(), filepath.Join(dir, "jobs.db"), "id", result)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]fileXMLNode
	if err = json.Unmarshal(body, &parsed); err != nil || parsed["personalisation"].Name != "provider" || parsed["personalisation"].Attributes[0].Value != "000123" {
		t.Fatal(string(body), err)
	}
	if result["file_json"] != nil {
		t.Fatal("modified original result")
	}
	if _, err = callback.prepareBody(context.Background(), filepath.Join(dir, "jobs.db"), "id", map[string]any{}); err == nil {
		t.Fatal("missing download accepted")
	}
}
