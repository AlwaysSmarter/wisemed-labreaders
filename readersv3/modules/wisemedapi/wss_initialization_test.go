package wisemedapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"gopkg.in/yaml.v3"
	"wisemed-labreaders/readersv3/core/module"
)

type equipmentTestRuntime struct {
	path     string
	settings map[string]interface{}
	services map[string]interface{}
	mux      *http.ServeMux
}

func (r *equipmentTestRuntime) ConfigPath() string          { return r.path }
func (r *equipmentTestRuntime) ConfigDir() string           { return filepath.Dir(r.path) }
func (r *equipmentTestRuntime) ReaderID() string            { return "test-reader" }
func (r *equipmentTestRuntime) Logf(string, ...interface{}) {}
func (r *equipmentTestRuntime) ModuleSettings(id string) map[string]interface{} {
	if id == "wisemed-api" {
		return r.settings
	}
	return map[string]interface{}{}
}
func (r *equipmentTestRuntime) ResolvePath(path string) string {
	return filepath.Join(r.ConfigDir(), path)
}
func (r *equipmentTestRuntime) AddMenu(...module.MenuEntry) {}
func (r *equipmentTestRuntime) Handle(pattern string, handler http.Handler) {
	r.mux.Handle(pattern, handler)
}
func (r *equipmentTestRuntime) Mux() *http.ServeMux { return r.mux }
func (r *equipmentTestRuntime) RegisterService(name string, service interface{}) {
	r.services[name] = service
}
func (r *equipmentTestRuntime) Service(name string) (interface{}, bool) {
	s, ok := r.services[name]
	return s, ok
}

type forbiddenEquipmentWS struct{ calls int32 }

func (s *forbiddenEquipmentWS) Connected() bool { return true }
func (s *forbiddenEquipmentWS) Request(context.Context, string, map[string]interface{}) (map[string]interface{}, error) {
	atomic.AddInt32(&s.calls, 1)
	return nil, fmt.Errorf("initialization must use HTTP")
}

func newEquipmentTestModule(t *testing.T, endpoint string) (*Module, *equipmentTestRuntime) {
	t.Helper()
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	rt := &equipmentTestRuntime{path: filepath.Join(t.TempDir(), "config.yaml"), mux: http.NewServeMux(), services: map[string]interface{}{}, settings: map[string]interface{}{
		"cfg_wisemed_protocol": parsed.Scheme, "cfg_wisemed_ip": parsed.Hostname(), "cfg_wisemed_port": parsed.Port(), "cfg_wisemed_path": "/api", "cfg_wisemed_key": "test-only-wiseMED-key",
		"unitate_medicala_id": "1", "tip_de_echipament_id": "2", "cod_echipament": "TEST", "numar_serial_echipament": "SN-TEST",
	}}
	data, err := yaml.Marshal(map[string]interface{}{"reader": map[string]string{"id": "test-reader"}, "modules": map[string]interface{}{"wisemed-api": rt.settings, "untouched": map[string]string{"setting": "preserve-me"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rt.path, data, 0600); err != nil {
		t.Fatal(err)
	}
	m := &Module{}
	if err := m.Init(rt); err != nil {
		t.Fatal(err)
	}
	return m, rt
}

func TestEquipmentInitializationUsesHTTPOnceAndPersists(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Method != http.MethodPut || r.URL.Path != "/api/administrative/analyzer" {
			t.Errorf("unexpected registration: %s %s", r.Method, r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("missing API JWT")
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["online"] != true || body["cod_echipament"] != "TEST" {
			t.Errorf("unexpected registration payload: %+v", body)
		}
		fmt.Fprint(w, `{"echipament_id":123,"api_key_echipament":"device-key-for-equipment-123"}`)
	}))
	defer server.Close()
	m, rt := newEquipmentTestModule(t, server.URL)
	ws := &forbiddenEquipmentWS{}
	rt.services["wisemed-ws"] = ws
	rt.services["wisemed-ws-client"] = ws
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.EnsureEquipmentInitialized(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if atomic.LoadInt32(&calls) != 1 || atomic.LoadInt32(&ws.calls) != 0 {
		t.Fatalf("HTTP calls=%d WSS calls=%d", calls, ws.calls)
	}
	// Starting the API module after WSS initialized it must not register again.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatal("startup duplicated registration")
	}
	data, err := os.ReadFile(rt.path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted struct {
		Modules map[string]map[string]interface{} `yaml:"modules"`
	}
	if err := yaml.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	settings := persisted.Modules["wisemed-api"]
	if settings["echipament_id"] != "123" || settings["api_key_echipament"] != "device-key-for-equipment-123" || persisted.Modules["untouched"]["setting"] != "preserve-me" {
		t.Fatalf("bad persistence: %+v", persisted)
	}
	if _, err := m.EnsureEquipmentOnline(map[string]interface{}{"label": "Changed"}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatal("explicit refresh did not use HTTP")
	}
}

func TestEquipmentInitializationRetriesAfterHTTPFailure(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			http.Error(w, "temporary outage", 503)
			return
		}
		fmt.Fprint(w, `{"echipament_id":7}`)
	}))
	defer server.Close()
	m, _ := newEquipmentTestModule(t, server.URL)
	if _, err := m.EnsureEquipmentInitialized(); err == nil {
		t.Fatal("outage accepted")
	}
	if _, err := m.EnsureEquipmentInitialized(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.EnsureEquipmentInitialized(); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("unexpected retries: %d", calls)
	}
}

