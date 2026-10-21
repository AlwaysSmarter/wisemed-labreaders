package siuidata

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const physiciansFixture = `<Catalogues xmlns="http://www.cnas.ro/siui/2.0" issueDate="2026-10-21T12:34:15">
<InsuranceHouseTypes><InsuranceHouseType code="CJAS" description="Casă județeană"/></InsuranceHouseTypes>
<InsuranceHouses><InsuranceHouse code="CAS-BV" name="CAS Brașov" type="CJAS" validFrom="2020-01-01"/></InsuranceHouses>
<Specialities><Speciality code="CARD" name="Cardiologie" validFrom="2020-01-01"/><Speciality code="DERM" name="Dermatologie" validFrom="2020-01-01"/></Specialities>
<Physicians><Physician stencil="P1" name="Ștefan Popescu" validFrom="2020-01-01"/><Physician stencil="P2" name="Ana Ionescu" validFrom="2020-01-01"/><Physician stencil="OLD" name="Istoric" validFrom="2020-01-01" validTo="2025-01-01"/></Physicians>
<PhysicianSpecialities>
<PhysicianSpeciality stencil="P1" contractNo="100" insuranceHouse="CAS-BV" contractType="CLN" physicianType="A" specialityCode="CARD" validFrom="2020-01-01"/>
<PhysicianSpeciality stencil="P1" contractNo="200" insuranceHouse="CAS-BV" contractType="SPT" physicianType="H" specialityCode="DERM" validFrom="2020-01-01"/>
<PhysicianSpeciality stencil="P1" contractNo="OLD" insuranceHouse="CAS-BV" contractType="CLN" physicianType="A" specialityCode="CARD" validFrom="2020-01-01" validTo="2025-01-01"/>
<PhysicianSpeciality stencil="MISSING" contractNo="300" insuranceHouse="CAS-BV" contractType="SPT" physicianType="H" specialityCode="CARD" validFrom="2020-01-01"/>
</PhysicianSpecialities></Catalogues>`

