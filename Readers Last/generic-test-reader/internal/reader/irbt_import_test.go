package reader

import (
	"os"
	"path/filepath"
	"testing"

	"wisemed-labreaders/readerslast/generic-test-reader/internal/config"
	"wisemed-labreaders/readerslast/generic-test-reader/internal/model"
	"wisemed-labreaders/readerslast/generic-test-reader/internal/storage"
)

func TestProcessIRBiotyperRowCreatesAndUpdatesSingleResult(t *testing.T) {
	tmp := t.TempDir()
	store, err := storage.Open(filepath.Join(tmp, "reader.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	cfg := config.Default()
	cfg.Reader.ID = "reader-irbt-001"
	cfg.Reader.ClientID = "reader-irbt-001"
	cfg.Reader.Label = "IRBT"
	cfg.Reader.DBName = filepath.Join(tmp, "reader.db")
	cfg.Comm.Type = config.CommTypeFile
	cfg.Comm.Protocol = "IRBIOTYPER"
	cfg.Comm.ProtocolExtra = map[string]interface{}{
		"mapping_file": filepath.Join(tmp, "irbt-mapping.json"),
	}
	if err := os.WriteFile(filepath.Join(tmp, "irbt-mapping.json"), []byte(`[
{"modelKey":"Salmonella O-groups v3","bestValue":"O:9 (D1)","analyte_tag":"SALM_D1"}
]`), 0o644); err != nil {
		t.Fatalf("write mapping file: %v", err)
	}

	_, err = store.UpsertOrder(model.Order{
		RoundNo:  1,
		SampleID: "238886",
		Status:   "scheduled",
	})
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}

	app := New(cfg, store)
	row := irbtRow{
		IsolateIdentifier: "238886",
		ModelKey:          "Salmonella O-groups v3",
		BestValue:         "O:9 (D1)",
		Rating:            "A",
		ScorePercent:      "100",
	}
	mapper, err := loadIRBiotyperMapper(cfg)
	if err != nil {
		t.Fatalf("load mapper: %v", err)
	}

	first, err := app.processIRBiotyperRow("run.csv", row, mapper)
	if err != nil {
		t.Fatalf("first processIRBiotyperRow: %v", err)
	}
	if first["mapping_found"] != true {
		t.Fatalf("expected mapping_found=true, got %#v", first["mapping_found"])
	}

	row.Rating = "B"
	row.ScorePercent = "80"
	second, err := app.processIRBiotyperRow("run.csv", row, mapper)
	if err != nil {
		t.Fatalf("second processIRBiotyperRow: %v", err)
	}

	results, err := store.ListResults(10)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after upsert, got %d", len(results))
	}
	if results[0].OrderAnalysisID == 0 {
		t.Fatalf("missing order_analysis_id in result")
	}
	if results[0].Interpreted != "low_confidence" {
		t.Fatalf("expected low_confidence after second import, got %s", results[0].Interpreted)
	}
	if results[0].Flags["rating"] != "B" {
		t.Fatalf("expected updated rating flag, got %#v", results[0].Flags["rating"])
	}

	analysisMap := second["analysis"].(map[string]interface{})
	analysis, err := store.GetAnalysis(int64(analysisMap["id"].(int64)))
	if err != nil {
		t.Fatalf("get analysis: %v", err)
	}
	if analysis.Status != "completed" {
		t.Fatalf("expected completed status, got %s", analysis.Status)
	}
}

func TestProcessIRBiotyperRowAutoCreatesAnalyteFromTuple(t *testing.T) {
	tmp := t.TempDir()
	store, err := storage.Open(filepath.Join(tmp, "reader.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	cfg := config.Default()
	cfg.Reader.ID = "reader-irbt-001"
	cfg.Reader.ClientID = "reader-irbt-001"
	cfg.Reader.Label = "IRBT"
	cfg.Reader.DBName = filepath.Join(tmp, "reader.db")
	cfg.Comm.Type = config.CommTypeFile
	cfg.Comm.Protocol = "IRBIOTYPER"
	cfg.Comm.ProtocolExtra = map[string]interface{}{
		"mapping_file": filepath.Join(tmp, "irbt-mapping.json"),
	}
	if err := os.WriteFile(filepath.Join(tmp, "irbt-mapping.json"), []byte(`[]`), 0o644); err != nil {
		t.Fatalf("write mapping file: %v", err)
	}

	if _, err := store.UpsertOrder(model.Order{
		RoundNo:  1,
		SampleID: "238999",
		Status:   "scheduled",
	}); err != nil {
		t.Fatalf("insert order: %v", err)
	}

	app := New(cfg, store)
	mapper, err := loadIRBiotyperMapper(cfg)
	if err != nil {
		t.Fatalf("load mapper: %v", err)
	}
	row := irbtRow{
		IsolateIdentifier: "238999",
		ModelKey:          "Unknown model",
		BestValue:         "Unknown result",
		Rating:            "A",
		ScorePercent:      "100",
	}

	outcome, err := app.processIRBiotyperRow("run.csv", row, mapper)
	if err != nil {
		t.Fatalf("processIRBiotyperRow: %v", err)
	}
	if outcome["mapping_found"] != false {
		t.Fatalf("expected mapping_found=false, got %#v", outcome["mapping_found"])
	}
	if outcome["analyte_created"] != true {
		t.Fatalf("expected analyte_created=true, got %#v", outcome["analyte_created"])
	}

	analyte, err := store.GetAnalyteByCode(irbtMapKey(row.ModelKey, row.BestValue))
	if err != nil {
		t.Fatalf("GetAnalyteByCode: %v", err)
	}
	if analyte.Tag == "" {
		t.Fatalf("expected generated analyte tag")
	}
}
