// Package astm contains reusable ASTM conventions shared by protocol modules.
package astm

import (
	"fmt"
	"strings"
)

// ResolveSpecimenCode maps a WiseMED/API specimen code to the value expected by
// a specific analyzer. Configuration lives in modules.protocol-astm:
// specimen_code_default and specimen_code_map.
func ResolveSpecimenCode(settings map[string]interface{}, sourceCode, fallback string) string {
	sourceCode = normalizeCode(sourceCode)
	for source, target := range stringMap(settings["specimen_code_map"]) {
		if normalizeCode(source) == sourceCode && strings.TrimSpace(target) != "" {
			return strings.TrimSpace(target)
		}
	}
	return firstNonEmpty(asString(settings["specimen_code_default"]), fallback)
}

func stringMap(value interface{}) map[string]string {
	out := map[string]string{}
	switch values := value.(type) {
	case map[string]string:
		for key, item := range values {
			out[key] = item
		}
	case map[string]interface{}:
		for key, item := range values {
			out[key] = asString(item)
		}
	}
	return out
}

func asString(value interface{}) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func normalizeCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
