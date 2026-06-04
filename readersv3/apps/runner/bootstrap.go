package runner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/modules/wisemedapi"
)

func ensureBootstrap(cfg *config.Config, reconfigure bool) (bool, error) {
	if !reconfigure && !needsBootstrap(cfg) {
		cfg.ApplyDefaults()
		return false, nil
	}

	reader := bufio.NewReader(os.Stdin)
	changed := reconfigure

	settings := cfg.ModuleSettings("wisemed-api")
	if cfg.Modules == nil {
		cfg.Modules = map[string]map[string]interface{}{}
	}
	if settings == nil {
		settings = map[string]interface{}{}
		cfg.Modules["wisemed-api"] = settings
	}
	if reconfigure || strSetting(settings, "cfg_wisemed_protocol") == "" {
		promptString(reader, "WiseMED API protocol", settings, "cfg_wisemed_protocol", "https")
		changed = true
	}
	if reconfigure || strSetting(settings, "cfg_wisemed_ip") == "" {
		promptString(reader, "WiseMED API host / IP", settings, "cfg_wisemed_ip", "")
		changed = true
	}
	if reconfigure || strSetting(settings, "cfg_wisemed_port") == "" {
		promptString(reader, "WiseMED API port", settings, "cfg_wisemed_port", "443")
		changed = true
	}
	if reconfigure || strSetting(settings, "cfg_wisemed_path") == "" {
		promptString(reader, "WiseMED API path", settings, "cfg_wisemed_path", "/api")
		changed = true
	}
	if reconfigure || strSetting(settings, "cfg_wisemed_key") == "" {
		promptString(reader, "WiseMED API key", settings, "cfg_wisemed_key", "")
		changed = true
	}
	apiClient := wisemedapi.NewBootstrapClient(stringSettings(settings), callerTypeForProtocol(cfg.Analyzer.Protocol))

	if reconfigure || cfg.Reader.ID == "" {
		promptTopLevelString(reader, "Reader ID", &cfg.Reader.ID, defaultReaderID(cfg))
		changed = true
	}
	if reconfigure || cfg.Reader.Label == "" {
		promptTopLevelString(reader, "Reader label", &cfg.Reader.Label, defaultReaderLabel(cfg))
		changed = true
	}
	if reconfigure || cfg.Reader.AnalyzerName == "" {
		promptTopLevelString(reader, "Analyzer name", &cfg.Reader.AnalyzerName, cfg.Reader.Label)
		changed = true
	}
	if reconfigure || cfg.Reader.AnalyzerCode == "" {
		promptTopLevelString(reader, "Analyzer code", &cfg.Reader.AnalyzerCode, strings.ToLower(strings.ReplaceAll(cfg.Reader.Label, " ", "-")))
		changed = true
	}
	if reconfigure || cfg.Reader.DBName == "" {
		promptTopLevelString(reader, "Reader DB name", &cfg.Reader.DBName, defaultDBName(cfg))
		changed = true
	}

	if reconfigure || strSetting(settings, "unitate_medicala_id") == "" {
		if err := promptMedicalUnitSelection(reader, apiClient, settings); err != nil {
			return changed, err
		}
		changed = true
	}
	if reconfigure || strSetting(settings, "tip_de_echipament_id") == "" {
		if err := promptEquipmentTypeSelection(reader, apiClient, settings); err != nil {
			return changed, err
		}
		changed = true
	}
	if reconfigure || strSetting(settings, "cod_echipament") == "" {
		promptString(reader, "Equipment code", settings, "cod_echipament", cfg.Reader.AnalyzerCode)
		changed = true
	}
	if reconfigure || strSetting(settings, "numar_serial_echipament") == "" {
		promptString(reader, "Equipment serial number", settings, "numar_serial_echipament", cfg.Reader.AnalyzerCode+"-001")
		changed = true
	}

	if reconfigure || cfg.LocalHTTP.Address == "" {
		promptTopLevelString(reader, "Local HTTP address", &cfg.LocalHTTP.Address, "127.0.0.1:18080")
		changed = true
	}
	if reconfigure || cfg.LocalHTTP.Language == "" {
		promptTopLevelString(reader, "Local HTTP language", &cfg.LocalHTTP.Language, "ro")
		changed = true
	}
	if reconfigure || cfg.WiseMedWS.URL == "" {
		promptTopLevelString(reader, "WiseMEDWS URL", &cfg.WiseMedWS.URL, "wss://wslocal.wisemed.eu/ws")
		changed = true
	}
	if reconfigure || cfg.Analyzer.CommType == "" {
		promptTopLevelChoice(reader, "Communication type", &cfg.Analyzer.CommType, supportedCommTypes(cfg), defaultCommType(cfg))
		changed = true
	}
	if reconfigure || cfg.Analyzer.Protocol == "" {
		promptTopLevelChoice(reader, "Protocol", &cfg.Analyzer.Protocol, supportedProtocols(cfg), defaultProtocol(cfg))
		changed = true
	}

	bootstrapModuleSettings(reader, cfg, reconfigure)
	cfg.Modules["wisemed-api"] = settings
	syncModuleMirrors(cfg)
	cfg.ApplyDefaults()
	if changed {
		if err := registerEquipment(cfg, apiClient); err != nil {
			return changed, err
		}
	}
	return changed, nil
}

