package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"wisemed-labreaders/readersv3/core/config"
)

func ensureUpdateServerBootstrap(cfg *config.Config, reconfigure bool) (bool, error) {
	needs := reconfigure || strings.TrimSpace(cfg.LocalHTTP.Address) == "" || stringValue(cfg.ModuleSettings("wisemed-api")["cfg_wisemed_key"]) == ""
	if !needs {
		cfg.ApplyDefaults()
		return false, nil
	}
	if cfg.Modules == nil {
		cfg.Modules = map[string]map[string]interface{}{}
	}
	if _, ok := cfg.Modules["wisemed-api"]; !ok {
		cfg.Modules["wisemed-api"] = map[string]interface{}{}
	}
	if _, ok := cfg.Modules["app-update-server"]; !ok {
		cfg.Modules["app-update-server"] = map[string]interface{}{}
	}
	reader := bufio.NewReader(os.Stdin)
	promptString(reader, "WiseMED API protocol", cfg.Modules["wisemed-api"], "cfg_wisemed_protocol", firstNonEmpty(stringValue(cfg.Modules["wisemed-api"]["cfg_wisemed_protocol"]), "https"))
	promptString(reader, "WiseMED API host / IP", cfg.Modules["wisemed-api"], "cfg_wisemed_ip", stringValue(cfg.Modules["wisemed-api"]["cfg_wisemed_ip"]))
	promptString(reader, "WiseMED API port", cfg.Modules["wisemed-api"], "cfg_wisemed_port", firstNonEmpty(stringValue(cfg.Modules["wisemed-api"]["cfg_wisemed_port"]), "443"))
	promptString(reader, "WiseMED API path", cfg.Modules["wisemed-api"], "cfg_wisemed_path", firstNonEmpty(stringValue(cfg.Modules["wisemed-api"]["cfg_wisemed_path"]), "/wisemed-api/apiv2"))
	promptString(reader, "WiseMED API key", cfg.Modules["wisemed-api"], "cfg_wisemed_key", stringValue(cfg.Modules["wisemed-api"]["cfg_wisemed_key"]))
	promptTopLevelString(reader, "Local HTTP address", &cfg.LocalHTTP.Address, firstNonEmpty(cfg.LocalHTTP.Address, "127.0.0.1:19090"))
	cfg.LocalHTTP.Enabled = true
	promptString(reader, "Update files directory", cfg.Modules["app-update-server"], "files_dir", firstNonEmpty(stringValue(cfg.Modules["app-update-server"]["files_dir"]), "./files"))
	promptString(reader, "Update database path", cfg.Modules["app-update-server"], "db_path", firstNonEmpty(stringValue(cfg.Modules["app-update-server"]["db_path"]), "./app-update-server.db"))
	promptString(reader, "Allowed admin user types", cfg.Modules["app-update-server"], "allowed_user_types", firstNonEmpty(stringValue(cfg.Modules["app-update-server"]["allowed_user_types"]), "1,9,10"))
	promptString(reader, "Public base URL", cfg.Modules["app-update-server"], "public_base_url", stringValue(cfg.Modules["app-update-server"]["public_base_url"]))
	cfg.Reader.ID = firstNonEmpty(cfg.Reader.ID, "app-update-server")
	cfg.Reader.Label = firstNonEmpty(cfg.Reader.Label, "App Update Server")
	cfg.Reader.AnalyzerName = firstNonEmpty(cfg.Reader.AnalyzerName, "App Update Server")
	cfg.Reader.AnalyzerCode = firstNonEmpty(cfg.Reader.AnalyzerCode, "app-update-server")
	cfg.Reader.DBName = firstNonEmpty(cfg.Reader.DBName, "app-update-server.db")
	cfg.Analyzer.CommType = "utility"
	cfg.Analyzer.Protocol = "app-update-server"
	cfg.Modules["local-http"] = map[string]interface{}{
		"enabled": true,
		"address": cfg.LocalHTTP.Address,
	}
	cfg.ApplyDefaults()
	fmt.Printf("Configuration will be saved to %s\n", cfg.Path())
	return true, nil
}

func promptString(reader *bufio.Reader, label string, target map[string]interface{}, key, fallback string) {
	current := firstNonEmpty(stringValue(target[key]), fallback)
	fmt.Printf("%s (%s): ", label, current)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text == "" {
		text = current
	}
	target[key] = text
}

func promptTopLevelString(reader *bufio.Reader, label string, target *string, fallback string) {
	current := firstNonEmpty(strings.TrimSpace(*target), fallback)
	fmt.Printf("%s (%s): ", label, current)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text == "" {
		text = current
	}
	*target = text
}

func stringValue(raw interface{}) string {
	if raw == nil {
		return ""
	}
	if text, ok := raw.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
