package siuidata

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const cimFixture = `<Cim10s>
<Cim10 code="5" name="Tulburări mentale" entityLevel="0"/>
<Cim10 code="5_111" name="Tulburări nevrotice și somatoforme" entityLevel="1" parentCode="5"/>
<Cim10 code="5_111_F45.1" name="Tulburare somatoforma nediferentiata,-1,0,0,0,0,0" entityLevel="2" parentCode="5_111"/>
<Cim10 code="5_111_F45.2" name="Tulburare somatoforma, alta&quot;,-1,0,0,0,0," entityLevel="2" parentCode="5_111"/>
<Cim10 code="13" name="Musculoscheletal" entityLevel="0"/>
<Cim10 code="13_202" name="Disc intervertebral" entityLevel="1" parentCode="13"/>
<Cim10 code="13_202_M51.1+" name="Tulburari ale discului cu radiculopatie (G55.1*),0,-1,0,0,0,0" entityLevel="2" parentCode="13_202"/>
<Cim10 code="14_160_N51.0*" name="Afectiuni ale prostatei,0,0,-1,0,0,0" entityLevel="2" parentCode="14_160"/>
<Cim10 code="19_75_T87.4" name="Infectie a bontului,0,0,0,-1,0,0" entityLevel="2" parentCode="19_75"/>
<Cim10 code="20_59_Y90.0" name="Nivelul alcoolului,0,0,0,0,-1,0" entityLevel="2" parentCode="20_59"/>
<Cim10 code="19_181_T90.0" name="Sechele ale leziunilor,0,0,0,0,0,-1" entityLevel="2" parentCode="19_181"/>
<Cim10 code="20_1_Y88.3" name="Sechele ale procedurilor&quot;,0,0,0,0,0,-" entityLevel="2" parentCode="20_1"/>
<Cim10 code="22_274_U07.1" name="COVID-19, virus identificat" entityLevel="2" parentCode="22_274"/>
</Cim10s>`

func cimXML() string {
	return strings.Replace(currentFixture(), "</Catalogues>", cimFixture+"</Catalogues>", 1)
}

func TestCim10NameParsing(t *testing.T) {
	for _, tc := range []struct {
		raw, name, status string
		marked            int
	}{
		{"Tulburare,-1,0,0,0,0,0", "Tulburare", "complete", 0},
		{`Nume, cu virgula",0,0,0,0,-1,`, "Nume, cu virgula", "incomplete", 4},
		{`Sechele",0,0,0,0,0,-`, "Sechele", "incomplete", -1},
		{"COVID-19, virus identificat", "COVID-19, virus identificat", "absent", -1},
		{"Nume,1,0,0,0,0,0", "Nume,1,0,0,0,0,0", "absent", -1},
	} {
		p := parseCim10Name(tc.raw)
		if p.Name != tc.name || p.Status != tc.status {
			t.Fatalf("%q: %+v", tc.raw, p)
		}
		if tc.marked >= 0 && (p.Flags[tc.marked] == nil || !*p.Flags[tc.marked]) {
			t.Fatalf("lost active flag: %+v", p)
		}
		if tc.status == "incomplete" && p.Flags[5] != nil {
			t.Fatal("unknown last flag became false")
		}
	}
}