func bootstrapModuleSettings(reader *bufio.Reader, cfg *config.Config, reconfigure bool) {
	if cfg.Modules == nil {
		cfg.Modules = map[string]map[string]interface{}{}
	}
	if _, ok := cfg.Modules["app-updates"]; !ok {
		cfg.Modules["app-updates"] = map[string]interface{}{}
	}
	if reconfigure || strSetting(cfg.Modules["app-updates"], "enabled") == "" {
		promptString(reader, "Update server enabled (true/false)", cfg.Modules["app-updates"], "enabled", "true")
	}
	if reconfigure || strSetting(cfg.Modules["app-updates"], "app_id") == "" {
		promptString(reader, "Update app ID", cfg.Modules["app-updates"], "app_id", defaultReaderID(cfg))
	}
	if reconfigure || strSetting(cfg.Modules["app-updates"], "channel") == "" {
		promptString(reader, "Update channel", cfg.Modules["app-updates"], "channel", "stable")
	}
	if reconfigure || strSetting(cfg.Modules["app-updates"], "base_url") == "" {
		promptString(reader, "Update server base URL", cfg.Modules["app-updates"], "base_url", "http://127.0.0.1:19090")
	}
	if reconfigure || strSetting(cfg.Modules["app-updates"], "auto_download") == "" {
		promptString(reader, "Auto download updates (true/false)", cfg.Modules["app-updates"], "auto_download", "true")
	}
	if reconfigure || strSetting(cfg.Modules["app-updates"], "download_dir") == "" {
		promptString(reader, "Update download directory", cfg.Modules["app-updates"], "download_dir", "./updates")
	}
	if reconfigure || cfg.ModuleSettings("storage-sqlite")["path"] == nil {
		if _, ok := cfg.Modules["storage-sqlite"]; !ok {
			cfg.Modules["storage-sqlite"] = map[string]interface{}{}
		}
		promptString(reader, "SQLite path", cfg.Modules["storage-sqlite"], "path", "./"+defaultDBName(cfg))
	}
	if _, ok := cfg.Modules["result-sync"]; !ok {
		cfg.Modules["result-sync"] = map[string]interface{}{}
	}
	if reconfigure || strSetting(cfg.Modules["result-sync"], "enabled") == "" {
		promptString(reader, "WiseMED result sync enabled (true/false)", cfg.Modules["result-sync"], "enabled", "true")
	}
	if reconfigure || strSetting(cfg.Modules["result-sync"], "interval_minutes") == "" {
		promptString(reader, "WiseMED result sync interval minutes", cfg.Modules["result-sync"], "interval_minutes", "5")
	}
	if reconfigure || cfg.Modules["result-sync"]["sample_prefixes"] == nil {
		promptString(reader, "Sample prefixes to strip (comma separated)", cfg.Modules["result-sync"], "sample_prefixes", "")
	}
	if reconfigure || cfg.Modules["result-sync"]["sample_suffixes"] == nil {
		promptString(reader, "Sample suffixes to strip (comma separated)", cfg.Modules["result-sync"], "sample_suffixes", "")
	}
	if reconfigure || cfg.Modules["result-sync"]["separators"] == nil {
		promptString(reader, "Sample separators (comma separated)", cfg.Modules["result-sync"], "separators", "-")
	}
	if reconfigure || cfg.Modules["result-sync"]["qc_prefixes"] == nil {
		promptString(reader, "QC prefixes prefix:true|false (comma separated)", cfg.Modules["result-sync"], "qc_prefixes", "")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Analyzer.CommType)) {
	case "file", "utility":
		if _, ok := cfg.Modules["transport-file"]; !ok {
			cfg.Modules["transport-file"] = map[string]interface{}{}
		}
		if reconfigure || strSetting(cfg.Modules["transport-file"], "import_dir") == "" {
			promptString(reader, "Import directory", cfg.Modules["transport-file"], "import_dir", "./inbox")
		}
		if reconfigure || strSetting(cfg.Modules["transport-file"], "processed_dir") == "" {
			promptString(reader, "Processed directory", cfg.Modules["transport-file"], "processed_dir", "./processed")
		}
		if reconfigure || strSetting(cfg.Modules["transport-file"], "failed_dir") == "" {
			promptString(reader, "Failed directory", cfg.Modules["transport-file"], "failed_dir", "./failed")
		}
		if reconfigure || strSetting(cfg.Modules["transport-file"], "export_dir") == "" {
			promptString(reader, "Export directory", cfg.Modules["transport-file"], "export_dir", "./outbox")
		}
		if reconfigure || strSetting(cfg.Modules["transport-file"], "pattern") == "" {
			defaultPattern := "*.csv"
			if strings.Contains(strings.ToLower(cfg.Analyzer.Protocol), "seegene") {
				defaultPattern = "*.xlsx"
			}
			promptString(reader, "Import file pattern", cfg.Modules["transport-file"], "pattern", defaultPattern)
		}
	case "tcpip":
		if _, ok := cfg.Modules["transport-tcpip"]; !ok {
			cfg.Modules["transport-tcpip"] = map[string]interface{}{}
		}
		if supportsTCPModeSelection(cfg) && (reconfigure || strSetting(cfg.Modules["transport-tcpip"], "mode") == "") {
			promptChoice(reader, "TCP/IP mode", cfg.Modules["transport-tcpip"], "mode", supportedTCPModes(cfg), defaultTCPMode(cfg))
		}
		if strings.EqualFold(strSetting(cfg.Modules["transport-tcpip"], "mode"), "client") {
			if reconfigure || strSetting(cfg.Modules["transport-tcpip"], "remote_host") == "" {
				promptString(reader, "TCP/IP remote host", cfg.Modules["transport-tcpip"], "remote_host", defaultTCPRemoteHost(cfg))
			}
			if reconfigure || strSetting(cfg.Modules["transport-tcpip"], "remote_port") == "" {
				promptString(reader, "TCP/IP remote port", cfg.Modules["transport-tcpip"], "remote_port", defaultTCPRemotePort(cfg))
			}
		} else {
			if reconfigure || strSetting(cfg.Modules["transport-tcpip"], "host") == "" {
				promptString(reader, "TCP/IP listen host", cfg.Modules["transport-tcpip"], "host", defaultTCPHost(cfg))
			}
			if reconfigure || strSetting(cfg.Modules["transport-tcpip"], "port") == "" {
				promptString(reader, "TCP/IP listen port", cfg.Modules["transport-tcpip"], "port", defaultTCPPort(cfg))
			}
		}
	case "serial":
		if _, ok := cfg.Modules["transport-serial"]; !ok {
			cfg.Modules["transport-serial"] = map[string]interface{}{}
		}
		if reconfigure || strSetting(cfg.Modules["transport-serial"], "port") == "" {
			promptString(reader, "Serial port", cfg.Modules["transport-serial"], "port", "")
		}
		if reconfigure || strSetting(cfg.Modules["transport-serial"], "baud") == "" {
			promptString(reader, "Serial baud", cfg.Modules["transport-serial"], "baud", "9600")
		}
	}
}