func physicianArchive(t *testing.T) []byte { return physicianArchiveDate(t, "2026-10-08") }
func physicianArchiveDate(t *testing.T, opsDate string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, source := range []string{"CNAS", "OPSNAJ"} {
		w, e := z.Create("Medici" + source + "_20261021.xml")
		if e != nil {
			t.Fatal(e)
		}
		xml := physiciansFixture
		if source == "OPSNAJ" {
			xml = strings.ReplaceAll(xml, "2026-10-21T12:34:15", opsDate+"T09:18:40")
			xml = strings.ReplaceAll(xml, "Ștefan Popescu", "Popescu OPSNAJ")
		}
		w.Write([]byte(xml))
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}

func TestPhysicianImportAndQueries(t *testing.T) {
	_, s := newTestModule(t, testPostgresSettings(t))
	id := upload(t, s, "PHYS", bytes.NewReader(physicianArchive(t)))
	job := awaitImport(t, s, id, "ready")
	if job["physicians"] != float64(4) || job["sourceDates"].(map[string]any)["OPSNAJ"] != "2026-10-08" {
		t.Fatalf("counts and source dates: %+v", job)
	}
	for _, tc := range []struct {
		q           string
		total, size int
	}{
		{"physicians?nameAll[]=POPESCU&nameAll[]=stefan", 1, 1},
		{"physicians?qAll[]=Popescu&qAll[]=stefan", 1, 1},
		{"physicians?qAll[]=Popescu&qAll[]=Ionescu", 0, 0},
		{"physicians?specialtyNameAll[]=cardio&specialtyNameAll[]=dermato", 0, 0},
		{"simple-physicians?source=CNAS&specialtyNameAll[]=cardio&specialtyNameAll[]=logie&insuranceHouseNameAll[]=CAS&insuranceHouseNameAll[]=BRASOV", 1, 1},
		{"physicians?source=CNAS&specialityNameAll[]=cardio&specialityNameAll[]=logie&contractType=SPT", 0, 0},
		{"physician-records/Physician?source=CNAS&nameAll[]=STEFAN&nameAll[]=popescu", 1, 1},
		{"physician-records/Physician?source=CNAS&qAll[]=stefan&qAll[]=popescu", 1, 1},
		{"physicians", 4, 4}, {"physicians?name=stefan", 1, 1}, {"physicians?parafa=P1", 2, 2},
		{"physicians?source=CNAS&specialties[]=CARD&specialties[]=DERM", 1, 1},
		{"physicians?source=CNAS&specialty=CARD&contractType=SPT", 0, 0},
		{"physicians?source=CNAS&insuranceHouseName=brasov&contractNo=200&physicianType=H", 1, 1},
		{"simple-physicians?source=CNAS", 3, 3}, {"simple-physicians?stencil=P1&source=CNAS&specialty=DERM", 1, 1},
		{"simple-physicians?stencil=P1&source=CNAS&limit=1&start=1", 2, 1},
		{"simple-physicians?stencil=P1&source=CNAS&start=99", 2, 0},
		{"physician-records/PhysicianSpeciality?source=CNAS", 3, 3},
	} {
		status, data := apiCall(t, s, "GET", prefix+tc.q, "", nil)
		var p flatPage
		json.Unmarshal(data, &p)
		if status != 200 || p.Total != tc.total || len(p.Rows) != tc.size || !p.Success {
			t.Fatalf("%s: %d %s", tc.q, status, data)
		}
		if strings.HasPrefix(tc.q, "simple-") {
			readFlatPage(t, data, status, tc.total, tc.size)
		}
		if strings.Contains(tc.q, "specialty=DERM") && len(p.Rows) > 0 {
			if p.Rows[0]["speciality_code"] != "DERM" || p.Rows[0]["contractType"] != "SPT" {
				t.Fatalf("filtered row: %s", data)
			}
		}
	}
	status, data := apiCall(t, s, "GET", prefix+"physicians/P1?source=CNAS", "", nil)
	var physician map[string]any
	decodeData(t, data, &physician)
	if status != 200 || len(physician["specialities"].([]any)) != 2 {
		t.Fatalf("detail %d %s", status, data)
	}
	for _, tc := range []struct {
		q      string
		status int
	}{{"physicians/P1", 409}, {"physicians/UNKNOWN", 404}, {"physicians?type=CLIN", 400}, {"physicians?after=P1", 400}, {"physicians?unknown=value", 400}} {
		status, data := apiCall(t, s, "GET", prefix+tc.q, "", nil)
		if status != tc.status {
			t.Fatalf("%s %d %s", tc.q, status, data)
		}
	}
	failed := upload(t, s, "PHYS", bytes.NewReader(zipXML(t, "MediciCNAS_20261022.xml", physiciansFixture)))
	awaitImport(t, s, failed, "failed")
	status, data = apiCall(t, s, "GET", prefix+"physicians?source=CNAS", "", nil)
	var p flatPage
	json.Unmarshal(data, &p)
	if status != 200 || p.Total != 2 {
		t.Fatalf("failed import replaced previous physicians %s", data)
	}
	status, data = apiCall(t, s, "GET", prefix+"versions?type=PHYS", "", nil)
	if status != 200 || !bytes.Contains(data, []byte(`"physicians":4`)) {
		t.Fatalf("versions %d %s", status, data)
	}
}

func TestLocalPhysicianPair(t *testing.T) {
	c := testPostgresSettings(t)
	m, s := newTestModule(t, c)
	c.LocalDir = t.TempDir()
	physicianDir := filepath.Join(c.LocalDir, "getPhysicianCatalogues")
	if e := os.Mkdir(physicianDir, 0700); e != nil {
		t.Fatal(e)
	}
	scanner := newLocalScanner()
	write := func(name string) {
		t.Helper()
		if e := os.WriteFile(filepath.Join(physicianDir, name), []byte(physiciansFixture), 0600); e != nil {
			t.Fatal(e)
		}
	}
	scan := func() {
		t.Helper()
		if e := scanner.scan(context.Background(), m, c); e != nil {
			t.Fatal(e)
		}
	}
	db, e := m.database(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	count := func() int {
		var n int
		db.QueryRow(context.Background(), `SELECT count(*) FROM siui_data_imports WHERE catalogue_type='PHYS'`).Scan(&n)
		return n
	}
	write("MediciCNAS_20261021.xml")
	scan()
	scan()
	if count() != 0 {
		t.Fatal("partial pair was imported")
	}
	write("MediciOPSNAJ_20261021.xml")
	scan()
	if count() != 0 {
		t.Fatal("unstable pair was imported")
	}
	scan()
	var id string
	if e = db.QueryRow(context.Background(), `SELECT id::text FROM siui_data_imports WHERE catalogue_type='PHYS'`).Scan(&id); e != nil {
		t.Fatal(e)
	}
	awaitImport(t, s, id, "ready")
	scanner = newLocalScanner()
	scan()
	scan()
	if count() != 1 {
		t.Fatal("same physician pair reimported")
	}
	write("MediciCNAS_20261022.xml")
	scan()
	scan()
	if count() != 1 {
		t.Fatal("unpaired newer source replaced old pair")
	}
}

func TestRealPhysicianCatalogue(t *testing.T) {
	archive := os.Getenv("SIUI_DATA_REAL_PHYSICIANS")
	if archive == "" {
		t.Skip("set SIUI_DATA_REAL_PHYSICIANS to a real getPhysicianCatalogues ZIP")
	}
	_, s := newTestModule(t, testPostgresSettings(t))
	f, e := os.Open(archive)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	id := upload(t, s, "PHYS", f)
	job := awaitImport(t, s, id, "ready")
	t.Logf("real physician import: physicians=%v records=%v dates=%v statistics=%v", job["physicians"], job["records"], job["sourceDates"], job["statistics"])
	for _, q := range []string{"physicians?source=CNAS&name=popescu&specialtyName=cardio&limit=5", "simple-physicians?source=OPSNAJ&contractType=CLN&limit=5", "simple-physicians?stencil[]=A72003&stencil[]=D85112"} {
		status, data := apiCall(t, s, "GET", prefix+q, "", nil)
		var p flatPage
		json.Unmarshal(data, &p)
		if status != 200 || !p.Success || p.Total < 1 {
			t.Fatalf("%s %d %s", q, status, data)
		}
		if strings.HasPrefix(q, "simple-") {
			readFlatPage(t, data, status, p.Total, len(p.Rows))
		}
	}
}

func TestPhysicianWSMSync(t *testing.T) {
	m, s := newTestModule(t, testPostgresSettings(t))
	bridge := &fakeBridge{data: physicianArchive(t)}
	m.rt.(*testRuntime).bridge = bridge
	sync := func() string {
		t.Helper()
		status, data := apiCall(t, s, "POST", prefix+"bridge/sync", "application/json", strings.NewReader(`{"partnerCategory":"PHYS"}`))
		if status != 202 {
			t.Fatalf("sync %d %s", status, data)
		}
		var job map[string]any
		decodeData(t, data, &job)
		return job["importId"].(string)
	}
	awaitImport(t, s, sync(), "ready")
	awaitImport(t, s, sync(), "unchanged")
	bridge.mu.Lock()
	bridge.data = physicianArchiveDate(t, "2026-10-09")
	bridge.mu.Unlock()
	awaitImport(t, s, sync(), "ready") // OPSNAJ changed while maximum date (CNAS) stayed the same.
	bridge.mu.Lock()
	bridge.data = physicianArchive(t)
	bridge.mu.Unlock()
	awaitImport(t, s, sync(), "failed") // A lagging bridge must not regress one source.
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if len(bridge.operations) != 4 || bridge.operations[0] != "/api/siui/general/getPhysicianCatalogues" {
		t.Fatalf("wrong operation: %v", bridge.operations)
	}
}
