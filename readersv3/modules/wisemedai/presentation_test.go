package wisemedai

import (
	"github.com/google/uuid"
	"os"
	"strings"
	"testing"
	"time"
)

func presentationFixture() Template {
	return Template{Name: "Generic nested report", HTML: `<section data-section="examen"><h2>Examinare</h2><p>Examen: {{examen}}</p><section data-section="componenta"><h3>Componentă</h3><p>Descriere: {{aspect}}; {{masura}}</p></section></section>`, Fields: []Field{{Key: "examen", Label: "Examen", Type: "string", Section: "examen"}, {Key: "aspect", Label: "Aspect", Type: "string", Section: "componenta"}, {Key: "masura", Label: "Măsurătoare documentată", Type: "number", Section: "componenta"}}, Sections: []Section{{ID: "examen", Label: "Examen"}, {ID: "componenta", Parent: "examen", Label: "Componentă", Variants: []SectionVariant{{ID: "normal_scurt", Label: "Normal scurt", Detail: "brief", ConditionField: "aspect", ConditionValue: "normal", Text: "Aspect {{aspect}}."}, {ID: "normal_detaliat", Label: "Normal detaliat", Detail: "detailed", ConditionField: "aspect", ConditionValue: "normal", Text: "Aspect {{aspect}}; măsurătoare documentată {{masura}}."}, {ID: "descriere", Label: "Constatare documentată", Detail: "detailed", ConditionField: "aspect", ConditionValue: "descris", Text: "Constatare: {{aspect}}."}}}}}
}
func entry(value any) map[string]any { return map[string]any{"value": value, "state": "present"} }
func TestGenericNestedPresentationEvidenceAndDetail(t *testing.T) {
	tpl := presentationFixture()
	if e := tpl.validate(); e != nil {
		t.Fatal(e)
	}
	data := map[string]any{"aspect": entry("normal"), "masura": entry(42)}
	tpl.Presentation = PresentationOptions{Detail: "brief"}
	doc, choices, _ := composeSections(tpl, data)
	if !strings.Contains(doc, `Aspect <span data-review-field="aspect">normal</span>.`) || strings.Contains(doc, "42") || choices[1].VariantID != "normal_scurt" {
		t.Fatal(doc, choices)
	}
	tpl.Presentation = PresentationOptions{Detail: "brief", Sections: map[string]string{"componenta": "detailed"}}
	doc, choices, _ = composeSections(tpl, data)
	if !strings.Contains(doc, "42") || choices[1].VariantID != "normal_detaliat" {
		t.Fatal(doc, choices)
	}
	tpl.Presentation = PresentationOptions{Sections: map[string]string{"examen": "detailed"}}
	if sectionDetail(tpl, "componenta") != "detailed" {
		t.Fatal("child did not inherit detail")
	}
	delete(data, "masura")
	doc, choices, _ = composeSections(tpl, data)
	if choices[1].VariantID != "" || strings.Contains(doc, "măsurătoare documentată") {
		t.Fatal("missing value invented", doc, choices)
	}
	data = map[string]any{}
	doc, choices, _ = composeSections(tpl, data)
	if choices[1].VariantID != "" || strings.Contains(doc, `Aspect <span data-review-field="aspect">normal</span>.`) {
		t.Fatal("absence treated as normal", doc)
	}
	data["aspect"] = map[string]any{"value": "normal", "state": "unknown"}
	_, choices, _ = composeSections(tpl, data)
	if choices[1].VariantID != "" {
		t.Fatal("unknown treated as normal")
	}
	// The parent changes its own body while preserving the nested section and its independent style.
	tpl.Sections[0].Variants = []SectionVariant{{ID: "intro", Label: "Introducere", Detail: "detailed", ConditionField: "examen", ConditionValue: "efectuat", Text: "Examen {{examen}}."}}
	data = map[string]any{"examen": entry("efectuat"), "aspect": entry("normal"), "masura": entry(42)}
	doc, _, _ = composeSections(tpl, data)
	if !strings.Contains(doc, `Examen <span data-review-field="examen">efectuat</span>.`) || !strings.Contains(doc, "42") || !strings.Contains(doc, `data-section="componenta"`) {
		t.Fatal("nested section lost", doc)
	}
	tpl.Sections[1].Variants = append(tpl.Sections[1].Variants, tpl.Sections[1].Variants[1])
	_, choices, warnings := composeSections(tpl, data)
	if choices[1].Reason != "ambiguous_variants" || len(warnings) != 1 {
		t.Fatal(choices, warnings)
	}
	tpl.Sections[1].Variants[0].ConditionField = "missing"
	if tpl.validate() == nil {
		t.Fatal("unknown variant condition accepted")
	}
}
func TestPostgresPresentationRevisionNoAIAndApproval(t *testing.T) {
	m, rt := integrationModule(t)
	password, _ := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	_, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(password)})
	token := out["token"].(string)
	tpl := presentationFixture()
	tpl.Specialty = "Test"
	code, out := request(t, m, token, "POST", "templates", tpl)
	if code != 201 {
		t.Fatal(code, out)
	}
	code, out = request(t, m, token, "POST", "sessions", map[string]any{"template_version": out["version_id"], "patient_ref": "SYNTHETIC-PRESENTATION"})
	if code != 201 {
		t.Fatal(code, out)
	}
	id := out["id"].(string)
	code, out = request(t, m, token, "POST", "sessions/"+id+"/transcript-chunks", map[string]any{"id": uuid.NewString(), "sequence": 1, "revision": 1, "is_final": true, "timestamp": time.Now().UTC(), "text": "aspect: normal\nmasura: 42\nexamen: efectuat"})
	if code != 200 {
		t.Fatal(code, out)
	}
	// A provider call here would fail (there is no key); presentation must reuse supported facts.
	m.cfg.Provider = "openai"
	m.cfg.OpenAIKeyEnv = "WAI_TEST_PRESENTATION_UNUSED"
	t.Setenv(m.cfg.OpenAIKeyEnv, "")
	code, out = request(t, m, token, "POST", "sessions/"+id+"/presentation", map[string]any{"revision": 1, "detail": "brief", "sections": map[string]string{"componenta": "detailed"}})
	if code != 200 || out["revision"] != float64(2) || !strings.Contains(out["result"].(map[string]any)["rendered_html"].(string), "42") {
		t.Fatal(code, out)
	}
	code, _ = request(t, m, token, "POST", "sessions/"+id+"/presentation", map[string]any{"revision": 1, "detail": "brief"})
	if code != 409 {
		t.Fatal("stale presentation accepted", code)
	}
	code, _ = request(t, m, token, "POST", "sessions/"+id+"/presentation", map[string]any{"revision": 2, "detail": "detailed", "sections": map[string]string{"missing": "brief"}})
	if code != 400 {
		t.Fatal(code)
	}
	code, out = request(t, m, token, "POST", "sessions/"+id+"/finalize", nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = request(t, m, token, "POST", "sessions/"+id+"/presentation", map[string]any{"revision": 3, "detail": "brief"})
	if code != 200 || out["status"] != "review" {
		t.Fatal(code, out)
	}
	code, _ = request(t, m, token, "POST", "sessions/"+id+"/approve", map[string]any{"revision": 3, "confirmed": true})
	if code != 409 {
		t.Fatal("old approval accepted", code)
	}
	code, out = request(t, m, token, "POST", "sessions/"+id+"/approve", map[string]any{"revision": 4, "confirmed": true})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, _ = request(t, m, token, "POST", "sessions/"+id+"/presentation", map[string]any{"revision": 4, "detail": "detailed"})
	if code != 409 {
		t.Fatal("approved report changed", code)
	}
	var calls int
	m.db.QueryRow(t.Context(), `SELECT count(*) FROM wai_usage_records`).Scan(&calls)
	if calls != 0 {
		t.Fatal("presentation made a billed call", calls)
	}
}

func TestVariantCannotExposeInvisibleField(t *testing.T) {
	tpl := presentationFixture()
	tpl.Fields[1].Invisible = true
	if tpl.validate() == nil {
		t.Fatal("variant exposed invisible field")
	}
}