func needsBootstrap(cfg *config.Config) bool {
	ws := cfg.ModuleSettings("wisemed-api")
	requiredSettings := []string{
		"cfg_wisemed_protocol",
		"cfg_wisemed_ip",
		"cfg_wisemed_port",
		"cfg_wisemed_path",
		"cfg_wisemed_key",
		"unitate_medicala_id",
		"tip_de_echipament_id",
		"cod_echipament",
		"numar_serial_echipament",
	}
	for _, key := range requiredSettings {
		if strSetting(ws, key) == "" {
			return true
		}
	}
	if strings.TrimSpace(cfg.Reader.ID) == "" || strings.TrimSpace(cfg.Reader.Label) == "" ||
		strings.TrimSpace(cfg.Reader.AnalyzerName) == "" || strings.TrimSpace(cfg.Reader.AnalyzerCode) == "" ||
		strings.TrimSpace(cfg.Reader.DBName) == "" {
		return true
	}
	if strings.TrimSpace(cfg.LocalHTTP.Address) == "" || strings.TrimSpace(cfg.WiseMedWS.URL) == "" {
		return true
	}
	if strings.TrimSpace(cfg.Analyzer.CommType) == "" || strings.TrimSpace(cfg.Analyzer.Protocol) == "" {
		return true
	}
	requiredUpdateSettings := []string{"enabled", "app_id", "channel", "base_url", "auto_download", "download_dir"}
	updateSettings := cfg.ModuleSettings("app-updates")
	for _, key := range requiredUpdateSettings {
		if strSetting(updateSettings, key) == "" {
			return true
		}
	}
	return false
}

