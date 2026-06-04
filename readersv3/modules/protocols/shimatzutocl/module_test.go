package shimatzutocl

import (
	"os"
	"path/filepath"
	"testing"

	coremodel "wisemed-labreaders/readersv3/modules/core/model"
)

func TestParseShimatzuTOCL(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "export.txt")
	content := "[Header]\n" +
		"System\tTOC-LCPH\n\n" +
		"[Data]\n" +
		"Type\tAnal.\tSample Name\tSample ID\tResult(TOC)\tResult(TC)\tResult(IC)\tResult(POC)\tResult(NPOC)\tResult(TN)\tUnit\tVial\tDate / Time\n" +
		"Unknown\tNPOC\tet\tpc_1ppm\t\t\t\t\t1.249\t\tmg/L\t2\t10/15/2025 9:40:41 AM\n" +
		"Unknown\tNPOC\tapa potabila\tC_AC_915\t2.001\t2.100\t0.101\t0.220\t2.553\t1.400\tmg/L\t5\t10/15/2025 11:49:58 AM\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := parseShimatzuTOCL(path)
	if err != nil {
		t.Fatalf("parseShimatzuTOCL() error = %v", err)
	}
	if len(data.Analytes) != 6 {
		t.Fatalf("unexpected analytes: %#v", data.Analytes)
	}
	if len(data.SampleRecords) != 6 {
		t.Fatalf("unexpected sample records count: %d", len(data.SampleRecords))
	}
	foundTOC := false
	foundNPOC := false
	for _, rec := range data.SampleRecords {
		if got := rec.Record.SampleID; got != "C_AC_915" {
			t.Fatalf("sample id = %q, want %q", got, "C_AC_915")
		}
		switch rec.Record.AnalyteTag {
		case "TOC":
			foundTOC = rec.Record.ResultValue == "2.001"
		case "NPOC":
			foundNPOC = rec.Record.ResultValue == "2.553"
		}
	}
	if !foundTOC || !foundNPOC {
		t.Fatalf("expected TOC and NPOC records, got %#v", data.SampleRecords)
	}
	if len(data.QCRecords) != 1 {
		t.Fatalf("unexpected qc records count: %d", len(data.QCRecords))
	}
	if got := data.QCRecords[0].ControlLabel; got != "PC_1PPM" {
		t.Fatalf("qc control label = %q, want %q", got, "PC_1PPM")
	}
	if len(data.QCRecords[0].Results) != 1 {
		t.Fatalf("unexpected qc analyte count: %d", len(data.QCRecords[0].Results))
	}
}

func TestNormalizeImportedSampleID(t *testing.T) {
	t.Parallel()

	rules := sampleCodeRules{
		SamplePrefixes: []string{"C_AC_", "M_AP_"},
		SampleSuffixes: []string{"_PB", "_PV"},
		Separators:     []string{"-", "_"},
	}

	if got := normalizeImportedSampleParts("C_AC_915", rules); got.Normalized != "915" || got.FileID != "915" {
		t.Fatalf("normalizeImportedSampleParts(prefix) = %#v", got)
	}
	if got := normalizeImportedSampleParts("M_AP_915_PB", rules); got.Normalized != "915" || got.FileID != "915" {
		t.Fatalf("normalizeImportedSampleParts(prefix+suffix) = %#v", got)
	}
	if got := normalizeImportedSampleParts("C_AC_123-456-7", rules); got.Normalized != "123-456-7" || got.FileID != "123" || got.SampleCodeID != "456" || got.SpecimenCode != "7" {
		t.Fatalf("normalizeImportedSampleParts(split) = %#v", got)
	}
	if got := normalizeImportedSampleParts("PC_1PPM", rules); got.Normalized != "PC_1PPM" {
		t.Fatalf("normalizeImportedSampleParts(qc) = %#v", got)
	}
}

func TestNormalizeImportedRecord(t *testing.T) {
	t.Parallel()

	record := normalizeImportedRecord(coremodel.ImportedRecord{
		SampleID:    "C_AC_915-23442-2",
		FileID:      "C_AC_915-23442-2",
		PatientID:   "C_AC_915-23442-2",
		PatientName: "C_AC_915-23442-2",
		Flags: map[string]interface{}{
			"sample_raw": "C_AC_915-23442-2",
		},
		Meta: map[string]interface{}{},
	}, sampleCodeRules{
		SamplePrefixes: []string{"C_AC_"},
		Separators:     []string{"-"},
	})

	if record.SampleID != "915-23442-2" || record.FileID != "915" || record.PatientID != "23442" || record.PatientName != "2" {
		t.Fatalf("normalizeImportedRecord() = %#v", record)
	}
	if got := record.Meta["sent_sample_code"]; got != "C_AC_915-23442-2" {
		t.Fatalf("sent_sample_code = %#v, want %q", got, "C_AC_915-23442-2")
	}
}
