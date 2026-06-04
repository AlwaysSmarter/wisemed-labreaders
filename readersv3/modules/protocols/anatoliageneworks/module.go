package anatoliageneworks

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/extrame/xls"

	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
	"wisemed-labreaders/readersv3/modules/protocols/fileimportbase"
)

func New() module.Module {
	return fileimportbase.New(fileimportbase.Spec{
		ID:                 "protocol-anatolia-geneworks",
		MenuID:             "protocol-anatolia-geneworks",
		MenuLabel:          "Protocol Anatolia Geneworks",
		MenuPath:           "/settings/protocol/anatolia-geneworks",
		MenuOrder:          51,
		ProtocolMeta:       "anatolia-geneworks",
		ResponseProtocol:   "ANATOLIA_GENEWORKS_XLS",
		AnalyteDescription: "Auto-generated from Anatolia Geneworks XLS exports",
		QCTargetNotes:      "Creat automat din import QC Anatolia Geneworks. Definiti media si 1SD in Setari QC.",
		Parse:              parseAnatolia,
	})
}

func parseAnatolia(path string, _ module.Runtime) (fileimportbase.ImportData, error) {
	wb, err := xls.Open(path, "utf-8")
	if err != nil {
		return fileimportbase.ImportData{}, err
	}
	sheet := wb.GetSheet(0)
	if sheet == nil {
		return fileimportbase.ImportData{}, nil
	}
	if sheet.MaxRow < 1 {
		return fileimportbase.ImportData{}, nil
	}
	headerRow := sheet.Row(0)
	headers := []string{}
	for c := headerRow.FirstCol(); c < headerRow.LastCol(); c++ {
		headers = append(headers, strings.TrimSpace(headerRow.Col(c)))
	}
	sourceFile := filepath.Base(path)
	analytes := map[string]fileimportbase.AnalyteDef{}
	samples := []fileimportbase.SampleRecord{}
	qcByKey := map[string]fileimportbase.QCRecord{}
	for r := 1; r <= int(sheet.MaxRow); r++ {
		row := sheet.Row(r)
		if row == nil {
			continue
		}
		data := map[string]string{}
		for idx, header := range headers {
			data[header] = strings.TrimSpace(row.Col(idx))
		}
		target := strings.TrimSpace(data["Target"])
		if target == "" {
			continue
		}
		tag := normalizeTarget(target)
		ct := strings.TrimSpace(data["Ct"])
		value := normalizeCt(ct)
		conclusion := strings.TrimSpace(data["Conclusion"])
		label := strings.TrimSpace(data["Label"])
		caseID := strings.TrimSpace(data["Case ID"])
		sampleID := deriveSampleID(caseID, label, strconv.Itoa(r))
		sampleRaw := firstNonEmpty(label, caseID, sampleID)
		rowType := strings.ToUpper(strings.TrimSpace(data["Type"]))
		analytes[tag] = fileimportbase.AnalyteDef{
			Tag:              tag,
			Code:             tag,
			Name:             target,
			ResultType:       "numeric",
			ResultFormatting: "raw",
			ResultWeighting:  1,
			Unit:             "Ct",
		}
		flags := map[string]interface{}{
			"source":      "anatolia_geneworks_xls",
			"sample_raw":  sampleRaw,
			"well":        strings.TrimSpace(data["Well"]),
			"channel":     strings.TrimSpace(data["CH"]),
			"type":        rowType,
			"conclusion":  conclusion,
			"label":       label,
			"source_file": sourceFile,
		}
		interpreted := buildInterpreted(target, ct, conclusion)
		if rowType != "UNKNOWN" {
			key := rowType + "|" + tag
			record := qcByKey[key]
			if len(record.Results) == 0 {
				record = fileimportbase.QCRecord{
					ControlLabel: rowType,
					ControlLevel: detectQCLevel(rowType),
					LotNo:        rowType,
					FileID:       rowType,
					Status:       "completed",
					Meta: map[string]interface{}{
						"label": label,
						"well":  strings.TrimSpace(data["Well"]),
					},
				}
			}
			record.Results = append(record.Results, fileimportbase.QCResult{
				AnalyteTag:  tag,
				AnalyteName: target,
				ResultValue: value,
				RawValue:    ct,
				Interpreted: interpreted,
				Unit:        "Ct",
				Flags:       flags,
			})
			qcByKey[key] = record
			continue
		}
		samples = append(samples, fileimportbase.SampleRecord{
			Record: coremodel.ImportedRecord{
				SampleID:    sampleID,
				FileID:      sampleID,
				PatientID:   sampleID,
				PatientName: label,
				AnalyteTag:  tag,
				AnalyteName: target,
				ResultValue: value,
				RawValue:    ct,
				Interpreted: interpreted,
				Flags:       flags,
				Unit:        "Ct",
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

func normalizeTarget(value string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", "_"))
}

func normalizeCt(value string) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "No Ct") {
		return ""
	}
	return fileimportbase.NormalizeNumber(value)
}

func deriveSampleID(caseID, label, fallback string) string {
	if strings.TrimSpace(caseID) != "" {
		return strings.TrimSpace(caseID)
	}
	fields := strings.Fields(label)
	if len(fields) > 0 {
		return fields[0]
	}
	return fallback
}

func detectQCLevel(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch value {
	case "NTC":
		return "negativ"
	case "POSITIVE":
		return "pozitiv"
	default:
		return "QC"
	}
}

func buildInterpreted(target, ct, conclusion string) string {
	parts := []string{"Analit=" + strings.TrimSpace(target)}
	if strings.TrimSpace(ct) != "" {
		parts = append(parts, "Ct="+strings.TrimSpace(ct))
	}
	if strings.TrimSpace(conclusion) != "" {
		parts = append(parts, "Concluzie="+strings.TrimSpace(conclusion))
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
