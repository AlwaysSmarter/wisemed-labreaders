package runner

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/modules/wisemedapi"
)

const siuiDataWSSScopes = "route:reply,route:event,route:command,devices:debug,connections:read,server:status,api:invoke"

// PostgreSQL and the WiseMED/WSM equipment identity are both part of setup.
// The utility does not require SQLite or an analyzer transport.
func ensureSIUIDataBootstrap(cfg *config.Config, reconfigure bool) (bool, error) {
	if !reconfigure && !needsSIUIDataBootstrap(cfg) {
		return false, nil
	}
	return configureSIUIData(cfg, reconfigure, bufio.NewReader(os.Stdin))
}

func needsSIUIDataBootstrap(cfg *config.Config) bool {
	s := cfg.ModuleSettings("siui-data")
	if strSetting(s, "postgres_database") == "" || strSetting(s, "postgres_user") == "" {
		return true
	}
	api := cfg.ModuleSettings("wisemed-api")
	for _, key := range []string{"cfg_wisemed_protocol", "cfg_wisemed_ip", "cfg_wisemed_port", "cfg_wisemed_path", "cfg_wisemed_key", "unitate_medicala_id", "tip_de_echipament_id", "cod_echipament", "numar_serial_echipament"} {
		if strSetting(api, key) == "" {
			return true
		}
	}
	if intSetting(api, "echipament_id", 0) <= 0 {
		return true
	}
	return validateSIUIDataWSS(cfg) != nil
}

func configureSIUIData(cfg *config.Config, reconfigure bool, r *bufio.Reader) (bool, error) {
	s := cfg.ModuleSettings("siui-data")
	if cfg.Modules == nil {
		cfg.Modules = map[string]map[string]interface{}{}
	}
	cfg.Modules["siui-data"] = s
	fmt.Println("WiseMED SIUI Data · PostgreSQL, identitate WiseMED și WSM (Enter păstrează valoarea curentă)")
	if e := promptSIUIDataDatabase(r, s); e != nil {
		return false, e
	}
	api := cfg.ModuleSettings("wisemed-api")
	cfg.Modules["wisemed-api"] = api
	for _, field := range []struct{ key, label, fallback string }{
		{"cfg_wisemed_protocol", "WiseMED API protocol", "https"}, {"cfg_wisemed_ip", "WiseMED API host / IP", ""},
		{"cfg_wisemed_port", "WiseMED API port", "443"}, {"cfg_wisemed_path", "WiseMED API path", "/wisemed-api/apiv2"},
	} {
		promptString(r, field.label, api, field.key, field.fallback)
	}
	if e := promptSIUIDataSecret(r, "WiseMED API key (gol = păstrează)", api, "cfg_wisemed_key"); e != nil {
		return false, e
	}
	if e := configureSIUIDataEquipment(cfg, r, reconfigure); e != nil {
		return true, e
	}
	promptTopLevelString(r, "Local HTTPS address", &cfg.LocalHTTP.Address, "127.0.0.1:19114")
	if e := promptSIUIDataWSS(cfg, r); e != nil {
		return true, e
	}
	// Shared update settings remain available in first setup and reconfiguration.
	bootstrapModuleSettings(r, cfg, reconfigure)
	syncModuleMirrors(cfg)
	cfg.ApplyDefaults()
	return true, nil
}