func TestEquipmentInitializationRejectsInvalidIDs(t *testing.T) {
	for _, value := range []string{`0`, `-1`, `"abc"`, `null`, `1.5`, `""`} {
		t.Run(value, func(t *testing.T) {
			var calls int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				fmt.Fprintf(w, `{"echipament_id":%s}`, value)
			}))
			defer server.Close()
			m, rt := newEquipmentTestModule(t, server.URL)
			if _, err := m.SaveSetup(map[string]string{"echipament_id": "99", "api_key_echipament": "previous-valid-key"}); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(rt.path)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if _, err := m.EnsureEquipmentInitialized(); err == nil {
					t.Fatalf("invalid ID %s accepted", value)
				}
			}
			if atomic.LoadInt32(&calls) != 2 {
				t.Fatal("failed validation cached as success")
			}
			after, err := os.ReadFile(rt.path)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || m.Settings()["echipament_id"] != "99" || m.Settings()["api_key_echipament"] != "previous-valid-key" {
				t.Fatal("invalid registration corrupted existing configuration")
			}
		})
	}
}

func TestWSSAccessTokenUsesAuthenticatedJSONRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/device/wss-token" || r.Header.Get("Content-Type") != "application/json" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("bad token request: %+v", r)
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["equipment_id"] != "123" {
			t.Errorf("missing equipment identity: %+v", payload)
		}
		fmt.Fprint(w, `{"token":"issued.jwt.token"}`)
	}))
	defer server.Close()
	m, _ := newEquipmentTestModule(t, server.URL)
	token, err := m.WSSAccessToken(context.Background(), "/device/wss-token", map[string]interface{}{"equipment_id": "123"})
	if err != nil || token != "issued.jwt.token" {
		t.Fatalf("token=%q err=%v", token, err)
	}
}

func TestWSSAccessTokenRejectsUnsafePathsAndRedirects(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Redirect(w, r, "/api/redirect-target", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	m, _ := newEquipmentTestModule(t, server.URL)
	for _, path := range []string{"", "https://other.example/token", "//other.example/token", "relative", "/token?key=abc", "/token#fragment"} {
		if _, err := m.WSSAccessToken(context.Background(), path, nil); err == nil {
			t.Errorf("accepted unsafe path %q", path)
		}
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatal("unsafe path reached network")
	}
	if _, err := m.WSSAccessToken(context.Background(), "/token", nil); err == nil {
		t.Fatal("redirect accepted")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatal("token endpoint followed redirect")
	}
}

func TestWSSAccessTokenRejectsBadResponsesAndCancellation(t *testing.T) {
	for _, body := range []string{`{}`, `{"token":null}`, `not-json`, `{"token":"` + strings.Repeat("a", 17000) + `"}`} {
		t.Run(fmt.Sprint(len(body)), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			m, _ := newEquipmentTestModule(t, server.URL)
			if _, err := m.WSSAccessToken(context.Background(), "/token", nil); err == nil {
				t.Fatal("bad token response accepted")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := m.WSSAccessToken(ctx, "/token", nil); err == nil {
				t.Fatal("canceled request succeeded")
			}
		})
	}
}

func TestEquipmentInitializationRequiresIDInResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"api_key_echipament":"should-not-persist"}`)
	}))
	defer server.Close()
	m, rt := newEquipmentTestModule(t, server.URL)
	if _, err := m.SaveSetup(map[string]string{"echipament_id": "99", "api_key_echipament": "original-key"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(rt.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.EnsureEquipmentInitialized(); err == nil {
		t.Fatal("missing response ID accepted using stale stored identity")
	}
	after, err := os.ReadFile(rt.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || m.Settings()["api_key_echipament"] != "original-key" {
		t.Fatal("failed registration changed credentials")
	}
}