func confirmSaveAndStart(cfg *config.Config) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\nConfiguration summary:")
	fmt.Printf("- Reader ID: %s\n", cfg.Reader.ID)
	fmt.Printf("- Reader label: %s\n", cfg.Reader.Label)
	fmt.Printf("- Analyzer: %s (%s)\n", cfg.Reader.AnalyzerName, cfg.Reader.AnalyzerCode)
	fmt.Printf("- Local HTTP: %s\n", cfg.LocalHTTP.Address)
	fmt.Printf("- WiseMEDWS: %s\n", cfg.WiseMedWS.URL)
	fmt.Printf("- Communication: %s / %s\n", cfg.Analyzer.CommType, cfg.Analyzer.Protocol)
	fmt.Printf("- Config file: %s\n", cfg.Path())
	fmt.Print("Save configuration and start application? [Y/n]: ")
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(strings.ToLower(text))
	return text == "" || text == "y" || text == "yes"
}

func syncModuleMirrors(cfg *config.Config) {
	if cfg.Modules == nil {
		cfg.Modules = map[string]map[string]interface{}{}
	}
	if _, ok := cfg.Modules["local-http"]; !ok {
		cfg.Modules["local-http"] = map[string]interface{}{}
	}
	cfg.Modules["local-http"]["address"] = cfg.LocalHTTP.Address
	cfg.Modules["local-http"]["enabled"] = cfg.LocalHTTP.Enabled
	cfg.Modules["local-http"]["language"] = cfg.LocalHTTP.Language
	cfg.Modules["local-http"]["tls"] = cfg.LocalHTTP.TLS

	if _, ok := cfg.Modules["wisemed-ws"]; !ok {
		cfg.Modules["wisemed-ws"] = map[string]interface{}{}
	}
	cfg.Modules["wisemed-ws"]["enabled"] = cfg.WiseMedWS.Enabled
	cfg.Modules["wisemed-ws"]["url"] = cfg.WiseMedWS.URL
	cfg.Modules["wisemed-ws"]["heartbeat_ms"] = cfg.WiseMedWS.HeartbeatMS
	cfg.Modules["wisemed-ws"]["reconnect_delay_ms"] = cfg.WiseMedWS.ReconnectDelayMS

	if sqlite := cfg.ModuleSettings("storage-sqlite"); sqlite != nil && strSetting(sqlite, "path") != "" {
		cfg.Reader.DBName = filepath.Base(strSetting(sqlite, "path"))
	}
}

