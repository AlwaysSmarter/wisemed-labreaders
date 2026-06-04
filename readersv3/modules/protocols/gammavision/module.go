package gammavision

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
	"wisemed-labreaders/readersv3/modules/protocols/fileimportbase"
)

var whitespaceRE = regexp.MustCompile(`\s{2,}`)

func New() module.Module {
	return fileimportbase.New(fileimportbase.Spec{
		ID:                 "protocol-gammavision",
		MenuID:             "protocol-gammavision",
		MenuLabel:          "Protocol GammaVision",
		MenuPath:           "/settings/protocol/gammavision",
		MenuOrder:          49,
		ProtocolMeta:       "gammavision",
		ResponseProtocol:   "GAMMAVISION_TXT",
		AnalyteDescription: "Auto-generated from GammaVision exports",
		Parse:              parseGammaVision,
	})
}

func parseGammaVision(path string, _ module.Runtime) (fileimportbase.ImportData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fileimportbase.ImportData{}, err
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	spectrumName := extractAfter(lines, "Spectrum name:")
	sampleDescription := strings.TrimSpace(strings.Join(extractBlock(lines, "Sample description", "Spectrum Filename:"), " "))
	startTime := extractAfter(lines, "Start time:")
	runDate := fileimportbase.ParseDate(startTime)
	measuredAt := fileimportbase.ParseTimestamp(startTime)
	sourceFile := filepath.Base(path)

	sampleID := fileimportbase.NormalizeSampleID(firstNonEmpty(spectrumName, sampleDescription, strings.TrimSuffix(sourceFile, filepath.Ext(sourceFile))))
	analytes := map[string]fileimportbase.AnalyteDef{}
	samples := []fileimportbase.SampleRecord{}
	inSummary := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "SUMMARY OF NUCLIDES IN SAMPLE") {
			inSummary = true
			continue
		}
		if !inSummary {
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "__") || strings.HasPrefix(trimmed, "# -") || strings.HasPrefix(trimmed, "* -") {
			continue
		}
		if strings.HasPrefix(trimmed, "-----------------------------") || strings.HasPrefix(trimmed, "Total Activity") {
			break
		}
		fields := whitespaceRE.Split(trimmed, -1)
		if len(fields) < 4 {
			continue
		}
		nuclide := normalizeNuclide(fields[0])
		if nuclide == "" {
			continue
		}
		if len(fields) >= 3 && fields[1] == "<" {
			continue
		}
		value := fileimportbase.NormalizeNumber(fields[1])
		uncertainty := ""
		sigmaTotal := ""
		mda := ""
		if len(fields) > 2 {
			uncertainty = fileimportbase.NormalizeNumber(fields[2])
		}
		if len(fields) > 3 {
			sigmaTotal = fileimportbase.NormalizeNumber(fields[3])
		}
		if len(fields) > 4 {
			mda = fileimportbase.NormalizeNumber(fields[4])
		}
		if value == "" {
			continue
		}
		analytes[nuclide] = fileimportbase.AnalyteDef{
			Tag:              nuclide,
			Code:             nuclide,
			Name:             nuclide,
			ResultType:       "numeric",
			ResultFormatting: "raw",
			ResultWeighting:  1,
			Unit:             "Bq/L",
		}
		flags := map[string]interface{}{
			"source":               "gammavision_txt",
			"sample_raw":           spectrumName,
			"sample_description":   sampleDescription,
			"measured_at":          measuredAt,
			"source_file":          sourceFile,
			"uncertainty_counting": uncertainty,
			"two_sigma_total":      sigmaTotal,
			"mda":                  mda,
		}
		interpreted := buildInterpreted(nuclide, value, uncertainty, sigmaTotal, mda, measuredAt)
		samples = append(samples, fileimportbase.SampleRecord{
			RunDate: runDate,
			Record: coremodel.ImportedRecord{
				SampleID:    sampleID,
				FileID:      sampleID,
				PatientID:   sampleID,
				PatientName: sampleDescription,
				AnalyteTag:  nuclide,
				AnalyteName: nuclide,
				ResultValue: value,
				RawValue:    value,
				Interpreted: interpreted,
				Flags:       flags,
				Unit:        "Bq/L",
				Meta:        map[string]interface{}{},
			},
		})
	}
	analyteList := make([]fileimportbase.AnalyteDef, 0, len(analytes))
	for _, item := range analytes {
		analyteList = append(analyteList, item)
	}
	data := fileimportbase.ImportData{SampleRecords: samples, Analytes: analyteList}
	fileimportbase.SortImportData(&data)
	return data, nil
}

func extractAfter(lines []string, prefix string) string {
	for _, line := range lines {
		if idx := strings.Index(line, prefix); idx >= 0 {
			return strings.TrimSpace(line[idx+len(prefix):])
		}
	}
	return ""
}

func extractBlock(lines []string, startMarker, endMarker string) []string {
	out := []string{}
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, startMarker) {
			inBlock = true
			continue
		}
		if inBlock && strings.HasPrefix(trimmed, endMarker) {
			break
		}
		if inBlock && trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func normalizeNuclide(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ReplaceAll(strings.ReplaceAll(value, "#", ""), " ", "")
}

func buildInterpreted(tag, value, uncertainty, sigmaTotal, mda, measuredAt string) string {
	parts := []string{"Analit=" + tag, "Valoare=" + value, "UM=Bq/L"}
	if uncertainty != "" {
		parts = append(parts, "Incertitudine="+uncertainty)
	}
	if sigmaTotal != "" {
		parts = append(parts, "2Sigma="+sigmaTotal)
	}
	if mda != "" {
		parts = append(parts, "MDA="+mda)
	}
	if measuredAt != "" {
		parts = append(parts, "Data="+measuredAt)
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
