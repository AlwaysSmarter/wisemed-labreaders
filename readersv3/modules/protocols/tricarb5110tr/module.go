package tricarb5110tr

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
	"wisemed-labreaders/readersv3/modules/protocols/fileimportbase"
)

var dateTokenRE = regexp.MustCompile(`(\d{4})(\d{2})(\d{2})`)

func New() module.Module {
	return fileimportbase.New(fileimportbase.Spec{
		ID:                 "protocol-tricarb-5110-tr",
		MenuID:             "protocol-tricarb-5110-tr",
		MenuLabel:          "Protocol TriCARB 5110 TR",
		MenuPath:           "/settings/protocol/tricarb-5110-tr",
		MenuOrder:          50,
		ProtocolMeta:       "tricarb-5110-tr",
		ResponseProtocol:   "TRICARB_5110_TR_CSV",
		AnalyteDescription: "Auto-generated from TriCARB 5110 TR exports",
		Parse:              parseTriCarb,
	})
}

func parseTriCarb(path string, _ module.Runtime) (fileimportbase.ImportData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fileimportbase.ImportData{}, err
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	runDate := inferRunDate(lines, path)
	sourceFile := filepath.Base(path)
	analytes := []fileimportbase.AnalyteDef{
		{Tag: "CPMA", Code: "CPMA", Name: "CPMA", ResultType: "numeric", ResultFormatting: "raw", ResultWeighting: 1},
		{Tag: "SIS", Code: "SIS", Name: "SIS", ResultType: "numeric", ResultFormatting: "raw", ResultWeighting: 1},
	}
	samples := []fileimportbase.SampleRecord{}
	headerIndex := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "S#,Count Time,CPMA,SIS") {
			headerIndex = i
			break
		}
	}
	if headerIndex < 0 {
		return fileimportbase.ImportData{}, nil
	}
	for _, line := range lines[headerIndex+1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		cols := strings.Split(trimmed, ",")
		if len(cols) < 4 {
			continue
		}
		sampleID := strings.TrimSpace(cols[0])
		countTime := fileimportbase.NormalizeNumber(cols[1])
		cpma := fileimportbase.NormalizeNumber(cols[2])
		sis := fileimportbase.NormalizeNumber(cols[3])
		message := ""
		if len(cols) > 4 {
			message = strings.TrimSpace(cols[4])
		}
		for _, item := range []struct {
			tag   string
			name  string
			value string
		}{
			{tag: "CPMA", name: "CPMA", value: cpma},
			{tag: "SIS", name: "SIS", value: sis},
		} {
			if item.value == "" {
				continue
			}
			flags := map[string]interface{}{
				"source":      "tricarb_5110_tr_csv",
				"sample_raw":  sampleID,
				"count_time":  countTime,
				"messages":    message,
				"source_file": sourceFile,
			}
			samples = append(samples, fileimportbase.SampleRecord{
				RunDate: runDate,
				Record: coremodel.ImportedRecord{
					SampleID:    sampleID,
					FileID:      sampleID,
					PatientID:   sampleID,
					PatientName: sampleID,
					AnalyteTag:  item.tag,
					AnalyteName: item.name,
					ResultValue: item.value,
					RawValue:    item.value,
					Interpreted: buildInterpreted(item.tag, item.value, countTime, message),
					Flags:       flags,
					Meta:        map[string]interface{}{},
				},
			})
		}
	}
	data := fileimportbase.ImportData{SampleRecords: samples, Analytes: analytes}
	fileimportbase.SortImportData(&data)
	return data, nil
}

func inferRunDate(lines []string, path string) string {
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Output Data Path:") || strings.HasPrefix(strings.TrimSpace(line), "Raw Results Path:") {
			if matches := dateTokenRE.FindStringSubmatch(line); len(matches) == 4 {
				return matches[1] + "-" + matches[2] + "-" + matches[3]
			}
		}
	}
	if matches := dateTokenRE.FindStringSubmatch(path); len(matches) == 4 {
		return matches[1] + "-" + matches[2] + "-" + matches[3]
	}
	return ""
}

func buildInterpreted(tag, value, countTime, message string) string {
	parts := []string{"Analit=" + tag, "Valoare=" + value}
	if countTime != "" {
		parts = append(parts, "Timp="+countTime)
	}
	if strings.TrimSpace(message) != "" {
		parts = append(parts, "Mesaj="+strings.TrimSpace(message))
	}
	return strings.Join(parts, " · ")
}
