package labnovationld560

import "testing"

func TestParseSimpleResults(t *testing.T) {
	raw := []byte(`<TRANSMIT><M>LD560|LD560-001</M><I>sample|2018-03-15 22:34:54|3105|10|1|10|0</I><R>HbA1a|1.04HbA1b|1.01HbF|1.5L-A1C|1.0HbA1c|7.19HbA0|92eAG|4.5</R></TRANSMIT>`)
	items, err := parseSimpleResults(raw, simpleSettingsFromMap(defaultSimpleSettings()))
	if err != nil {
		t.Fatalf("parseSimpleResults error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 message, got %d", len(items))
	}
	if items[0].SampleID != "10" {
		t.Fatalf("expected sample id 10, got %q", items[0].SampleID)
	}
	if len(items[0].Results) != 7 {
		t.Fatalf("expected 7 results, got %d", len(items[0].Results))
	}
}

func TestParseHL7Results(t *testing.T) {
	raw := []byte("\x0bMSH|^~\\&|LD560|LAB|LIS|WM|202605271230||ORU^R01|MSG1|P|2.3\rPID|1||P001||DOE^JOHN\rOBR|1||SAMPLE-10||||202605271229\rOBX|1|NM|HbA1c^HbA1c||7.19|%||||F\rOBX|2|NM|HbF^HbF||1.5|%||||F\r\x1c\r")
	settings := hl7SettingsFromMap(defaultHL7Settings())
	items, err := parseHL7Results(raw, settings)
	if err != nil {
		t.Fatalf("parseHL7Results error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 message, got %d", len(items))
	}
	if items[0].SampleID != "SAMPLE-10" {
		t.Fatalf("expected sample id SAMPLE-10, got %q", items[0].SampleID)
	}
	if len(items[0].Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(items[0].Results))
	}
	if items[0].Results[0].AnalyteTag != "HbA1c" {
		t.Fatalf("expected HbA1c tag, got %q", items[0].Results[0].AnalyteTag)
	}
}
