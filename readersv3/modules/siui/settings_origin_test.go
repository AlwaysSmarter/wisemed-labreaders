package siui

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"wisemed-labreaders/readersv3/core/config"
)

func TestSettingsReportsRejectedOriginField(t *testing.T) {
	for _, field := range []string{"base_url", "pe_base_url", "einvoice_base_url", "docmed_base_url"} {
		t.Run(field, func(t *testing.T) {
			m, rt := serviceTestModule(t, &operationBackend{t: t})
			for _, address := range []string{"https://www.siui.ro:8443", "https://custom.example", "https://sipe.siui.ro/service"} {
				body := map[string]string{"username": "123_CAS-DJ", "certificate_store": "CurrentUser", "certificate_thumbprint": m.cfg.Thumbprint, "base_url": "https://www.siui.ro", "pe_base_url": "https://sipe.siui.ro", "einvoice_base_url": "https://www.siui.ro"}
				body[field] = address
				payload, _ := json.Marshal(body)
				req := httptest.NewRequest("PUT", "/api/siui/settings", strings.NewReader(string(payload)))
				req.Header.Set("Authorization", "test-local-session")
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				rt.mux.ServeHTTP(w, req)
				if w.Code != 400 || !strings.Contains(w.Body.String(), field) {
					t.Fatalf("%s: status=%d body=%s", address, w.Code, w.Body.String())
				}
				if m.cfg.BaseURL != "https://www.siui.ro" || m.cfg.PEBaseURL != "https://sipe.siui.ro" {
					t.Fatal("rejected request changed active settings")
				}
			}
		})
	}
}

func TestSettingsAcceptsEPiasSIUIOrigin(t *testing.T) {
	m, rt := serviceTestModule(t, &operationBackend{t: t})
	body := `{"fiscal_code":"7991962","cas_code":"CAS-DJ","certificate_store":"CurrentUser","base_url":"https://siui.e-pias.ro","pe_base_url":"https://sipe.siui.ro","einvoice_base_url":"https://siui.e-pias.ro"}`
	req := httptest.NewRequest("PUT", "/api/siui/settings", strings.NewReader(body))
	req.Header.Set("Authorization", "test-local-session")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	rt.mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("save e-pias settings: %d %s", w.Code, w.Body.String())
	}
	saved, err := config.Load(rt.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, settings := range []map[string]interface{}{saved.ModuleSettings("siui"), {"base_url": m.cfg.BaseURL, "pe_base_url": m.cfg.PEBaseURL, "einvoice_base_url": m.cfg.EInvoiceBaseURL}} {
		if settings["base_url"] != "https://siui.e-pias.ro" || settings["einvoice_base_url"] != "https://siui.e-pias.ro" || settings["pe_base_url"] != "https://sipe.siui.ro" {
			t.Fatalf("origins not retained: %v", settings)
		}
	}
	for _, address := range []string{"https://siui.e-pias.ro.attacker.example", "https://unrelated.e-pias.ro", "https://siui.e-pias.ro:8443"} {
		if knownOrigin(address) {
			t.Errorf("unexpected allowed origin: %s", address)
		}
	}
}
