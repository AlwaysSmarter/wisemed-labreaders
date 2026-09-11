package astm

import (
	"fmt"
	"path/filepath"
	"testing"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
	"wisemed-labreaders/readersv3/modules/storage/sqlite"
)

// Regression using all 27 result codes and values from the reported H500 run.
func TestYumizenDistinctAnalytesThroughStorage(t *testing.T) {
	tags := []string{"PCT", "MCV", "NEU#", "P-LCR", "NEU%", "RDW-CV", "RBC", "MPV", "P-LCC", "MON#", "PLT", "WBC", "LIC%", "MON%", "LIC#", "LYM#", "PDW", "HGB", "LYM%", "RDW-SD", "BAS%", "BAS#", "MCH", "MCHC", "HCT", "EOS#", "EOS%"}
	values := []string{"0.33", "95.7", "13.64", "12.3", "86.9", "12.3", "5.16", "7.7", "54", "0.22", "436", "16.33", "4.1", "1.4", "0.64", "0.94", "9.0", "16.8", "6.0", "45.4", "0.9", "0.14", "32.5", "34.0", "49.4", "0.75", "4.8"}
	payload := "H|\\^&|||H500^712YOXH01195^2.1.0.1c\rO|1|23455||^^^DIF|R|20260909115507\r"
	for i, tag := range tags {
		payload += fmt.Sprintf("R|%d|^^^%s|%s\r", i+1, tag, values[i])
	}
	payload += "L|1|N\r"
	cfg := tcpConfig{SampleIDPaths: []string{"O.2.1"}, ResultIDPaths: []string{"R.2.4"}, ResultValue: []string{"R.3.1"}}
	items := parseBatch(parseRecords(payload), cfg)
	if len(items) != 27 {
		t.Fatalf("results=%d", len(items))
	}
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "reader.db"))
	if err != nil {
		t.Fatal(err)
	}
	ids := map[int64]string{}
	analyteIDs := map[int64]string{}
	expected := map[string]string{}
	for i, item := range items {
		tag := normalizeTag(mappedAnalyte(nil, item.AnalyteTag))
		if tag != tags[i] || item.Value != values[i] {
			t.Fatalf("result %d: %+v", i, item)
		}
		definition, err := store.SaveAnalyte(coremodel.Analyte{Tag: tag, Name: tag})
		if err != nil {
			t.Fatal(err)
		}
		if previous, ok := analyteIDs[definition.ID]; ok {
			t.Fatalf("definition %s overwrote %s", tag, previous)
		}
		analyteIDs[definition.ID] = tag
		_, analysis, _, err := store.RecordImportedResult("2026-09-09", 1, coremodel.ImportedRecord{SampleID: item.Order.SampleID, AnalyteTag: tag, ResultValue: item.Value}, "astm")
		if err != nil {
			t.Fatal(err)
		}
		if previous, ok := ids[analysis.ID]; ok {
			t.Fatalf("%s overwrote %s", tag, previous)
		}
		ids[analysis.ID] = tag
		expected[tag] = item.Value
	}
	for id, tag := range ids {
		a, err := store.GetOrderAnalysis(id)
		if err != nil || a.AnalyteTag != tag || a.ResultValue != expected[tag] {
			t.Fatalf("saved %s: %+v %v", tag, a, err)
		}
	}
	if got := normalizeTag(mappedAnalyte(map[string]string{"NEU#": "ABS#"}, "NEU#")); got != "ABS#" {
		t.Fatal(got)
	}
}
