package cfx96quantitation

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
	"wisemed-labreaders/readersv3/modules/protocols/fileimportbase"
)

const (
	defaultDetectedMaxCq = 40.0
	runInfoSheetName     = "Run Information"
)

func New() module.Module {
	return fileimportbase.New(fileimportbase.Spec{
		ID:                 "protocol-cfx96-quantitation",
		MenuID:             "protocol-cfx96-quantitation",
		MenuLabel:          "Protocol CFX-96 Quantitation",
		MenuPath:           "/settings/protocol/cfx96-quantitation",
		MenuOrder:          52,
		ProtocolMeta:       "cfx96-quantitation",
		ResponseProtocol:   "CFX96_QUANTITATION_XLSX",
		AnalyteDescription: "Auto-generated from CFX-96 Quantitation XLSX exports",
		QCTargetNotes:      "Creat automat din import QC CFX-96. Definiti media si 1SD in Setari QC daca folositi controale dedicate.",
		Parse:              parseCFX96Quantitation,
	})
}

func parseCFX96Quantitation(path string, rt module.Runtime) (fileimportbase.ImportData, error) {
	book, err := openWorkbook(path)
	if err != nil {
		return fileimportbase.ImportData{}, err
	}
	defer book.Close()

	sheetNames := book.SheetNames()
	if len(sheetNames) == 0 {
		return fileimportbase.ImportData{}, errors.New("xlsx parse failed: workbook has no sheets")
	}
	if !looksLikePlateViewWorkbook(sheetNames) {
		return fileimportbase.ImportData{}, fmt.Errorf("xlsx parse failed: unsupported sheet layout %v", sheetNames)
	}

	threshold := detectedMaxCq(rt)
	existingThresholds := loadAnalyteThresholds(rt)
	ctFallback, _ := loadSiblingCtResults(path)
	runDate := extractRunDate(book)
	sourceFile := filepath.Base(path)

	analytes := map[string]fileimportbase.AnalyteDef{}
	samples := []fileimportbase.SampleRecord{}
	qcByKey := map[string]fileimportbase.QCRecord{}

	for _, fluor := range sheetNames {
		if strings.EqualFold(strings.TrimSpace(fluor), runInfoSheetName) {
			continue
		}
		rows, err := book.ReadRows(fluor)
		if err != nil {
			return fileimportbase.ImportData{}, err
		}
		if len(rows) < 4 {
			continue
		}

		colWells := plateColumns(rows[0])
		for idx := 1; idx+2 < len(rows); idx++ {
			contentRow := rows[idx]
			sampleRow := rows[idx+1]
			cqRow := rows[idx+2]
			if !strings.EqualFold(cell(sampleRow, "B"), "Sample") {
				continue
			}
			rowLetter := strings.ToUpper(strings.TrimSpace(cell(sampleRow, "A")))
			if rowLetter == "" {
				continue
			}
			for _, col := range sortedPlateColumns(colWells) {
				well := rowLetter + colWells[col]
				sampleRaw := strings.TrimSpace(cell(sampleRow, col))
				content := strings.TrimSpace(cell(contentRow, col))
				cqRaw := strings.TrimSpace(cell(cqRow, col))
				fallback := ctFallback[keyFor(fluor, well)]
				if sampleRaw == "" && content == "" && cqRaw == "" && fallback.Sample == "" && fallback.Content == "" && fallback.Cq == "" {
					continue
				}
				if sampleRaw == "" {
					sampleRaw = fallback.Sample
				}
				if content == "" {
					content = fallback.Content
				}
				if cqRaw == "" {
					cqRaw = fallback.Cq
				}
				targetName := firstNonEmpty(fallback.Target, fluor)
				tag := normalizeTag(targetName)
				if tag == "" {
					continue
				}
				analyteThreshold := threshold
				if value, ok := existingThresholds[tag]; ok && value > 0 {
					analyteThreshold = value
				}

				analytes[tag] = fileimportbase.AnalyteDef{
					Tag:              tag,
					Code:             tag,
					Name:             targetName,
					ResultType:       "numeric",
					ResultFormatting: "raw",
					ResultWeighting:  1,
					Unit:             "Ct",
					ProtocolOptions: map[string]interface{}{
						"channel":         fluor,
						"detected_max_cq": analyteThreshold,
					},
				}

				sampleID := deriveSampleID(sampleRaw, well)
				flags := map[string]interface{}{
					"source":           "cfx96_quantitation_xlsx",
					"channel":          fluor,
					"well":             well,
					"content":          content,
					"sample_raw":       sampleRaw,
					"target_name":      targetName,
					"detected_max_cq":  analyteThreshold,
					"ct_source":        sourceFile,
					"plate_source":     sourceFile,
					"source_file":      sourceFile,
					"raw_cq":           cqRaw,
					"ct_results_match": fallback.Source,
				}

				resultValue := normalizeCq(cqRaw)
				qualitative := qualitativeResult(resultValue, analyteThreshold)
				interpreted := buildInterpreted(targetName, cqRaw, qualitative, analyteThreshold, well, content)

				if isQCContent(content, sampleRaw) {
					key := detectQCKey(content, sampleRaw, fluor)
					record := qcByKey[key]
					if len(record.Results) == 0 {
						record = fileimportbase.QCRecord{
							RunDate:      runDate,
							ControlLabel: firstNonEmpty(sampleRaw, content, fluor),
							ControlLevel: detectQCLevel(content, sampleRaw),
							LotNo:        firstNonEmpty(sampleRaw, content, fluor),
							FileID:       firstNonEmpty(sampleID, well, fluor),
							Status:       "completed",
							Meta: map[string]interface{}{
								"channel": fluor,
								"well":    well,
								"content": content,
							},
						}
					}
					record.Results = append(record.Results, fileimportbase.QCResult{
						AnalyteTag:  tag,
						AnalyteName: targetName,
						ResultValue: resultValue,
						RawValue:    strings.TrimSpace(cqRaw),
						Interpreted: interpreted,
						Unit:        "Ct",
						Flags:       cloneMap(flags),
					})
					qcByKey[key] = record
					continue
				}

				samples = append(samples, fileimportbase.SampleRecord{
					RunDate: runDate,
					Record: coremodel.ImportedRecord{
						SampleID:    sampleID,
						FileID:      sampleID,
						PatientID:   sampleID,
						PatientName: sampleRaw,
						AnalyteTag:  tag,
						AnalyteName: targetName,
						ResultValue: resultValue,
						RawValue:    strings.TrimSpace(cqRaw),
						Interpreted: interpreted,
						Flags:       flags,
						Unit:        "Ct",
						Meta:        map[string]interface{}{},
					},
				})
			}
		}
	}

	qcRecords := make([]fileimportbase.QCRecord, 0, len(qcByKey))
	for _, item := range qcByKey {
		qcRecords = append(qcRecords, item)
	}
	analyteList := make([]fileimportbase.AnalyteDef, 0, len(analytes))
	for _, item := range analytes {
		analyteList = append(analyteList, item)
	}

	data := fileimportbase.ImportData{
		SampleRecords: samples,
		QCRecords:     qcRecords,
		Analytes:      analyteList,
	}
	if len(data.SampleRecords) == 0 && len(data.QCRecords) == 0 {
		return fileimportbase.ImportData{}, errors.New("xlsx parse failed: no usable Sample/Cq rows found in Plate View export")
	}
	fileimportbase.SortImportData(&data)
	return data, nil
}

