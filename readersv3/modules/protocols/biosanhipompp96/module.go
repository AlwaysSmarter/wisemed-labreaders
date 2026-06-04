package biosanhipompp96

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
		ID:                 "protocol-biosan-hipo-mpp96",
		MenuID:             "protocol-biosan-hipo-mpp96",
		MenuLabel:          "Protocol Biosan HIPO MPP-96",
		MenuPath:           "/settings/protocol/biosan-hipo-mpp96",
		MenuOrder:          48,
		ProtocolMeta:       "biosan-hipo-mpp96",
		ResponseProtocol:   "BIOSAN_HIPO_MPP96_CSV",
		AnalyteDescription: "Auto-generated from Biosan HIPO MPP-96 exports",
		QCTargetNotes:      "Creat automat din import QC Biosan HIPO MPP-96. Definiti media si 1SD in Setari QC.",
		Parse:              parseBiosan,
	})
}

func parseBiosan(path string, _ module.Runtime) (fileimportbase.ImportData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fileimportbase.ImportData{}, err
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	sourceFile := filepath.Base(path)
	analytes := map[string]fileimportbase.AnalyteDef{}
	samples := []fileimportbase.SampleRecord{}
	qcByKey := map[string]fileimportbase.QCRecord{}
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if line == "" {
			continue
		}
		cols := strings.Split(line, ";")
		if len(cols) < 15 {
			continue
		}
		well := cleanField(cols[0])
		slotCode := cleanField(cols[1])
		label := cleanField(cols[2])
		sampleCode := cleanField(cols[4])
		value := fileimportbase.NormalizeNumber(cleanField(cols[5]))
		conclusion := cleanField(cols[6])
		analyteName := cleanField(cols[14])
		if analyteName == "" || value == "" {
			continue
		}
		analyteTag := normalizeTag(analyteName)
		analytes[analyteTag] = fileimportbase.AnalyteDef{
			Tag:              analyteTag,
			Code:             analyteTag,
			Name:             analyteName,
			ResultType:       "numeric",
			ResultFormatting: "raw",
			ResultWeighting:  1,
		}
		flags := map[string]interface{}{
			"source":      "biosan_hipo_mpp96_csv",
			"sample_raw":  sampleCode,
			"well":        well,
			"slot_code":   slotCode,
			"slot_label":  label,
			"conclusion":  conclusion,
			"source_file": sourceFile,
		}
		interpreted := buildInterpreted(analyteTag, value, conclusion)
		if strings.HasPrefix(strings.ToUpper(slotCode), "T") {
			sampleID := firstNonEmpty(sampleCode, slotCode)
			samples = append(samples, fileimportbase.SampleRecord{
				Record: coremodel.ImportedRecord{
					SampleID:    sampleID,
					FileID:      sampleID,
					PatientID:   sampleID,
					PatientName: label,
					AnalyteTag:  analyteTag,
					AnalyteName: analyteName,
					ResultValue: value,
					RawValue:    value,
					Interpreted: interpreted,
					Flags:       flags,
					Meta:        map[string]interface{}{},
				},
			})
			continue
		}
		controlID := firstNonEmpty(slotCode, label, well)
		key := controlID + "|" + analyteTag
		record := qcByKey[key]
		if len(record.Results) == 0 {
			record = fileimportbase.QCRecord{
				ControlLabel: controlID,
				ControlLevel: detectControlLevel(controlID, label),
				LotNo:        controlID,
				FileID:       controlID,
				Status:       "completed",
				Meta: map[string]interface{}{
					"well":       well,
					"slot_label": label,
				},
			}
		}
		record.Results = append(record.Results, fileimportbase.QCResult{
			AnalyteTag:  analyteTag,
			AnalyteName: analyteName,
			ResultValue: value,
			RawValue:    value,
			Interpreted: interpreted,
			Flags:       flags,
		})
		qcByKey[key] = record
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

func cleanField(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"")
	return strings.TrimSpace(value)
}

func normalizeTag(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	return strings.ReplaceAll(value, " ", "_")
}

func detectControlLevel(controlID, label string) string {
	text := strings.ToUpper(strings.TrimSpace(controlID + " " + label))
	switch {
	case strings.Contains(text, "NEG"):
		return "negativ"
	case strings.Contains(text, "POS"):
		return "pozitiv"
	default:
		return "QC"
	}
}

func buildInterpreted(tag, value, conclusion string) string {
	parts := []string{"Analit=" + tag, "Valoare=" + value}
	if strings.TrimSpace(conclusion) != "" {
		parts = append(parts, "Calitativ="+strings.TrimSpace(conclusion))
	}
	return strings.Join(parts, " · ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
