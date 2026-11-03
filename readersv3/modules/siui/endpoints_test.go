package siui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"wisemed-labreaders/readersv3/core/config"
)

func TestProductionEndpointsAndExplicitLegacySettings(t *testing.T) {
	_, rt := serviceTestModule(t, &operationBackend{t: t})
	for _, tc := range []struct {
		settings         map[string]interface{}
		siui, pe, docmed string
	}{
		{map[string]interface{}{}, DefaultBaseURL, DefaultPEBaseURL, DefaultDocMedBaseURL},
		{map[string]interface{}{"base_url": "https://www.siui.ro:444", "pe_base_url": "https://testsipe.siui.ro:444", "docmed_base_url": "https://custom.example:8443"}, "https://www.siui.ro:444", "https://testsipe.siui.ro:444", "https://custom.example:8443"},
	} {
		copyRT := *rt
		copyRT.mux = http.NewServeMux()
		copyRT.settings = tc.settings
		m := &Module{}
		if err := m.Init(&copyRT); err != nil {
			t.Fatal(err)
		}
		if m.cfg.BaseURL != tc.siui || m.cfg.PEBaseURL != tc.pe || m.cfg.DocMedBaseURL != tc.docmed {
			t.Fatal("wrong defaults or explicit settings overwritten")
		}
		endpoints := dashboardEndpoints(m.cfg)
		want := map[string]string{"OCSP": tc.siui + "/OCSP/validator", "SiuiWS": tc.siui + "/svapntws/services/SiuiWS", "SiuiEInvoiceWS": tc.siui + "/svapntws/services/SiuiEInvoiceWS", "EPrescriptionWS": tc.pe + "/svapntws/services/EPrescriptionWS", "DocMedService": tc.docmed + "/loader/ws", "DocumentSeries": tc.docmed + "/seq/ws", "MedicalDocumentDownloader": tc.docmed + "/download/ws", "DocStatusChangerService": tc.docmed + "/status/ws"}
		for _, endpoint := range endpoints {
			if address, ok := want[endpoint.Service]; ok {
				if endpoint.URL != address {
					t.Fatalf("%s: %s", endpoint.Service, endpoint.URL)
				}
				delete(want, endpoint.Service)
			}
		}
		if len(want) != 0 {
			t.Fatalf("missing destinations: %v", want)
		}
	}
}

func TestDocMedProfileSettingsPersistAndRemainOptional(t *testing.T) {
	m, rt := serviceTestModule(t, &operationBackend{t: t})
	body := map[string]string{"fiscal_code": "7991962", "cas_code": "CAS-DJ", "certificate_store": "CurrentUser", "certificate_thumbprint": m.cfg.Thumbprint, "base_url": DefaultBaseURL, "pe_base_url": DefaultPEBaseURL, "einvoice_base_url": DefaultBaseURL, "docmed_base_url": DefaultDocMedBaseURL}
	for attempt := 0; attempt < 2; attempt++ {
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest("PUT", "/api/siui/settings", strings.NewReader(string(payload)))
		req.Header.Set("Authorization", "test-local-session")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		rt.mux.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		saved, err := config.Load(rt.path)
		if err != nil {
			t.Fatal(err)
		}
		copyRT := *rt
		copyRT.mux = http.NewServeMux()
		copyRT.settings = saved.ModuleSettings("siui")
		restarted := &Module{}
		if err := restarted.Init(&copyRT); err != nil {
			t.Fatal(err)
		}
		if restarted.cfg.DocMedBaseURL != DefaultDocMedBaseURL || len(restarted.cfg.Profiles) != 1 || restarted.cfg.Profiles[0].DocMedBaseURL != DefaultDocMedBaseURL {
			t.Fatal("DocMed profile was not persisted")
		}
		req = httptest.NewRequest("GET", "/api/siui/settings", nil)
		req.Header.Set("Authorization", "test-local-session")
		w = httptest.NewRecorder()
		copyRT.mux.ServeHTTP(w, req)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"DocMedService":"https://docmed.e-pias.ro/loader/ws"`) {
			t.Fatal(w.Body.String())
		}
		delete(body, "docmed_base_url") // Older clients preserve the saved DocMed setting.
	}
}

func TestOfficialOriginsRestrictNewHostsToHTTPSDefaultPort(t *testing.T) {
	for _, host := range []string{"siui.e-pias.ro", "sipe.e-pias.ro", "docmed.e-pias.ro", "testsiui.siui.ro", "testsipe.siui.ro"} {
		for _, suffix := range []string{"", ":443"} {
			if !knownOrigin("https://" + host + suffix) {
				t.Fatal(host, suffix)
			}
		}
	}
	for _, address := range []string{"https://sipe.e-pias.ro:444", "https://docmed.e-pias.ro:444", "https://siui.e-pias.ro:444", "https://siui.e-pias.ro.attacker.example", "https://unrelated.e-pias.ro"} {
		if knownOrigin(address) {
			t.Fatal("unexpected implicit destination", address)
		}
	}
	// An operator can still explicitly configure a private/legacy port locally.
	cfg := settings{BaseURL: "https://siui.e-pias.ro:444"}
	if !cfg.acceptsOrigin(cfg.BaseURL) {
		t.Fatal("explicit local setting lost")
	}
}
