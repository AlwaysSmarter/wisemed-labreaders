package siuidata

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const drugFixture = `
<Countries><Country code="ROM" name="România"/></Countries>
<PharmaceuticalForms><PharmaceuticalForm code="COMPR." validFrom="2020-01-01"/></PharmaceuticalForms>
<Concentrations><Concentration concentration="500 mg" validFrom="2020-01-01"/></Concentrations>
<ActiveSubstances><ActiveSubstance code="PARACETAMOLUM" validFrom="2020-01-01"/><ActiveSubstance code="IBUPROFENUM" validFrom="2020-01-01"/></ActiveSubstances>
<ATCS><ATC code="N" description="Sistem nervos" validFrom="2020-01-01"/><ATC code="N02BE01" parentATC="N" description="Paracetamol" validFrom="2020-01-01"/><ATC code="M01AE01" description="Ibuprofen" validFrom="2020-01-01"/></ATCS>
<DiseaseCategories><DiseaseCategory code="G1" description="Boală cardiacă" validFrom="2020-01-01"/><DiseaseCategory code="G2" description="Diabet zaharat" validFrom="2020-01-01"/></DiseaseCategories>
<ICD10S><ICD10 code="994" name="Diabet tip doi" diseaseCategory="G2" validFrom="2020-01-01"/></ICD10S>
<NHPS><NHP code="P1" description="Program oncologic" validFrom="2020-01-01"/><NHP code="P2" description="Program diabet" validFrom="2020-01-01"/></NHPS>
<CopaymentListTypes><CopaymentListType code="A" description="Lista A" percent="50" validFrom="2020-01-01"/><CopaymentListType code="C1" description="Lista C1" percent="100" validFrom="2020-01-01"/></CopaymentListTypes>
<Drugs>
<Drug code="D1" name="Paracetamol Știință" activeSubstance="PARACETAMOLUM" pharmaceuticalForm="COMPR." concentration="500 mg" presentationMode="Cutie cu 20 comprimate" company="Producător" country="ROM" atc="N02BE01" isSpecial="true" isFractional="false" hasBioEchiv="true" isNarcotic="0" pricePerPackage="5.2500" qtyPerPackage="20" validFrom="2020-01-01"/>
<Drug code="D2" name="Ibuprofen" activeSubstance="IBUPROFENUM" pharmaceuticalForm="COMPR." concentration="500 mg" presentationMode="Cutie cu 10 comprimate" company="Alt producător" country="ROM" atc="M01AE01" isSpecial="false" isFractional="true" hasBioEchiv="false" isNarcotic="2" pricePerPackage="10.00" qtyPerPackage="10" validFrom="2020-01-01"/>
<Drug code="OLD" name="Istoric" activeSubstance="PARACETAMOLUM" pharmaceuticalForm="COMPR." concentration="500 mg" country="ROM" atc="N02BE01" validFrom="2020-01-01" validTo="2025-01-01"/>
</Drugs>
<CopaymentListDrugs>
<CopaymentListDrug drug="D1" copaymentListType="A" nhpCode="P1" diseaseCode="G1" maxPrice="10.0000" referencePrice="2.000001" maxPriceUT="0.5" needApproval="P" specialLaw="true" openCircuit="1" validFrom="2020-01-01"/>
<CopaymentListDrug drug="D1" copaymentListType="C1" nhpCode="P2" diseaseCode="G2" maxPrice="100.00" referencePrice="20" needApproval="0" hgIcd10="994" validFrom="2020-01-01"/>
<CopaymentListDrug drug="D2" copaymentListType="A" nhpCode="P1" diseaseCode="G1" maxPrice="20" referencePrice="3" needApproval="S" validFrom="2020-01-01"/>
<CopaymentListDrug drug="D1" copaymentListType="A" nhpCode="P1" maxPrice="999" referencePrice="999" validFrom="2020-01-01" validTo="2025-01-01"/>
</CopaymentListDrugs>
<ActSubstICD10S><ActSubstICD10 activeSubstance="PARACETAMOLUM" icd10="994" validFrom="2020-01-01"/></ActSubstICD10S>
<CopaymentListActiveSubsts><CopaymentListActiveSubst activeSubstance="PARACETAMOLUM" aTC="N02BE01" copaymentListType="C1" diseasecategory="G2" nhpCode="P2" needApproval="0" validFrom="2020-01-01"/></CopaymentListActiveSubsts>
<CopaymentListProtocolTherapeutics><CopaymentListProtocolTherapeutic codeProtocolTherap="L1" descProtocolTherap="Protocol diabet" activeSubstance="PARACETAMOLUM" atc="N02BE01" copaymentListType="C1" diseaseCategory="G2" nhpCode="P2" icd10="994" needApproval="J" validFrom="2020-01-01"/></CopaymentListProtocolTherapeutics>
<NHP_DRUG_TYPES><NHP_DRUG_TYPE code="DRG" description="Medicament"/><NHP_DRUG_TYPE code="ACS" description="Substanță activă"/></NHP_DRUG_TYPES>
<NHP_DRUGS><NHP_DRUG code="D2" type="DRG" nhp="P2" price="30" validFrom="2020-01-01"/><NHP_DRUG code="PARACETAMOLUM" type="ACS" nhp="P2" validFrom="2020-01-01"/></NHP_DRUGS>
`

