package analytikjenaplasmaquantmselite

import (
	"os"
	"path/filepath"
	"strings"

	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
	"wisemed-labreaders/readersv3/modules/protocols/fileimportbase"
)

func New() module.Module {
	return fileimportbase.New(fileimportbase.Spec{
		ID:                 "protocol-analytikjena-plasmaquantms-elite",
		MenuID:             "protocol-analytikjena-plasmaquantms-elite",
		MenuLabel:          "Protocol AnalytikJena PlasmaQuantMS Elite",
		MenuPath:           "/settings/protocol/analytikjena-plasmaquantms-elite",
		MenuOrder:          52,
		ProtocolMeta:       "analytikjena-plasmaquantms-elite",
		ResponseProtocol:   "ANALYTIKJENA_PLASMAQUANTMS_ELITE_CSV",
		AnalyteDescription: "Auto-generated from AnalytikJena PlasmaQuantMS Elite exports",
		QCTargetNotes:      "Creat automat din import QC AnalytikJena PlasmaQuantMS Elite. Definiti media si 1SD in Setari QC.",
		Parse:              parseWorksheet,
	})
}

func parseWorksheet(path string, _ module.Runtime) (fileimportbase.ImportData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fileimportbase.ImportData{}, err
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	var header []string
	sourceFile := filepath.Base(path)
	analytes := map[string]fileimportbase.AnalyteDef{}
	samples := []fileimportbase.SampleRecord{}
	qcByKey := map[string]fileimportbase.QCRecord{}
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "ASpect MS Worksheet Export from:") {
			continue
		}
		cols := strings.Split(line, ";")
		if header == nil {
			if len(cols) > 3 && strings.EqualFold(strings.TrimSpace(cols[0]), "Label") && strings.EqualFold(strings.TrimSpace(cols[2]), "Element") {
				header = cols
			}
			continue
		}
		if len(cols) < len(header) {
			continue
		}
		row := map[string]string{}
		for i, name := range header {
			row[strings.TrimSpace(name)] = cleanValue(cols[i])
		}
		label := strings.TrimSpace(row["Label"])
		if label == "" {
			continue
		}
		element := strings.TrimSpace(row["Element"])
		tag := normalizeElementTag(element)
		if tag == "" || strings.HasSuffix(tag, "_CONT") {
			continue
		}
		resultValue := pickResult(row)
		if resultValue == "" {
			continue
		}
		unit := firstNonEmpty(strings.TrimSpace(row["Units"]), "ppb")
		runDate := strings.TrimSpace(row["Date"])
		measuredAt := buildTimestamp(row["Date"], row["Time"])
		analytes[tag] = fileimportbase.AnalyteDef{
			Tag:              tag,
			Code:             tag,
			Name:             element,
			ResultType:       "numeric",
			ResultFormatting: "raw",
			ResultWeighting:  1,
			Unit:             unit,
			ProtocolOptions: map[string]interface{}{
				"element_raw": element,
			},
		}
		flags := map[string]interface{}{
			"source":          "analytikjena_plasmaquantms_elite_csv",
			"sample_raw":      label,
			"type":            strings.TrimSpace(row["Type"]),
			"flags":           strings.TrimSpace(row["Flags"]),
			"solution_conc":   normalizeMaybeNumber(row["Sol'n Conc"]),
			"corrected_conc":  normalizeMaybeNumber(row["Corr Conc"]),
			"cps":             normalizeMaybeNumber(row["c/s"]),
			"sd":              normalizeMaybeNumber(row["SD"]),
			"percent_rsd":     normalizeMaybeNumber(row["%RSD"]),
			"sd_cps":          normalizeMaybeNumber(row["SD(c/s)"]),
			"percent_rsd_cps": normalizeMaybeNumber(row["%RSD(c/s)"]),
			"actual_weight":   normalizeMaybeNumber(row["Act. Wt"]),
			"actual_volume":   normalizeMaybeNumber(row["Act. Vol"]),
			"dilution":        normalizeMaybeNumber(row["Dilution"]),
			"replicates":      strings.TrimSpace(row["Replicates"]),
			"measured_at":     measuredAt,
			"source_file":     sourceFile,
		}
		interpreted := buildInterpreted(element, resultValue, unit, flags)
		if isQCRow(label, row["Type"]) {
			controlID := qcLabel(label, row["Type"])
			key := runDate + "|" + controlID
			record := qcByKey[key]
			if len(record.Results) == 0 {
				record = fileimportbase.QCRecord{
					RunDate:      runDate,
					ControlLabel: controlID,
					ControlLevel: detectQCLevel(controlID),
					LotNo:        controlID,
					FileID:       controlID,
					Status:       "completed",
					Meta: map[string]interface{}{
						"sample_raw": label,
						"type":       strings.TrimSpace(row["Type"]),
					},
				}
			}
			record.Results = append(record.Results, fileimportbase.QCResult{
				AnalyteTag:  tag,
				AnalyteName: element,
				ResultValue: resultValue,
				RawValue:    resultValue,
				Interpreted: interpreted,
				Unit:        unit,
				Flags:       flags,
			})
			qcByKey[key] = record
			continue
		}
		sampleID := deriveSampleID(label)
		samples = append(samples, fileimportbase.SampleRecord{
			RunDate: runDate,
			Record: coremodel.ImportedRecord{
				SampleID:    sampleID,
				FileID:      sampleID,
				PatientID:   sampleID,
				PatientName: label,
				AnalyteTag:  tag,
				AnalyteName: element,
				ResultValue: resultValue,
				RawValue:    resultValue,
				Interpreted: interpreted,
				Flags:       flags,
				Unit:        unit,
				Meta:        map[string]interface{}{},
			},
		})
	}
	qcRecords := make([]fileimportbase.QCRecord, 0, len(qcByKey))
	for _, item := range qcByKey {
		qcRecords = append(qcRecords, item)
	}
	analyteList := make([]fileimportbase.AnalyteDef, 0, len(analytes))
	for _, item := range analytes {
		analyteList = append(analyteList, item)
	}
	data := fileimportbase.ImportData{SampleRecords: samples, QCRecords: qcRecords, Analytes: analyteList}
	fileimportbase.SortImportData(&data)
	return data, nil
}

func cleanValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"")
	return strings.TrimSpace(value)
}

func pickResult(row map[string]string) string {
	if value := normalizeMaybeNumber(row["Corr Conc"]); value != "" {
		return value
	}
	return normalizeMaybeNumber(row["Sol'n Conc"])
}

func normalizeMaybeNumber(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" || strings.Contains(value, "#") {
		return ""
	}
	return fileimportbase.NormalizeNumber(value)
}

func normalizeElementTag(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "_", "[", "_", "]", "", "/", "_", "\\", "_", "-", "_", ".", "_")
	value = strings.ToUpper(replacer.Replace(value))
	for strings.Contains(value, "__") {
		value = strings.ReplaceAll(value, "__", "_")
	}
	return strings.Trim(value, "_")
}

func buildTimestamp(dateValue, timeValue string) string {
	if ts, ok := fileimportbase.ParseTime(strings.TrimSpace(dateValue) + " " + strings.TrimSpace(timeValue)); ok {
		return ts.UTC().Format("2006-01-02T15:04:05Z")
	}
	return ""
}

func buildInterpreted(analyteName, value, unit string, flags map[string]interface{}) string {
	parts := []string{"Analit=" + analyteName, "Valoare=" + value}
	if unit != "" {
		parts = append(parts, "UM="+unit)
	}
	if corrected, ok := flags["corrected_conc"].(string); ok && corrected != "" {
		parts = append(parts, "Corectat="+corrected)
	}
	if dilution, ok := flags["dilution"].(string); ok && dilution != "" {
		parts = append(parts, "Dilutie="+dilution)
	}
	if measuredAt, ok := flags["measured_at"].(string); ok && measuredAt != "" {
		parts = append(parts, "Data="+measuredAt)
	}
	return strings.Join(parts, " · ")
}

func isQCRow(label, rowType string) bool {
	text := strings.ToLower(strings.TrimSpace(label + " " + rowType))
	return strings.Contains(text, "blk") ||
		strings.Contains(text, "blank") ||
		strings.Contains(text, "std") ||
		strings.Contains(text, "standard") ||
		strings.Contains(text, "cal.") ||
		strings.Contains(text, "ctr.") ||
		strings.Contains(text, "control") ||
		strings.Contains(text, "et.")
}

func qcLabel(label, rowType string) string {
	value := strings.TrimSpace(label)
	if value == "" {
		value = strings.TrimSpace(rowType)
	}
	return fileimportbase.NormalizeSampleID(value)
}

func detectQCLevel(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(v, "blank") || strings.Contains(v, "blk"):
		return "negativ"
	case strings.Contains(v, "std") || strings.Contains(v, "standard") || strings.Contains(v, "et."):
		return "pozitiv"
	default:
		return "QC"
	}
}

func deriveSampleID(label string) string {
	value := strings.TrimSpace(label)
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "sample ") {
		value = strings.TrimSpace(value[len("sample "):])
	}
	if strings.HasPrefix(lower, "sample") && len(value) > 6 {
		value = strings.TrimSpace(value[6:])
	}
	return fileimportbase.NormalizeSampleID(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
