package reader

import (
	"os"
	"testing"
)

func TestParseIRBiotyperCSV(t *testing.T) {
	raw := []byte(`#IR Biotyper Server 4.1.1.121-20250122-1003
#c969a2ef-9195-4766-98af-8a23d2c57ec4
#251015-salmo
"isolateIdentifier";"modelKey";"bestValue";"rating";"scorePercent"
"238886";"Salmonella O-groups v3";"O:9 (D1)";"A";"100"
"238891";"Salmonella O-groups v3";"O:4 (B)";"A";"100"
`)

	records, err := parseIRBiotyperCSV(raw)
	if err != nil {
		t.Fatalf("parseIRBiotyperCSV returned error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].SampleID != "238886" {
		t.Fatalf("unexpected sample id: %s", records[0].SampleID)
	}
	if records[0].AnalyteTag != "SALMONELLA_O_GROUPS_V3" {
		t.Fatalf("unexpected analyte tag: %s", records[0].AnalyteTag)
	}
	if records[0].ResultValue != "O:9 (D1)" {
		t.Fatalf("unexpected result value: %s", records[0].ResultValue)
	}
	if records[0].Flags["rating"] != "A" {
		t.Fatalf("unexpected rating flag: %#v", records[0].Flags["rating"])
	}
	if records[0].Meta["run_name"] != "251015-salmo" {
		t.Fatalf("unexpected run name: %#v", records[0].Meta["run_name"])
	}
}

func TestParseImportFileAutodetectsIRBiotyperShape(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/irbt.csv"
	raw := `#IR Biotyper Server 4.1.1.121-20250122-1003
#c969a2ef-9195-4766-98af-8a23d2c57ec4
#251015-salmo
"isolateIdentifier";"modelKey";"bestValue";"rating";"scorePercent"
"238886";"Salmonella O-groups v3";"O:9 (D1)";"A";"100"
`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write temp csv: %v", err)
	}
	records, err := parseImportFile(path, "GENERIC")
	if err != nil {
		t.Fatalf("parseImportFile returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].AnalyteTag != "SALMONELLA_O_GROUPS_V3" {
		t.Fatalf("unexpected analyte tag: %s", records[0].AnalyteTag)
	}
}

func TestParseCSVRecordsSkipsMetadataLines(t *testing.T) {
	raw := []byte(`#comment
sample_id;analyte_tag;result_value
123;ABC;42
`)
	records, err := parseCSVRecords(raw)
	if err != nil {
		t.Fatalf("parseCSVRecords returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].AnalyteTag != "ABC" {
		t.Fatalf("unexpected analyte tag: %s", records[0].AnalyteTag)
	}
}
