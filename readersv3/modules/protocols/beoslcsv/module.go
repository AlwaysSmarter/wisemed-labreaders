package beoslcsv

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
	"wisemed-labreaders/readersv3/modules/protocols/fileimportbase"
	"wisemed-labreaders/readersv3/modules/wisemedapi"
)

type dosimetrySaver interface {
	SaveDosimetry(entries []wisemedapi.DosimetryEntry) (*wisemedapi.DosimetryResponse, error)
}

type resultPublisher interface {
	SendOrderBundles([]coremodel.OrderBundle, string) (map[string]interface{}, error)
}

type Module struct {
	rt   module.Runtime
	base module.Module
}

func New() module.Module {
	spec := fileimportbase.Spec{
		ID:                 "protocol-beosl-csv",
		MenuID:             "protocol-beosl",
		MenuLabel:          "Protocol BEOSL",
		MenuPath:           "/settings/protocol/beosl",
		MenuOrder:          45,
		ProtocolMeta:       "beosl-csv",
		ResponseProtocol:   "BEOSL_CSV",
		AnalyteDescription: "Auto-generated from BEOSL CSV exports",
		QCTargetNotes:      "Creat automat din import BEOSL. Definiti media si 1SD in Setari QC daca folositi controale dedicate.",
		Parse:              parseBEOSL,
		AfterImport:        pushDosimetryToWiseMED,
	}
	return &Module{base: fileimportbase.New(spec)}
}

func (m *Module) ID() string { return "protocol-beosl-csv" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.RegisterService("result-publisher", m)
	return m.base.Init(rt)
}

func (m *Module) Start(ctx context.Context) error {
	starter, ok := m.base.(module.Starter)
	if !ok {
		<-ctx.Done()
		return nil
	}
	return starter.Start(ctx)
}

func (m *Module) SendOrderBundles(bundles []coremodel.OrderBundle, actor string) (map[string]interface{}, error) {
	service, ok := m.rt.Service("wisemed-api")
	if !ok {
		return nil, errors.New("wisemed-api service unavailable")
	}
	client, ok := service.(dosimetrySaver)
	if !ok {
		return nil, errors.New("wisemed-api dosimetry service unavailable")
	}
	entries := dosimetryEntriesFromBundles(bundles)
	if len(entries) == 0 {
		return nil, errors.New("no dosimetry results available to send")
	}
	resp, err := client.SaveDosimetry(entries)
	if err != nil {
		return nil, err
	}
	summary := map[string]interface{}{
		"ok":          true,
		"entries":     len(entries),
		"actor":       strings.TrimSpace(actor),
		"source":      "beosl",
		"auto":        false,
		"response_ok": resp != nil && resp.Success,
	}
	if resp != nil {
		summary["response"] = resp
	}
	if m.rt != nil {
		m.rt.Logf("protocol-beosl-csv manual dosimetry resend actor=%s entries=%d success=%t", strings.TrimSpace(actor), len(entries), resp != nil && resp.Success)
	}
	return summary, nil
}

func parseBEOSL(path string, _ module.Runtime) (fileimportbase.ImportData, error) {
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
	if len(rows) == 0 {
		return fileimportbase.ImportData{}, nil
	}

	headerIndex := indexHeader(rows[0])
	sourceFile := filepath.Base(path)
	analytes := map[string]fileimportbase.AnalyteDef{
		"HP007_CALC_DOSE": {
			Tag:              "HP007_CALC_DOSE",
			Code:             "HP007_CALC_DOSE",
			Name:             "Hp007 Calc. Dose",
			Description:      "Auto-generated from BEOSL CSV exports",
			ResultType:       "numeric",
			ResultFormatting: "raw",
			ResultWeighting:  1,
		},
		"HP10_CALC_DOSE": {
			Tag:              "HP10_CALC_DOSE",
			Code:             "HP10_CALC_DOSE",
			Name:             "Hp10 Calc. Dose",
			Description:      "Auto-generated from BEOSL CSV exports",
			ResultType:       "numeric",
			ResultFormatting: "raw",
			ResultWeighting:  1,
		},
	}

	samples := []fileimportbase.SampleRecord{}
	for _, row := range rows[1:] {
		dosimeterID := csvValue(row, headerIndex, "DOSIMETER ID")
		if dosimeterID == "" {
			continue
		}
		hp007 := fileimportbase.NormalizeNumber(csvValue(row, headerIndex, "HP007 CALC. DOSE"))
		hp10 := fileimportbase.NormalizeNumber(csvValue(row, headerIndex, "HP10 CALC. DOSE"))
		if hp007 == "" && hp10 == "" {
			continue
		}

		meta := map[string]interface{}{
			"source":       "beosl_csv",
			"source_file":  sourceFile,
			"dosimeter_id": dosimeterID,
		}
		if hp007 != "" {
			samples = append(samples, fileimportbase.SampleRecord{
				Record: coremodel.ImportedRecord{
					SampleID:    dosimeterID,
					FileID:      dosimeterID,
					PatientID:   dosimeterID,
					PatientName: dosimeterID,
					AnalyteTag:  "HP007_CALC_DOSE",
					AnalyteName: "Hp007 Calc. Dose",
					ResultValue: hp007,
					RawValue:    hp007,
					Interpreted: "Analit=HP007_CALC_DOSE · Valoare=" + hp007,
					Flags: map[string]interface{}{
						"source":       "beosl_csv",
						"source_file":  sourceFile,
						"dosimeter_id": dosimeterID,
					},
					Meta: meta,
				},
			})
		}
		if hp10 != "" {
			samples = append(samples, fileimportbase.SampleRecord{
				Record: coremodel.ImportedRecord{
					SampleID:    dosimeterID,
					FileID:      dosimeterID,
					PatientID:   dosimeterID,
					PatientName: dosimeterID,
					AnalyteTag:  "HP10_CALC_DOSE",
					AnalyteName: "Hp10 Calc. Dose",
					ResultValue: hp10,
					RawValue:    hp10,
					Interpreted: "Analit=HP10_CALC_DOSE · Valoare=" + hp10,
					Flags: map[string]interface{}{
						"source":       "beosl_csv",
						"source_file":  sourceFile,
						"dosimeter_id": dosimeterID,
					},
					Meta: meta,
				},
			})
		}
	}

	analyteList := make([]fileimportbase.AnalyteDef, 0, len(analytes))
	for _, item := range analytes {
		analyteList = append(analyteList, item)
	}

	data := fileimportbase.ImportData{
		SampleRecords: samples,
		Analytes:      analyteList,
	}
	fileimportbase.SortImportData(&data)
	return data, nil
}