func configureSIUIDataEquipment(cfg *config.Config, r *bufio.Reader, reconfigure bool) error {
	api := cfg.ModuleSettings("wisemed-api")
	// This application needs a real equipment identity even when WSM was
	// temporarily disabled. Do not inherit a login-only setup shortcut.
	delete(api, "skip_setup_completion")
	for _, key := range []string{"cfg_wisemed_protocol", "cfg_wisemed_ip", "cfg_wisemed_port", "cfg_wisemed_path", "cfg_wisemed_key"} {
		if strSetting(api, key) == "" {
			return fmt.Errorf("complete WiseMED API setting %s before equipment registration", key)
		}
	}
	client := wisemedapi.NewBootstrapClient(stringSettings(api), callerTypeForProtocol(cfg.Analyzer.Protocol))
	client.Tracef = startupConsolef
	if reconfigure || strSetting(api, "unitate_medicala_id") == "" {
		if e := promptMedicalUnitSelection(r, client, api); e != nil {
			return e
		}
	}
	if reconfigure || strSetting(api, "tip_de_echipament_id") == "" {
		if e := promptEquipmentTypeSelection(r, client, api); e != nil {
			return e
		}
	}
	for _, field := range []struct{ key, label, fallback string }{
		{"cod_echipament", "Equipment code", firstNonEmpty(cfg.Reader.AnalyzerCode, cfg.Reader.ID, "wisemed-siui-data")},
		{"producator_echipament", "Equipment manufacturer", "WiseMED"},
		{"numar_serial_echipament", "Equipment serial number", firstNonEmpty(cfg.Reader.ID, "wisemed-siui-data") + "-001"},
		{"nume_pe_raport_final", "Equipment display name", firstNonEmpty(cfg.Reader.AnalyzerName, cfg.Reader.Label, "WiseMED SIUI Data")},
	} {
		if reconfigure || strSetting(api, field.key) == "" {
			promptString(r, field.label, api, field.key, field.fallback)
		}
	}
	if _, ok := api["nr_rackuri"]; !ok {
		api["nr_rackuri"] = 0
	}
	if _, ok := api["pozitii_pe_rack"]; !ok {
		api["pozitii_pe_rack"] = 0
	}
	if e := registerEquipment(cfg, client); e != nil {
		return e
	}
	if intSetting(api, "echipament_id", 0) <= 0 {
		return fmt.Errorf("WiseMED did not return a valid equipment ID")
	}
	fmt.Printf("WiseMED equipment ID: %s\n", strSetting(api, "echipament_id"))
	return nil
}

func promptSIUIDataWSS(cfg *config.Config, r *bufio.Reader) error {
	wss := cfg.ModuleSettings("wisemed-ws")
	cfg.Modules["wisemed-ws"] = wss
	fallback := strconv.FormatBool(cfg.WiseMedWS.Enabled)
	promptChoice(r, "WSM enabled", wss, "enabled", []string{"true", "false"}, fallback)
	enabled, e := strconv.ParseBool(strSetting(wss, "enabled"))
	if e != nil {
		return fmt.Errorf("invalid WSM enabled value")
	}
	wss["enabled"] = enabled
	cfg.WiseMedWS.Enabled = enabled
	promptString(r, "WSM URL", wss, "url", firstNonEmpty(cfg.WiseMedWS.URL, "wss://wslocal.wisemed.eu/ws"))
	cfg.WiseMedWS.URL = strSetting(wss, "url")
	if !enabled {
		return nil
	}
	promptChoice(r, "WSM authentication", wss, "auth_mode", []string{"device_key", "token_file", "token_endpoint"}, "device_key")
	promptString(r, "WSM client ID", wss, "client_id", cfg.Reader.ID)
	switch strSetting(wss, "auth_mode") {
	case "device_key":
		for _, field := range []struct{ key, label, fallback string }{
			{"tenant_id", "WSM tenant ID", ""}, {"key_id", "WSM key ID", ""}, {"issuer", "WSM issuer", ""}, {"audience", "WSM audience", "wsm-server"}, {"subject", "WSM subject", cfg.Reader.ID},
		} {
			promptString(r, field.label, wss, field.key, field.fallback)
		}
		if e := promptSIUIDataSecret(r, "WSM device key (gol = păstrează; altfel secret_file / api_key_echipament)", wss, "device_secret"); e != nil {
			return e
		}
		promptString(r, "WSM secret file (optional)", wss, "secret_file", "")
	case "token_file":
		promptString(r, "WSM JWT token file", wss, "token_file", "")
	case "token_endpoint":
		promptString(r, "WiseMED WSM token endpoint", wss, "token_path", "")
	}
	promptString(r, "WSM scopes (bridge sync needs connections:read, route:command, api:invoke)", wss, "scopes", siuiDataWSSScopes)
	promptString(r, "WSM CA file (optional)", wss, "ca_file", "")
	return validateSIUIDataWSS(cfg)
}