func TestCim10ImportSearchAndTree(t *testing.T) {
	m, s := newTestModule(t, testPostgresSettings(t))
	awaitImport(t, s, upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "clin.xml", cimXML()))), "ready")
	for _, tc := range []struct {
		query string
		total int
		name  string
	}{
		{"cim10", 13, ""},
		{"cim10?name=somatoforma", 2, ""},
		{"cim10?nameAll[]=NEDIFERENTIATA&nameAll[]=somatoforma", 1, "Tulburare somatoforma nediferentiata"},
		{"cim10?q[]=nediferentiata&q[]=prostatei", 2, ""},
		{"cim10?qAll[]=virus&qAll[]=identificat", 1, "COVID-19, virus identificat"},
		{"cim10?isDagger=true", 1, "Tulburari ale discului cu radiculopatie (G55.1*) +"},
		{"cim10?isAsterisk=true", 1, "Afectiuni ale prostatei *"},
		{"cim10?isInjury=true", 1, "Infectie a bontului"},
		{"cim10?isExternalCause=true", 1, "Nivelul alcoolului"},
		{"cim10?isSequela=true", 1, "Sechele ale leziunilor"},
		{"cim10?isNormal=true&isDagger=true", 0, ""},
		{"cim10?isNormal[]=true&isNormal[]=false", 8, ""},
		{"cim10?flagStatus=absent", 5, ""},
		{"cim10?flagStatus=incomplete", 2, ""},
		{"cim10?parentCode=5", 1, "Tulburări nevrotice și somatoforme"},
		{"cim10?parentCode[]=5_111&parentCode[]=13_202", 3, ""},
		{"cim10?entityLevel=0", 2, ""},
		{"cim10?entityLevel[]=0&entityLevel[]=1", 4, ""},
		{"cim10?roots=true", 8, ""},
		{"cim10?name=somatoforma&limit=1&start=1", 2, "Tulburare somatoforma, alta"},
		{"cim10?code=5_111_F45.1", 1, "Tulburare somatoforma nediferentiata"},
	} {
		status, data := apiCall(t, s, "GET", prefix+tc.query, "", nil)
		var page struct {
			Success bool        `json:"success"`
			Total   int         `json:"totalCount"`
			Rows    []cim10Item `json:"rows"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			t.Fatal(err)
		}
		if status != 200 || !page.Success || page.Total != tc.total {
			t.Fatalf("%s: %d %s; expected %d", tc.query, status, data, tc.total)
		}
		if tc.name != "" && (len(page.Rows) == 0 || page.Rows[0].Name != tc.name) {
			t.Fatalf("%s: %s", tc.query, data)
		}
	}
	for _, tc := range []struct {
		query         string
		total, leaves int
	}{
		{"name=somatoforma", 2, 2}, {"name=somatoforma&limit=1&start=1", 2, 1}, {"parentCode=5_111&isNormal=true", 2, 2},
	} {
		status, data := apiCall(t, s, "GET", prefix+"cim10/tree?"+tc.query, "", nil)
		var page struct {
			Total int              `json:"totalCount"`
			Rows  []*cim10TreeItem `json:"rows"`
		}
		json.Unmarshal(data, &page)
		if status != 200 || page.Total != tc.total || len(page.Rows) != 1 || page.Rows[0].Code != "5" || len(page.Rows[0].Children) != 1 || page.Rows[0].Children[0].Code != "5_111" || len(page.Rows[0].Children[0].Children) != tc.leaves {
			t.Fatalf("tree %s: %d %s", tc.query, status, data)
		}
	}
	status, data := apiCall(t, s, "GET", prefix+"cim10/tree?isAsterisk=true", "", nil)
	var tree struct {
		Rows []*cim10TreeItem `json:"rows"`
	}
	json.Unmarshal(data, &tree)
	if status != 200 || len(tree.Rows) != 1 || tree.Rows[0].Code != "14_160_N51.0*" {
		t.Fatalf("orphan lost: %s", data)
	}
	for _, q := range []string{"entityLevel=NaN", "isDagger=maybe", "roots[]=true&roots[]=false", "unknown=yes", "name=[1]"} {
		if status, data := apiCall(t, s, "GET", prefix+"cim10?"+q, "", nil); status != 400 {
			t.Fatalf("invalid %s: %d %s", q, status, data)
		}
	}
	// A cyclic refresh cannot replace the active catalogue or its old tree.
	bad := strings.Replace(cimXML(), `code="5" name="Tulburări mentale" entityLevel="0"`, `code="5" name="Tulburări mentale" entityLevel="0" parentCode="5_111"`, 1)
	awaitImport(t, s, upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "bad.xml", bad))), "failed")
	if status, data := apiCall(t, s, "GET", prefix+"cim10?name=nediferentiata", "", nil); status != 200 || !bytes.Contains(data, []byte(`"totalCount":1`)) {
		t.Fatalf("old tree lost: %s", data)
	}
	db, err := m.database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var name, statusFlag string
	var last *bool
	if err = db.QueryRow(context.Background(), `SELECT name,flag_status,is_sequela FROM siui_data_cim10 WHERE code='5_111_F45.2'`).Scan(&name, &statusFlag, &last); err != nil {
		t.Fatal(err)
	}
	if name != "Tulburare somatoforma, alta" || statusFlag != "incomplete" || last != nil {
		t.Fatal("incomplete flags lost")
	}
}

func TestCim10BackfillExistingActiveImport(t *testing.T) {
	cfg := testPostgresSettings(t)
	m, s := newTestModule(t, cfg)
	awaitImport(t, s, upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "clin.xml", cimXML()))), "ready")
	db, err := m.database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(context.Background(), `DROP TABLE siui_data_cim10;DELETE FROM siui_data_schema_version WHERE version>=5`); err != nil {
		t.Fatal(err)
	}
	if err = ensurePostgresSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if status, data := apiCall(t, s, "GET", prefix+"cim10?isDagger=true", "", nil); status != 200 || !bytes.Contains(data, []byte(`"totalCount":1`)) {
		t.Fatalf("upgrade did not populate tree: %d %s", status, data)
	}
}

func TestRealCim10Catalogue(t *testing.T) {
	path := os.Getenv("SIUI_DATA_REAL_CIM10_XML")
	if path == "" {
		t.Skip("set SIUI_DATA_REAL_CIM10_XML to test the actual CIM10 section")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var section strings.Builder
	section.WriteString("<Cim10s>")
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 65536), 16<<20)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "<Cim10 ") {
			section.WriteString(line)
			count++
		}
	}
	if err = scanner.Err(); err != nil {
		t.Fatal(err)
	}
	section.WriteString("</Cim10s>")
	if count < 16000 {
		t.Fatalf("expected full real catalogue, got %d", count)
	}
	m, s := newTestModule(t, testPostgresSettings(t))
	xml := strings.Replace(currentFixture(), "</Catalogues>", section.String()+"</Catalogues>", 1)
	awaitImport(t, s, upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "clin.xml", xml))), "ready")
	status, data := apiCall(t, s, "GET", prefix+"cim10?limit=1", "", nil)
	var page struct {
		Total int `json:"totalCount"`
	}
	json.Unmarshal(data, &page)
	if status != 200 || page.Total != count {
		t.Fatalf("%d %s expected %d", status, data, count)
	}
	for _, q := range []string{"cim10?code=5_111_F45.1", "cim10/tree?nameAll[]=somatoforma&nameAll[]=nediferentiata", "cim10?isDagger=true", "cim10?isAsterisk=true", "cim10?parentCode=5_111"} {
		if status, data := apiCall(t, s, "GET", prefix+q, "", nil); status != 200 {
			t.Fatalf("%s: %d %s", q, status, data)
		}
	}
	db, err := m.database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var dirty, incomplete int
	if err = db.QueryRow(context.Background(), `SELECT count(*) FILTER(WHERE name ~ ',(-1|0),(-1|0),(-1|0),(-1|0),(-1|0),(-1|0|-)?$'),count(*) FILTER(WHERE flag_status='incomplete' AND is_sequela IS NULL) FROM siui_data_cim10`).Scan(&dirty, &incomplete); err != nil {
		t.Fatal(err)
	}
	if dirty != 0 || incomplete == 0 {
		t.Fatalf("real catalogue names/unknown flags: dirty=%d incomplete=%d", dirty, incomplete)
	}
	// Exercise upgrade of a real active catalogue as well as the fresh importer.
	if _, err = db.Exec(context.Background(), `DROP TABLE siui_data_cim10;DELETE FROM siui_data_schema_version WHERE version>=5`); err != nil {
		t.Fatal(err)
	}
	if err = ensurePostgresSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var projected int
	if err = db.QueryRow(context.Background(), `SELECT count(*) FROM siui_data_cim10`).Scan(&projected); err != nil || projected != count {
		t.Fatalf("real backfill: %d %v", projected, err)
	}
	t.Logf("Imported and backfilled all %d real CIM10 records", count)
}
