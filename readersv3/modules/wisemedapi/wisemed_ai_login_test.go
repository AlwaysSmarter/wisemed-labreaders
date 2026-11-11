package wisemedapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWiseMEDAIDoesNotRegisterAsLaboratoryEquipment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected equipment request: %s", r.URL.Path)
		http.Error(w, "unexpected", 500)
	}))
	defer server.Close()
	m, rt := newEquipmentTestModule(t, server.URL)
	rt.services["analyzer-config"] = map[string]interface{}{"protocol": "wisemed-ai"}
	for _, key := range []string{"unitate_medicala_id", "tip_de_echipament_id", "echipament_id", "cod_echipament", "numar_serial_echipament"} {
		delete(m.settings, key)
	}
	if !m.LoginOnly() || !m.SetupComplete() {
		t.Fatal("AI utility incorrectly requires laboratory registration")
	}
	if _, err := m.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.EnsureEquipmentOnline(nil); err != nil {
		t.Fatal(err)
	}
}
