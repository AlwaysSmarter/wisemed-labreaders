package seegeneexcel

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"

	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
	"wisemed-labreaders/readersv3/modules/protocols/fileimportbase"
)

func New() module.Module {
	return fileimportbase.New(fileimportbase.Spec{
		ID:                 "protocol-seegene-excel",
		MenuID:             "protocol-seegene",
		MenuLabel:          "Protocol SeeGene Viewer",
		MenuPath:           "/settings/protocol/seegene",
		MenuOrder:          45,
		ProtocolMeta:       "seegene-excel",
		ResponseProtocol:   "SEEGENE_VIEWER_CSV",
		AnalyteDescription: "Auto-generated from SeeGene Viewer CSV exports",
		QCTargetNotes:      "Creat automat din import QC SeeGene Viewer. Definiti media si 1SD in Setari QC daca folositi controale dedicate.",
		Parse:              parseSeeGeneViewerCSV,
	})
}

func parseSeeGeneViewerCSV(path string, _ module.Runtime) (fileimportbase.ImportData, error) {
	fh, err := os.Open(path)
	if err != nil {
		return fileimportbase.ImportData{}, err
	}
	defer fh.Close()

	reader := csv.NewReader(fh)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	rows, err := reader.ReadAll()
	if err != nil {
		return fileimportbase.ImportData{}, err
	}
	if len(rows) < 3 {
		return fileimportbase.ImportData{}, nil
	}

	header1 := trimCSVRow(rows[0])
	header2 := trimCSVRow(rows[1])
	sourceFile := filepath.Base(path)
	targets := parseTargets(header1, header2)
	if len(targets) == 0 {
		return fileimportbase.ImportData{}, nil
	}

	analytes := map[string]fileimportbase.AnalyteDef{}
	samples := []fileimportbase.SampleRecord{}
	qcByKey := map[string]fileimportbase.QCRecord{}

	for _, row := range rows[2:] {
		row = trimCSVRow(row)
		if rowEmpty(row) {
			continue
		}
		well := csvCell(row, 2)
		name := csvCell(row, 3)
		rowType := strings.ToUpper(csvCell(row, 4))
		if well == "" && name == "" && rowType == "" {
			continue
		}
		sampleNo := csvCell(row, 0)
		patientID := csvCell(row, 1)
		autoInterpretation := csvCell(row, len(row)-2)
		comment := csvCell(row, len(row)-1)
		sampleID := deriveSampleID(patientID, name, sampleNo, well)
		rowFlags := map[string]interface{}{
			"source":              "seegene_viewer_csv",
			"sample_no":           sampleNo,
			"patient_id_raw":      patientID,
			"well":                well,
			"name":                name,
			"type":                rowType,
			"auto_interpretation": autoInterpretation,
			"comment":             comment,
			"source_file":         sourceFile,
		}
		for _, target := range targets {
			tag := normalizeTag(target.Name)
			if tag == "" {
				continue
			}
			analytes[tag] = fileimportbase.AnalyteDef{
				Tag:              tag,
				Code:             tag,
				Name:             target.Name,
				ResultType:       "numeric",
				ResultFormatting: "raw",
				ResultWeighting:  1,
				Unit:             "Ct",
				ProtocolOptions: map[string]interface{}{
					"channel": target.Channel,
				},
			}

			sign := csvCell(row, target.ResultCol)
			ctRaw := csvCell(row, target.CtCol)
			ctValue := normalizeCt(ctRaw)
			flags := cloneMap(rowFlags)
			flags["channel"] = target.Channel
			flags["sign"] = sign
			flags["target_name"] = target.Name

			interpreted := buildInterpreted(target.Name, sign, ctRaw, autoInterpretation, comment)
			switch rowType {
			case "SAMPLE":
				samples = append(samples, fileimportbase.SampleRecord{
					Record: coremodel.ImportedRecord{
						SampleID:    sampleID,
						FileID:      sampleID,
						PatientID:   firstNonEmpty(patientID, sampleID),
						PatientName: name,
						AnalyteTag:  tag,
						AnalyteName: target.Name,
						ResultValue: ctValue,
						RawValue:    ctValue,
						Interpreted: interpreted,
						Flags:       flags,
						Unit:        "Ct",
						Meta:        map[string]interface{}{},
					},
				})
			case "NC", "PC":
				key := rowType + "|" + firstNonEmpty(name, well)
				record := qcByKey[key]
				if len(record.Results) == 0 {
					record = fileimportbase.QCRecord{
						ControlLabel: firstNonEmpty(name, rowType, well),
						ControlLevel: detectControlLevel(rowType, name, comment),
						LotNo:        firstNonEmpty(name, rowType),
						FileID:       firstNonEmpty(sampleID, rowType, well),
						Status:       "completed",
						Meta: map[string]interface{}{
							"well":                well,
							"type":                rowType,
							"auto_interpretation": autoInterpretation,
							"comment":             comment,
						},
					}
				}
				record.Results = append(record.Results, fileimportbase.QCResult{
					AnalyteTag:  tag,
					AnalyteName: target.Name,
					ResultValue: ctValue,
					RawValue:    ctValue,
					Interpreted: interpreted,
					Unit:        "Ct",
					Flags:       flags,
				})
				qcByKey[key] = record
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
	fileimportbase.SortImportData(&data)
	return data, nil
}

type targetColumn struct {
	Channel   string
	Name      string
	ResultCol int
	CtCol     int
}

func parseTargets(header1, header2 []string) []targetColumn {
	limit := len(header1)
	if len(header2) < limit {
		limit = len(header2)
	}
	out := []targetColumn{}
	currentChannel := ""
	for idx := 5; idx+1 < limit; idx += 2 {
		if value := strings.TrimSpace(header1[idx]); value != "" {
			currentChannel = value
		}
		name := strings.TrimSpace(header2[idx])
		ctLabel := strings.TrimSpace(header2[idx+1])
		if name == "" || !strings.EqualFold(ctLabel, "C(t)") {
			continue
		}
		out = append(out, targetColumn{
			Channel:   currentChannel,
			Name:      name,
			ResultCol: idx,
			CtCol:     idx + 1,
		})
	}
	return out
}

func trimCSVRow(row []string) []string {
	out := make([]string, len(row))
	for idx, value := range row {
		value = strings.TrimSpace(strings.TrimPrefix(value, "\ufeff"))
		out[idx] = value
	}
	return out
}

func rowEmpty(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func csvCell(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func normalizeTag(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func normalizeCt(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "N/A") {
		return ""
	}
	return fileimportbase.NormalizeNumber(value)
}

func deriveSampleID(patientID, name, sampleNo, well string) string {
	if strings.TrimSpace(patientID) != "" {
		return strings.TrimSpace(patientID)
	}
	fields := strings.Fields(strings.TrimSpace(name))
	if len(fields) > 0 {
		return strings.TrimSpace(fields[0])
	}
	return firstNonEmpty(sampleNo, well)
}

func detectControlLevel(rowType, name, comment string) string {
	text := strings.ToUpper(strings.TrimSpace(rowType + " " + name + " " + comment))
	switch {
	case strings.Contains(text, "NEGATIVE"), strings.Contains(text, "NC"):
		return "negativ"
	case strings.Contains(text, "POSITIVE"), strings.Contains(text, "PC"):
		return "pozitiv"
	default:
		return "QC"
	}
}

func buildInterpreted(targetName, sign, ctRaw, autoInterpretation, comment string) string {
	parts := []string{"Analit=" + strings.TrimSpace(targetName)}
	if strings.TrimSpace(sign) != "" {
		parts = append(parts, "Calitativ="+strings.TrimSpace(sign))
	}
	if value := normalizeCt(ctRaw); value != "" {
		parts = append(parts, "Ct="+value)
	}
	if strings.TrimSpace(autoInterpretation) != "" {
		parts = append(parts, "Interpretare="+strings.TrimSpace(autoInterpretation))
	}
	if strings.TrimSpace(comment) != "" {
		parts = append(parts, "Comentariu="+strings.TrimSpace(comment))
	}
	return strings.Join(parts, " · ")
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
