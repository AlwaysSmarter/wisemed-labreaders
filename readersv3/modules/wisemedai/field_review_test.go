package wisemedai

import (
	"context"
	"github.com/google/uuid"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFieldFailureDoesNotDiscardOtherObservations(t *testing.T) {
	tpl := testTemplate()
	tpl.SemanticMapping = map[string]string{"dimensiune": "measurement", "ficat": "description"}
	for _, raw := range []string{"12 mm", "02", "zero virgulă doi"} {
		chunks := []Chunk{{ID: "c", Revision: 1, Text: "ficat: omogen\ndimensiune: " + raw}}
		facts := []Fact{{Field: "ficat", Value: "omogen", Quote: "ficat: omogen", ChunkID: "c", State: "present"}, {Field: "dimensiune", Value: raw, Quote: "dimensiune: " + raw, ChunkID: "c", State: "present", Suggestions: []string{"0.2"}}}
		r, e := buildResult(tpl, chunks, facts, "s", 1)
		if e != nil || len(r.FieldIssues) != 1 || len(r.Evidence) != 2 || displayValue(r.Data["ficat"]) != "omogen" || displayValue(r.Data["dimensiune"]) != raw || r.CanonicalData["measurement"] != nil || r.CanonicalData["description"] == nil {
			t.Fatal(raw, r, e)
		}
	}
}
func TestIncrementalFieldsSurviveOmissionsAndManualEdits(t *testing.T) {
	tpl := testTemplate()
	chunks := []Chunk{{ID: "c1", Revision: 1, Text: "ficat: omogen"}, {ID: "c2", Revision: 1, Text: "dimensiune: 140"}}
	first, _ := buildResult(tpl, chunks[:1], mockExtract(tpl, chunks[:1]), "s", 1)
	s := Session{ID: "s", Revision: 1, Chunks: chunks, Result: &first, Context: map[string]any{}}
	next, e := buildSessionResult(tpl, s, mockExtract(tpl, chunks[1:]), "")
	if e != nil || len(next.Data) != 2 || next.Revision != 2 {
		t.Fatal(next, e)
	}
	s.Result = &next
	s.Revision = 2
	s.Context["field_overrides"] = map[string]any{"dimensiune": "0,2"}
	manual, e := buildSessionResult(tpl, s, nil, "")
	if e != nil || len(manual.ValidationErrors) != 0 || displayValue(manual.Data["dimensiune"]) != "0.2" || !manual.Data["dimensiune"].(map[string]any)["user_edited"].(bool) {
		t.Fatal(manual, e)
	}
	s.Result = &manual
	preserved, _ := buildSessionResult(tpl, s, mockExtract(tpl, chunks[1:]), "")
	if displayValue(preserved.Data["dimensiune"]) != "0.2" || len(preserved.FieldIssues) != 0 {
		t.Fatal("user value overwritten", preserved)
	}
	delete(s.Context["field_overrides"].(map[string]any), "dimensiune")
	redictated, _ := buildSessionResult(tpl, s, mockExtract(tpl, chunks[1:]), "dimensiune")
	if displayValue(redictated.Data["dimensiune"]) != "140" || displayValue(redictated.Data["ficat"]) != "omogen" {
		t.Fatal(redictated)
	}
	// Revising source invalidates only facts tied to that revision.
	s.Chunks[0].Revision = 2
	s.Chunks[0].Text = "alt text"
	revised, _ := buildSessionResult(tpl, s, nil, "")
	if revised.Data["ficat"] != nil {
		t.Fatal("stale evidence survived", revised)
	}
}
func TestMissingFieldsHaveDashAndCanBeEdited(t *testing.T) {
	r, e := buildResult(testTemplate(), nil, nil, "s", 1)
	if e != nil || !strings.Contains(r.HTML, "—") || strings.Contains(r.HTML, "[nedocumentat]") {
		t.Fatal(r, e)
	}
	scoped, ok := fieldTemplate(presentationFixture(), "masura")
	if !ok || len(scoped.Fields) != 1 || len(scoped.Sections) != 1 || scoped.Sections[0].ID != "componenta" {
		t.Fatal(scoped)
	}
}

func TestPostgresIncrementalReviewAndTargetedCorrection(t *testing.T) {
	m, rt := integrationModule(t)
	password, err := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	if err != nil {
		t.Fatal(err)
	}
	code, login := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(password)})
	if code != 200 {
		t.Fatal(code, login)
	}
	token := login["token"].(string)
	tenant := login["profile"].(map[string]any)["tenant_id"].(string)
	var version string
	if err = m.db.QueryRow(context.Background(), `SELECT current_version FROM wai_templates WHERE tenant_id=$1 AND name='Ecografie abdominală'`, tenant).Scan(&version); err != nil {
		t.Fatal(err)
	}
	code, s := request(t, m, token, "POST", "sessions", map[string]any{"template_version": version, "patient_ref": "SYNTHETIC-FIELD-REVIEW"})
	if code != 201 {
		t.Fatal(code, s)
	}
	path := "sessions/" + s["id"].(string)
	send := func(seq int, text, target string) map[string]any {
		t.Helper()
		input := map[string]any{"id": uuid.NewString(), "sequence": seq, "revision": 1, "is_final": true, "timestamp": time.Now().UTC(), "text": text, "target_field": target}
		code, out := request(t, m, token, "POST", path+"/transcript-chunks", input)
		if code != 200 || out["accepted"] != true {
			t.Fatal("field issue rejected a chunk", code, out)
		}
		code, duplicate := request(t, m, token, "POST", path+"/transcript-chunks", input)
		if code != 200 || duplicate["duplicate"] != true {
			t.Fatal("acknowledged chunk reprocessed", duplicate)
		}
		return out["result"].(map[string]any)
	}
	r := send(1, "ficat: omogen\nficat_mm: 02", "")
	if len(r["field_issues"].([]any)) != 1 || r["structured_data"].(map[string]any)["ficat"] == nil {
		t.Fatal(r)
	}
	r = send(2, "splina: omogenă", "")
	data := r["structured_data"].(map[string]any)
	if data["ficat"] == nil || data["ficat_mm"] == nil || data["splina"] == nil {
		t.Fatal("prior data discarded", r)
	}
	code, edited := request(t, m, token, "POST", path+"/field-edit", map[string]any{"revision": r["revision"], "field": "ficat_mm", "value": "0,2"})
	if code != 200 {
		t.Fatal(code, edited)
	}
	r = edited["result"].(map[string]any)
	if len(r["field_issues"].([]any)) != 0 || len(r["validation_errors"].([]any)) != 0 {
		t.Fatal("user edit not accepted", r)
	}
	code, _ = request(t, m, token, "POST", path+"/field-edit", map[string]any{"revision": 1, "field": "ficat_mm", "value": "55"})
	if code != 409 {
		t.Fatal("stale edit accepted", code)
	}
	r = send(3, "140", "ficat_mm")
	data = r["structured_data"].(map[string]any)
	if data["ficat_mm"].(map[string]any)["value"] != float64(140) || data["ficat"].(map[string]any)["value"] != "omogen" || data["splina"] == nil {
		t.Fatal("targeted correction changed other fields", r)
	}
	var loaded map[string]any
	// Verify target metadata and revision survive a reconnect.
	code, loaded = request(t, m, token, "GET", path, nil)
	if code != 200 || loaded["chunks"].([]any)[2].(map[string]any)["target_field"] != "ficat_mm" {
		t.Fatal(loaded)
	}
}
