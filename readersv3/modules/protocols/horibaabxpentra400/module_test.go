package horibaabxpentra400

import (
	"strings"
	"testing"
	"time"
)

func TestBuildOrderRecordUsesValidDefaultSpecimenType(t *testing.T) {
	record := buildOrderRecord("23032", []string{"^^^25"}, firstNonEmpty(asString(nil), "1"), time.Date(2026, 8, 22, 16, 15, 0, 0, time.UTC))
	fields := strings.Split(record, "|")
	if got := fields[15]; got != "1" {
		t.Fatalf("O.16 specimen type = %q, want 1; record=%s", got, record)
	}
	if strings.Contains(record, "<nil>") {
		t.Fatalf("ASTM order must never contain <nil>: %s", record)
	}
}