func detectedMaxCq(rt module.Runtime) float64 {
	if rt == nil {
		return defaultDetectedMaxCq
	}
	settings := rt.ModuleSettings("protocol-cfx96-quantitation")
	value := strings.TrimSpace(fmt.Sprint(settings["detected_max_cq"]))
	if value == "" || value == "<nil>" {
		return defaultDetectedMaxCq
	}
	parsed, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", "."), 64)
	if err != nil || parsed <= 0 {
		return defaultDetectedMaxCq
	}
	return parsed
}

func looksLikePlateViewWorkbook(sheetNames []string) bool {
	hasRunInfo := false
	hasFluor := false
	for _, name := range sheetNames {
		switch strings.TrimSpace(strings.ToLower(name)) {
		case strings.ToLower(runInfoSheetName):
			hasRunInfo = true
		default:
			hasFluor = true
		}
	}
	return hasRunInfo && hasFluor
}

func loadSiblingCtResults(path string) (map[string]ctResultEntry, error) {
	candidates := []string{
		strings.Replace(path, "Plate View Results.xlsx", "Cq Results.xlsx", 1),
		strings.Replace(path, "Plate View Results.xlsx", "Ct Results.xlsx", 1),
	}
	seen := map[string]struct{}{}
	for _, sibling := range candidates {
		if sibling == path {
			continue
		}
		if _, ok := seen[sibling]; ok {
			continue
		}
		seen[sibling] = struct{}{}
		if _, err := os.Stat(sibling); err != nil {
			continue
		}
		book, err := openWorkbook(sibling)
		if err != nil {
			return nil, err
		}
		defer book.Close()
		return parseCtResults(book, filepath.Base(sibling))
	}
	return map[string]ctResultEntry{}, nil
}