func medicinesXML() string {
	return strings.Replace(currentFixture(), "</Catalogues>", drugFixture+"</Catalogues>", 1)
}

// Multiple alternatives keep OR semantics; All requires every fragment in one field.
func TestMedicineTextFragments(t *testing.T) {
	_, server := newTestModule(t, testPostgresSettings(t))
	xml := strings.Replace(medicinesXML(), `</ICD10S>`, `<ICD10 code="995" name="Alt diagnostic" validFrom="2020-01-01"/></ICD10S>`, 1)
	xml = strings.Replace(xml, `</ActSubstICD10S>`, `<ActSubstICD10 activeSubstance="PARACETAMOLUM" icd10="995" validFrom="2020-01-01"/></ActSubstICD10S>`, 1)
	awaitImport(t, server, upload(t, server, "CLIN", bytes.NewReader(zipXML(t, "clin.xml", xml))), "ready")
	tests := []struct {
		query string
		total int
	}{
		{"presentationMode[]=20&presentationMode[]=10", 2},
		{"qAll[]=stiinta&qAll[]=paracet", 1},
		{"qAll[]=paracet&qAll[]=ibupro", 0},
		{"presentationModeAll[]=20&presentationModeAll[]=CUTIE", 1},
		{"presentationModeAll[]=20&presentationModeAll[]=10", 0},
		{"presentationModeAll=20", 1},
		{"presentationModeAll=%5B%2220%22,%22cutie%22%5D", 1},
		{"presentationModeAll[0]=20&presentationModeAll[1]=cutie", 1},
		{"presentationMode[]=20&presentationMode[]=10&presentationModeAll[]=cutie&presentationModeAll[]=10", 1},
		{"presentationModeAll[]=cutie&presentationModeAll[]=20&activeSubstanceAll[]=ibupro", 0},
		{"presentationModeAll[]=cutie&presentationModeAll[]=20&activeSubstanceAll[]=MOLUM&activeSubstanceAll[]=paracet", 1},
		{"nameAll[]=STIINTA&nameAll[]=paracet", 1},
		{"packagingAll[]=20&packagingAll[]=cutie&substanceAll[]=paracet&medicineAll[]=stiinta", 1},
		{"companyAll[]=PRODUCATOR&companyAll[]=alt&pharmaceuticalFormAll[]=comp&concentrationAll[]=mg&concentrationAll[]=500", 1},
		{"presentationModeAll[]=%25", 0},
		{"presentationModeAll[]=_", 0},
		{"diseaseNameAll[]=cardiaca&diseaseNameAll[]=BOALA&nationalProgramNameAll[]=onco&nationalProgramNameAll[]=program", 2},
		{"diseaseNameAll[]=cardiaca&diseaseNameAll[]=diabet", 0},
		{"copaymentListNameAll[]=lista&copaymentListNameAll[]=c1&commissionNameAll[]=FARA&commissionNameAll[]=aprobare", 1},
		{"diseaseNameAll[]=cardiaca&nationalProgramNameAll[]=diabet", 0},
		{"diagnosisNameAll[]=doi&diagnosisNameAll[]=diabet", 1},
		{"diagnosisNameAll[]=diabet&diagnosisNameAll[]=diagnostic", 0},
	}
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			status, data := apiCall(t, server, "GET", prefix+"drugs?"+tc.query, "", nil)
			var page struct {
				Success bool `json:"success"`
				Total   int  `json:"totalCount"`
			}
			if err := json.Unmarshal(data, &page); err != nil {
				t.Fatal(err)
			}
			if status != 200 || !page.Success || page.Total != tc.total {
				t.Fatalf("got %d %s; want %d", status, data, tc.total)
			}
		})
	}
	for _, route := range []string{"drugs", "simple-drugs", "drug-compensations", "drug-relations"} {
		t.Run(route, func(t *testing.T) {
			status, data := apiCall(t, server, "GET", prefix+route+"?presentationModeAll[]=20&presentationModeAll[]=cutie&diagnosisNameAll[]=doi&diagnosisNameAll[]=diabet&copaymentListNameAll[]=c1&copaymentListNameAll[]=lista&sourceKind=CopaymentListDrug", "", nil)
			var page struct {
				Success bool             `json:"success"`
				Total   int              `json:"totalCount"`
				Rows    []map[string]any `json:"rows"`
			}
			if err := json.Unmarshal(data, &page); err != nil {
				t.Fatal(err)
			}
			if status != 200 || !page.Success || page.Total != 1 || len(page.Rows) != 1 {
				t.Fatalf("%d %s", status, data)
			}
			if route == "simple-drugs" && page.Rows[0]["diagnosis_code"] != "994" {
				t.Fatalf("diagnosis must match all fragments: %s", data)
			}
		})
	}
}