func promptMedicalUnitSelection(reader *bufio.Reader, client *wisemedapi.BootstrapClient, settings map[string]interface{}) error {
	items, err := client.ListMedicalUnits()
	if err != nil {
		return fmt.Errorf("list WiseMED medical units: %w", err)
	}
	if len(items) == 0 {
		return fmt.Errorf("WiseMED returned no medical units")
	}
	fmt.Println("\nWiseMED medical units:")
	for i, item := range items {
		fmt.Printf("%d. [%s] %s\n", i+1, itemString(item, "medical_unit_id", "id"), itemString(item, "medical_unit_name", "name"))
	}
	current := strSetting(settings, "unitate_medicala_id")
	for {
		fmt.Printf("Select medical unit number (%s): ", current)
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		if text == "" && current != "" {
			return nil
		}
		idx, err := strconv.Atoi(text)
		if err == nil && idx >= 1 && idx <= len(items) {
			settings["unitate_medicala_id"] = itemString(items[idx-1], "medical_unit_id", "id")
			return nil
		}
		fmt.Println("Invalid selection")
	}
}

func promptEquipmentTypeSelection(reader *bufio.Reader, client *wisemedapi.BootstrapClient, settings map[string]interface{}) error {
	items, err := client.ListEquipmentTypes()
	if err != nil {
		return fmt.Errorf("list WiseMED equipment types: %w", err)
	}
	if len(items) == 0 {
		return fmt.Errorf("WiseMED returned no equipment types")
	}
	fmt.Println("\nWiseMED equipment types:")
	for i, item := range items {
		fmt.Printf("%d. [%s] %s\n", i+1, itemString(item, "analyzer_type_id", "id"), itemString(item, "analyzer_type_name", "name"))
	}
	current := strSetting(settings, "tip_de_echipament_id")
	for {
		fmt.Printf("Select equipment type number (%s): ", current)
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		if text == "" && current != "" {
			return nil
		}
		idx, err := strconv.Atoi(text)
		if err == nil && idx >= 1 && idx <= len(items) {
			settings["tip_de_echipament_id"] = itemString(items[idx-1], "analyzer_type_id", "id")
			return nil
		}
		fmt.Println("Invalid selection")
	}
}

func registerEquipment(cfg *config.Config, client *wisemedapi.BootstrapClient) error {
	settings := cfg.ModuleSettings("wisemed-api")
	payload := map[string]interface{}{
		"cod_echipament":          strSetting(settings, "cod_echipament"),
		"nume_echipament":         firstNonEmpty(strings.TrimSpace(cfg.Reader.AnalyzerName), strings.TrimSpace(cfg.Reader.Label), strings.TrimSpace(cfg.Reader.ID)),
		"api_key_echipament":      strSetting(settings, "api_key_echipament"),
		"producator_echipament":   "thinkIT",
		"tip_analizor":            callerTypeForProtocol(cfg.Analyzer.Protocol),
		"numar_serial_echipament": strSetting(settings, "numar_serial_echipament"),
		"ip":                      "0.0.0.0",
		"port":                    "0",
		"online":                  true,
		"nr_rackuri":              "0",
		"pozitii_pe_rack":         "0",
		"echipament_id":           strSetting(settings, "echipament_id"),
		"unitate_medicala_id":     strSetting(settings, "unitate_medicala_id"),
		"tip_de_echipament_id":    strSetting(settings, "tip_de_echipament_id"),
	}
	resp, err := client.RegisterEquipment(payload)
	if err != nil {
		return fmt.Errorf("register equipment in WiseMED: %w", err)
	}
	if value := itemString(resp, "echipament_id", "equipment_id"); value != "" {
		settings["echipament_id"] = value
	}
	if value := itemString(resp, "api_key_echipament", "api_key"); value != "" {
		settings["api_key_echipament"] = value
	}
	cfg.Modules["wisemed-api"] = settings
	return nil
}

func promptString(reader *bufio.Reader, label string, section map[string]interface{}, key, fallback string) {
	current := strSetting(section, key)
	if current == "" {
		current = fallback
	}
	if strings.TrimSpace(label) == "" {
		label = key
	}
	fmt.Printf("%s (%s): ", label, current)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text == "" {
		section[key] = current
		return
	}
	section[key] = text
}

