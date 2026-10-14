package wisemedapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFormatterKeepsProvisionedWSSIdentityWithoutMedicalRegistration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected registration/bootstrap HTTP request: %s", r.URL.Path)
		http.Error(w, "unexpected", 500)
	}))
	defer server.Close()
	m, rt := newEquipmentTestModule(t, server.URL)
	rt.services["analyzer-config"] = map[string]interface{}{"protocol": "html-print-formatter"}
	for _, key := range []string{"unitate_medicala_id", "tip_de_echipament_id", "cod_echipament", "numar_serial_echipament"} {
		delete(m.settings, key)
	}
	m.settings["echipament_id"] = "77"
	m.settings["api_key_echipament"] = "dedicated-device-key-long-enough-for-wss"
	if !m.SetupComplete() || !m.HasEquipmentID() {
		t.Fatal("provisioned utility identity rejected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	out, err := m.EnsureEquipmentInitialized()
	if err != nil || out["echipament_id"] != "77" {
		t.Fatalf("WSS identity lost: %+v %v", out, err)
	}
	if m.Settings()["api_key_echipament"] == "" {
		t.Fatal("WSS credential erased")
	}
	delete(m.settings, "echipament_id")
	if !m.SetupComplete() || m.HasEquipmentID() {
		t.Fatal("HTTP/login incorrectly depends on WSS identity")
	}
}