func TestMedicineGraphAndFilters(t *testing.T) {
	m, s := newTestModule(t, testPostgresSettings(t))
	first := upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "clin.xml", medicinesXML())))
	job := awaitImport(t, s, first, "ready")
	if job["drugs"] != float64(2) {
		t.Fatalf("drug count: %+v", job)
	}
	tests := []struct {
		query string
		total int
		code  string
	}{
		{"drugs", 2, "D1"}, {"simple-drugs?name=stiinta", 5, "D1"}, {"drugs?q=paracetamol+stiinta", 1, "D1"},
		{"drugs?code[]=D1&code[]=D2&limit=1&start=1", 2, "D2"}, {"drugs?code[]=D1&limit=1&start=99", 1, ""},
		{"drugs?activeSubstance[]=paracet&activeSubstance[]=ibupro", 2, "D1"}, {"drugs?pharmaceuticalForm=COMPR.&concentration=500&presentationMode=20", 1, "D1"},
		{"drugs?isSpecial=true", 1, "D1"}, {"drugs?isSpecial[]=true&isSpecial[]=false", 2, "D1"}, {"drugs?isNarcotic=2", 1, "D2"},
		{"drugs?nhpCode=P1&maxPriceMin=50", 0, ""}, {"drugs?copaymentListType=A&referencePriceMin=10", 0, ""},
		{"drugs?copaymentListType=A&maxPriceMin=10&maxPriceMax=10", 1, "D1"}, {"drugs?referencePriceMin=2.0000009&referencePriceMax=2.000001", 1, "D1"},
		{"drugs?copaymentListType[]=A&copaymentListType[]=C1&commission=P", 1, "D1"}, {"drugs?diseaseName=cardiaca&nhpCode=P1", 2, "D1"},
		{"drugs?diseaseCode=G2&nhpCode=P2", 1, "D1"}, {"drugs?sourceKind=NHP_DRUG&nationalProgram=P2", 2, "D1"},
		{"drugs?diagnosisCode=994&diagnosisName=diabet", 1, "D1"}, {"drugs?attr.company=Produc%C4%83tor&compensation.attr.needApproval=P", 1, "D1"},
		{"drugs?atcPrefix=N02", 1, "D1"}, {"drugs?pricePerPackageMin=5.25&pricePerPackageMax=5.25", 1, "D1"},
		{"drug-compensations?code=D1", 2, ""}, {"drug-relations?code=D1", 5, ""}, {"drug-records/ActiveSubstance", 2, ""},
		{"drug-records/ActiveSubstance?q=paracet", 1, ""}, {"drug-records/Concentration?code=500+mg", 1, ""},
	}
	for _, tc := range tests {
		status, data := apiCall(t, s, "GET", prefix+tc.query, "", nil)
		var page struct {
			Success bool             `json:"success"`
			Total   int              `json:"totalCount"`
			Rows    []map[string]any `json:"rows"`
		}
		json.Unmarshal(data, &page)
		if status != 200 || !page.Success || page.Total != tc.total {
			t.Fatalf("%s: %d %s", tc.query, status, data)
		}
		if tc.code != "" && (len(page.Rows) == 0 || page.Rows[0]["code"] != tc.code) {
			t.Fatalf("%s: %s", tc.query, data)
		}
	}
	_, data := apiCall(t, s, "GET", prefix+"drugs/D1", "", nil)
	var drug map[string]any
	decodeData(t, data, &drug)
	for key, want := range map[string]int{"atcHierarchy": 2, "compensations": 2, "substanceCompensations": 1, "nationalProgramDrugs": 1, "therapeuticProtocols": 1, "diagnoses": 1} {
		if len(drug[key].([]any)) != want {
			t.Fatalf("%s: %s", key, data)
		}
	}
	if !bytes.Contains(data, []byte(`"2.000001"`)) {
		t.Fatal("decimal source precision was lost")
	}
	for _, query := range []string{"drugs?maxPriceMin=2&maxPriceMax=1", "drugs?maxPriceMin=NaN", "drugs?referencePriceMax=-1", "drugs?maxPriceMin[]=1&maxPriceMin[]=2", "drugs?unknownPriceMin=10", "drugs?isSpecial=wrong", "drug-catalogues?kind=ClinicService"} {
		status, data := apiCall(t, s, "GET", prefix+query, "", nil)
		if status != 400 || !bytes.Contains(data, []byte(`"success":false`)) {
			t.Fatalf("invalid %s: %d %s", query, status, data)
		}
	}
	// Failed medicine parsing leaves the previous entire generation available.
	bad := strings.Replace(medicinesXML(), `maxPrice="10.0000"`, `maxPrice="NaN"`, 1)
	awaitImport(t, s, upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "bad.xml", bad))), "failed")
	if status, data := apiCall(t, s, "GET", prefix+"drugs/D1", "", nil); status != 200 {
		t.Fatalf("lost old drugs %s", data)
	}
	// Old imports without a projection give an explicit upgrade action.
	db, e := m.database(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(context.Background(), `UPDATE siui_data_imports SET drug_projection_version=0 WHERE id=$1`, first); e != nil {
		t.Fatal(e)
	}
	if status, _ := apiCall(t, s, "GET", prefix+"drugs", "", nil); status != 503 {
		t.Fatal("legacy medicine projection was silently treated as empty")
	}
	if status, _ := apiCall(t, s, "GET", prefix+"services/001", "", nil); status != 200 {
		t.Fatal("migration blocked existing services")
	}
}

