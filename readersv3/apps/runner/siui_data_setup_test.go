package runner

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/modules/wisemedapi"
)

func completeSIUIConfig() *config.Config {
	cfg := config.Default()
	cfg.Analyzer.Protocol = "siui-data"
	cfg.Modules["siui-data"] = map[string]interface{}{"postgres_database": "cnas", "postgres_user": "app"}
	cfg.Modules["wisemed-api"] = map[string]interface{}{"cfg_wisemed_protocol": "https", "cfg_wisemed_ip": "example.invalid", "cfg_wisemed_port": "443", "cfg_wisemed_path": "/api", "cfg_wisemed_key": "test-key", "unitate_medicala_id": "1", "tip_de_echipament_id": "2", "cod_echipament": "cnas", "numar_serial_echipament": "cnas-001", "echipament_id": "42"}
	cfg.WiseMedWS.Enabled = false
	cfg.Modules["wisemed-ws"] = map[string]interface{}{"enabled": false}
	return cfg
}

func TestSIUIDataSetupKeepsSecretsAndSkipsSQLite(t *testing.T) {
	settings := map[string]interface{}{"postgres_host": "db.internal", "postgres_database": "cnas", "postgres_user": "app", "postgres_password": "keep-private", "local_import_dir": "/catalogues"}
	if e := promptSIUIDataDatabase(bufio.NewReader(strings.NewReader(strings.Repeat("\n", 16))), settings); e != nil {
		t.Fatal(e)
	}
	if settings["postgres_password"] != "keep-private" || settings["postgres_host"] != "db.internal" || settings["local_import_dir"] != "/catalogues" || settings["postgres_port"] != 5432 {
		t.Fatal("setup reset operator choices")
	}
	cfg := completeSIUIConfig()
	cfg.Modules["siui-data"] = settings
	if changed, e := ensureBootstrap(cfg, false); e != nil || changed {
		t.Fatalf("configured utility ran bootstrap: %v %v", changed, e)
	}
	bootstrapModuleSettings(bufio.NewReader(strings.NewReader(strings.Repeat("\n", 10))), cfg, false)
	if _, ok := cfg.Modules["storage-sqlite"]; ok {
		t.Fatal("utility prompted for SQLite")
	}
	if _, ok := cfg.Modules["result-sync"]; ok {
		t.Fatal("utility enabled result sync")
	}
	delete(cfg.Modules["wisemed-api"], "echipament_id")
	if !needsSIUIDataBootstrap(cfg) {
		t.Fatal("PostgreSQL configuration bypassed equipment registration")
	}
}

func equipmentAPIServer(t *testing.T, cfg *config.Config, response string, check func(map[string]interface{})) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/administrative/medicalunits":
			w.Write([]byte(`[{"medical_unit_id":1,"medical_unit_name":"Test"}]`))
		case "/api/administrative/wmanalyzertypes":
			w.Write([]byte(`[{"analyzer_type_id":2,"analyzer_type_name":"Utility"}]`))
		case "/api/administrative/analyzer":
			if r.Method != "PUT" {
				t.Error("registration must use PUT")
			}
			var payload map[string]interface{}
			if e := json.NewDecoder(r.Body).Decode(&payload); e != nil {
				t.Error(e)
			}
			if check != nil {
				check(payload)
			}
			w.Write([]byte(response))
		default:
			t.Errorf("unexpected API path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	s := cfg.Modules["wisemed-api"]
	s["cfg_wisemed_protocol"] = "http"
	s["cfg_wisemed_ip"] = u.Hostname()
	s["cfg_wisemed_port"] = u.Port()
	return server
}

