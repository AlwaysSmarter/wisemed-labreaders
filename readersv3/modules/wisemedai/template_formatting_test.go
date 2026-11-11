package wisemedai

import (
	"strings"
	"testing"
	"time"
)

func TestTemplateFormattingSurvivesSanitizeAndRender(t *testing.T) {
	tpl := testTemplate()
	tpl.HTML = `<section data-section="descriere"><h2 style="text-align: center; position:fixed">Raport</h2><p style="text-align: justify"><u><s>{{ficat}}</s></u></p><hr><table><tr><th colspan="2">Organ</th></tr><tr><td rowspan="2"><span data-field="dimensiune">Dimensiune</span></td><td>mm</td></tr><tr><td>valoare</td></tr></table><ol start="3"><li><p>Control</p></li></ol></section>`
	tpl.Sections = []Section{{ID: "descriere"}}
	if err := tpl.validate(); err != nil {
		t.Fatal(err)
	}
	chunks := []Chunk{{ID: "c1", Text: "ficat: omogen\ndimensiune: 140", Timestamp: time.Now()}}
	result, err := buildResult(tpl, chunks, mockExtract(tpl, chunks), "session", 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-section="descriere"`, `style="text-align: center"`, `style="text-align: justify"`, `<u><s><span data-review-field="ficat">omogen</span></s></u>`, `<hr>`, `colspan="2"`, `rowspan="2"`, `start="3"`, `140`} {
		if !strings.Contains(result.HTML, want) {
			t.Errorf("missing %s in %s", want, result.HTML)
		}
	}
	if strings.Contains(result.HTML, "position") {
		t.Fatal("arbitrary CSS persisted")
	}
}

func TestTemplateFormattingRejectsUnsafeAttributes(t *testing.T) {
	src := `<p style="text-align: expression(alert(1));background:url(javascript:bad)">Text</p><h2 style="TEXT-ALIGN: RIGHT; color: red" onclick="bad()">Titlu</h2><table><tr><td colspan="0" rowspan="101" style="text-align:center" onmouseover="bad()">A</td><td colspan="2 onclick=bad()">B</td></tr></table><span style="text-align: center" data-field="ficat">Ficat</span>`
	got := sanitizeHTML(src)
	for _, bad := range []string{"expression", "background", "javascript", "color:", "onclick", "onmouseover", "colspan=", "rowspan=", `<span style=`, `<td style=`} {
		if strings.Contains(got, bad) {
			t.Errorf("unsafe %s in %s", bad, got)
		}
	}
	if !strings.Contains(got, `style="text-align: right"`) || !strings.Contains(got, `data-field="ficat"`) {
		t.Fatal(got)
	}
}
