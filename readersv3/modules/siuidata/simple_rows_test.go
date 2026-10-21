package siuidata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type flatPage struct {
	Success   bool             `json:"success"`
	Total     int              `json:"totalCount"`
	Rows      []map[string]any `json:"rows"`
	NextStart *int             `json:"nextStart"`
}

func readFlatPage(t *testing.T, data []byte, status, total, size int) flatPage {
	t.Helper()
	var p flatPage
	if e := json.Unmarshal(data, &p); e != nil || status != 200 || !p.Success || p.Total != total || len(p.Rows) != size {
		t.Fatalf("flat page: %d %s", status, data)
	}
	for _, row := range p.Rows {
		for key, value := range row {
			switch value.(type) {
			case []any, map[string]any:
				t.Fatalf("nested %s in flat page: %s", key, data)
			}
		}
	}
	return p
}

func TestSimpleServiceCombinations(t *testing.T) {
	_, s := newTestModule(t, testPostgresSettings(t))
	xml := strings.NewReplacer(
		"<Specialities>", `<Specialities><Speciality code="DERM" name="Dermatologie" validFrom="2020-01-01"/>`,
		"<ClinSrvGrpSpecialties>", `<ClinSrvGrpSpecialties><ClinSrvGrpSpecialty serviceGroup="G1" specialty="DERM" validFrom="2020-01-01"/>`,
		"<MedSrvPacks>", `<MedSrvPacks><MedSrvPack code="EXT" name="Extins" validFrom="2020-01-01"/>`,
		"<ClinSrvMedPacks>", `<ClinSrvMedPacks><ClinSrvMedPack service="001" medSrvPack="EXT" points="20.000" validFrom="2020-01-01"/>`,
		"<ClinicServices>", `<ClinicServices><ClinicService code="EMPTY" name="Fără relații" isExam="false" opRoom="false" isConnectedService="false" validFrom="2020-01-01"/>`,
	).Replace(currentFixture())
	awaitImport(t, s, upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "clin.xml", xml))), "ready")
	for i, pair := range []string{"BAZA/CARD", "BAZA/DERM", "EXT/CARD", "EXT/DERM"} {
		status, data := apiCall(t, s, "GET", prefix+fmt.Sprintf("simple-services?code=001&limit=1&start=%d", i), "", nil)
		p := readFlatPage(t, data, status, 4, 1)
		row := p.Rows[0]
		if row["package_code"].(string)+"/"+row["speciality_code"].(string) != pair {
			t.Fatalf("wrong combination: %s", data)
		}
		if i < 3 && (p.NextStart == nil || *p.NextStart != i+1) {
			t.Fatalf("wrong nextStart: %s", data)
		}
		if i == 3 && p.NextStart != nil {
			t.Fatalf("unexpected nextStart: %s", data)
		}
	}
	for _, tc := range []struct {
		q           string
		total, size int
	}{
		{"code=001&specialties[]=DERM&package[]=EXT", 1, 1},
		{"code=001&specialtyName=dermat&package=BAZA", 1, 1},
		{"code=001&specialties[]=CARD&specialties[]=DERM&package[]=EXT&package[]=BAZA", 4, 4},
		{"code=001&start=99", 4, 0},
		{"code=EMPTY", 1, 1},
	} {
		status, data := apiCall(t, s, "GET", prefix+"simple-services?"+tc.q, "", nil)
		p := readFlatPage(t, data, status, tc.total, tc.size)
		if tc.q == "code=EMPTY" && (p.Rows[0]["package_code"] != nil || p.Rows[0]["speciality_code"] != nil) {
			t.Fatalf("missing relations: %s", data)
		}
	}
	status, data := apiCall(t, s, "GET", prefix+"services?code=001", "", nil)
	var full flatPage
	json.Unmarshal(data, &full)
	if status != 200 || full.Total != 1 || len(full.Rows[0]["packages"].([]any)) != 2 || len(full.Rows[0]["specialties"].([]any)) != 2 {
		t.Fatalf("full service changed: %s", data)
	}
	status, _ = apiCall(t, s, "GET", prefix+"simple-services?after=001", "", nil)
	if status != 400 {
		t.Fatal("simple-services accepted ambiguous code cursor")
	}
}

func TestSimpleDrugCombinations(t *testing.T) {
	_, s := newTestModule(t, testPostgresSettings(t))
	xml := strings.NewReplacer(
		"<ICD10S>", `<ICD10S><ICD10 code="995" name="Alt diagnostic" diseaseCategory="G1" validFrom="2020-01-01"/>`,
		"<ActSubstICD10S>", `<ActSubstICD10S><ActSubstICD10 activeSubstance="PARACETAMOLUM" icd10="995" validFrom="2020-01-01"/>`,
		"<Drugs>", `<Drugs><Drug code="EMPTY" name="Fără relații" activeSubstance="NOREL" pharmaceuticalForm="COMPR." concentration="500 mg" country="ROM" atc="M01AE01" validFrom="2020-01-01"/>`,
		"<ActiveSubstances>", `<ActiveSubstances><ActiveSubstance code="NOREL" validFrom="2020-01-01"/>`,
	).Replace(medicinesXML())
	awaitImport(t, s, upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "clin.xml", xml))), "ready")
	seen := map[string]bool{}
	for i := 0; i < 10; i++ {
		status, data := apiCall(t, s, "GET", prefix+fmt.Sprintf("simple-drugs?code=D1&limit=1&start=%d", i), "", nil)
		p := readFlatPage(t, data, status, 10, 1)
		row := p.Rows[0]
		key := fmt.Sprint(row["sourceKind"], row["copayment_list_code"], row["diagnosis_code"])
		if seen[key] {
			t.Fatalf("duplicate page combination: %s", data)
		}
		seen[key] = true
		if i < 9 && (p.NextStart == nil || *p.NextStart != i+1) {
			t.Fatalf("wrong nextStart: %s", data)
		}
		if i == 9 && p.NextStart != nil {
			t.Fatalf("unexpected nextStart: %s", data)
		}
	}
	for _, tc := range []struct {
		q           string
		total, size int
	}{
		{"code=D1&sourceKind=CopaymentListDrug&copaymentListType=A&diagnosisCode=994", 1, 1},
		{"code=D1&copaymentListType=A&referencePriceMin=10", 0, 0},
		{"code=D1&copaymentListType[]=A&copaymentListType[]=C1&sourceKind=CopaymentListDrug", 4, 4},
		{"code=D1&diagnosisName=alt", 5, 5},
		{"code=D1&start=99", 10, 0},
		{"code=EMPTY", 1, 1},
	} {
		status, data := apiCall(t, s, "GET", prefix+"simple-drugs?"+tc.q, "", nil)
		p := readFlatPage(t, data, status, tc.total, tc.size)
		if strings.Contains(tc.q, "diagnosisCode=994") {
			row := p.Rows[0]
			if row["maxPrice"] != "10.0000" || row["referencePrice"] != "2.000001" || row["national_program_code"] != "P1" || row["approval_code"] != "P" || row["relation_specialLaw"] != "true" || row["diagnosis_code"] != "994" {
				t.Fatalf("lost relation fields: %s", data)
			}
		}
		if tc.q == "code=EMPTY" && (p.Rows[0]["sourceKind"] != nil || p.Rows[0]["diagnosis_code"] != nil) {
			t.Fatalf("missing relations: %s", data)
		}
	}
}
