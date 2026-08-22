package zoncixl1000i

import (
	"strings"
	"testing"
)

func TestNormalizeValue(t *testing.T) {
	if got := normalizeValue(" 484,56 "); got != "484.56" {
		t.Fatalf("got %q", got)
	}
}

func TestZonciResultRowsUseTabs(t *testing.T) {
	frame := "3 46988\nPT-S\t28.4\ts\t9.0\t14.0\nPT-INR\t2.01\t\t0.80\t1.20"
	if got := len(stringsSplitLines(frame)); got != 3 {
		t.Fatalf("rows=%d", got)
	}
}

func TestZonciCSVResultRowsAndDate(t *testing.T) {
	lines := stringsSplitLines("3 22414\nsampleNo,1\ndate,2026-07-23\nFIB,274,mg/dl,200,400")
	if got := zonciResultDate(lines, "VIRGULA"); got != "2026-07-23" {
		t.Fatalf("date=%q", got)
	}
	fields := zonciResultFields(lines[3], "VIRGULA")
	if len(fields) != 5 || fields[0] != "FIB" || fields[1] != "274" {
		t.Fatalf("fields=%#v", fields)
	}
}

func TestZonciMetadataIsNotAResult(t *testing.T) {
	lines := stringsSplitLines("3 22414\nsampleNo,1\npatientNo,22414\nsampleType,Plasma\ndate,2026-07-23\nFIB,274,mg/dl")
	metadata := zonciMetadata(lines, "VIRGULA")
	if metadata["patientno"] != "22414" || metadata["sampletype"] != "Plasma" {
		t.Fatalf("metadata=%#v", metadata)
	}
	if !zonciMetadataKey("date") || zonciMetadataKey("FIB") {
		t.Fatal("metadata key detection is incorrect")
	}
}

func stringsSplitLines(value string) []string { return strings.Split(value, "\n") }
