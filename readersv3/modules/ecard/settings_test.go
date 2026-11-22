package ecard

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
)

type settingsRuntime struct {
	module.Runtime
	path string
}

func (r settingsRuntime) ConfigPath() string { return r.path }

func TestUMConnectionSettings(t *testing.T) {
	defaults, err := readSettings(nil)
	if err != nil || defaults.UMHost != "umceas.siui.ro" || defaults.UMPort != 443 {
		t.Fatalf("defaults: %+v, %v", defaults, err)
	}
	for _, host := range []string{"testumceas.siui.ro", "custom.example", "127.0.0.1", "::1"} {
		s, err := readSettings(map[string]interface{}{"um_host": host, "um_port": 8443})
		if err != nil || s.UMHost != host || s.UMPort != 8443 {
			t.Fatalf("custom endpoint: %+v, %v", s, err)
		}
	}
	for _, host := range []string{"", " ", "tcp://umceas.siui.ro", "https://umceas.siui.ro", "umceas.siui.ro:443", "umceas.siui.ro/path", "a\nb", "-invalid.example"} {
		if _, err := readSettings(map[string]interface{}{"um_host": host}); err == nil {
			t.Errorf("accepted invalid host %q", host)
		}
	}
	for _, port := range []int{0, -1, 65536} {
		if _, err := readSettings(map[string]interface{}{"um_port": port}); err == nil {
			t.Errorf("accepted invalid port %d", port)
		}
	}
}

func TestSettingsApplyImmediatelyAndPersist(t *testing.T) {
	m := testModule(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("modules:\n  ecard:\n    operation_timeout_seconds: 210\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m.rt = settingsRuntime{path: path}
	body := `{"sdk_order":["1.3.0.4","2.0.0.4"],"reader_name":"Gemalto USB Smart Card Reader 0","um_host":"testumceas.siui.ro","um_port":8443}`
	save := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		m.saveSettings(w, httptest.NewRequest("PUT", "/api/ecard/settings", bytes.NewBufferString(body)))
		return w
	}
	m.active["busy"] = &activeJob{}
	if w := save(); w.Code != 409 {
		t.Fatalf("busy save: %d", w.Code)
	}
	delete(m.active, "busy")
	m.queue <- "queued"
	if w := save(); w.Code != 409 {
		t.Fatalf("queued save: %d", w.Code)
	}
	<-m.queue
	if m.settings.UMHost != "umceas.siui.ro" {
		t.Fatal("busy save changed settings")
	}
	w := save()
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var reply struct {
		Restart bool `json:"restart_required"`
	}
	json.Unmarshal(w.Body.Bytes(), &reply)
	if reply.Restart {
		t.Fatal("restart required")
	}
	active := m.settingsSnapshot()
	if active.UMHost != "testumceas.siui.ro" || active.UMPort != 8443 || active.Reader != "Gemalto USB Smart Card Reader 0" || active.SDKOrder[0] != "1.3.0.4" {
		t.Fatalf("not applied: %+v", active)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := readSettings(cfg.ModuleSettings("ecard"))
	if err != nil {
		t.Fatal(err)
	}
	if saved.UMHost != active.UMHost || saved.OperationTimeout != 210 {
		t.Fatalf("persistence: %+v", saved)
	}
	m.rt = settingsRuntime{path: filepath.Join(t.TempDir(), "missing.yaml")}
	before := m.settingsSnapshot()
	if w := save(); w.Code != 500 {
		t.Fatal("expected persistence failure")
	}
	if m.settings.UMHost != before.UMHost {
		t.Fatal("failed save changed settings")
	}
}