func parseCtResults(book *workbook, source string) (map[string]ctResultEntry, error) {
	sheets := book.SheetNames()
	if len(sheets) == 0 {
		return map[string]ctResultEntry{}, nil
	}
	rows, err := book.ReadRows(sheets[0])
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return map[string]ctResultEntry{}, errors.New("xlsx parse failed: Cq/Ct Results workbook has too few rows")
	}
	header := headerIndex(rows[0])
	required := []string{"well", "fluor", "sample"}
	for _, item := range required {
		if _, ok := header[item]; !ok {
			return map[string]ctResultEntry{}, nil
		}
	}

	out := map[string]ctResultEntry{}
	for _, row := range rows[1:] {
		well := strings.ToUpper(strings.TrimSpace(cellByIndex(row, header["well"])))
		fluor := strings.TrimSpace(cellByIndex(row, header["fluor"]))
		if well == "" || fluor == "" {
			continue
		}
		out[keyFor(fluor, well)] = ctResultEntry{
			Well:    well,
			Fluor:   fluor,
			Target:  strings.TrimSpace(cellByIndex(row, header["target"])),
			Content: strings.TrimSpace(cellByIndex(row, header["content"])),
			Sample:  strings.TrimSpace(cellByIndex(row, header["sample"])),
			Cq:      strings.TrimSpace(cellByIndex(row, header["cq"])),
			Source:  source,
		}
	}
	return out, nil
}

func extractRunDate(book *workbook) string {
	rows, err := book.ReadRows(runInfoSheetName)
	if err != nil {
		return ""
	}
	for _, row := range rows {
		label := strings.TrimSpace(cellByIndex(row, 0))
		value := normalizeRunInfoTime(cellByIndex(row, 1))
		switch {
		case strings.EqualFold(label, "Run Ended"):
			if runDate := fileimportbase.ParseDate(value); runDate != "" {
				return runDate
			}
		case strings.EqualFold(label, "Run Started"):
			if runDate := fileimportbase.ParseDate(value); runDate != "" {
				return runDate
			}
		}
	}
	return ""
}

func normalizeRunInfoTime(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, " UTC")
	return strings.TrimSpace(value)
}

