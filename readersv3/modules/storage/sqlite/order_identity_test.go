package sqlite

import (
	"errors"
	"path/filepath"
	"testing"
	model "wisemed-labreaders/readersv3/modules/core/model"
)

func TestCorrectOrderIdentityAuditAndRepeat(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "reader.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	order, analysis, result, err := s.RecordImportedResult("2026-09-11", 1, model.ImportedRecord{SampleID: "POPESCU", FileID: "OLD", AnalyteTag: "GLU", ResultValue: "100", Meta: map[string]interface{}{"wisemed_match_flag": 4, "sent_sample_code": "POPESCU ION", "sync_lookup": "old-patient"}, Flags: map[string]interface{}{"wisemed_send_status": "sent", "abnormal": true}}, "serial:COM1")
	if err != nil {
		t.Fatal(err)
	}
	analysis.DefaultResultID = result.ID
	analysis.WiseMEDFSMID = "old-fsm"
	analysis.WiseMEDSMID = "old-sm"
	if _, err = s.SaveOrderAnalysis(analysis); err != nil {
		t.Fatal(err)
	}
	change := model.OrderIDChange{OrderID: order.ID, ExpectedID: "POPESCU", NewID: "001234", Confirmation: "yes"}
	if _, err = s.CorrectOrderID(change, "Ana Pop"); !errors.Is(err, model.ErrOrderIdentityInvalid) {
		t.Fatal("accepted missing consent", err)
	}
	change.Confirmation = "deacord"
	change.Reason = "Cod verificat"
	corrected, err := s.CorrectOrderID(change, "Ana Pop")
	if err != nil {
		t.Fatal(err)
	}
	if corrected.SampleID != "001234" || corrected.FileID != "001234" || corrected.ManualFileID() != "001234" {
		t.Fatal(corrected)
	}
	a, err := s.GetOrderAnalysis(analysis.ID)
	if err != nil {
		t.Fatal(err)
	}
	if a.ResultValue != "100" || a.DefaultResultID != result.ID || a.WiseMEDFSMID != "" || a.Flags["abnormal"] != true {
		t.Fatal(a)
	}
	if _, exists := corrected.Meta["sync_lookup"]; exists {
		t.Fatal("old association retained")
	}
	if _, err = s.UpsertOrder(order); !errors.Is(err, model.ErrOrderIdentityConflict) {
		t.Fatal("stale sync overwrote identity", err)
	}
	if _, err = s.CorrectOrderID(change, "Other"); !errors.Is(err, model.ErrOrderIdentityConflict) {
		t.Fatal("stale edit accepted", err)
	}
	change.ExpectedID = "001234"
	change.ExpectedRevision = 1
	change.NewID = "005678"
	change.Reason = ""
	corrected, err = s.CorrectOrderID(change, "Maria Ionescu")
	if err != nil {
		t.Fatal(err)
	}
	c := corrected.Meta["id_correction"].(map[string]interface{})
	if c["original_id"] != "POPESCU ION" || c["actor"] != "Maria Ionescu" || c["reason"] != "" {
		t.Fatal(c)
	}
	if len(corrected.Meta["id_correction_history"].([]interface{})) != 2 {
		t.Fatal(corrected.Meta)
	}
	var audits int
	if err = s.db.QueryRow(`select count(*) from audit_logs where event_type='order-id-change'`).Scan(&audits); err != nil || audits != 2 {
		t.Fatal(audits, err)
	}
	repeated, _, _, err := s.RecordImportedResult("2026-09-11", 1, model.ImportedRecord{SampleID: "POPESCU", FileID: "POPESCU", AnalyteTag: "GLU", ResultValue: "105"}, "serial:COM1")
	if err != nil {
		t.Fatal(err)
	}
	if repeated.ID != order.ID || repeated.SampleID != "005678" || repeated.FileID != "005678" {
		t.Fatal("repeat lost correction", repeated)
	}
}
func TestIDChangeIsAtomicAndRejectsCollision(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "reader.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	a, err := s.UpsertOrder(model.Order{OrderDate: "2026-09-11", SampleID: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpsertOrder(model.Order{OrderDate: "2026-09-11", SampleID: "B"}); err != nil {
		t.Fatal(err)
	}
	change := model.OrderIDChange{OrderID: a.ID, ExpectedID: "A", NewID: "B", Confirmation: "deacord"}
	if _, err = s.CorrectOrderID(change, "User"); !errors.Is(err, model.ErrOrderIdentityConflict) {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`create trigger reject_id_audit before insert on audit_logs begin select raise(ABORT,'audit unavailable'); end`); err != nil {
		t.Fatal(err)
	}
	change.NewID = "C"
	if _, err = s.CorrectOrderID(change, "User"); err == nil {
		t.Fatal("expected failure")
	}
	got, err := s.GetOrder(a.ID)
	if err != nil || got.SampleID != "A" {
		t.Fatal("partial correction", got, err)
	}
}