func TestPHMMedicineImport(t *testing.T) {
	_, s := newTestModule(t, testPostgresSettings(t))
	xml := `<Catalogues xmlns="http://www.cnas.ro/siui/2.0">` + drugFixture + `</Catalogues>`
	awaitImport(t, s, upload(t, s, "PHM", bytes.NewReader(zipXML(t, "phm.xml", xml))), "ready")
	status, data := apiCall(t, s, "GET", prefix+"drugs?type=PHM&isSpecial=true", "", nil)
	if status != 200 || !bytes.Contains(data, []byte(`"totalCount":1`)) {
		t.Fatalf("PHM %d %s", status, data)
	}
	if typ, day := sourceFilename("NomenclatoareFarmacii_20261001.xml"); typ != "PHM" || day != "2026-10-01" {
		t.Fatal(typ, day)
	}
}

func TestRealMedicineGraph(t *testing.T) {
	dir := os.Getenv("SIUI_DATA_REAL_CATALOGUES")
	if dir == "" {
		t.Skip("set SIUI_DATA_REAL_CATALOGUES to verify medicine graph on real CNAS data")
	}
	_, s := newTestModule(t, testPostgresSettings(t))
	for _, source := range []struct{ typ, file string }{{"CLIN", "NomenclatoareClinice_20261001.xml.zip"}, {"PHM", "NomenclatoareFarmacii_20261001.xml.zip"}} {
		f, e := os.Open(filepath.Join(dir, source.file))
		if e != nil {
			t.Fatal(e)
		}
		id := upload(t, s, source.typ, f)
		f.Close()
		job := awaitImport(t, s, id, "ready")
		t.Logf("%s drugs=%v records=%v", source.typ, job["drugs"], job["records"])
		for _, q := range []string{"drugs?type=" + source.typ + "&q=paracetamol&limit=5", "simple-drugs?type=" + source.typ + "&copaymentListType=C1&maxPriceMin=10&maxPriceMax=100&limit=5", "drug-compensations?type=" + source.typ + "&needApproval=P&limit=2"} {
			status, data := apiCall(t, s, "GET", prefix+q, "", nil)
			if status != 200 {
				t.Fatalf("%s: %d %s", q, status, data)
			}
		}
	}
}

