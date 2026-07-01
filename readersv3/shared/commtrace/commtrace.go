package commtrace

import (
	"encoding/hex"
	"fmt"
	"strings"
)

func Format(direction, transport, details string, payload []byte, level int) string {
	arrow := "==>"
	label := "IN"
	if strings.EqualFold(strings.TrimSpace(direction), "out") {
		arrow = "<=="
		label = "OUT"
	}
	transport = strings.ToUpper(strings.TrimSpace(transport))
	details = strings.TrimSpace(details)
	if details != "" {
		details = " " + details
	}
	text := strings.TrimSpace(string(payload))
	if level <= 4 {
		return fmt.Sprintf("%s %s %s%s %s", arrow, label, transport, details, singleLine(text))
	}
	if len(payload) == 0 {
		return fmt.Sprintf("%s %s %s%s", arrow, label, transport, details)
	}
	return fmt.Sprintf("%s %s %s%s\nTEXT:\n%s\nHEX:\n%s", arrow, label, transport, details, preserveTrailingNewline(text), strings.TrimRight(hex.Dump(payload), "\n"))
}

func singleLine(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "(empty)"
	}
	return value
}

func preserveTrailingNewline(value string) string {
	if value == "" {
		return "(empty)"
	}
	if strings.HasSuffix(value, "\n") {
		return value
	}
	return value + "\n"
}