func promptTopLevelString(reader *bufio.Reader, label string, target *string, fallback string) {
	current := strings.TrimSpace(*target)
	if current == "" {
		current = fallback
	}
	fmt.Printf("%s (%s): ", label, current)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text == "" {
		*target = current
		return
	}
	*target = text
}

func promptTopLevelChoice(reader *bufio.Reader, label string, target *string, choices []string, fallback string) {
	if len(choices) == 0 {
		promptTopLevelString(reader, label, target, fallback)
		return
	}
	current := strings.TrimSpace(*target)
	if current == "" {
		current = fallback
	}
	if !containsChoice(choices, current) {
		current = fallback
	}
	fmt.Printf("%s [%s] (%s): ", label, strings.Join(choices, "/"), current)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text == "" {
		*target = current
		return
	}
	if containsChoice(choices, text) {
		*target = text
		return
	}
	fmt.Printf("Invalid choice, keeping %s\n", current)
	*target = current
}

func promptChoice(reader *bufio.Reader, label string, section map[string]interface{}, key string, choices []string, fallback string) {
	current := strSetting(section, key)
	if current == "" {
		current = fallback
	}
	if !containsChoice(choices, current) {
		current = fallback
	}
	fmt.Printf("%s [%s] (%s): ", label, strings.Join(choices, "/"), current)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text == "" {
		section[key] = current
		return
	}
	if containsChoice(choices, text) {
		section[key] = text
		return
	}
	fmt.Printf("Invalid choice, keeping %s\n", current)
	section[key] = current
}

func strSetting(section map[string]interface{}, key string) string {
	if section == nil {
		return ""
	}
	value, ok := section[key]
	if !ok || value == nil {
		return ""
	}
	switch t := value.(type) {
	case string:
		return strings.TrimSpace(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func stringSettings(section map[string]interface{}) map[string]string {
	out := map[string]string{}
	for key := range section {
		out[key] = strSetting(section, key)
	}
	return out
}

func itemString(item map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key]; ok {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func callerTypeForProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "seegene-excel", "beosl-csv":
		return "Microbiology"
	case "cary60-uvvis", "generic-file", "barcodeprinter":
		return "Biochemestry"
	case "astm":
		return "Immunology"
	default:
		return "Undefined"
	}
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return strings.TrimSpace(item)
		}
	}
	return ""
}

func defaultReaderID(cfg *config.Config) string {
	if strings.TrimSpace(cfg.Reader.ID) != "" {
		return cfg.Reader.ID
	}
	code := strings.TrimSpace(cfg.Reader.AnalyzerCode)
	if code == "" {
		code = strings.ToLower(strings.ReplaceAll(defaultReaderLabel(cfg), " ", "-"))
	}
	return code
}

func defaultReaderLabel(cfg *config.Config) string {
	if strings.TrimSpace(cfg.Reader.Label) != "" {
		return cfg.Reader.Label
	}
	if strings.TrimSpace(cfg.Reader.AnalyzerName) != "" {
		return cfg.Reader.AnalyzerName
	}
	return "WiseMED Reader"
}

func defaultDBName(cfg *config.Config) string {
	if strings.TrimSpace(cfg.Reader.DBName) != "" {
		return cfg.Reader.DBName
	}
	return strings.ToLower(strings.ReplaceAll(defaultReaderID(cfg), " ", "-")) + ".db"
}

func defaultCommType(cfg *config.Config) string {
	if strings.TrimSpace(cfg.Analyzer.CommType) != "" {
		return cfg.Analyzer.CommType
	}
	if values := supportedCommTypes(cfg); len(values) == 1 {
		return values[0]
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Analyzer.Protocol)) {
	case "astm":
		return "tcpip"
	case "hl7", "simple":
		return "tcpip"
	case "barcodeprinter":
		return "utility"
	default:
		return "file"
	}
}

func defaultProtocol(cfg *config.Config) string {
	if strings.TrimSpace(cfg.Analyzer.Protocol) != "" {
		return cfg.Analyzer.Protocol
	}
	if values := supportedProtocols(cfg); len(values) == 1 {
		return values[0]
	}
	return "generic-file"
}

