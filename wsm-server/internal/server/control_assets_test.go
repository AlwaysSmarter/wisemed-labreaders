package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestControlServesRequiredDocMedModulesOnly(t *testing.T) {
	cfg := testConfig(t)
	dir := t.TempDir()
	cfg.Server.ControlUIDir = dir
	for _, name := range []string{"index.html", "app.js", "docmed-forms.js", "siui-invoice.js", "config.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture:"+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s := New(cfg)
	t.Cleanup(s.Close)
	for _, path := range []string{"/docmed-forms.js", "/control/docmed-forms.js", "/siui-invoice.js", "/control/siui-invoice.js"} {
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || (!strings.Contains(w.Body.String(), "fixture:docmed-forms.js") && !strings.Contains(w.Body.String(), "fixture:siui-invoice.js")) {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, httptest.NewRequest("GET", "/control/config.yaml", nil))
	if strings.Contains(w.Body.String(), "fixture:config.yaml") {
		t.Fatal("control directory exposes non-frontend files")
	}
}