func pushDosimetryToWiseMED(path string, data fileimportbase.ImportData, rt module.Runtime) error {
	if !autoConfirmEnabled(rt) {
		rt.Logf("protocol-beosl-csv auto confirm disabled; skip WiseMED push for %s", filepath.Base(path))
		return nil
	}
	service, ok := rt.Service("wisemed-api")
	if !ok {
		return errors.New("wisemed-api service unavailable")
	}
	client, ok := service.(dosimetrySaver)
	if !ok {
		return errors.New("wisemed-api dosimetry service unavailable")
	}
	entries := dosimetryEntriesFromImportData(data)
	if len(entries) == 0 {
		rt.Logf("protocol-beosl-csv no dosimetry entries to send for %s", filepath.Base(path))
		return nil
	}
	resp, err := client.SaveDosimetry(entries)
	if err != nil {
		return err
	}
	if resp != nil {
		rt.Logf("protocol-beosl-csv dosimetry push completed file=%s entries=%d success=%t", filepath.Base(path), len(entries), resp.Success)
	} else {
		rt.Logf("protocol-beosl-csv dosimetry push completed file=%s entries=%d", filepath.Base(path), len(entries))
	}
	return nil
}

func autoConfirmEnabled(rt module.Runtime) bool {
	raw := rt.ModuleSettings("results")
	switch value := raw["auto_confirm_wisemed"].(type) {
	case bool:
		return value
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "y", "on", "da":
			return true
		}
	}
	return false
}

func dosimetryEntriesFromImportData(data fileimportbase.ImportData) []wisemedapi.DosimetryEntry {
	grouped := map[string]*wisemedapi.DosimetryEntry{}
	for _, item := range data.SampleRecords {
		appendDosimetryRecord(grouped, item.Record.SampleID, item.Record.FileID, item.Record.AnalyteTag, item.Record.ResultValue)
	}
	return flattenEntries(grouped)
}

func dosimetryEntriesFromBundles(bundles []coremodel.OrderBundle) []wisemedapi.DosimetryEntry {
	grouped := map[string]*wisemedapi.DosimetryEntry{}
	for _, bundle := range bundles {
		for _, item := range bundle.Analyses {
			appendDosimetryRecord(grouped, bundle.Order.SampleID, bundle.Order.FileID, item.Analysis.AnalyteTag, item.Analysis.ResultValue)
		}
	}
	return flattenEntries(grouped)
}

func appendDosimetryRecord(grouped map[string]*wisemedapi.DosimetryEntry, sampleID, fileID, analyteTag, resultValue string) {
	serial := strings.TrimSpace(sampleID)
	if serial == "" {
		serial = strings.TrimSpace(fileID)
	}
	if serial == "" {
		return
	}
	entry := grouped[serial]
	if entry == nil {
		entry = &wisemedapi.DosimetryEntry{Serial: serial}
		grouped[serial] = entry
	}
	switch strings.ToUpper(strings.TrimSpace(analyteTag)) {
	case "HP10_CALC_DOSE":
		entry.HP10 = strings.TrimSpace(resultValue)
	case "HP007_CALC_DOSE":
		entry.HP007 = strings.TrimSpace(resultValue)
	}
}

func flattenEntries(grouped map[string]*wisemedapi.DosimetryEntry) []wisemedapi.DosimetryEntry {
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([]wisemedapi.DosimetryEntry, 0, len(keys))
	for _, key := range keys {
		entry := grouped[key]
		if entry == nil || strings.TrimSpace(entry.Serial) == "" {
			continue
		}
		entries = append(entries, *entry)
	}
	return entries
}

func indexHeader(header []string) map[string]int {
	index := make(map[string]int, len(header))
	for i, item := range header {
		key := strings.ToUpper(strings.TrimSpace(strings.Trim(item, "\"")))
		if key != "" {
			index[key] = i
		}
	}
	return index
}

func csvValue(row []string, index map[string]int, key string) string {
	pos, ok := index[key]
	if !ok || pos < 0 || pos >= len(row) {
		return ""
	}
	return strings.TrimSpace(strings.Trim(row[pos], "\""))
}

func (m *Module) String() string {
	return fmt.Sprintf("beoslcsv(%s)", m.ID())
}