func plateColumns(headerRow map[string]string) map[string]string {
	out := map[string]string{}
	for col, value := range headerRow {
		if !isPlateColumn(col) {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		num, err := strconv.Atoi(value)
		if err != nil || num <= 0 {
			continue
		}
		out[col] = fmt.Sprintf("%02d", num)
	}
	return out
}

func sortedPlateColumns(cols map[string]string) []string {
	keys := make([]string, 0, len(cols))
	for key := range cols {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int { return columnIndex(a) - columnIndex(b) })
	return keys
}

func isPlateColumn(col string) bool {
	return columnIndex(col) >= columnIndex("C")
}

func columnIndex(col string) int {
	col = strings.ToUpper(strings.TrimSpace(col))
	if col == "" {
		return -1
	}
	total := 0
	for _, r := range col {
		if r < 'A' || r > 'Z' {
			return -1
		}
		total = total*26 + int(r-'A'+1)
	}
	return total - 1
}

func headerIndex(row map[string]string) map[string]int {
	out := map[string]int{}
	for col, value := range row {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		out[value] = columnIndex(col)
	}
	return out
}

func deriveSampleID(sampleRaw, well string) string {
	fields := strings.Fields(strings.TrimSpace(sampleRaw))
	if len(fields) > 0 {
		return strings.TrimSpace(fields[0])
	}
	return strings.TrimSpace(well)
}

func normalizeTag(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func normalizeCq(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	if lower == "n/a" || lower == "no cq" || lower == "no ct" || lower == "undetermined" {
		return ""
	}
	normalized := fileimportbase.NormalizeNumber(value)
	parsed, err := strconv.ParseFloat(normalized, 64)
	if err != nil || parsed <= 0 {
		return ""
	}
	return normalized
}

func qualitativeResult(cq string, threshold float64) string {
	if cq == "" {
		return "Nedetectat"
	}
	parsed, err := strconv.ParseFloat(cq, 64)
	if err != nil {
		return "Nedetectat"
	}
	if threshold <= 0 || parsed <= threshold {
		return "Detectat"
	}
	return "Nedetectat"
}

func buildInterpreted(targetName, cqRaw, qualitative string, threshold float64, well, content string) string {
	parts := []string{"Analit=" + strings.TrimSpace(targetName)}
	if strings.TrimSpace(well) != "" {
		parts = append(parts, "Well="+strings.TrimSpace(well))
	}
	if value := normalizeCq(cqRaw); value != "" {
		parts = append(parts, "Ct="+value)
	}
	if strings.TrimSpace(content) != "" {
		parts = append(parts, "Tip="+strings.TrimSpace(content))
	}
	parts = append(parts, "Interpretare="+qualitative)
	if threshold > 0 {
		parts = append(parts, "PragCt<="+strconv.FormatFloat(threshold, 'f', -1, 64))
	}
	return strings.Join(parts, " · ")
}

func isQCContent(content, sample string) bool {
	text := strings.ToUpper(strings.TrimSpace(content + " " + sample))
	return strings.Contains(text, "NTC") || strings.Contains(text, "POS CTRL") || strings.Contains(text, "POSITIVE") || strings.Contains(text, "NEGATIVE CTRL") || strings.Contains(text, "NEG CTRL")
}

func detectQCLevel(content, sample string) string {
	text := strings.ToUpper(strings.TrimSpace(content + " " + sample))
	switch {
	case strings.Contains(text, "NTC"), strings.Contains(text, "NEG"):
		return "negativ"
	case strings.Contains(text, "POS"), strings.Contains(text, "PC"):
		return "pozitiv"
	default:
		return "QC"
	}
}

func detectQCKey(content, sample, fluor string) string {
	level := detectQCLevel(content, sample)
	return level + "|" + firstNonEmpty(sample, content, fluor)
}

func keyFor(fluor, well string) string {
	return strings.ToUpper(strings.TrimSpace(fluor)) + "|" + strings.ToUpper(strings.TrimSpace(well))
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cell(row map[string]string, col string) string {
	return strings.TrimSpace(row[strings.ToUpper(strings.TrimSpace(col))])
}

func cellByIndex(row map[string]string, idx int) string {
	for col, value := range row {
		if columnIndex(col) == idx {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type ctResultEntry struct {
	Well    string
	Fluor   string
	Target  string
	Content string
	Sample  string
	Cq      string
	Source  string
}

type analyteThresholdStore interface {
	ListAnalytes() ([]coremodel.Analyte, error)
}

func loadAnalyteThresholds(rt module.Runtime) map[string]float64 {
	out := map[string]float64{}
	if rt == nil {
		return out
	}
	service, ok := rt.Service("storage")
	if !ok {
		return out
	}
	store, ok := service.(analyteThresholdStore)
	if !ok {
		return out
	}
	items, err := store.ListAnalytes()
	if err != nil {
		return out
	}
	for _, item := range items {
		tag := normalizeTag(item.Tag)
		if tag == "" {
			continue
		}
		if value, ok := parseThresholdValue(item.ProtocolOptions["detected_max_cq"]); ok {
			out[tag] = value
		}
	}
	return out
}

func parseThresholdValue(raw interface{}) (float64, bool) {
	value := strings.TrimSpace(fmt.Sprint(raw))
	if value == "" || value == "<nil>" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", "."), 64)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

type workbook struct {
	rc      *zip.ReadCloser
	nameMap map[string]*zip.File
	sheets  []sheetRef
	strings []string
}

type sheetRef struct {
	Name string
	Path string
}

type workbookXML struct {
	Sheets []sheetXML `xml:"sheets>sheet"`
}

type sheetXML struct {
	Name string `xml:"name,attr"`
	ID   string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
}

type relsXML struct {
	Relationships []relXML `xml:"Relationship"`
}

type relXML struct {
	ID     string `xml:"Id,attr"`
	Target string `xml:"Target,attr"`
}

type sharedStringsXML struct {
	Items []sharedStringItem `xml:"si"`
}

type sharedStringItem struct {
	Text string `xml:"t"`
	Runs []run  `xml:"r"`
}

type run struct {
	Text string `xml:"t"`
}

type worksheetXML struct {
	Rows []rowXML `xml:"sheetData>row"`
}

type rowXML struct {
	Cells []cellXML `xml:"c"`
}

type cellXML struct {
	Ref       string      `xml:"r,attr"`
	Type      string      `xml:"t,attr"`
	Value     string      `xml:"v"`
	InlineStr inlineValue `xml:"is"`
}

type inlineValue struct {
	Text string `xml:"t"`
	Runs []run  `xml:"r"`
}

func openWorkbook(path string) (*workbook, error) {
	rc, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	book := &workbook{rc: rc, nameMap: map[string]*zip.File{}}
	for _, fh := range rc.File {
		book.nameMap[normalizeZipName(fh.Name)] = fh
	}
	if err := book.loadSharedStrings(); err != nil {
		rc.Close()
		return nil, err
	}
	if err := book.loadSheets(); err != nil {
		rc.Close()
		return nil, err
	}
	return book, nil
}

func (b *workbook) Close() error {
	return b.rc.Close()
}

func (b *workbook) SheetNames() []string {
	out := make([]string, 0, len(b.sheets))
	for _, item := range b.sheets {
		out = append(out, item.Name)
	}
	return out
}

func (b *workbook) ReadRows(sheetName string) ([]map[string]string, error) {
	for _, item := range b.sheets {
		if strings.EqualFold(item.Name, sheetName) {
			var doc worksheetXML
			if err := b.decodeXML(item.Path, &doc); err != nil {
				return nil, err
			}
			rows := make([]map[string]string, 0, len(doc.Rows))
			for _, row := range doc.Rows {
				values := map[string]string{}
				for _, cell := range row.Cells {
					col := columnLetters(cell.Ref)
					if col == "" {
						continue
					}
					values[col] = b.cellValue(cell)
				}
				rows = append(rows, values)
			}
			return rows, nil
		}
	}
	return nil, fmt.Errorf("sheet not found: %s", sheetName)
}

func (b *workbook) loadSharedStrings() error {
	var doc sharedStringsXML
	if err := b.decodeXML("xl/sharedstrings.xml", &doc); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	b.strings = make([]string, 0, len(doc.Items))
	for _, item := range doc.Items {
		if item.Text != "" {
			b.strings = append(b.strings, item.Text)
			continue
		}
		parts := make([]string, 0, len(item.Runs))
		for _, run := range item.Runs {
			parts = append(parts, run.Text)
		}
		b.strings = append(b.strings, strings.Join(parts, ""))
	}
	return nil
}

func (b *workbook) loadSheets() error {
	var wb workbookXML
	if err := b.decodeXML("xl/workbook.xml", &wb); err != nil {
		return err
	}
	var rels relsXML
	if err := b.decodeXML("xl/_rels/workbook.xml.rels", &rels); err != nil {
		return err
	}
	relMap := map[string]string{}
	for _, rel := range rels.Relationships {
		relMap[rel.ID] = strings.TrimPrefix(strings.ReplaceAll(rel.Target, "\\", "/"), "/")
	}
	b.sheets = make([]sheetRef, 0, len(wb.Sheets))
	for _, item := range wb.Sheets {
		target := relMap[item.ID]
		if target == "" {
			continue
		}
		b.sheets = append(b.sheets, sheetRef{Name: item.Name, Path: target})
	}
	return nil
}

func (b *workbook) cellValue(cell cellXML) string {
	switch strings.TrimSpace(cell.Type) {
	case "s":
		idx, err := strconv.Atoi(strings.TrimSpace(cell.Value))
		if err != nil || idx < 0 || idx >= len(b.strings) {
			return ""
		}
		return strings.TrimSpace(b.strings[idx])
	case "inlineStr":
		if strings.TrimSpace(cell.InlineStr.Text) != "" {
			return strings.TrimSpace(cell.InlineStr.Text)
		}
		parts := make([]string, 0, len(cell.InlineStr.Runs))
		for _, run := range cell.InlineStr.Runs {
			parts = append(parts, run.Text)
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	default:
		return strings.TrimSpace(cell.Value)
	}
}

func (b *workbook) decodeXML(name string, target interface{}) error {
	fh, ok := b.nameMap[normalizeZipName(name)]
	if !ok {
		return os.ErrNotExist
	}
	rc, err := fh.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	blob, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	return xml.Unmarshal(blob, target)
}

func normalizeZipName(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
}

func columnLetters(ref string) string {
	ref = strings.ToUpper(strings.TrimSpace(ref))
	var out strings.Builder
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		out.WriteRune(r)
	}
	return out.String()
}