func validateSIUIDataWSS(cfg *config.Config) error {
	wss := cfg.ModuleSettings("wisemed-ws")
	enabled := cfg.WiseMedWS.Enabled
	if raw := strSetting(wss, "enabled"); raw != "" {
		v, e := strconv.ParseBool(raw)
		if e != nil {
			return fmt.Errorf("invalid WSM enabled value")
		}
		enabled = v
	}
	if !enabled {
		return nil
	}
	u, e := url.Parse(firstNonEmpty(strSetting(wss, "url"), cfg.WiseMedWS.URL))
	if e != nil || u.Scheme != "wss" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("configure WSM wss:// URL without query or credentials")
	}
	switch firstNonEmpty(strSetting(wss, "auth_mode"), "device_key") {
	case "device_key":
		for _, key := range []string{"tenant_id", "key_id", "issuer"} {
			if strSetting(wss, key) == "" {
				return fmt.Errorf("configure WSM %s", key)
			}
		}
		secret := strSetting(wss, "device_secret")
		if secret != "" && len(secret) < 32 {
			return fmt.Errorf("WSM device key must have at least 32 bytes")
		}
		if secret == "" && strSetting(wss, "secret_file") == "" && len(strSetting(cfg.ModuleSettings("wisemed-api"), "api_key_echipament")) < 32 {
			return fmt.Errorf("configure WSM device key (minimum 32 bytes) or secret_file")
		}
	case "token_file":
		if strSetting(wss, "token_file") == "" {
			return fmt.Errorf("configure WSM token_file")
		}
	case "token_endpoint":
		if strSetting(wss, "token_path") == "" {
			return fmt.Errorf("configure WSM token endpoint")
		}
	default:
		return fmt.Errorf("invalid WSM auth_mode")
	}
	return nil
}

func promptSIUIDataDatabase(r *bufio.Reader, s map[string]interface{}) error {
	for _, field := range []struct{ key, label, fallback string }{
		{"postgres_host", "PostgreSQL host", "127.0.0.1"}, {"postgres_database", "PostgreSQL database", ""}, {"postgres_user", "PostgreSQL user", ""},
	} {
		promptString(r, field.label, s, field.key, field.fallback)
	}
	if e := promptSIUIDataSecret(r, "PostgreSQL password (gol = păstrează)", s, "postgres_password"); e != nil {
		return e
	}
	for _, field := range []struct {
		key, label         string
		fallback, min, max int
	}{
		{"postgres_port", "PostgreSQL port", 5432, 1, 65535}, {"postgres_max_conns", "PostgreSQL maximum connections", 20, 4, 500},
		{"max_upload_mib", "Maximum ZIP size (MiB)", 4096, 1, 1 << 20}, {"max_xml_mib", "Maximum expanded XML (MiB)", 16384, 1, 1 << 20},
		{"import_timeout_minutes", "Import timeout (minutes)", 120, 1, 1440},
		{"local_poll_seconds", "Local import poll interval (seconds)", 30, 1, 3600},
	} {
		promptString(r, field.label, s, field.key, strconv.Itoa(field.fallback))
		v, e := strconv.Atoi(strSetting(s, field.key))
		if e != nil || v < field.min || v > field.max {
			return fmt.Errorf("invalid %s", field.key)
		}
		s[field.key] = v
	}
	promptChoice(r, "PostgreSQL TLS", s, "postgres_sslmode", []string{"disable", "allow", "prefer", "require", "verify-ca", "verify-full"}, "prefer")
	promptString(r, "PostgreSQL root CA file", s, "postgres_root_cert", "")
	promptString(r, "Upload staging directory", s, "upload_dir", "./catalogue-uploads")
	promptString(r, "Local import directory (empty = disabled)", s, "local_import_dir", "")
	return nil
}

func promptSIUIDataSecret(r *bufio.Reader, label string, s map[string]interface{}, key string) error {
	fmt.Printf("%s: ", label)
	var value string
	if term.IsTerminal(int(os.Stdin.Fd())) && r.Buffered() == 0 {
		b, e := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if e != nil {
			return e
		}
		value = string(b)
	} else {
		value, _ = r.ReadString('\n')
	}
	value = strings.TrimRight(value, "\r\n")
	if value != "" {
		s[key] = value
	}
	return nil
}
