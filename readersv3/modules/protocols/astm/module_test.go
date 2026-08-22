package astm

import "testing"

func TestASTMExpectedTrailerBytes(t *testing.T) {
	if got := astmExpectedTrailerBytes(tcpConfig{ChecksumMode: "astm", TrailerMode: "crlf"}); got != 4 {
		t.Fatalf("standard trailer bytes = %d, want 4", got)
	}
	if got := astmExpectedTrailerBytes(tcpConfig{ChecksumMode: "none", TrailerMode: "none"}); got != 0 {
		t.Fatalf("raw trailer bytes = %d, want 0", got)
	}
}

func TestFormatASTMPackageTextKeepsRecordSeparatorsVisible(t *testing.T) {
	payload := "H|\\^&\rP|1\nL|1|N\r"
	if got, want := formatASTMPackageText(payload), `H|\^&\rP|1\nL|1|N\r`; got != want {
		t.Fatalf("formatted package = %q, want %q", got, want)
	}
}

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

func TestParseBatchUsesLifotronicOrderSampleID(t *testing.T) {
	cfg := tcpConfig{
		SampleIDPaths:     []string{"O.2.1", "O.3.1"},
		SampleIDTrimRight: "-",
		ResultIDPaths:     []string{"R.2.4"},
		ResultName:        []string{"R.2.4"},
		ResultValue:       []string{"R.3.1"},
	}
	payload := "O|1|23124-------------^1519^***^03|****|^^^HbA1c|R\rR|1|^^^HbA1c|5.5|%\rL|1|N\r"
	items := parseBatch(parseRecords(payload), cfg)
	if len(items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(items))
	}
	if got, want := items[0].Order.SampleID, "23124"; got != want {
		t.Fatalf("sample id = %q, want %q", got, want)
	}
}

func TestParseBatchTrimsConfiguredSampleIDCharacters(t *testing.T) {
	cfg := tcpConfig{
		SampleIDPaths:     []string{"O.2.1"},
		SampleIDTrimLeft:  "*",
		SampleIDTrimRight: "-",
		ResultIDPaths:     []string{"R.2.4"},
		ResultName:        []string{"R.2.4"},
		ResultValue:       []string{"R.3.1"},
	}
	payload := "O|1|***23124---\rR|1|^^^HbA1c|5.5|%\rL|1|N\r"
	items := parseBatch(parseRecords(payload), cfg)
	if len(items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(items))
	}
	if got, want := items[0].Order.SampleID, "23124"; got != want {
		t.Fatalf("sample id = %q, want %q", got, want)
	}
}

func TestParseBatchUsesAutoLumoResultFields(t *testing.T) {
	cfg := tcpConfig{
		SampleIDPaths: []string{"O.3.2", "O.3.1", "O.2.1"},
		ResultIDPaths: []string{"R.2.2", "R.2.1"},
		ResultName:    []string{"R.5.1", "R.2.1"},
		ResultValue:   []string{"R.3.2", "R.3.1"},
	}
	payload := "H|\\^&|||AutoLumo S900||0|||||REQ5|1394-97|20260821230912\rP|1|||||||||||||||||||||||||||||||||\rO|1||^23159^9^9|1256^214^||||||||||||||||||||||||||\rR|1|1256^214^|109856625^35.467^||25-OH Vitamin D^214^20260310^03/09/2027|||F||||20260821123402|\rL|1|N\r"
	items := parseBatch(parseRecords(payload), cfg)
	if len(items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(items))
	}
	got := items[0]
	if got.Order.SampleID != "23159" {
		t.Fatalf("sample id = %q, want %q", got.Order.SampleID, "23159")
	}
	if got.AnalyteTag != "214" || got.AnalyteName != "25-OH Vitamin D" || got.Value != "35.467" {
		t.Fatalf("unexpected result: %#v", got)
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
