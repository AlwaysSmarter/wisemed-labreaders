package astm

import "testing"

func TestParseResultBatch(t *testing.T) {
	raw := "H|\\^&|||MAGLUMI|||||P|1\rO|1|^^^12345||^^^GLU\rR|1|^^^GLU|5.6|mmol/L\rL|1|N\r"
	items := ParseResultBatch(raw)
	if len(items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(items))
	}
	if items[0].SampleID != "12345" {
		t.Fatalf("unexpected sample id: %s", items[0].SampleID)
	}
	if items[0].AnalyteTag != "GLU" {
		t.Fatalf("unexpected analyte tag: %s", items[0].AnalyteTag)
	}
	if items[0].ResultValue != "5.6" {
		t.Fatalf("unexpected result value: %s", items[0].ResultValue)
	}
}