func TestMedicineImportKeepsServingPreviousVersion(t *testing.T) {
	cfg := testPostgresSettings(t)
	m, s := newTestModule(t, cfg)
	xml := `<Catalogues xmlns="http://www.cnas.ro/siui/2.0">` + drugFixture + `</Catalogues>`
	first := upload(t, s, "PHM", bytes.NewReader(zipXML(t, "phm.xml", xml)))
	awaitImport(t, s, first, "ready")
	db, e := m.database(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	block, e := db.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer block.Rollback(context.Background())
	if _, e = block.Exec(context.Background(), `SELECT * FROM siui_data_active WHERE catalogue_type='PHM' FOR UPDATE`); e != nil {
		t.Fatal(e)
	}
	next := upload(t, s, "PHM", bytes.NewReader(zipXML(t, "phm.xml", strings.Replace(xml, "Paracetamol Știință", "Versiune nouă", 1))))
	_, replica := newTestModule(t, cfg)
	for i := 0; i < 3; i++ {
		status, data := apiCall(t, replica, "GET", prefix+"drugs/D1?type=PHM", "", nil)
		if status != 200 || !bytes.Contains(data, []byte("Paracetamol")) {
			t.Fatalf("old medicine unavailable during import: %d %s", status, data)
		}
	}
	if e = block.Commit(context.Background()); e != nil {
		t.Fatal(e)
	}
	awaitImport(t, s, next, "ready")
	if status, data := apiCall(t, s, "GET", prefix+"drugs/D1?type=PHM", "", nil); status != 200 || !bytes.Contains(data, []byte("Versiune nouă")) {
		t.Fatalf("new medicine version was not activated: %d %s", status, data)
	}
}

func TestMedicineSchemaUpgradePreservesExistingServices(t *testing.T) {
	cfg := testPostgresSettings(t)
	m, s := newTestModule(t, cfg)
	id := upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "clin.xml", currentFixture())))
	awaitImport(t, s, id, "ready")
	db, e := m.database(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	// Recreate the shape of a version-1 database in this disposable test DB.
	if _, e = db.Exec(context.Background(), `DROP TABLE siui_data_drug_terms; DROP TABLE siui_data_drugs; ALTER TABLE siui_data_imports DROP COLUMN drug_count,DROP COLUMN drug_projection_version; DELETE FROM siui_data_schema_version WHERE version>=2`); e != nil {
		t.Fatal(e)
	}
	_, replica := newTestModule(t, cfg)
	if status, data := apiCall(t, replica, "GET", prefix+"services/001", "", nil); status != 200 {
		t.Fatalf("schema upgrade lost old service: %d %s", status, data)
	}
	if status, data := apiCall(t, replica, "GET", prefix+"drugs", "", nil); status != 503 || !bytes.Contains(data, []byte("reimport")) {
		t.Fatalf("legacy projection not reported: %d %s", status, data)
	}
}

