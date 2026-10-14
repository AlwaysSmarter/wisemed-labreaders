package localhttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wisemed-labreaders/readersv3/modules/wisemedapi"
)

type formatterAuthRuntime struct {
	*wssSecretRuntime
	path     string
	settings map[string]interface{}
	mux      *http.ServeMux
}

func (r *formatterAuthRuntime) ConfigPath() string { return r.path }
func (r *formatterAuthRuntime) ConfigDir() string  { return filepath.Dir(r.path) }
func (r *formatterAuthRuntime) ModuleSettings(id string) map[string]interface{} {
	if id == "wisemed-api" {
		return r.settings
	}
	return nil
}
func (r *formatterAuthRuntime) Handle(path string, h http.Handler) { r.mux.Handle(path, h) }
func (r *formatterAuthRuntime) Logf(string, ...interface{})        {}

func TestFormatterLoginWithoutMedicalUnitOrEquipment(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/administrative/login" || r.Method != "PUT" {
			t.Errorf("unexpected equipment request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 500)
			return
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if _, present := body["medical_unit_id"]; present {
			t.Error("medical unit sent for utility login")
		}
		if body["password"] != "correct" {
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user_id":7,"login":"admin","user_type":0,"login_token":"server-token-private"}`))
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	rt := &formatterAuthRuntime{wssSecretRuntime: &wssSecretRuntime{services: map[string]interface{}{"analyzer-config": map[string]interface{}{"protocol": "html-print-formatter"}}}, path: filepath.Join(t.TempDir(), "config.yaml"), mux: http.NewServeMux(), settings: map[string]interface{}{"cfg_wisemed_protocol": "http", "cfg_wisemed_ip": u.Hostname(), "cfg_wisemed_port": u.Port(), "cfg_wisemed_path": "/api", "cfg_wisemed_key": "private-api-key"}}
	data, _ := json.Marshal(map[string]interface{}{"modules": map[string]interface{}{"wisemed-api": rt.settings}})
	if err := os.WriteFile(rt.path, data, 0600); err != nil {
		t.Fatal(err)
	}
	api := wisemedapi.New()
	if err := api.Init(rt); err != nil {
		t.Fatal(err)
	}
	m := &Module{rt: rt, utilitySessionKey: bytes.Repeat([]byte{42}, 32), sessions: map[string]session{}, sessionAuth: map[string]sessionAuth{}}
	service := m.wiseMEDAPI()
	if !service.SetupComplete() || service.HasEquipmentID() {
		t.Fatal("utility setup incorrectly requires equipment")
	}
	if _, err := service.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnsureEquipmentOnline(nil); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("bootstrap or registration contacted server")
	}
	request := func(handler http.HandlerFunc, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	bad := request(m.handleLogin, "POST", "/api/session/login", `{"username":"admin","password":"wrong"}`, nil)
	if bad.Code != 401 {
		t.Fatalf("invalid login accepted: %d %s", bad.Code, bad.Body.String())
	}
	good := request(m.handleLogin, "POST", "/api/session/login", `{"username":"admin","password":"correct"}`, nil)
	if good.Code != 200 {
		t.Fatalf("login: %d %s", good.Code, good.Body.String())
	}
	cookies := good.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing session")
	}
	if calls != 2 {
		t.Fatalf("login must not register equipment; calls=%d", calls)
	}
	protected := m.requireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, httptest.NewRequest("PUT", "/api/reader-settings", nil))
	if w.Code != 401 {
		t.Fatalf("settings not protected: %d", w.Code)
	}
	r := httptest.NewRequest("PUT", "/api/reader-settings", nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	protected.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal("valid login cannot configure")
	}
	for _, cookie := range []*http.Cookie{nil, cookies[0]} {
		saved := request(m.handleWiseMEDSetup, "PUT", "/api/wisemed/setup", `{"cfg_wisemed_path":"/api"}`, cookie)
		expected := 200
		if cookie == nil {
			expected = 401
		}
		if saved.Code != expected {
			t.Fatalf("setup protection: %d %s", saved.Code, saved.Body.String())
		}
	}
	public := request(m.handleWiseMEDSetup, "GET", "/api/wisemed/setup", "", nil)
	if strings.Contains(public.Body.String(), "private-api-key") || strings.Contains(public.Body.String(), "server-token-private") || !strings.Contains(public.Body.String(), `"login_only":true`) {
		t.Fatalf("unsafe setup: %s", public.Body.String())
	}
}