func supportedProtocols(cfg *config.Config) []string {
	out := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || containsChoice(out, value) {
			return
		}
		out = append(out, value)
	}
	for _, moduleID := range cfg.EnabledModules {
		switch strings.TrimSpace(moduleID) {
		case "protocol-labnovation-ld560":
			add("hl7")
			add("simple")
		case "protocol-astm":
			add("astm")
		case "protocol-ir-biotyper":
			add("ir-biotyper")
		case "protocol-cary60-uvvis":
			add("cary60-uvvis")
		case "protocol-biosan-hipo-mpp96":
			add("biosan-hipo-mpp96")
		case "protocol-gammavision":
			add("gammavision")
		case "protocol-shimatzu-tocl":
			add("shimatzu-tocl")
		case "protocol-shimatzu-generic":
			add("shimatzu-generic")
		case "protocol-seegene-excel":
			add("seegene-excel")
		case "protocol-beosl-csv":
			add("beoslcsv")
			add("beosl-csv")
		case "protocol-tricarb-5110-tr":
			add("tricarb-5110-tr")
		case "protocol-anatolia-geneworks":
			add("anatolia-geneworks")
		case "protocol-generic-file":
			add("generic-file")
		}
	}
	if len(out) == 0 && strings.TrimSpace(cfg.Analyzer.Protocol) != "" {
		add(cfg.Analyzer.Protocol)
	}
	return out
}

func supportedCommTypes(cfg *config.Config) []string {
	out := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || containsChoice(out, value) {
			return
		}
		out = append(out, value)
	}
	for _, protocol := range supportedProtocols(cfg) {
		switch strings.ToLower(strings.TrimSpace(protocol)) {
		case "hl7", "simple", "astm", "ir-biotyper":
			add("tcpip")
		case "seegene-excel", "beosl-csv", "beoslcsv", "cary60-uvvis", "shimatzu-tocl", "shimatzu-generic", "biosan-hipo-mpp96", "gammavision", "tricarb-5110-tr", "anatolia-geneworks", "generic-file":
			add("file")
		case "barcodeprinter":
			add("utility")
		}
	}
	if len(out) == 0 && strings.TrimSpace(cfg.Analyzer.CommType) != "" {
		add(cfg.Analyzer.CommType)
	}
	return out
}

func supportedTCPModes(cfg *config.Config) []string {
	for _, item := range cfg.EnabledModules {
		if strings.EqualFold(strings.TrimSpace(item), "protocol-labnovation-ld560") {
			return []string{"server", "client"}
		}
	}
	return []string{"server"}
}

func supportsTCPModeSelection(cfg *config.Config) bool {
	return len(supportedTCPModes(cfg)) > 1
}

func defaultTCPMode(cfg *config.Config) string {
	if value := strSetting(cfg.ModuleSettings("transport-tcpip"), "mode"); value != "" {
		return value
	}
	return "server"
}

func defaultTCPHost(cfg *config.Config) string {
	if value := strSetting(cfg.ModuleSettings("transport-tcpip"), "host"); value != "" {
		return value
	}
	if hasEnabledModule(cfg, "protocol-labnovation-ld560") {
		return "0.0.0.0"
	}
	return "127.0.0.1"
}

func defaultTCPPort(cfg *config.Config) string {
	if value := strSetting(cfg.ModuleSettings("transport-tcpip"), "port"); value != "" {
		return value
	}
	if hasEnabledModule(cfg, "protocol-labnovation-ld560") {
		return "8000"
	}
	return "9000"
}

func defaultTCPRemoteHost(cfg *config.Config) string {
	if value := strSetting(cfg.ModuleSettings("transport-tcpip"), "remote_host"); value != "" {
		return value
	}
	return "127.0.0.1"
}

func defaultTCPRemotePort(cfg *config.Config) string {
	if value := strSetting(cfg.ModuleSettings("transport-tcpip"), "remote_port"); value != "" {
		return value
	}
	return defaultTCPPort(cfg)
}

func hasEnabledModule(cfg *config.Config, target string) bool {
	for _, item := range cfg.EnabledModules {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func containsChoice(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}