func TestSIUIDataFullSetupRegistersEquipmentAndPersistsWSM(t *testing.T) {
	cfg := completeSIUIConfig()
	api := cfg.Modules["wisemed-api"]
	for _, key := range []string{"echipament_id", "unitate_medicala_id", "tip_de_echipament_id", "cod_echipament", "numar_serial_echipament"} {
		delete(api, key)
	}
	cfg.Modules["siui-data"]["postgres_password"] = "private-postgres"
	cfg.Modules["wisemed-ws"] = map[string]interface{}{"enabled": true, "url": "wss://wsm.example.invalid/ws", "auth_mode": "device_key", "tenant_id": "tenant", "key_id": "key", "issuer": "issuer", "device_secret": strings.Repeat("s", 32)}
	equipmentAPIServer(t, cfg, `{"echipament_id":123,"api_key_echipament":"equipment-key"}`, func(p map[string]interface{}) {
		if p["unitate_medicala_id"] != "1" || p["tip_de_echipament_id"] != "2" {
			t.Error("selection not used in registration")
		}
	})
	// 14 database fields, 5 API fields, 2 selections, then defaults for identity/WSM/updates.
	input := strings.Repeat("\n", 19) + "1\n1\n" + strings.Repeat("\n", 40)
	if changed, e := configureSIUIData(cfg, false, bufio.NewReader(strings.NewReader(input))); e != nil || !changed {
		t.Fatalf("setup: %v %v", changed, e)
	}
	if strSetting(api, "echipament_id") != "123" || !cfg.WiseMedWS.Enabled || needsSIUIDataBootstrap(cfg) {
		t.Fatal("setup did not complete equipment/WSM identity")
	}
	// Reconfiguration keeps the equipment identity, rather than creating an unrelated one.
	equipmentAPIServer(t, cfg, `{"equipment_id":123}`, func(p map[string]interface{}) {
		if p["echipament_id"] != "123" {
			t.Error("reconfigure dropped existing equipment ID")
		}
	})
	if e := configureSIUIDataEquipment(cfg, bufio.NewReader(strings.NewReader(strings.Repeat("\n", 8))), true); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	b, e := yaml.Marshal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	loaded, e := config.Load(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = loaded.Save(); e != nil {
		t.Fatal(e)
	}
	loaded, e = config.Load(path)
	if e != nil {
		t.Fatal(e)
	}
	if strSetting(loaded.Modules["wisemed-api"], "echipament_id") != "123" || strSetting(loaded.Modules["wisemed-ws"], "device_secret") != strings.Repeat("s", 32) || strSetting(loaded.Modules["siui-data"], "postgres_password") != "private-postgres" {
		t.Fatal("saved configuration lost identity or secrets")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("configuration permissions expose secrets")
	}
}

func TestSIUIDataRejectsInvalidRegistrationWithoutReplacingIdentity(t *testing.T) {
	for _, response := range []string{`{}`, `{"echipament_id":0,"api_key":"wrong"}`, `{"equipment_id":"bad"}`} {
		t.Run(response, func(t *testing.T) {
			cfg := completeSIUIConfig()
			cfg.Modules["wisemed-api"]["api_key_echipament"] = "keep"
			equipmentAPIServer(t, cfg, response, nil)
			client := wisemedapi.NewBootstrapClient(stringSettings(cfg.Modules["wisemed-api"]), callerTypeForProtocol(cfg.Analyzer.Protocol))
			if e := registerEquipment(cfg, client); e == nil {
				t.Fatal("invalid registration accepted")
			}
			if strSetting(cfg.Modules["wisemed-api"], "echipament_id") != "42" || cfg.Modules["wisemed-api"]["api_key_echipament"] != "keep" {
				t.Fatal("failed registration replaced identity")
			}
		})
	}
}

func TestSIUIDataWSMAuthenticationValidation(t *testing.T) {
	cfg := completeSIUIConfig()
	s := map[string]interface{}{"enabled": true, "url": "wss://wsm.example.invalid/ws", "auth_mode": "device_key", "tenant_id": "tenant", "key_id": "key", "issuer": "issuer", "device_secret": strings.Repeat("x", 32)}
	cfg.Modules["wisemed-ws"] = s
	if e := promptSIUIDataWSS(cfg, bufio.NewReader(strings.NewReader(strings.Repeat("\n", 20)))); e != nil {
		t.Fatal(e)
	}
	if s["device_secret"] != strings.Repeat("x", 32) || !strings.Contains(strSetting(s, "scopes"), "api:invoke") {
		t.Fatal("WSM secret/scopes lost")
	}
	s["device_secret"] = "short"
	if validateSIUIDataWSS(cfg) == nil {
		t.Fatal("short key accepted")
	}
	s["auth_mode"] = "token_file"
	if validateSIUIDataWSS(cfg) == nil {
		t.Fatal("missing token file accepted")
	}
	s["token_file"] = "token.jwt"
	if e := validateSIUIDataWSS(cfg); e != nil {
		t.Fatal(e)
	}
	s["auth_mode"] = "token_endpoint"
	if validateSIUIDataWSS(cfg) == nil {
		t.Fatal("missing token endpoint accepted")
	}
	s["token_path"] = "/token"
	if e := validateSIUIDataWSS(cfg); e != nil {
		t.Fatal(e)
	}
	s["url"] = "ws://insecure/ws"
	if validateSIUIDataWSS(cfg) == nil {
		t.Fatal("insecure WSM URL accepted")
	}
}

func TestSIUIDataEquipmentSelectionsRejectEOF(t *testing.T) {
	cfg := completeSIUIConfig()
	equipmentAPIServer(t, cfg, `{}`, nil)
	client := wisemedapi.NewBootstrapClient(stringSettings(cfg.Modules["wisemed-api"]), callerTypeForProtocol(cfg.Analyzer.Protocol))
	for _, selectFn := range []func(*bufio.Reader, *wisemedapi.BootstrapClient, map[string]interface{}) error{promptMedicalUnitSelection, promptEquipmentTypeSelection} {
		if e := selectFn(bufio.NewReader(strings.NewReader("")), client, map[string]interface{}{}); e == nil {
			t.Fatal("selection accepted EOF")
		}
	}
}
