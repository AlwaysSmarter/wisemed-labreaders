package astm

import "testing"

func TestParseBatchExtractsSampleResult(t *testing.T) {
	cfg := tcpConfig{
		SampleIDPaths: []string{"O.3.1", "O.2.1"},
		PatientIDPath: []string{"P.3.1"},
		PatientName:   []string{"P.5.1"},
		RunDatePaths:  []string{"O.7.1"},
		ResultIDPaths: []string{"R.2.4", "R.2.1"},
		ResultName:    []string{"R.2.4", "R.2.1"},
		ResultValue:   []string{"R.3.1"},
		ResultUnit:    []string{"R.4.1"},
		ResultFlag:    []string{"R.6.1"},
		QCPrefixes:    []string{"QC"},
	}
	payload := "H|\\^&||||||||||||20260625\rP|1||PAT-01||DOE^JOHN\rO|1|FILE-01|SAMPLE-10||||20260625\rR|1|^^^ESBL|1|POS|||FINAL\rL|1|N\r"
	records := parseRecords(payload)
	items := parseBatch(records, cfg)
	if len(items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(items))
	}
	got := items[0]
	if got.Order.SampleID != "SAMPLE-10" {
		t.Fatalf("sample id = %q, want %q", got.Order.SampleID, "SAMPLE-10")
	}
	if got.Order.PatientID != "PAT-01" {
		t.Fatalf("patient id = %q, want %q", got.Order.PatientID, "PAT-01")
	}
	if got.AnalyteTag != "ESBL" {
		t.Fatalf("analyte tag = %q, want %q", got.AnalyteTag, "ESBL")
	}
	if got.Value != "1" {
		t.Fatalf("value = %q, want %q", got.Value, "1")
	}
}

func TestParseBatchMarksQCSamples(t *testing.T) {
	cfg := tcpConfig{
		SampleIDPaths: []string{"O.3.1"},
		ResultIDPaths: []string{"R.2.4"},
		ResultName:    []string{"R.2.4"},
		ResultValue:   []string{"R.3.1"},
		QCPrefixes:    []string{"QC"},
	}
	payload := "O|1||QC-L1\rR|1|^^^CARBA|0\rL|1|N\r"
	items := parseBatch(parseRecords(payload), cfg)
	if len(items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(items))
	}
	if !items[0].Order.IsQC {
		t.Fatal("expected QC sample to be detected")
	}
}

func TestParseBatchFallsBackToPatientIDWhenOrderSampleMissing(t *testing.T) {
	cfg := tcpConfig{
		SampleIDPaths: []string{"O.3.1", "O.2.1"},
		PatientIDPath: []string{"P.3.1"},
		RunDatePaths:  []string{"O.7.1", "O.6.1"},
		ResultIDPaths: []string{"R.2.4", "R.2.1"},
		ResultName:    []string{"R.2.4", "R.2.1"},
		ResultValue:   []string{"R.3.1"},
	}
	payload := "H|\\^&|||\rP|1||240302\rO|1|||^^^Rubella M EURO||20260625094850\rR|1|^^^Rubella M EURO|NEGATIV||\rL|1|N\r"
	items := parseBatch(parseRecords(payload), cfg)
	if len(items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(items))
	}
	if items[0].Order.SampleID != "240302" {
		t.Fatalf("sample id = %q, want %q", items[0].Order.SampleID, "240302")
	}
	if items[0].Value != "NEGATIV" {
		t.Fatalf("value = %q, want %q", items[0].Value, "NEGATIV")
	}
	if items[0].AnalyteTag != "RUBELLA_M_EURO" {
		t.Fatalf("analyte tag = %q, want %q", items[0].AnalyteTag, "RUBELLA_M_EURO")
	}
}
