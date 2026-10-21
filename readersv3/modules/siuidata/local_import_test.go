package siuidata

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalDirectoryVersions(t *testing.T) {
	fixtureData := currentFixture()
	c := testPostgresSettings(t)
	m, server := newTestModule(t, c)
	c.LocalDir = t.TempDir()
	scanner := newLocalScanner()
	write := func(name, content string) {
		t.Helper()
		if e := os.WriteFile(filepath.Join(c.LocalDir, name), []byte(content), 0600); e != nil {
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
		t.Helper()
		var n int
		db.QueryRow(context.Background(), `SELECT count(*) FROM siui_data_imports WHERE source_kind='local'`).Scan(&n)
		return n
	}
	latest := func() string {
		t.Helper()
		var id string
		if e := db.QueryRow(context.Background(), `SELECT id::text FROM siui_data_imports WHERE source_kind='local' ORDER BY created_at DESC LIMIT 1`).Scan(&id); e != nil {
			t.Fatal(e)
		}
		return id
	}
	write("NomenclatoareClinice_20261001.xml", fixtureData)
	scan()
	if count() != 0 {
		t.Fatal("import started without stable observation")
	}
	scan()
	first := latest()
	job := awaitImport(t, server, first, "ready")
	if job["sourceDate"] != "2026-10-01" {
		t.Fatalf("source date %+v", job)
	}
	scan()
	if count() != 1 {
		t.Fatal("same file imported twice")
	}
	// Restarting the watcher still consults the persisted successful watermark.
	scanner = newLocalScanner()
	scan()
	scan()
	if count() != 1 {
		t.Fatal("watcher restart reimported old file")
	}
	write("NomenclatoareClinice_20260930.xml", fixtureData)
	scan()
	scan()
	if count() != 1 {
		t.Fatal("older catalogue imported")
	}
	write("NomenclatoareClinice_20261002.xml", strings.ReplaceAll(fixtureData, `"001"`, `"002"`))
	scan()
	scan()
	awaitImport(t, server, latest(), "ready")
	write("NomenclatoareParaclinice_20260901.xml", paraFixture)
	scan()
	scan()
	awaitImport(t, server, latest(), "ready")
	write("NomenclatoareClinice_20261003.xml", `<Catalogues><broken>`)
	scan()
	scan()
	awaitImport(t, server, latest(), "failed")
	before := count()
	scan()
	if count() != before {
		t.Fatal("unchanged failed file retried endlessly")
	}
	status, _ := apiCall(t, server, "GET", prefix+"services/002?type=CLIN", "", nil)
	if status != 200 {
		t.Fatal("local failure replaced active version")
	}
	write("NomenclatoareClinice_20261003.xml", strings.ReplaceAll(fixtureData, `"001"`, `"003"`))
	scan()
	scan()
	awaitImport(t, server, latest(), "ready")
	status, data := apiCall(t, server, "GET", prefix+"versions?partnerCategory=CLIN", "", nil)
	if status != 200 {
		t.Fatalf("versions %d %s", status, data)
	}
	var versions struct {
		Items []struct {
			Type       string `json:"type"`
			State      string `json:"state"`
			Statistics map[string]struct {
				Total  int `json:"total"`
				Active int `json:"active"`
			} `json:"statistics"`
		} `json:"rows"`
	}
	json.Unmarshal(data, &versions)
	if len(versions.Items) == 0 || versions.Items[0].Type != "CLIN" || versions.Items[0].State != "success" || versions.Items[0].Statistics["ClinicService"].Total != 3 || versions.Items[0].Statistics["ClinicService"].Active != 1 {
		t.Fatalf("statistics %s", data)
	}
	status, data = apiCall(t, server, "GET", prefix+"versions?partnerCategory=PHM", "", nil)
	if status != 200 {
		t.Fatalf("known unsupported category filter: %d %s", status, data)
	}
}

func TestLocalFilenameDates(t *testing.T) {
	for _, name := range []string{"NomenclatoareClinice_20261301.xml", "Other_20261001.xml", "NomenclatoareClinice_20260230.xml"} {
		if typ, _ := sourceFilename(name); typ != "" {
			t.Fatal("invalid filename accepted", name)
		}
	}
	if typ, day := sourceFilename("NomenclatoareParaclinice20261001.xml"); typ != "PARA" || day != "2026-10-01" {
		t.Fatal(typ, day)
	}
}
