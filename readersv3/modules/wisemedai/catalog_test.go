package wisemedai

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestPostgresCatalogSeedIsIdempotent(t *testing.T) {
	m, _ := integrationModule(t)
	ctx := context.Background()
	var templates, markers, specialties int
	for q, out := range map[string]*int{`SELECT count(*) FROM wai_templates`: &templates, `SELECT count(*) FROM wai_seed_catalog`: &markers, `SELECT count(DISTINCT specialty) FROM wai_templates`: &specialties} {
		if e := m.db.QueryRow(ctx, q).Scan(out); e != nil {
			t.Fatal(e)
		}
	}
	if templates != len(specialtyCatalog)+3 || markers != len(specialtyCatalog) || specialties < 35 {
		t.Fatal("catalog coverage", templates, markers, specialties)
	}
	t.Logf("catalog: %d templates, %d specialties", templates, specialties)
}

func TestAllCatalogExamplesUseGenericModel(t *testing.T) {
	for _, tpl := range Catalog() {
		if tpl.ModelVersion < 2 || len(tpl.Sections) < 4 {
			t.Fatal(tpl.Name, "not adapted")
		}
		if e := tpl.validate(); e != nil {
			t.Fatal(tpl.Name, e)
		}
		for _, f := range tpl.Fields {
			if f.Section == "" {
				t.Fatal(tpl.Name, f.Key, "unassigned field")
			}
		}
		for _, s := range tpl.Sections {
			if tpl.ModelVersion == 2 && s.Parent != "" && len(s.Variants) == 0 && s.ID != "ecocardiografie" {
				t.Fatal(tpl.Name, s.ID, "no wording variants")
			}
		}
		empty, e := buildResult(tpl, nil, nil, "synthetic", 0)
		if e != nil {
			t.Fatal(e)
		}
		for _, f := range tpl.Fields {
			if f.Required {
				found := false
				for _, key := range empty.Missing {
					found = found || key == f.Key
				}
				if !found {
					t.Fatal("hidden section masked required field", tpl.Name, f.Key)
				}
			}
		}
	}
}
func TestPostgresCatalogModelUpgradePreservesVersionsAndEdits(t *testing.T) {
	m, _ := integrationModule(t)
	ctx := context.Background()
	var tenant, user, id string
	m.db.QueryRow(ctx, `SELECT tenant_id::text,user_id::text FROM wai_memberships LIMIT 1`).Scan(&tenant, &user)
	m.db.QueryRow(ctx, `SELECT id::text FROM wai_templates WHERE name='Ecografie abdominală'`).Scan(&id)
	tx, _ := m.db.Begin(ctx)
	legacy := rawBaseTemplates()[0]
	legacy.ID = id
	if e := saveTemplate(ctx, tx, tenant, &legacy); e != nil {
		t.Fatal(e)
	}
	old := legacy.VersionID
	tx.Commit(ctx)
	if e := m.ensureCatalog(ctx); e != nil {
		t.Fatal(e)
	}
	var current string
	m.db.QueryRow(ctx, `SELECT current_version::text FROM wai_templates WHERE id=$1`, id).Scan(&current)
	if current == old {
		t.Fatal("untouched legacy example not upgraded")
	}
	tx, _ = m.db.Begin(ctx)
	pinned, e := loadTemplate(ctx, tx, tenant, old)
	if e != nil || pinned.ModelVersion != 0 {
		t.Fatal("old version rewritten", pinned, e)
	}
	tx.Rollback(ctx)
	if e = m.ensureCatalog(ctx); e != nil {
		t.Fatal(e)
	}
	var again string
	m.db.QueryRow(ctx, `SELECT current_version::text FROM wai_templates WHERE id=$1`, id).Scan(&again)
	if again != current {
		t.Fatal("repeated upgrade duplicated version")
	}
	tx, _ = m.db.Begin(ctx)
	legacy.ID = id
	legacy.HTML = "<p>PERSONALIZED EXAMPLE {{ficat}}</p>"
	saveTemplate(ctx, tx, tenant, &legacy)
	if e = audit(ctx, tx, principal{Tenant: tenant, User: user}, "template.publish", legacy.VersionID); e != nil {
		t.Fatal(e)
	}
	tx.Commit(ctx)
	if e = m.ensureCatalog(ctx); e != nil {
		t.Fatal(e)
	}
	m.db.QueryRow(ctx, `SELECT current_version::text FROM wai_templates WHERE id=$1`, id).Scan(&again)
	if again != legacy.VersionID {
		t.Fatal("user-edited example overwritten")
	}
}

