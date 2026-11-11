package wisemedai

import (
	"html"
	"strings"
)

func echocardiographyTemplate(spec catalogSpec) Template {
	t := Template{
		ModelVersion: 3, Name: spec.Name, Specialty: spec.Specialty, DefaultDetail: "standard",
		Rules: []Rule{}, Fragments: map[string]string{
			"normal":    `<p>Cavități de dimensiuni normale. Funcția sistolică și diastolică a VS păstrate. Funcția sistolică longitudinală a VD păstrată. Fără valvulopatii semnificative hemodinamic. Fără semne indirecte de hipertensiune pulmonară. Aortă în limite. Septuri intacte. Pericard liber.</p>`,
			"patologic": `<p>[Descrieți anomalia observată, segmentul sau valva, măsurătorile și severitatea documentată. Formulați concluzia pe baza examinării efectuate.]</p>`,
		},
		SemanticMapping: map[string]string{"concluzii_documentate": "encounter.assessment_documented"},
	}
	var doc strings.Builder
	doc.WriteString("<h2>Buletin ecocardiografic</h2><p><strong>Referință pacient</strong> {{patient_ref}} · <strong>Medic</strong> {{doctor_name}}</p>")
	section := func(id, label string, body func()) {
		t.Sections = append(t.Sections, Section{ID: id, Label: label})
		doc.WriteString(`<section data-section="` + id + `"><h3>` + html.EscapeString(label) + "</h3>")
		body()
		doc.WriteString("</section>")
	}
	field := func(id, key, label, unit, help string, aliases ...string) string {
		kind := "string"
		if unit != "" {
			kind = "number"
		}
		t.Fields = append(t.Fields, Field{Key: key, Label: label, Type: kind, Unit: unit, Section: id, Aliases: aliases, Help: help})
		value := "{{" + key + "}}"
		if unit != "" {
			value += " " + html.EscapeString(unit)
		}
		return value
	}
	row := func(tag string, cells ...string) {
		doc.WriteString("<tr>")
		for _, cell := range cells {
			doc.WriteString("<" + tag + ">" + cell + "</" + tag + ">")
		}
		doc.WriteString("</tr>")
	}
	section("identificare", "Identificarea examinării", func() {
		doc.WriteString("<p>Data " + field("identificare", "data_examinarii", "Data examinării", "", "", "data ecografiei") +
			" · Nume " + field("identificare", "nume_pacient", "Nume pacient", "", "", "numele pacientului") +
			" · Prenume " + field("identificare", "prenume_pacient", "Prenume pacient", "", "", "prenumele pacientului") + "</p>")
	})
	section("dimensiuni", "Dimensiuni cardiace și vasculare", func() {
		doc.WriteString("<p>Coloana de referință reproduce intervalele din modelul cabinetului.</p><table><thead>")
		row("th", "Parametru", "Măsurat", "Referință", "Parametru", "Măsurat", "Referință")
		doc.WriteString("</thead><tbody>")
		measure := func(key, label, help string, aliases ...string) string {
			return field("dimensiuni", key, label, "mm", help, aliases...)
		}
		row("td", "AO inel", measure("ao_inel", "Inel aortic", "", "AO inel", "inel AO"), "16–24 mm",
			"VS diastolă", measure("vs_diastola", "Diametru VS în diastolă", "", "VS diastolă", "VS diastola", "diametru telediastolic VS", "DTDVS"), "42–59 mm")
		row("td", "AO ascendentă", measure("ao_ascendenta", "Aortă ascendentă", "", "AO ascendentă", "AO ascendenta"), "25–36 mm",
			"VS sistolă", measure("vs_sistola", "Diametru VS în sistolă", "", "VS sistolă", "VS sistola", "diametru telesistolic VS", "DTSVS"), "21–40 mm")
		row("td", "Crosă aortică", measure("ao_crosa", "Crosă aortică", "", "AO crosa", "arc aortic"), "25–36 mm",
			"VD bazal", measure("vd_bazal", "Diametru bazal VD", "", "VD bazal", "ventricul drept bazal"), "27–33 mm")
		row("td", "AO descendentă", measure("ao_descendenta", "Aortă descendentă", "", "AO descendentă", "AO descendenta"), "20–30 mm",
			"AD", measure("ad", "Atriu drept", "", "AD", "diametru atriu drept"), "29–45 mm")
		row("td", "AO abdominală", measure("ao_abdominala", "Aortă abdominală", "", "AO abdominală", "AO abdominala"), "20–28 mm",
			"AP inel", measure("ap_inel", "Inel pulmonar", "", "AP inel", "inel arteră pulmonară"), "16–24 mm")
		row("td", "AS", field("dimensiuni", "as", "Atriu stâng · dimensiuni (mm)", "", "Păstrează ambele măsurători dacă sunt dictate.", "AS", "atriu stâng", "atriu stang"), "25–40 mm",
			"AP trunchi", measure("ap_trunchi", "Trunchi arteră pulmonară", "", "AP trunchi", "trunchi pulmonar"), "15–25 mm")
		row("td", "SIV", measure("siv", "Sept interventricular", "SIV și sept interventricular desemnează același câmp.", "SIV", "sept interventricular", "grosime sept interventricular"), "7–10 mm",
			"VCI", measure("vci", "Venă cavă inferioară", "Diametrul și colapsul inspirator se completează separat.", "VCI", "vena cava inferioara", "vena cavă inferioară"), "12–20 mm")
		row("td", "PPVS", measure("ppvs", "Perete posterior al VS", "", "PPVS", "perete posterior al ventriculului stâng", "perete posterior al ventriculului stang", "perete posterior VS"), "7–10 mm",
			"Colaps inspirator", field("dimensiuni", "vci_colaps", "Colaps inspirator VCI", "", "Consemnează prezent sau absent dacă este dictat.", "CI", "colaps inspirator", "colaps VCI"), "—")
		doc.WriteString("</tbody></table>")
	})
	section("functie_vs", "Funcția ventriculului stâng", func() {
		doc.WriteString("<table><thead>")
		row("th", "Parametru", "Valoare măsurată / calculată", "Valoare estimată", "Referință din model")
		doc.WriteString("</thead><tbody>")
		row("td", "FS", field("functie_vs", "fs", "Fracție de scurtare", "%", "", "FS", "fracție de scurtare", "fractie scurtare"), "—", "25–45%")
		row("td", "FEVS", field("functie_vs", "fevs", "Fracție de ejecție VS calculată", "%", "", "FEVS", "FE calculată", "fracție de ejecție a ventriculului stâng", "fractie de ejectie", "fracție de ejecție"),
			field("functie_vs", "fevs_estimata", "FEVS estimată", "%", "", "FEVS estimată", "fracție de ejecție estimată"), "50–70%")
		for _, item := range []struct {
			key, label, unit, help string
			aliases                []string
		}{
			{"unda_e", "Unda E mitrală", "m/s", "", []string{"E", "unda E", "E mitral"}},
			{"unda_a", "Unda A mitrală", "m/s", "", []string{"A", "unda A", "A mitral"}},
			{"tde", "Timpul de decelerare al undei E", "ms", "", []string{"TDE", "timp de decelerare E"}},
			{"vtdvs_4c", "VTDVS apical 4 camere", "mL", "", []string{"AP 4 CAM VTDVS", "VTDVS 4 camere"}},
			{"vtdvs_2c", "VTDVS apical 2 camere", "mL", "", []string{"AP 2 CAM VTDVS", "VTDVS 2 camere"}},
			{"vtsvs_4c", "VTSVS apical 4 camere", "mL", "", []string{"AP 4 CAM VTSVS", "VTSVS 4 camere"}},
			{"vtsvs_2c", "VTSVS apical 2 camere", "mL", "", []string{"AP 2 CAM VTSVS", "VTSVS 2 camere"}},
		} {
			row("td", item.label, field("functie_vs", item.key, item.label, item.unit, item.help, item.aliases...), "—", "—")
		}
		doc.WriteString("</tbody></table>")
	})
	section("tdi", "Doppler tisular (TDI)", func() {
		doc.WriteString("<table><thead>")
		row("th", "Undă", "Mitral septal", "Mitral lateral", "Tricuspidian lateral")
		doc.WriteString("</thead><tbody>")
		for _, wave := range []string{"S", "E", "A"} {
			cells := []string{wave + "′"}
			for _, site := range []struct{ key, label string }{{"mitral_septal", "mitral septal"}, {"mitral_lateral", "mitral lateral"}, {"tricuspidian_lateral", "tricuspidian lateral"}} {
				key := "tdi_" + strings.ToLower(wave) + "_" + site.key
				cells = append(cells, field("tdi", key, wave+"′ "+site.label, "cm/s", "Indică explicit unda și sediul măsurătorii.", wave+"′ "+site.label, wave+"' "+site.label, wave+" prim "+site.label))
			}
			row("td", cells...)
		}
		doc.WriteString("</tbody></table>")
	})
	section("valve", "Evaluare valvulară", func() {
		doc.WriteString("<table><thead>")
		row("th", "Parametru", "Mitrală", "Aortică", "Tricuspidiană", "Pulmonară")
		doc.WriteString("</thead><tbody>")
		for _, parameter := range []struct{ key, label, alias, unit, help string }{
			{"vmax", "Vmax", "viteză maximă", "m/s", ""},
			{"gmax", "Gradient maxim", "Gmax", "mmHg", ""},
			{"gmed", "Gradient mediu", "Gmed", "mmHg", ""},
			{"arie", "Arie valvulară", "arie", "cm²", ""},
			{"regurgitare", "Regurgitare", "insuficiență", "", "Descrie prezența și gradul dictat: ușoară, moderată sau severă."},
			{"morfologie", "Morfologie", "aspect", "", "Descriere liberă a morfologiei observate."},
		} {
			cells := []string{parameter.label}
			for _, valve := range []struct{ key, label string }{{"mitrala", "mitrală"}, {"aortica", "aortică"}, {"tricuspidiana", "tricuspidiană"}, {"pulmonara", "pulmonară"}} {
				cells = append(cells, field("valve", valve.key+"_"+parameter.key, parameter.label+" · valva "+valve.label, parameter.unit, parameter.help, parameter.label+" "+valve.label, parameter.alias+" "+valve.label))
			}
			row("td", cells...)
		}
		doc.WriteString("</tbody></table><p><strong>TAP pulmonar</strong> " + field("valve", "tap", "Timp de accelerare pulmonară", "ms", "", "TAP", "timp de accelerare pulmonară") + "</p>")
	})
	section("alte_constatari", "Funcția VD și alte constatări", func() {
		doc.WriteString("<p><strong>TAPSE</strong> " + field("alte_constatari", "tapse", "TAPSE", "mm", "Excursia sistolică a planului inelului tricuspidian.", "excursia sistolică a planului inelului tricuspidian", "excursia sistolica a planului inelului tricuspidian") + "</p>")
		doc.WriteString("<p><strong>PAPs</strong> " + field("alte_constatari", "paps", "PAPs", "mmHg", "Presiunea sistolică în artera pulmonară.", "presiunea sistolică în artera pulmonară", "presiunea sistolica in artera pulmonara", "PSAP") + "</p>")
		doc.WriteString("<p><strong>Cinetică parietală</strong> " + field("alte_constatari", "cinetica", "Cinetică parietală", "", "Consemnează aspectul și segmentul documentat. În model apar păstrată, hipokinezie, akinezie sau diskinezie.", "cinetica parietala", "cinetică", "mobilitate parietală") + "</p>")
		doc.WriteString("<p><strong>Pericard</strong> " + field("alte_constatari", "pericard", "Pericard", "", "Descriere liberă a aspectului pericardului.", "aspect pericard", "pericard liber", "revărsat pericardic") + "</p>")
	})
	section("concluzii", "Concluzii", func() {
		doc.WriteString("<p>" + field("concluzii", "concluzii_documentate", "Concluzii ecocardiografice", "", "Formulează sinteza examinării. Folosește fragmentul normal numai pentru constatările confirmate.", "concluzii", "concluzie ecocardiografică") + "</p>")
	})
	t.HTML = doc.String()
	return t
}
