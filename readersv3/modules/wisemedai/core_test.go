package wisemedai

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testTemplate() Template {
	return Template{Name: "Ecografie test", VersionID: "v1", HTML: `<p>Ficat: {{ficat}}</p><p><span data-field="dimensiune">Dimensiune</span></p><section data-section="leziuni"><p>Leziuni</p></section>`, Fields: []Field{{Key: "ficat", Label: "Ficat", Type: "string", Required: true}, {Key: "dimensiune", Label: "Dimensiune", Type: "number", Min: num(20), Max: num(400)}}, Sections: []Section{{ID: "leziuni", ConditionField: "ficat", ConditionValue: "leziune"}}}
}
func TestSanitizeAndRender(t *testing.T) {
	src := `<p onclick="alert(1)">OK</p><script>alert(2)</script><svg onload="alert(1)"><script>x</script></svg><iframe src="https://evil.test"></iframe><a href="javascript:evil()">link</a><span data-field="ficat" style="color:red">Ficat</span>`
	got := sanitizeHTML(src)
	for _, bad := range []string{"onclick", "alert", "<script", "<svg", "iframe", "javascript:", "style="} {
		if strings.Contains(got, bad) {
			t.Fatalf("unsafe %s in %s", bad, got)
		}
	}
	tpl := testTemplate()
	chunks := []Chunk{{ID: "c1", Text: "ficat: omogen\ndimensiune: 140", Timestamp: time.Now()}}
	r, e := buildResult(tpl, chunks, mockExtract(tpl, chunks), "s", 1)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(r.HTML, "Leziuni") || !strings.Contains(r.HTML, "140") || !strings.Contains(r.HTML, "omogen") || len(r.Missing) > 0 {
		t.Fatal(r)
	}
}
func TestRejectInventedEvidence(t *testing.T) {
	tpl := testTemplate()
	chunks := []Chunk{{ID: "c1", Text: "ficat: omogen"}}
	for _, f := range []Fact{
		{Field: "ficat", Value: "normal", State: "present", ChunkID: "c1", Quote: "ficat: omogen"},
		{Field: "ficat", Value: "omogen", State: "present", ChunkID: "other", Quote: "ficat: omogen"},
		{Field: "unknown", Value: "omogen", State: "present", ChunkID: "c1", Quote: "ficat: omogen"},
	} {
		result, e := buildResult(tpl, chunks, []Fact{f}, "s", 1)
		if e != nil || len(result.FieldIssues) != 1 || len(result.CanonicalData) != 0 {
			t.Fatal("unsupported fact did not produce a local issue", result, e)
		}
		if f.Field != "unknown" && result.Data[f.Field].(map[string]any)["state"] != "needs_review" {
			t.Fatal("unsupported fact treated as accepted", result)
		}
	}
}
func TestMedicalDisplayPreservesEvidenceAndMeasurements(t *testing.T) {
	tpl := testTemplate()
	raw := "lob drept 1,57 pe per 1,13 3,57 cm, cu mici stiruri fibroase"
	corrected := "Lob drept de 1,57 × 1,13 × 3,57 cm, cu mici striuri fibroase."
	chunks := []Chunk{{ID: "c1", Text: raw}}
	fact := Fact{Field: "ficat", Value: raw, State: "present", ChunkID: "c1", Quote: raw, DisplayValue: corrected}
	result, err := buildResult(tpl, chunks, []Fact{fact}, "s", 1)
	if err != nil || !strings.Contains(result.HTML, corrected) || result.Evidence[0].Quote != raw || result.Evidence[0].Value != raw {
		t.Fatalf("corrected text or literal evidence missing: %#v, %v", result, err)
	}
	for _, invalid := range []string{
		strings.Replace(corrected, "3,57", "3,75", 1),
		strings.Replace(corrected, "cm", "mm", 1),
		strings.Replace(corrected, "drept", "stâng", 1),
		strings.Replace(corrected, "cu", "fără", 1),
	} {
		fact.DisplayValue = invalid
		result, err := buildResult(tpl, chunks, []Fact{fact}, "s", 1)
		if err != nil || !strings.Contains(result.HTML, raw) || !result.Evidence[0].NeedsReview || result.Evidence[0].DisplayValue != "" {
			t.Fatalf("unsafe rewrite was not discarded: %#v, %v", result, err)
		}
	}
	if literalSupported("Este 0-2 cm", "0") || literalSupported("Este 0-2 cm", "2") {
		t.Fatal("ambiguous range accepted as an exact scalar")
	}
}

