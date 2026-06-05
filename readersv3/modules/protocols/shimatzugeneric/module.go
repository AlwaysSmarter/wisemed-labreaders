package shimatzugeneric

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
	"wisemed-labreaders/readersv3/modules/protocols/fileimportbase"
)

func New() module.Module {
	return fileimportbase.New(fileimportbase.Spec{
		ID:                 "protocol-shimatzu-generic",
		MenuID:             "protocol-shimatzu-generic",
		MenuLabel:          "Protocol SHIMATZU Generic",
		MenuPath:           "/settings/protocol/shimatzu-generic",
		MenuOrder:          47,
		ProtocolMeta:       "shimatzu-generic",
		ResponseProtocol:   "SHIMATZU_GENERIC_TXT",
		AnalyteDescription: "Auto-generated from SHIMATZU LabSolutions exports",
		QCTargetNotes:      "Creat automat din import QC SHIMATZU Generic. Definiti media si 1SD in Setari QC.",
		Parse:              parseShimadzuGeneric,
	})
}

func parseShimadzuGeneric(path string, rt module.Runtime) (fileimportbase.ImportData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fileimportbase.ImportData{}, err
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	header := parseKeyValueSection(lines, "Header")
	sampleInfo := parseKeyValueSection(lines, "Sample Information")
	compoundRows := firstTableSection(lines,
		"Compound Results(Detector A)",
		"Compound Results (Detector A)",
		"Compound Results(Ch1)",
		"Compound Results (Ch1)",
	)

	sampleID := fileimportbase.PreferredSampleCode(sampleInfo["Sample ID"], sampleInfo["Sample Name"])
	if sampleID == "" {
		sampleID = fileimportbase.NormalizeSampleID(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	}
	sampleRaw := strings.TrimSpace(fileimportbase.PreferredRawSampleCode(sampleInfo["Sample ID"], sampleInfo["Sample Name"]))
	if sampleRaw == "" {
		sampleRaw = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	sampleName := strings.TrimSpace(sampleInfo["Sample Name"])
	runDate := fileimportbase.ParseDate(firstNonEmpty(sampleInfo["Acquired"], sampleInfo["Acquisition Date"], header["Output Date"]))
	measuredAt := fileimportbase.ParseTimestamp(firstNonEmpty(sampleInfo["Acquired"], sampleInfo["Acquisition Date"], firstNonEmpty(header["Output Date"], "")+" "+firstNonEmpty(header["Output Time"], "")))
	subtype := strings.TrimSpace(firstNonEmpty(asString(rt.ModuleSettings("protocol-shimatzu-generic")["subtype"]), inferSubtype(path, compoundRows)))
	sourceFile := filepath.Base(path)

	analytes := []fileimportbase.AnalyteDef{}
	samples := []fileimportbase.SampleRecord{}
	qcByKey := map[string]fileimportbase.QCRecord{}
	analyteSeen := map[string]struct{}{}
	for _, row := range compoundRows {
		name := strings.TrimSpace(row["Name"])
		tag := normalizeAnalyteTag(name)
		conc := fileimportbase.NormalizeNumber(row["Conc."])
		if name == "" || tag == "" || conc == "" {
			continue
		}
		if _, ok := analyteSeen[tag]; !ok {
			analyteSeen[tag] = struct{}{}
			analytes = append(analytes, fileimportbase.AnalyteDef{
				Tag:              tag,
				Code:             tag,
				Name:             name,
				ResultType:       "numeric",
				ResultFormatting: "raw",
				ResultWeighting:  1,
				ProtocolOptions: map[string]interface{}{
					"compound_name": name,
					"subtype":       subtype,
				},
			})
		}
		flags := map[string]interface{}{
			"source":              "shimatzu_generic_txt",
			"sample_name":         sampleName,
			"sample_raw":          sampleRaw,
			"sample_type":         strings.TrimSpace(firstNonEmpty(sampleInfo["Sample Type"], sampleInfo["Type"])),
			"subtype":             subtype,
			"measured_at":         measuredAt,
			"source_file":         sourceFile,
			"retention_time_min":  fileimportbase.NormalizeNumber(row["R.Time"]),
			"area":                fileimportbase.NormalizeNumber(row["Area"]),
			"height":              fileimportbase.NormalizeNumber(row["Height"]),
			"area_ratio":          fileimportbase.NormalizeNumber(row["Area Ratio"]),
			"height_ratio":        fileimportbase.NormalizeNumber(row["Height Ratio"]),
			"concentration_ratio": fileimportbase.NormalizeNumber(row["Conc. %"]),
			"compound_id":         strings.TrimSpace(row["ID#"]),
		}
		interpreted := fileimportbase.BuildInterpreted(tag, conc, "", measuredAt)
		if isControlSample(sampleID, sampleInfo) {
			key := runDate + "|" + sampleID
			record := qcByKey[key]
			if len(record.Results) == 0 {
				record = fileimportbase.QCRecord{
					RunDate:      runDate,
					ControlLabel: sampleID,
					ControlLevel: "QC",
					LotNo:        sampleID,
					FileID:       sampleID,
					Status:       "completed",
					Meta: map[string]interface{}{
						"subtype":     subtype,
						"sample_name": sampleName,
						"sample_raw":  sampleRaw,
					},
				}
			}
			record.Results = append(record.Results, fileimportbase.QCResult{
				AnalyteTag:  tag,
				AnalyteName: name,
				ResultValue: conc,
				RawValue:    conc,
				Interpreted: interpreted,
				Flags:       flags,
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
				PatientName: sampleID,
				AnalyteTag:  tag,
				AnalyteName: name,
				ResultValue: conc,
				RawValue:    conc,
				Interpreted: interpreted,
				Flags:       flags,
				Meta:        map[string]interface{}{},
			},
		})
	}
	qcRecords := make([]fileimportbase.QCRecord, 0, len(qcByKey))
	for _, item := range qcByKey {
		qcRecords = append(qcRecords, item)
	}
	data := fileimportbase.ImportData{SampleRecords: samples, QCRecords: qcRecords, Analytes: analytes}
	fileimportbase.SortImportData(&data)
	return data, nil
}

func parseKeyValueSection(lines []string, name string) map[string]string {
	section := map[string]string{}
	target := "[" + name + "]"
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == target {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			break
		}
		if !inSection || trimmed == "" {
			continue
		}
		cols := fileimportbase.SplitColumns(line, "\t")
		if len(cols) >= 2 {
			section[cols[0]] = strings.Join(cols[1:], "\t")
		}
	}
	return section
}

func parseTableSection(lines []string, name string) []map[string]string {
	target := "[" + name + "]"
	inSection := false
	var header []string
	rows := []map[string]string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == target {
			inSection = true
			header = nil
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			break
		}
		if !inSection || trimmed == "" {
			continue
		}
		cols := fileimportbase.SplitColumns(line, "\t")
		if len(cols) == 0 || strings.HasPrefix(cols[0], "# of ") {
			continue
		}
		if header == nil {
			header = cols
			continue
		}
		row := map[string]string{}
		for i, column := range header {
			if i < len(cols) {
				row[column] = cols[i]
			} else {
				row[column] = ""
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func firstTableSection(lines []string, names ...string) []map[string]string {
	for _, name := range names {
		rows := parseTableSection(lines, name)
		if len(rows) > 0 {
			return rows
		}
	}
	return nil
}

func normalizeAnalyteTag(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	tag := strings.ToUpper(name)
	replacer := strings.NewReplacer(" ", "_", "/", "_", "\\", "_", "-", "_", ".", "_", "(", "", ")", "", ",", "", "%", "PCT")
	tag = replacer.Replace(tag)
	for strings.Contains(tag, "__") {
		tag = strings.ReplaceAll(tag, "__", "_")
	}
	return strings.Trim(tag, "_")
}

func isControlSample(sampleID string, sampleInfo map[string]string) bool {
	sampleID = strings.ToUpper(strings.TrimSpace(sampleID))
	sampleType := strings.ToUpper(strings.TrimSpace(sampleInfo["Sample Type"]))
	return strings.HasPrefix(sampleID, "PC") ||
		strings.Contains(sampleType, "CONTROL") ||
		strings.Contains(sampleType, "STANDARD")
}

func inferSubtype(path string, rows []map[string]string) string {
	base := strings.ToLower(filepath.Base(path))
	if strings.Contains(base, "ion") || strings.Contains(base, "cromatograph") || strings.Contains(base, "ic") {
		return "ion-cromatograph"
	}
	if strings.Contains(base, "hplc") {
		return "hplc"
	}
	if strings.Contains(base, "gc-2010") || strings.Contains(base, "gc2010") || strings.Contains(base, "gcsolution") || strings.HasSuffix(base, ".txt") {
		for _, row := range rows {
			if strings.TrimSpace(row["Curve"]) != "" || strings.TrimSpace(row["Constant"]) != "" {
				return "gc-2010"
			}
		}
	}
	for _, row := range rows {
		name := strings.ToUpper(strings.TrimSpace(row["Name"]))
		if strings.Contains(name, "CLO") || strings.Contains(name, "BRO") {
			return "ion-cromatograph"
		}
	}
	return "auto"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func asString(value interface{}) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}