func TestMedicineCataloguesKeepAuxiliaryPhysicianModels(t *testing.T) {
	_, s := newTestModule(t, testPostgresSettings(t))
	// Regular CNAS catalogues have a different PhysicianSpeciality shape from
	// getPhysicianCatalogues. Preserve these records without PHYS-only checks.
	aux := `<Physicians><Physician code="AUX" validFrom="2020-01-01"/></Physicians><PhysicianSpecialities><PhysicianSpeciality physician="AUX" specialty="CARD" validFrom="2020-01-01"/></PhysicianSpecialities>`
	for _, typ := range []string{"CLIN", "PHM"} {
		xml := strings.Replace(medicinesXML(), "</Catalogues>", aux+"</Catalogues>", 1)
		id := upload(t, s, typ, bytes.NewReader(zipXML(t, "catalogue.xml", xml)))
		job := awaitImport(t, s, id, "ready")
		if job["drugs"] != float64(2) {
			t.Fatalf("%s lost drugs: %+v", typ, job)
		}
		status, data := apiCall(t, s, "GET", prefix+"drugs?type="+typ, "", nil)
		if status != 200 || !bytes.Contains(data, []byte(`"totalCount":2`)) {
			t.Fatalf("%s %d %s", typ, status, data)
		}
	}
	// Dedicated PHYS imports still enforce the documented physician schema.
	var ordinal int64
	source := newCatalogueRows(context.Background(), strings.NewReader(`<Catalogues><PhysicianSpecialities><PhysicianSpeciality physician="AUX" specialty="CARD" validFrom="2020-01-01"/></PhysicianSpecialities></Catalogues>`), models["PHYS"], "2026-10-22", &ordinal)
	for source.Next() {
	}
	if source.Err() == nil {
		t.Fatal("PHYS schema validation was disabled")
	}
	if typ, day := sourceFilename("NomenclatoarePHM_20261001.xml.zip"); typ != "PHM" || day != "2026-10-01" {
		t.Fatalf("PHM filename not recognized: %s %s", typ, day)
	}
}

func TestRealPHMCatalogue(t *testing.T) {
	archive := os.Getenv("SIUI_DATA_REAL_PHM_ARCHIVE")
	if archive == "" {
		t.Skip("set SIUI_DATA_REAL_PHM_ARCHIVE for the real PHM upload regression")
	}
	_, s := newTestModule(t, testPostgresSettings(t))
	f, e := os.Open(archive)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	job := awaitImport(t, s, upload(t, s, "PHM", f), "ready")
	if job["drugs"].(float64) == 0 {
		t.Fatal("real PHM import lost medicines")
	}
	t.Logf("PHM medicines=%v raw records=%v", job["drugs"], job["records"])
	for _, route := range []string{"drugs?type=PHM&limit=1", "simple-drugs?type=PHM&limit=1"} {
		status, data := apiCall(t, s, "GET", prefix+route, "", nil)
		var page flatPage
		json.Unmarshal(data, &page)
		if status != 200 || page.Total == 0 || len(page.Rows) != 1 {
			t.Fatalf("PHM API %s %d %s", route, status, data)
		}
	}
}
