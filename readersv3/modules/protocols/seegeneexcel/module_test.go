package seegeneexcel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSeeGeneViewerCSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "export.csv")
	content := `Sample No,Patient Id,Well,Name,Type,FAM,,,,HEX,,,,Cal Red 610,,,,Quasar 670,,,,Auto   Interpretation,Comment
,,,,,S gene,C(t),RSV,C(t),RdRP gene,C(t),Flu B,C(t),N gene,C(t),Flu A,C(t),Endo IC,C(t),Exo IC,C(t),,
,,A05,235138 COMBO,SAMPLE,-,N/A,-,N/A,-,N/A,-,N/A,-,N/A,+,32.76,+,27.70,+,22.94,Flu A,
,,D05,NTC COMBO,NC,-,N/A,-,N/A,-,N/A,-,N/A,-,N/A,-,N/A,-,N/A,-,N/A,Negative Control(-),
,,E05,PC COMBO,PC,+,20.55,+,21.23,+,23.47,+,20.63,+,19.73,+,21.72,+,22.22,+,20.69,Positive Control(+),
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := parseSeeGeneViewerCSV(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Analytes) != 8 {
		t.Fatalf("expected 8 analytes, got %d", len(data.Analytes))
	}
	if len(data.SampleRecords) != 8 {
		t.Fatalf("expected 8 sample records, got %d", len(data.SampleRecords))
	}
	if len(data.QCRecords) != 2 {
		t.Fatalf("expected 2 qc records, got %d", len(data.QCRecords))
	}
	if got := data.SampleRecords[0].Record.SampleID; got != "235138" {
		t.Fatalf("expected sample id 235138, got %q", got)
	}
	foundFluA := false
	for _, item := range data.SampleRecords {
		if item.Record.AnalyteTag == "FLU_A" {
			foundFluA = true
			if item.Record.ResultValue != "32.76" {
				t.Fatalf("expected FLU_A Ct 32.76, got %q", item.Record.ResultValue)
			}
			if item.Record.Flags["sign"] != "+" {
				t.Fatalf("expected FLU_A sign +, got %#v", item.Record.Flags["sign"])
			}
		}
	}
	if !foundFluA {
		t.Fatal("expected FLU_A sample result")
	}
	if got := data.QCRecords[0].ControlLevel; got == "" {
		t.Fatal("expected qc control level")
	}
}
