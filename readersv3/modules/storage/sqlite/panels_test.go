package sqlite

import (
	"path/filepath"
	"testing"
	model "wisemed-labreaders/readersv3/modules/core/model"
)

func TestPanelRepeatSelectionAndRollback(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	records := []model.ImportedRecord{{SampleID: "00123", FileID: "00123", AnalyteTag: "PRO", ResultValue: "100", RawValue: "100"}, {SampleID: "00123", FileID: "00123", AnalyteTag: "GLU", ResultValue: "NEG", RawValue: "NEG"}}
	id, err := s.RecordImportedPanel("2026-05-05", records, "serial:COM1")
	if err != nil {
		t.Fatal(err)
	}
	records[0].ResultValue = "30"
	records[1].ResultValue = "50"
	id2, err := s.RecordImportedPanel("2026-05-05", records, "serial:COM1")
	if err != nil {
		t.Fatal(err)
	}
	if id != id2 {
		t.Fatal("repeat created another order")
	}
	analyses, err := s.ListOrderAnalyses(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(analyses) != 2 {
		t.Fatal(analyses)
	}
	histories := make([][]model.OrderAnalysisResult, 2)
	for i, a := range analyses {
		histories[i], err = s.ListResultsForAnalysis(a.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(histories[i]) != 2 {
			t.Fatal(histories[i])
		}
	}
	// Force identical timestamps: grouping must depend on panel identity alone.
	if _, err = s.db.Exec(`update order_analysis_results set created_at='2026-05-05T10:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	if err = s.SetDefaultResult(analyses[0].ID, histories[0][1].ID, "grouped"); err != nil {
		t.Fatal(err)
	}
	for i, a := range analyses {
		got, e := s.GetOrderAnalysis(a.ID)
		if e != nil {
			t.Fatal(e)
		}
		if got.DefaultResultID != histories[i][1].ID || got.SourceResultValue != histories[i][1].SourceResultValue {
			t.Fatalf("panel selection missed analyte: %+v", got)
		}
	}
	if err = s.SetDefaultResult(analyses[1].ID, histories[1][0].ID, "grouped"); err != nil {
		t.Fatal(err)
	}
	for i, a := range analyses {
		got, e := s.GetOrderAnalysis(a.ID)
		if e != nil {
			t.Fatal(e)
		}
		if got.DefaultResultID != histories[i][0].ID {
			t.Fatalf("new panel selection missed analyte: %+v", got)
		}
	}
	// Fail on the second result, after the first has already been inserted in the transaction.
	if _, err = s.db.Exec(`create trigger reject_panel before insert on order_analysis_results when NEW.result_value='FAIL' begin select raise(ABORT,'test failure'); end`); err != nil {
		t.Fatal(err)
	}
	records[0].ResultValue = "999"
	records[1].ResultValue = "FAIL"
	if _, err = s.RecordImportedPanel("2026-05-05", records, "serial:COM1"); err == nil {
		t.Fatal("expected failed transaction")
	}
	for i, a := range analyses {
		got, e := s.GetOrderAnalysis(a.ID)
		if e != nil {
			t.Fatal(e)
		}
		history, e := s.ListResultsForAnalysis(a.ID)
		if e != nil {
			t.Fatal(e)
		}
		if len(history) != 2 || got.DefaultResultID != histories[i][0].ID {
			t.Fatal("partial panel persisted")
		}
	}
}
