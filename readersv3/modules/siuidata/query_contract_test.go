package siuidata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestFilterArrays(t *testing.T) {
	for _, query := range []string{"specialty=CARD", "specialties[]=CARD&specialties[]=DERM", "specialty[0]=CARD&specialty[1]=DERM", "specialty=CARD&specialty=DERM", "specialty=%5B%22CARD%22,%22DERM%22%5D"} {
		q, _ := url.ParseQuery(query)
		f, e := parseFilters(q)
		if e != nil || len(f["specialty"]) == 0 {
			t.Fatalf("%s: %v", query, e)
		}
	}
	for _, query := range []string{"q[bad]=x", "q=[invalid", "q=[null]", "limit[]=1&limit[]=2", "start=-1", "limit=0"} {
		q, _ := url.ParseQuery(query)
		f, e := parseFilters(q)
		if e == nil {
			_, _, e = pagination(f)
		}
		if e == nil {
			t.Fatalf("invalid input accepted: %s", query)
		}
	}
}

func TestWiseMEDQueryContract(t *testing.T) {
	_, s := newTestModule(t, testPostgresSettings(t))
	xml := strings.NewReplacer(
		"<Specialities>", `<Specialities><Speciality code="DERM" name="Dermatologie" validFrom="2020-01-01"/>`,
		"<ClinicServiceGroups>", `<ClinicServiceGroups><ClinicServiceGroup code="G2" description="Investigații" validFrom="2020-01-01"/>`,
		"<ClinSrvGrpSpecialties>", `<ClinSrvGrpSpecialties><ClinSrvGrpSpecialty serviceGroup="G1" specialty="DERM" validFrom="2020-01-01"/><ClinSrvGrpSpecialty serviceGroup="G2" specialty="DERM" validFrom="2020-01-01"/>`,
		"<ClinicServices>", `<ClinicServices><ClinicService code="002" name="Control" serviceGroup="G2" isExam="false" opRoom="true" isConnectedService="false" validFrom="2020-01-01" extra="B"/><ClinicService code="003" name="Consultație repetată" serviceGroup="G1" isExam="false" opRoom="false" isConnectedService="false" validFrom="2020-01-01" extra="C"/>`,
	).Replace(currentFixture())
	awaitImport(t, s, upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "clin.xml", xml))), "ready")
	awaitImport(t, s, upload(t, s, "PARA", bytes.NewReader(zipXML(t, "para.xml", paraFixture))), "ready")
	type page struct {
		Success bool             `json:"success"`
		Total   int64            `json:"totalCount"`
		Rows    []map[string]any `json:"rows"`
	}
	for _, tc := range []struct {
		path  string
		total int64
		size  int
		code  string
	}{
		{"services?specialty=CARD", 2, 2, "001"},
		{"services?specialties[]=CARD&specialties[]=DERM", 3, 3, "001"},
		{"services?specialties[]=CARD&specialties[]=DERM&limit=1&start=1", 3, 1, "002"},
		{"services?specialties[]=CARD&specialties[]=DERM&limit=1&start=100", 3, 0, ""},
		{"services?specialty[]=CARD&group[]=G2&group[]=G1&isExam[]=false&isExam[]=true", 2, 2, "001"},
		{"services?attr.extra[]=B&attr.extra[]=C", 2, 2, "002"},
		{"services?q[]=control&q[]=repetata", 2, 2, "002"},
		{"services?nameAll[]=REPETATA&nameAll[]=consultatie", 1, 1, "003"},
		{"services?name[]=control&name[]=repetata", 2, 2, "002"},
		{"services?qAll[]=repetata&qAll[]=consultatie", 1, 1, "003"},
		{"services?qAll[]=control&qAll[]=repetata", 0, 0, ""},
		{"services?specialtyNameAll[]=CARDIO&specialtyNameAll[]=logie", 2, 2, "001"},
		{"services?specialtyNameAll[]=cardio&specialtyNameAll[]=dermato", 0, 0, ""},
		{"simple-services?specialtyNameAll[]=CARDIO&specialtyNameAll[]=logie", 2, 2, "001"},
		{"simple-services?nameAll[]=repetata&nameAll[]=consultatie&limit=1&start=1", 2, 1, "003"},
		{"records/Speciality?qAll[]=CARDIO&qAll[]=logie", 1, 1, ""},
		{"records/Speciality?nameAll[]=cardio&nameAll[]=dermato", 0, 0, ""},
		{"services?code[]=001&code[]=003", 2, 2, "001"},
		{"services?type[]=CLIN&type[]=PARA", 4, 4, "001"},
		{"records/Speciality?code[]=CARD&code[]=DERM&limit=1&start=1", 2, 1, ""},
		{"simple-services?specialties[]=CARD&specialties[]=DERM&limit=1", 5, 1, "001"},
		{"simple-services?type=PARA", 1, 1, "001"},
		{"versions?partnerCategory[]=CLIN&partnerCategory[]=PARA&limit=1&start=1", 2, 1, ""},
		{"categories?code[]=CLIN&code[]=PARA&limit=1&start=1", 2, 1, "PARA"},
	} {
		status, data := apiCall(t, s, "GET", prefix+tc.path, "", nil)
		var p page
		if e := json.Unmarshal(data, &p); e != nil || status != 200 || !p.Success || p.Total != tc.total || len(p.Rows) != tc.size {
			t.Fatalf("%s: %d %s", tc.path, status, data)
		}
		if tc.code != "" && p.Rows[0]["code"] != tc.code {
			t.Fatalf("page order %s: %s", tc.path, data)
		}
	}
	_, data := apiCall(t, s, "GET", prefix+"simple-services?code=001", "", nil)
	var p page
	json.Unmarshal(data, &p)
	v := p.Rows[0]

	if len(v) != 15 || v["isExam"] != true || v["opRoom"] != false || v["isConnectedService"] != true || v["group_code"] != "G1" {
		t.Fatalf("simple service fields: %s", data)
	}
	if v["package_code"] != "BAZA" || v["package_points"] != "12.3500" || v["package_value_lei"] != "42.00" || v["package_points_srg"] != "7.5" {
		t.Fatalf("flat package fields: %s", data)
	}
	if p.Total != 2 || len(p.Rows) != 2 || p.Rows[0]["speciality_code"] != "CARD" || p.Rows[1]["speciality_code"] != "DERM" {
		t.Fatalf("flat specialties: %s", data)
	}
	for _, row := range p.Rows {
		for _, value := range row {
			switch value.(type) {
			case []any, map[string]any:
				t.Fatalf("simple service contains nested values: %s", data)
			}
		}
	}
	for _, path := range []string{"status", "services/001", "imports/invalid", "services?start=-1", "services?isExam[]=true&isExam[]=invalid"} {
		status, data := apiCall(t, s, "GET", prefix+path, "", nil)
		var body map[string]any
		json.Unmarshal(data, &body)
		if status >= 400 {
			if body["success"] != false || body["error"] == nil || body["data"] != nil {
				t.Fatalf("error contract %s", data)
			}
		} else if body["success"] != true || body["data"] == nil {
			t.Fatalf("single contract %s", data)
		}
	}
	// Counts are totals, not the number of rows remaining after the requested start.
	status, data := apiCall(t, s, "GET", prefix+fmt.Sprintf("catalogues?type=CLIN&limit=1&start=%d", 100), "", nil)
	if status != 200 {
		t.Fatal(string(data))
	}
	json.Unmarshal(data, &p)
	if p.Total == 0 || len(p.Rows) != 0 {
		t.Fatal(string(data))
	}
}
