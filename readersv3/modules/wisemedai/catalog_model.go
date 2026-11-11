package wisemedai

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"github.com/jackc/pgx/v5"
)

func adaptCatalogTemplate(t Template) Template {
	t.ModelVersion = 2
	t.DefaultDetail = "standard"
	roots := []Section{{ID: "istoric", Label: "Istoric și context"}, {ID: "examen", Label: "Examinare și constatări"}, {ID: "sinteza", Label: "Concluzii și recomandări documentate"}}
	// Keep existing keys/types/units and clinical provenance; add optional consultation topics.
	if strings.Contains(t.Name, "Consultație") || t.Name == "Vizită zilnică" {
		for _, pair := range [][2]string{{"antecedente_personale", "Antecedente personale"}, {"antecedente_familiale", "Antecedente heredocolaterale"}, {"tratament_anterior", "Tratament anterior declarat"}, {"boli_cronice", "Boli cronice documentate"}, {"diagnostic_final", "Diagnostic formulat de medic"}, {"recomandari_documentate", "Recomandări formulate de medic"}, {"reteta_documentata", "Rețetă dictată de medic"}} {
			found := false
			for _, f := range t.Fields {
				found = found || f.Key == pair[0]
			}
			if !found {
				t.Fields = append(t.Fields, Field{Key: pair[0], Label: pair[1], Type: "string"})
			}
		}
	}
	if t.Specialty == "Cardiologie" {
		roots = append(roots, Section{ID: "ecocardiografie", Label: "Ecocardiografie documentată", Parent: "examen"})
		for _, prefix := range []struct{ key, label string }{{"vs", "Ventricul stâng"}, {"vd", "Ventricul drept"}} {
			for _, part := range []struct{ key, label string }{{"aspect", "Aspect"}, {"dimensiuni", "Dimensiuni documentate"}, {"functie", "Funcție documentată"}} {
				t.Fields = append(t.Fields, Field{Key: prefix.key + "_" + part.key, Label: prefix.label + " · " + part.label, Type: "string", Section: prefix.key})
			}
		}
	}
	groups := map[string][]int{}
	labels := map[string]string{}
	parents := map[string]string{}
	order := []string{}
	for i := range t.Fields {
		f := &t.Fields[i]
		parent := "examen"
		switch {
		case f.Section == "istoric" || strings.Contains(f.Key, "antecedent") || strings.Contains(f.Key, "istoric") || f.Key == "anamneza" || f.Key == "evolutie" || f.Key == "tratament_anterior" || f.Key == "boli_cronice":
			parent = "istoric"
		case f.Section == "sinteza" || strings.Contains(f.Key, "diagnostic") || strings.Contains(f.Key, "concluz") || strings.Contains(f.Key, "recomand") || strings.Contains(f.Key, "reteta") || f.Key == "plan" || f.Key == "plan_documentat":
			parent = "sinteza"
		}
		group := fmt.Sprintf("componenta_%02d", i)
		label := f.Label
		if f.Key == "ficat" || f.Key == "ficat_mm" {
			group = "ficat"
			label = "Ficat"
		}
		if strings.HasPrefix(f.Key, "vs_") {
			group = "ventricul_stang"
			label = "Ventricul stâng"
			parent = "ecocardiografie"
		}
		if strings.HasPrefix(f.Key, "vd_") {
			group = "ventricul_drept"
			label = "Ventricul drept"
			parent = "ecocardiografie"
		}
		if _, ok := groups[group]; !ok {
			order = append(order, group)
		}
		groups[group] = append(groups[group], i)
		labels[group] = label
		parents[group] = parent
		f.Section = group
	}
	for i := range roots {
		roots[i].HideWhenEmpty = true
	}
	t.Sections = roots
	for _, group := range order {
		indices := groups[group]
		primary := t.Fields[indices[0]]
		section := Section{HideWhenEmpty: true, ID: group, Label: labels[group], Parent: parents[group]}
		line := func(f Field) string {
			unit := ""
			if f.Unit != "" {
				unit = " " + f.Unit
			}
			return f.Label + ": {{" + f.Key + "}}" + unit + "."
		}
		detailed := []string{}
		for _, i := range indices {
			detailed = append(detailed, line(t.Fields[i]))
		}
		for _, level := range []struct{ id, text string }{{"brief", "{{" + primary.Key + "}}" + func() string {
			if primary.Unit != "" {
				return " " + primary.Unit
			}
			return ""
		}()}, {"standard", strings.Join(detailed, "\n")}, {"detailed", strings.Join(detailed, "\n")}} {
			section.Variants = append(section.Variants, SectionVariant{ID: "descris_" + level.id, Label: map[string]string{"brief": "Descriere scurtă", "standard": "Descriere standard", "detailed": "Descriere detaliată"}[level.id], Detail: level.id, ConditionOperator: "documented", ConditionField: primary.Key, Text: level.text})
			if primary.Type == "string" {
				section.Variants = append(section.Variants, SectionVariant{ID: "normal_" + level.id, Label: "Normal explicit · " + level.id, Detail: level.id, ConditionField: primary.Key, ConditionValue: "normal", Text: level.text})
			}
		}
		t.Sections = append(t.Sections, section)
	}
	var render func(string) string
	render = func(parent string) string {
		var b strings.Builder
		for _, s := range t.Sections {
			if s.Parent != parent {
				continue
			}
			b.WriteString(`<section data-section="` + s.ID + `"><h3>` + html.EscapeString(s.Label) + `</h3>`)
			for _, f := range t.Fields {
				if f.Section == s.ID {
					b.WriteString("<p>" + html.EscapeString(f.Label) + ": {{" + f.Key + "}}" + func() string {
						if f.Unit != "" {
							return " " + html.EscapeString(f.Unit)
						}
						return ""
					}() + "</p>")
				}
			}
			b.WriteString(render(s.ID))
			b.WriteString("</section>")
		}
		return b.String()
	}
	t.HTML = "<h2>" + html.EscapeString(t.Name) + "</h2><p>Referință pacient: {{patient_ref}} · Medic: {{doctor_name}}</p>" + render("")
	return t
}

// Upgrade only untouched shared examples. Existing immutable versions and user publications remain intact.
func (m *Module) upgradeCatalogModel(ctx context.Context, tx pgx.Tx, tenant string) error {
	for _, example := range Catalog() {
		var id, version string
		var raw []byte
		e := tx.QueryRow(ctx, `SELECT t.id::text,t.current_version::text,v.document FROM wai_templates t JOIN wai_template_versions v ON v.id=t.current_version WHERE t.tenant_id=$1 AND t.name=$2 AND t.owner_id IS NULL AND NOT EXISTS(SELECT 1 FROM wai_audit_events a WHERE a.tenant_id=t.tenant_id AND a.action='template.publish' AND a.entity_id=t.current_version::text) ORDER BY t.created_at LIMIT 1 FOR UPDATE OF t`, tenant, example.Name).Scan(&id, &version, &raw)
		if e == pgx.ErrNoRows {
			continue
		}
		if e != nil {
			return e
		}
		var current Template
		if e = json.Unmarshal(raw, &current); e != nil {
			return e
		}
		if current.ModelVersion >= example.ModelVersion {
			continue
		}
		example.ID = id
		if e = m.saveCatalogTemplate(ctx, tx, tenant, &example); e != nil {
			return e
		}
	}
	return nil
}
