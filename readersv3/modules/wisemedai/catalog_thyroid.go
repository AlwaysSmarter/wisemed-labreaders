package wisemedai

import (
	"html"
	"strings"
)

func thyroidUltrasoundTemplate(spec catalogSpec) Template {
	t := Template{
		ModelVersion:  4,
		Name:          spec.Name,
		Specialty:     spec.Specialty,
		Specialties:   []string{spec.Specialty, "Endocrinologie"},
		DefaultDetail: "standard",
		Rules:         []Rule{},
		Fragments: map[string]string{
			"normal":    `<p>Capsulă hiperecogenă, integră, cu contur regulat. Parenchim omogen. Vascularizație tiroidiană în limite normale. Fără leziuni focale tiroidiene sau adenopatii laterocervicale la examinare.</p>`,
			"patologic": `<p><strong>Modificări difuze</strong> · [lob și segment, ecogenitate, omogenitate, microplaje, striuri fibroase sau aspect pseudonodular].</p><p><strong>Leziune focală</strong> · [localizare, dimensiuni în cm, structură, ecogenitate, delimitare, halou, calcificări, vascularizație intra-/perinodulară].</p><p><strong>Ganglioni cervicali</strong> · [lateralitate, dimensiuni și aspect documentat].</p>`,
		},
		SemanticMapping: map[string]string{
			"concluzii_documentate": "encounter.assessment_documented",
			"plan_documentat":       "encounter.plan_proposed",
		},
	}
	groups := []struct {
		id, label string
		fields    []Field
	}{

		{"tiroida", "Capsulă și aspect general", []Field{
			{Key: "capsula", Label: "Capsulă și contur", Aliases: []string{"capsulă", "capsula", "contur"}},
			{Key: "parenchim_normal", Label: "Aspect parenchimatos normal documentat"},
			{Key: "absenta_leziunilor", Label: "Absența leziunilor focale, dacă este consemnată"},
		}},
		{"lob_drept", "Lob drept", []Field{
			{Key: "lob_drept", Label: "Lob drept · dimensiuni (cm)", Aliases: []string{"lob drept", "dimensiuni lob drept", "lob tiroidian drept"}},
			{Key: "ecostructura_dreapta", Label: "Lob drept · ecogenitate și ecostructură", Aliases: []string{"ecostructură lob drept", "parenchim lob drept"}},
			{Key: "noduli_dreapta", Label: "Lob drept · leziuni focale", Repeatable: true, Aliases: []string{"noduli lob drept", "formațiune nodulară dreaptă"}},
		}},
		{"istm", "Istm și joncțiuni istmico-lobare", []Field{
			{Key: "istm", Label: "Istm · grosime", Type: "number", Unit: "cm", Aliases: []string{"istm", "grosime istm", "istm tiroidian"}},
			{Key: "modificari_istm", Label: "Istm · modificări de ecostructură"},
			{Key: "noduli_istm", Label: "Istm și joncțiuni istmico-lobare · leziuni focale", Repeatable: true, Aliases: []string{"nodul istmic", "istmico-lobar stâng", "istmico-lobar drept", "istmico-lobar stg"}},
		}},
		{"lob_stang", "Lob stâng", []Field{
			{Key: "lob_stang", Label: "Lob stâng · dimensiuni (cm)", Aliases: []string{"lob stâng", "lob sting", "dimensiuni lob stâng", "lob tiroidian stâng"}},
			{Key: "ecostructura_stanga", Label: "Lob stâng · ecogenitate și ecostructură", Aliases: []string{"ecostructură lob stâng", "parenchim lob stâng"}},
			{Key: "noduli_stanga", Label: "Lob stâng · leziuni focale", Repeatable: true, Aliases: []string{"noduli lob stâng", "formațiune microchistică", "formațiune nodulară stângă"}},
		}},
		{"vascularizatie", "Vascularizație tiroidiană", []Field{
			{Key: "vascularizatie_normala", Label: "Vascularizație în limite normale"},
			{Key: "vascularizatie", Label: "Vascularizație Doppler modificată", Aliases: []string{"vascularizație tiroidiană accentuată", "hipervascularizație"}},
		}},
		{"ganglioni", "Ganglioni laterocervicali", []Field{
			{Key: "absenta_adenopatiilor", Label: "Absența adenopatiilor laterocervicale, dacă este consemnată", Aliases: []string{"fără adenopatie laterocervicală", "fără adenopatii"}},
			{Key: "ganglioni", Label: "Ganglioni laterocervicali · localizare și aspect", Repeatable: true, Aliases: []string{"adenopatie", "micropoliadenopatie", "ganglioni cervicali"}},
		}},
		{"sinteza", "Concluzii și recomandări", []Field{
			{Key: "clasificare_documentata", Label: "Clasificare consemnată de medic"},
			{Key: "concluzii_documentate", Label: "Concluzii formulate de medic"},
			{Key: "plan_documentat", Label: "Recomandări consemnate de medic"},
		}},
	}
	configureThyroidContext(&t)
	var doc strings.Builder
	doc.WriteString("<h2>Buletin de ecografie tiroidiană</h2><p><strong>Referință pacient</strong> {{patient_ref}} · <strong>Medic</strong> {{doctor_name}}</p>")
	for _, group := range groups {
		t.Sections = append(t.Sections, Section{ID: group.id, Label: group.label, HideWhenEmpty: true, SpeechTerms: thyroidStructureTerms(group.id)})
		doc.WriteString(`<section data-section="` + group.id + `"><h3>` + html.EscapeString(group.label) + "</h3>")
		for _, f := range group.fields {
			f.Section = "camp_" + f.Key
			t.Sections = append(t.Sections, Section{ID: f.Section, Label: f.Label, Parent: group.id, HideWhenEmpty: true, SpeechTerms: thyroidFieldTerms(f.Key)})
			if f.Type == "" {
				f.Type = "string"
			}
			f.Help = "Folosește numai datele dictate pentru această structură. Păstrează lateralitatea, unitățile, negațiile și calificativele; nu copia valorile din exemple."
			t.Fields = append(t.Fields, f)
			doc.WriteString(`<section data-section="` + f.Section + `"><p><strong>` + html.EscapeString(f.Label) + "</strong> {{" + f.Key + "}}")
			if f.Unit != "" {
				doc.WriteString(" " + f.Unit)
			}
			doc.WriteString("</p></section>")
		}
		doc.WriteString("</section>")
	}
	t.HTML = doc.String()
	return t
}