func TestMissingIsNotNormal(t *testing.T) {
	tpl := testTemplate()
	r, e := buildResult(tpl, nil, nil, "s", 1)
	if e != nil || len(r.Missing) != 1 || len(r.Data) != 0 || strings.Contains(r.HTML, "normal") {
		t.Fatal(r, e)
	}
}
func TestRangeAndRepeatedFacts(t *testing.T) {
	tpl := testTemplate()
	tpl.Fields = append(tpl.Fields, Field{Key: "leziune", Label: "Leziune", Type: "string", Repeatable: true})
	c := []Chunk{{ID: "c", Text: "dimensiune: 500\nleziune: una\nleziune: alta"}}
	r, e := buildResult(tpl, c, mockExtract(tpl, c), "s", 1)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Warnings) != 2 || len(r.Data["leziune"].([]any)) != 2 {
		t.Fatal(r)
	}
}
func TestTemplateValidation(t *testing.T) {
	tpl := testTemplate()
	if e := tpl.validate(); e != nil {
		t.Fatal(e)
	}
	tpl.Fields = append(tpl.Fields, tpl.Fields[0])
	if e := tpl.validate(); e == nil {
		t.Fatal("duplicate accepted")
	}
	tpl = testTemplate()
	tpl.Sections = []Section{{ID: "a", Parent: "b"}, {ID: "b", Parent: "a"}}
	if e := tpl.validate(); e == nil {
		t.Fatal("cycle accepted")
	}
}
func TestEncryptionScope(t *testing.T) {
	box, e := loadBox(t.TempDir() + "/key")
	if e != nil {
		t.Fatal(e)
	}
	raw, e := box.seal(map[string]string{"text": "synthetic"}, "tenant-a")
	if e != nil {
		t.Fatal(e)
	}
	var v map[string]string
	if box.open(raw, "tenant-b", &v) == nil {
		t.Fatal("cross-tenant decrypt accepted")
	}
	if e = box.open(raw, "tenant-a", &v); e != nil || v["text"] != "synthetic" {
		t.Fatal(v, e)
	}
}
func TestConnectorAllowlist(t *testing.T) {
	for _, u := range []string{"http://his.test/api", "https://evil.test/api", "https://user:pw@his.test/api", "file:///etc/passwd"} {
		if validateConnector(map[string]any{"url": u, "token": "x"}, []string{"his.test"}) == nil {
			t.Fatal("unsafe connector", u)
		}
	}
	if e := validateConnector(map[string]any{"url": "https://his.test/api", "token": "x"}, []string{"his.test"}); e != nil {
		t.Fatal(e)
	}
}
func TestRedactNestedSecrets(t *testing.T) {
	v := redact(map[string]any{"token": "secret", "nested": map[string]any{"api_key": "secret"}, "name": "x"})
	if strings.Contains(strings.TrimSpace(toJSON(v)), "secret\"") {
		t.Fatal(v)
	}
}

func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestCommonSpecialtyCatalog(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range specialtyCatalog {
		if seen[s.Slug] {
			t.Fatal("duplicate specialty", s.Slug)
		}
		seen[s.Slug] = true
		tpl := catalogTemplate(s)
		if e := tpl.validate(); e != nil {
			t.Fatal(s.Slug, e)
		}
		if len(tpl.Fields) < 9 {
			t.Fatal("incomplete template", s.Slug)
		}
		for _, f := range tpl.Fields {
			if !strings.Contains(tpl.HTML, "{{"+f.Key+"}}") && !f.Invisible {
				t.Fatal("missing visible field", s.Slug, f.Key)
			}
		}
	}
	for _, slug := range []string{"neurologie", "cardiologie", "medicina_interna", "pediatrie", "ginecologie", "urgenta"} {
		if !seen[slug] {
			t.Fatal("missing requested specialty", slug)
		}
	}
	if len(seen) < 40 {
		t.Fatal("insufficient specialty coverage", len(seen))
	}
}

func TestPairingQRIsOpaque(t *testing.T) {
	qr := pairingQR(strings.Repeat("a", 64))
	if !strings.HasPrefix(qr, "data:image/png;base64,") {
		t.Fatal("missing QR PNG")
	}
}

func TestLiteralAndNegationSafety(t *testing.T) {
	for _, tc := range []struct{ quote, value string }{{"ficat: anormal", "normal"}, {"dimensiune: 140", "14"}} {
		if literalSupported(tc.quote, tc.value) {
			t.Fatal("partial clinical literal accepted", tc)
		}
	}
	if stateSupported(Fact{State: "present", Quote: "fără dilatații", Value: "dilatații"}) {
		t.Fatal("negation lost")
	}
}

func TestTechnicalMappingCannotInventDefaults(t *testing.T) {
	r := Result{SessionID: "s", Revision: 3, Data: map[string]any{"ficat": map[string]any{"value": "omogen", "state": "present"}}}
	mapped := technicalPayload(r, map[string]any{"liver": "ficat", "spleen": "missing"})
	data := mapped["mapped_data"].(map[string]any)
	if len(data) != 1 || data["liver"] == nil {
		t.Fatal(mapped)
	}
}
func TestInactiveConditionalRequiredField(t *testing.T) {
	tpl := testTemplate()
	tpl.Fields = append(tpl.Fields, Field{Key: "detaliu", Label: "Detaliu", Type: "string", Required: true, Section: "leziuni"})
	r, e := buildResult(tpl, nil, nil, "s", 1)
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range r.Missing {
		if f == "detaliu" {
			t.Fatal("inactive required field", r)
		}
	}
}

func TestVectorValidation(t *testing.T) {
	v := make([]float64, 1536)
	if _, e := vectorLiteral(v); e == nil {
		t.Fatal("zero cosine embedding accepted")
	}
	v[0] = 1
	if _, e := vectorLiteral(v); e != nil {
		t.Fatal(e)
	}
	if _, e := vectorLiteral(v[:10]); e == nil {
		t.Fatal("wrong embedding dimensions")
	}
}
