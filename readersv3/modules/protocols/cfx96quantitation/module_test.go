package cfx96quantitation

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePlateViewWorkbook(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run -  Quantitation Plate View Results.xlsx")
	if err := writeWorkbook(path, map[string]string{
		"[content_types].xml": workbookContentTypesXML,
		"xl\\workbook.xml": `<?xml version="1.0" encoding="UTF-8"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="FAM" sheetId="1" r:id="rId1"/>
    <sheet name="Run Information" sheetId="2" r:id="rId2"/>
  </sheets>
</workbook>`,
		"xl\\_rels\\workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="/xl/worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="/xl/worksheets/sheet2.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="/xl/sharedstrings.xml"/>
</Relationships>`,
		"xl\\sharedstrings.xml": sharedStrings([]string{"1", "2", "3", "4", "A", "Content", "Unkn", "NTC", "Sample", "230070 dil comb", "ntc", "Cq", "33.40", "copy number", "Run Ended", "06/21/2024 13:17:45 UTC"}),
		"xl\\worksheets\\sheet1.xml": `<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1"><c r="C1" t="s"><v>0</v></c><c r="D1" t="s"><v>1</v></c><c r="E1" t="s"><v>2</v></c><c r="F1" t="s"><v>3</v></c><c r="G1"><v>5</v></c></row>
    <row r="2"><c r="A2" t="s"><v>4</v></c><c r="B2" t="s"><v>5</v></c><c r="F2" t="s"><v>6</v></c><c r="G2" t="s"><v>7</v></c></row>
    <row r="3"><c r="A3" t="s"><v>4</v></c><c r="B3" t="s"><v>8</v></c><c r="F3" t="s"><v>9</v></c><c r="G3" t="s"><v>10</v></c></row>
    <row r="4"><c r="A4" t="s"><v>4</v></c><c r="B4" t="s"><v>11</v></c><c r="F4" t="s"><v>12</v></c></row>
    <row r="5"><c r="A5" t="s"><v>4</v></c><c r="B5" t="s"><v>13</v></c></row>
  </sheetData>
</worksheet>`,
		"xl\\worksheets\\sheet2.xml": `<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1"><c r="A1" t="s"><v>14</v></c><c r="B1" t="s"><v>15</v></c></row>
  </sheetData>
</worksheet>`,
	}); err != nil {
		t.Fatal(err)
	}

	data, err := parseCFX96Quantitation(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Analytes) != 1 {
		t.Fatalf("expected 1 analyte, got %d", len(data.Analytes))
	}
	if len(data.SampleRecords) != 1 {
		t.Fatalf("expected 1 sample record, got %d", len(data.SampleRecords))
	}
	if len(data.QCRecords) != 1 {
		t.Fatalf("expected 1 qc record, got %d", len(data.QCRecords))
	}
	sample := data.SampleRecords[0]
	if sample.RunDate != "2024-06-21" {
		t.Fatalf("expected run date 2024-06-21, got %q", sample.RunDate)
	}
	if sample.Record.SampleID != "230070" {
		t.Fatalf("expected sample id 230070, got %q", sample.Record.SampleID)
	}
	if sample.Record.ResultValue != "33.40" {
		t.Fatalf("expected cq 33.40, got %q", sample.Record.ResultValue)
	}
	if !strings.Contains(sample.Record.Interpreted, "Interpretare=Detectat") {
		t.Fatalf("expected detected interpretation, got %q", sample.Record.Interpreted)
	}
	if sample.Record.Flags["well"] != "A04" {
		t.Fatalf("expected well A04, got %#v", sample.Record.Flags["well"])
	}
}

func TestParsePlateViewUsesSiblingCtResults(t *testing.T) {
	dir := t.TempDir()
	plateViewPath := filepath.Join(dir, "run -  Quantitation Plate View Results.xlsx")
	ctPath := filepath.Join(dir, "run -  Quantitation Ct Results.xlsx")

	if err := writeWorkbook(plateViewPath, map[string]string{
		"[content_types].xml": workbookContentTypesXML,
		"xl\\workbook.xml": `<?xml version="1.0" encoding="UTF-8"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="FAM" sheetId="1" r:id="rId1"/>
    <sheet name="Run Information" sheetId="2" r:id="rId2"/>
  </sheets>
</workbook>`,
		"xl\\_rels\\workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="/xl/worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="/xl/worksheets/sheet2.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="/xl/sharedstrings.xml"/>
</Relationships>`,
		"xl\\sharedstrings.xml": sharedStrings([]string{"4", "A", "Content", "Unkn", "Sample", "230070 dil comb", "Cq", "Run Ended", "06/21/2024 13:17:45 UTC"}),
		"xl\\worksheets\\sheet1.xml": `<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1"><c r="F1" t="s"><v>0</v></c></row>
    <row r="2"><c r="A2" t="s"><v>1</v></c><c r="B2" t="s"><v>2</v></c><c r="F2" t="s"><v>3</v></c></row>
    <row r="3"><c r="A3" t="s"><v>1</v></c><c r="B3" t="s"><v>4</v></c><c r="F3" t="s"><v>5</v></c></row>
    <row r="4"><c r="A4" t="s"><v>1</v></c><c r="B4" t="s"><v>6</v></c></row>
  </sheetData>
</worksheet>`,
		"xl\\worksheets\\sheet2.xml": `<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1"><c r="A1" t="s"><v>7</v></c><c r="B1" t="s"><v>8</v></c></row>
  </sheetData>
</worksheet>`,
	}); err != nil {
		t.Fatal(err)
	}

	if err := writeWorkbook(ctPath, map[string]string{
		"[content_types].xml": workbookContentTypesXML,
		"xl\\workbook.xml": `<?xml version="1.0" encoding="UTF-8"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets><sheet name="0" sheetId="1" r:id="rId1"/></sheets>
</workbook>`,
		"xl\\_rels\\workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="/xl/worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="/xl/sharedstrings.xml"/>
</Relationships>`,
		"xl\\sharedstrings.xml": sharedStrings([]string{"Well", "Fluor", "Target", "Content", "Sample", "Cq", "A04", "FAM", "Virus X", "Unkn", "230070 dil comb", "31.7"}),
		"xl\\worksheets\\sheet1.xml": `<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1">
      <c r="B1" t="s"><v>0</v></c><c r="C1" t="s"><v>1</v></c><c r="D1" t="s"><v>2</v></c><c r="E1" t="s"><v>3</v></c><c r="F1" t="s"><v>4</v></c><c r="H1" t="s"><v>5</v></c>
    </row>
    <row r="2">
      <c r="B2" t="s"><v>6</v></c><c r="C2" t="s"><v>7</v></c><c r="D2" t="s"><v>8</v></c><c r="E2" t="s"><v>9</v></c><c r="F2" t="s"><v>10</v></c><c r="H2" t="s"><v>11</v></c>
    </row>
  </sheetData>
</worksheet>`,
	}); err != nil {
		t.Fatal(err)
	}

	data, err := parseCFX96Quantitation(plateViewPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.SampleRecords) != 1 {
		t.Fatalf("expected 1 sample record, got %d", len(data.SampleRecords))
	}
	sample := data.SampleRecords[0]
	if sample.Record.AnalyteTag != "VIRUS_X" {
		t.Fatalf("expected analyte VIRUS_X, got %q", sample.Record.AnalyteTag)
	}
	if sample.Record.ResultValue != "31.7" {
		t.Fatalf("expected fallback cq 31.7, got %q", sample.Record.ResultValue)
	}
	if sample.Record.Flags["ct_results_match"] != filepath.Base(ctPath) {
		t.Fatalf("expected sibling source %q, got %#v", filepath.Base(ctPath), sample.Record.Flags["ct_results_match"])
	}
}

func writeWorkbook(path string, files map[string]string) error {
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fh.Close()

	zw := zip.NewWriter(fh)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(content)); err != nil {
			return err
		}
	}
	return zw.Close()
}

func sharedStrings(values []string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	for _, value := range values {
		sb.WriteString(`<si><t>`)
		sb.WriteString(value)
		sb.WriteString(`</t></si>`)
	}
	sb.WriteString(`</sst>`)
	return sb.String()
}

const workbookContentTypesXML = `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="xml" ContentType="application/xml"/>
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
</Types>`
