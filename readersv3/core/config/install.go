package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type EnsureResult struct {
	Created      bool
	Merged       bool
	ConfigPath   string
	TemplatePath string
}

func Ensure(path string) (EnsureResult, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return EnsureResult{}, errors.New("config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return EnsureResult{}, err
	}
	templatePath := InstallTemplatePath(path)
	template, hasTemplate, err := loadOptionalMap(templatePath)
	if err != nil {
		return EnsureResult{}, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if !hasTemplate {
			cfg := Default()
			cfg.path = path
			if err := cfg.Save(); err != nil {
				return EnsureResult{}, err
			}
			return EnsureResult{Created: true, ConfigPath: path, TemplatePath: templatePath}, nil
		}
		if err := writeMap(path, template); err != nil {
			return EnsureResult{}, err
		}
		return EnsureResult{Created: true, ConfigPath: path, TemplatePath: templatePath}, nil
	} else if err != nil {
		return EnsureResult{}, err
	}
	if !hasTemplate {
		return EnsureResult{ConfigPath: path, TemplatePath: templatePath}, nil
	}
	current, err := readMap(path)
	if err != nil {
		return EnsureResult{}, err
	}
	changed := preserveLegacyConnectionSettings(current)
	if mergeMissing(current, template) {
		changed = true
	}
	if !changed {
		return EnsureResult{ConfigPath: path, TemplatePath: templatePath}, nil
	}
	if err := writeMap(path, current); err != nil {
		return EnsureResult{}, err
	}
	return EnsureResult{Merged: true, ConfigPath: path, TemplatePath: templatePath}, nil
}

func InstallTemplatePath(configPath string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(configPath)), "config.install.yaml")
}

func Update(path string, next map[string]interface{}) error {
	raw, err := readMap(path)
	if err != nil {
		return err
	}
	for key, value := range next {
		if strings.TrimSpace(key) == "" || value == nil {
			continue
		}
		SetNestedValue(raw, strings.Split(key, "."), value)
	}
	return writeMap(path, raw)
}

func SetNestedValue(target map[string]interface{}, path []string, value interface{}) {
	if len(path) == 0 {
		return
	}
	if len(path) == 1 {
		target[path[0]] = value
		return
	}
	next, _ := target[path[0]].(map[string]interface{})
	if next == nil {
		next = map[string]interface{}{}
		target[path[0]] = next
	}
	SetNestedValue(next, path[1:], value)
}

func readMap(path string) (map[string]interface{}, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := map[string]interface{}{}
	if len(strings.TrimSpace(string(blob))) == 0 {
		return raw, nil
	}
	if err := yaml.Unmarshal(blob, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		raw = map[string]interface{}{}
	}
	return raw, nil
}

func loadOptionalMap(path string) (map[string]interface{}, bool, error) {
	raw, err := readMap(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func writeMap(path string, raw map[string]interface{}) error {
	if raw == nil {
		raw = map[string]interface{}{}
	}
	blob, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(path, blob, 0o644)
}

func mergeMissing(current, defaults map[string]interface{}) bool {
	changed := false
	for key, defaultValue := range defaults {
		currentValue, ok := current[key]
		if !ok || currentValue == nil {
			current[key] = cloneValue(defaultValue)
			changed = true
			continue
		}
		currentMap, currentIsMap := currentValue.(map[string]interface{})
		defaultMap, defaultIsMap := defaultValue.(map[string]interface{})
		if currentIsMap && defaultIsMap {
			if mergeMissing(currentMap, defaultMap) {
				changed = true
			}
		}
	}
	return changed
}

func cloneValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			out[key] = cloneValue(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, item := range typed {
			out[i] = cloneValue(item)
		}
		return out
	default:
		return typed
	}
}

// Preserve explicitly configured legacy connection values before an installer
// template fills missing module keys with its defaults.
func preserveLegacyConnectionSettings(current map[string]interface{}) bool {
	changed := false
	modules, _ := current["modules"].(map[string]interface{})
	if modules == nil {
		modules = map[string]interface{}{}
		current["modules"] = modules
	}
	if legacy, ok := current["wisemed_ws"].(map[string]interface{}); ok {
		ws, _ := modules["wisemed-ws"].(map[string]interface{})
		if ws == nil {
			ws = map[string]interface{}{}
			modules["wisemed-ws"] = ws
		}
		for _, key := range []string{"enabled", "url", "heartbeat_ms", "reconnect_delay_ms"} {
			if value, exists := legacy[key]; exists {
				if _, already := ws[key]; !already {
					ws[key] = value
					changed = true
				}
			}
		}
	}
	reader, _ := current["reader"].(map[string]interface{})
	analyzer, _ := current["analyzer"].(map[string]interface{})
	if reader["id"] == "signing-pad-utility" || analyzer["protocol"] == "signing-pad" {
		pad, _ := modules["signing-pad"].(map[string]interface{})
		if pad != nil {
			if shared, _ := pad["shared_http"].(bool); !shared {
				local, _ := current["local_http"].(map[string]interface{})
				if local == nil {
					local = map[string]interface{}{}
					current["local_http"] = local
				}
				for _, key := range []string{"address", "tls"} {
					if value, exists := pad[key]; exists {
						local[key] = value
					}
				}
				pad["shared_http"] = true
				changed = true
			}
		}
	}
	return changed
}