func TestUltrasoundCatalogLayoutAndAliases(t *testing.T) {
	for _, tpl := range Catalog() {
		switch tpl.Name {
		case "Raport ecocardiografie":
			for _, text := range []string{"SIV 8 mm", "Sept interventricular: 8 mm"} {
				chunks := []Chunk{{ID: "source", Text: text}}
				facts := mockExtract(tpl, chunks)
				if len(facts) != 1 || facts[0].Field != "siv" || facts[0].Value != "8" {
					t.Fatal(text, facts)
				}
				result, err := buildResult(tpl, chunks, facts, "synthetic", 1)
				if err != nil || strings.Count(result.HTML, "<table>") != 4 || !strings.Contains(result.HTML, `<td><span data-review-field="siv">8</span> mm</td>`) || strings.Contains(result.HTML, "desemnează același câmp") {
					t.Fatal("cardiac report layout or guidance", err)
				}
			}
		case "Ecografie tiroidiană":
			chunks := []Chunk{{ID: "source", Text: "Istm: 0,23 cm\nnoduli_dreapta: 1,01 × 0,68 cm\nnoduli_dreapta: 1,43 × 1 cm"}}
			result, err := buildResult(tpl, chunks, mockExtract(tpl, chunks), "synthetic", 1)
			if err != nil || !strings.Contains(result.HTML, `data-section="lob_drept"`) || strings.Contains(result.HTML, `data-section="lob_stang"`) || strings.Contains(result.HTML, "[nedocumentat]") {
				t.Fatal("thyroid report should show only observed sections and fields", err, result.HTML)
			}
			if entries, ok := result.Data["noduli_dreapta"].([]any); !ok || len(entries) != 2 {
				t.Fatal("multiple nodules were lost", result.Data)
			}
		}
	}
}

func TestPostgresTemplateVisibilityAcrossSpecialties(t *testing.T) {
	m, rt := integrationModule(t)
	password, _ := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	_, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(password)})
	super := out["token"].(string)
	_, doctor := createBillingUser(t, m, super, "specialties@example.invalid", "doctor")
	if got := billingList(t, m, doctor, "templates"); len(got) != 0 {
		t.Fatal("unselected specialties exposed", len(got))
	}
	var sharedID string
	for _, specialty := range []string{"Endocrinologie", "Radiologie și imagistică medicală"} {
		selectTestSpecialties(t, m, doctor, specialty)
		seen := 0
		for _, item := range billingList(t, m, doctor, "templates") {
			if item["name"] == "Raport ecocardiografie" {
				t.Fatal("cardiology template exposed")
			}
			if item["name"] != "Ecografie tiroidiană" {
				continue
			}
			seen++
			if sharedID == "" {
				sharedID = item["id"].(string)
			}
			if item["id"] != sharedID {
				t.Fatal("different thyroid template")
			}
			code, body := request(t, m, doctor, "POST", "sessions", map[string]any{"template_version": item["version_id"], "patient_ref": "SYNTHETIC"})
			if code != 201 {
				t.Fatal("shared template is not usable", code, body)
			}
		}
		if seen != 1 {
			t.Fatal("expected one shared template", specialty, seen)
		}
	}
	code, body := request(t, m, doctor, "POST", "templates", map[string]any{
		"name": "Outside specialties", "specialty": "Radiologie și imagistică medicală",
		"specialties": []string{"Radiologie și imagistică medicală", "Cardiologie"},
		"html":        "<p>{{examen}}</p>", "fields": []Field{{Key: "examen", Label: "Examen", Type: "string"}},
	})
	if code != 402 || body["error"] != "specialty_not_selected" {
		t.Fatal("publication outside active specialties", code, body)
	}
}
