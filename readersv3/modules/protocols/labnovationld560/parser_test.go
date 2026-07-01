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
	if items[0].InstrumentModel != "LD560" {
		t.Fatalf("expected model LD560, got %q", items[0].InstrumentModel)
	}
	if items[0].InstrumentSequentialNumber != "LD560-001" {
		t.Fatalf("expected instrument serial LD560-001, got %q", items[0].InstrumentSequentialNumber)
	}
	if items[0].RunDate != "2018-03-15" {
		t.Fatalf("expected run date 2018-03-15, got %q", items[0].RunDate)
	}
	if items[0].RunTime != "22:34:54" {
		t.Fatalf("expected run time 22:34:54, got %q", items[0].RunTime)
	}
	if items[0].SampleNo != "3105" {
		t.Fatalf("expected sample no 3105, got %q", items[0].SampleNo)
	}
	if items[0].SampleID != "10" {
		t.Fatalf("expected sample id 10, got %q", items[0].SampleID)
	}
	if items[0].FileID != "10" {
		t.Fatalf("expected file id 10 for documented format, got %q", items[0].FileID)
	}
	if len(items[0].Results) != 7 {
		t.Fatalf("expected 7 results, got %d", len(items[0].Results))
	}
}

func TestParseSimpleResultsObservedCompactFormat(t *testing.T) {
	raw := []byte(`<TRANSMIT><M>LD560|LD560-001<I>sample|20260618162524|32|370112|12|0</I><R>HbA1a|0.6HbA1b|0.3Hbf|1.6L-A1c|0.5HbA1c|7.7HbA0|90.0eAG|172.8</R></M></TRANSMIT>`)
	items, err := parseSimpleResults(raw, simpleSettingsFromMap(defaultSimpleSettings()))
	if err != nil {
		t.Fatalf("parseSimpleResults observed compact format error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 message, got %d", len(items))
	}
	got := items[0]
	if got.RunDate != "2026-06-18" {
		t.Fatalf("expected run date 2026-06-18, got %q", got.RunDate)
	}
	if got.RunTime != "16:25:24" {
		t.Fatalf("expected run time 16:25:24, got %q", got.RunTime)
	}
	if got.Sample != "sample" {
		t.Fatalf("expected sample marker 'sample', got %q", got.Sample)
	}
	if got.SampleNo != "32" {
		t.Fatalf("expected sample no 32, got %q", got.SampleNo)
	}
	if got.FileID != "370112" {
		t.Fatalf("expected file id 370112, got %q", got.FileID)
	}
	if got.SampleID != "370112" {
		t.Fatalf("expected sample id fallback 370112, got %q", got.SampleID)
	}
	if got.RackNo != "1" || got.RackPosition != "2" {
		t.Fatalf("expected rack 1 position 2, got rack=%q position=%q", got.RackNo, got.RackPosition)
	}
	if got.SampleType != "0" {
		t.Fatalf("expected sample type 0, got %q", got.SampleType)
	}
}

func TestParseSimpleResultsObservedCompactFormatMissingFields(t *testing.T) {
	raw := []byte(`<TRANSMIT><M>LD560|LD560-001<I>sample|20260618161812|36||1|0</I><R>HbA1a|0.0HbA1b|0.0Hbf|0.0L-A1c|0.0HbA1c|0.0HbA0|0.0eAG|0.0</R></M></TRANSMIT>`)
	items, err := parseSimpleResults(raw, simpleSettingsFromMap(defaultSimpleSettings()))
	if err != nil {
		t.Fatalf("parseSimpleResults missing fields format error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 message, got %d", len(items))
	}
	got := items[0]
	if got.SampleNo != "36" {
		t.Fatalf("expected sample no 36, got %q", got.SampleNo)
	}
	if got.FileID != "" {
		t.Fatalf("expected missing file id, got %q", got.FileID)
	}
	if got.SampleID != "36" {
		t.Fatalf("expected sample id fallback 36, got %q", got.SampleID)
	}
	if got.RackNo != "" || got.RackPosition != "1" {
		t.Fatalf("expected missing rack and tube 1, got rack=%q position=%q", got.RackNo, got.RackPosition)
	}
	if got.SampleType != "0" {
		t.Fatalf("expected sample type 0, got %q", got.SampleType)
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
