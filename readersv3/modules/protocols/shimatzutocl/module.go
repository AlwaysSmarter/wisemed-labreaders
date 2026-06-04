package shimatzutocl

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
)

type analyteStore interface {
	SaveAnalyte(item coremodel.Analyte) (coremodel.Analyte, error)
}

type importStore interface {
	CurrentRoundNo(orderDate string) (int, error)
	RecordImportedResult(orderDate string, roundNo int, rec coremodel.ImportedRecord, sourceFile string) (coremodel.Order, coremodel.OrderAnalysis, coremodel.OrderAnalysisResult, error)
	ListQCRecords(roundNo int, runDate string) ([]coremodel.QCRecord, error)
	ListQCAnalyses(recordID int64) ([]coremodel.QCAnalysis, error)
	ListQCTargets() ([]coremodel.QCTarget, error)
	SaveQCTarget(item coremodel.QCTarget) (coremodel.QCTarget, error)
	UpsertQCRecord(item coremodel.QCRecord) (coremodel.QCRecord, error)
	UpsertQCAnalysis(item coremodel.QCAnalysis) (coremodel.QCAnalysis, error)
}

type fileTransportMeta struct {
	ImportDir string
	Pattern   string
}

type sampleCodeRules struct {
	SamplePrefixes []string
	SampleSuffixes []string
	Separators     []string
}

type Module struct {
	rt      module.Runtime
	mu      sync.Mutex
	running map[string]struct{}
}

func New() module.Module     { return &Module{running: map[string]struct{}{}} }
func (m *Module) ID() string { return "protocol-shimatzu-tocl" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.AddMenu(module.MenuEntry{ID: "protocol-shimatzu-tocl", Group: "admin", Label: "Protocol Shimatzu TOC-L", Path: "/settings/protocol/shimatzu-tocl", Order: 46})
	rt.RegisterService("file-importer", m)
	rt.Handle("/api/protocol/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"ok":true,"protocol":"shimatzu-tocl"}`))
	}))
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	settings := m.rt.ModuleSettings(m.ID())
	pollSeconds := intSetting(settings, "poll_seconds", 2)
	if pollSeconds <= 0 {
		<-ctx.Done()
		return nil
	}
	ticker := time.NewTicker(time.Duration(pollSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.scanImportDir()
		}
	}
}

func (m *Module) ImportFileNow(path, orderDate string) (map[string]interface{}, error) {
	imported, warnings, err := m.importFile(path, orderDate)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ok":         true,
		"file_name":  filepath.Base(path),
		"imported":   imported,
		"warnings":   warnings,
		"protocol":   "SHIMATZU_TOCL_TXT",
		"order_date": effectiveDate(orderDate),
	}, nil
}

func (m *Module) scanImportDir() {
	meta := m.fileTransport()
	if meta.ImportDir == "" || meta.Pattern == "" {
		return
	}
	files, err := filepath.Glob(filepath.Join(meta.ImportDir, meta.Pattern))
	if err != nil {
		m.rt.Logf("shimatzu tocl glob failed: %v", err)
		return
	}
	for _, path := range files {
		if !m.begin(path) {
			continue
		}
		func() {
			defer m.end(path)
			if _, _, err := m.importFile(path, ""); err != nil {
				m.rt.Logf("shimatzu tocl import failed %s: %v", path, err)
				_ = m.archive(path, "failed_dir")
				return
			}
			_ = m.archive(path, "processed_dir")
		}()
	}
}

func (m *Module) importFile(path, fallbackDate string) (int, int, error) {
	store := m.importStore()
	if store == nil {
		return 0, 0, errors.New("storage service unavailable")
	}
	data, err := parseShimatzuTOCL(path)
	if err != nil {
		return 0, 0, err
	}
	rules := m.sampleCodeRules()
	for _, analyte := range data.Analytes {
		if err := m.ensureAnalyte(analyte); err != nil {
			return 0, 0, err
		}
	}
	roundCache := map[string]int{}
	imported := 0
	sourceFile := filepath.Base(path)
	for _, item := range data.SampleRecords {
		item.Record = normalizeImportedRecord(item.Record, rules)
		runDate := effectiveDate(firstNonEmpty(item.RunDate, fallbackDate))
		roundNo := roundCache[runDate]
		if roundNo == 0 {
			roundNo, err = store.CurrentRoundNo(runDate)
			if err != nil {
				return imported, 0, err
			}
			roundCache[runDate] = roundNo
		}
		if _, _, _, err := store.RecordImportedResult(runDate, roundNo, item.Record, sourceFile); err != nil {
			return imported, 0, err
		}
		imported++
	}
	for _, record := range data.QCRecords {
		runDate := effectiveDate(firstNonEmpty(record.RunDate, fallbackDate))
		for _, result := range record.Results {
			savedRecord, err := m.ensureQCRecord(store, record, runDate, sourceFile)
			if err != nil {
				return imported, 0, err
			}
			if err := m.ensureQCTarget(store, record, result); err != nil {
				return imported, 0, err
			}
			if _, err := m.ensureQCAnalysis(store, savedRecord.ID, result, sourceFile); err != nil {
				return imported, 0, err
			}
			imported++
		}
	}
	return imported, 0, nil
}

func (m *Module) ensureAnalyte(item shimatzuAnalyte) error {
	service, ok := m.rt.Service("storage")
	if !ok {
		return errors.New("storage service unavailable")
	}
	store, ok := service.(analyteStore)
	if !ok {
		return errors.New("analyte store unavailable")
	}
	_, err := store.SaveAnalyte(coremodel.Analyte{
		Active:            true,
		Tag:               item.Tag,
		Code:              item.Tag,
		Name:              item.Name,
		Description:       "Auto-generated from Shimatzu TOC-L exports",
		ResultType:        "numeric",
		ResultFormatting:  "raw",
		ResultWeighting:   1,
		ResultMeasureUnit: item.Unit,
		ProtocolOptions: map[string]interface{}{
			"worklist_label": firstNonEmpty(item.Unit, item.Name),
		},
	})
	return err
}

func (m *Module) importStore() importStore {
	service, ok := m.rt.Service("storage")
	if !ok {
		return nil
	}
	store, _ := service.(importStore)
	return store
}

func (m *Module) fileTransport() fileTransportMeta {
	service, ok := m.rt.Service("transport-file")
	if !ok {
		return fileTransportMeta{}
	}
	raw, _ := service.(map[string]interface{})
	meta := fileTransportMeta{}
	if value, _ := raw["import_dir"].(string); value != "" {
		meta.ImportDir = value
	}
	if value, _ := raw["pattern"].(string); value != "" {
		meta.Pattern = value
	}
	return meta
}

func (m *Module) sampleCodeRules() sampleCodeRules {
	settings := m.rt.ModuleSettings("result-sync")
	return sampleCodeRules{
		SamplePrefixes: readStringList(settings["sample_prefixes"]),
		SampleSuffixes: readStringList(settings["sample_suffixes"]),
		Separators:     readStringList(settings["separators"]),
	}
}

func (m *Module) archive(path, settingKey string) error {
	settings := m.rt.ModuleSettings("transport-file")
	target, _ := settings[settingKey].(string)
	if strings.TrimSpace(target) == "" {
		return nil
	}
	target = m.rt.ResolvePath(target)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	return os.Rename(path, filepath.Join(target, filepath.Base(path)))
}

func (m *Module) begin(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.running[path]; ok {
		return false
	}
	m.running[path] = struct{}{}
	return true
}

func (m *Module) end(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.running, path)
}

func (m *Module) ensureQCRecord(store importStore, item shimatzuQCRecord, runDate, sourceFile string) (coremodel.QCRecord, error) {
	return store.UpsertQCRecord(coremodel.QCRecord{
		RoundNo:      1,
		RunDate:      runDate,
		ControlLabel: item.ControlLabel,
		ControlLevel: firstNonEmpty(item.ControlLevel, "QC"),
		LotNo:        firstNonEmpty(item.LotNo, item.ControlLabel, "-"),
		FileID:       item.FileID,
		Status:       firstNonEmpty(item.Status, "completed"),
		SourceFile:   sourceFile,
	})
}

func (m *Module) ensureQCTarget(store importStore, record shimatzuQCRecord, item shimatzuQCResult) error {
	targets, err := store.ListQCTargets()
	if err != nil {
		return err
	}
	lotNo := firstNonEmpty(record.LotNo, record.ControlLabel, "-")
	controlLevel := firstNonEmpty(record.ControlLevel, "QC")
	for _, target := range targets {
		if strings.EqualFold(target.AnalyteTag, item.AnalyteTag) &&
			strings.EqualFold(target.ControlLevel, controlLevel) &&
			strings.EqualFold(target.LotNo, lotNo) {
			return nil
		}
	}
	_, err = store.SaveQCTarget(coremodel.QCTarget{
		Active:       true,
		AnalyteTag:   item.AnalyteTag,
		AnalyteName:  item.AnalyteName,
		ControlLevel: controlLevel,
		LotNo:        lotNo,
		Unit:         item.Unit,
		TargetMean:   0,
		TargetSD:     0,
		TargetCV:     0,
		Notes:        "Creat automat din import QC Shimatzu TOC-L. Definiti media si 1SD in Setari QC.",
	})
	return err
}

func (m *Module) ensureQCAnalysis(store importStore, qcRecordID int64, item shimatzuQCResult, sourceFile string) (coremodel.QCAnalysis, error) {
	analyses, err := store.ListQCAnalyses(qcRecordID)
	if err == nil {
		for _, existing := range analyses {
			if strings.EqualFold(existing.AnalyteTag, item.AnalyteTag) {
				return store.UpsertQCAnalysis(coremodel.QCAnalysis{
					ID:              existing.ID,
					QCRecordID:      qcRecordID,
					AnalyteID:       existing.AnalyteID,
					AnalyteTag:      item.AnalyteTag,
					AnalyteName:     firstNonEmpty(item.AnalyteName, existing.AnalyteName),
					ControlLevel:    firstNonEmpty(existing.ControlLevel, "QC"),
					LotNo:           firstNonEmpty(existing.LotNo, "-"),
					Status:          "completed",
					DefaultResultID: existing.DefaultResultID,
					ResultValue:     item.ResultValue,
					RawValue:        item.RawValue,
					Interpreted:     item.Interpreted,
					Unit:            item.Unit,
					SourceFile:      sourceFile,
					Flags:           cloneMap(item.Flags),
				})
			}
		}
	}
	return store.UpsertQCAnalysis(coremodel.QCAnalysis{
		QCRecordID:  qcRecordID,
		AnalyteTag:  item.AnalyteTag,
		AnalyteName: item.AnalyteName,
		Status:      "completed",
		ResultValue: item.ResultValue,
		RawValue:    item.RawValue,
		Interpreted: item.Interpreted,
		Unit:        item.Unit,
		SourceFile:  sourceFile,
		Flags:       cloneMap(item.Flags),
	})
}

type shimatzuAnalyte struct {
	Tag  string
	Name string
	Unit string
}

type shimatzuImportData struct {
	SampleRecords []shimatzuSampleRecord
	QCRecords     []shimatzuQCRecord
	Analytes      []shimatzuAnalyte
}

type shimatzuSampleRecord struct {
	RunDate string
	Record  coremodel.ImportedRecord
}

type shimatzuQCRecord struct {
	RunDate      string
	ControlLabel string
	ControlLevel string
	LotNo        string
	FileID       string
	Status       string
	Results      []shimatzuQCResult
}

type shimatzuQCResult struct {
	AnalyteTag  string
	AnalyteName string
	ResultValue string
	RawValue    string
	Interpreted string
	Unit        string
	Flags       map[string]interface{}
}

func parseShimatzuTOCL(path string) (shimatzuImportData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return shimatzuImportData{}, err
	}
	rows, err := parseDataRows(string(raw))
	if err != nil {
		return shimatzuImportData{}, err
	}
	analytesByTag := map[string]shimatzuAnalyte{}
	samplesByKey := map[string]shimatzuSampleRecord{}
	qcByKey := map[string]shimatzuQCRecord{}
	sourceFile := filepath.Base(path)
	for _, row := range rows {
		sampleID := normalizeSampleID(row["Sample ID"])
		if sampleID == "" {
			continue
		}
		runDate := parseDate(row["Date / Time"])
		for _, result := range rowResults(row) {
			analytesByTag[result.AnalyteTag] = shimatzuAnalyte{
				Tag:  result.AnalyteTag,
				Name: result.AnalyteName,
				Unit: result.Unit,
			}
			flags := map[string]interface{}{
				"source":       "shimatzu_tocl_txt",
				"sample_name":  strings.TrimSpace(row["Sample Name"]),
				"sample_raw":   strings.TrimSpace(row["Sample ID"]),
				"analysis":     strings.TrimSpace(row["Anal."]),
				"result_type":  result.AnalyteTag,
				"vial":         strings.TrimSpace(row["Vial"]),
				"measured_at":  parseTimestamp(row["Date / Time"]),
				"source_file":  sourceFile,
				"imported_tag": result.AnalyteTag,
			}
			interpreted := buildInterpreted(result.AnalyteTag, result.RawValue, result.Unit, flags["measured_at"])
			if isControlSample(sampleID) {
				key := sampleID + "|" + result.AnalyteTag + "|" + runDate
				qcResult := shimatzuQCResult{
					AnalyteTag:  result.AnalyteTag,
					AnalyteName: result.AnalyteName,
					ResultValue: result.RawValue,
					RawValue:    result.RawValue,
					Interpreted: interpreted,
					Unit:        result.Unit,
					Flags:       flags,
				}
				existing := qcByKey[key]
				if len(existing.Results) == 0 {
					qcByKey[key] = shimatzuQCRecord{
						RunDate:      runDate,
						ControlLabel: sampleID,
						ControlLevel: "QC",
						LotNo:        sampleID,
						FileID:       sampleID,
						Status:       "completed",
						Results:      []shimatzuQCResult{qcResult},
					}
				} else {
					replaced := false
					for i := range existing.Results {
						if strings.EqualFold(existing.Results[i].AnalyteTag, qcResult.AnalyteTag) {
							existing.Results[i] = qcResult
							replaced = true
							break
						}
					}
					if !replaced {
						existing.Results = append(existing.Results, qcResult)
					}
					qcByKey[key] = existing
				}
				continue
			}
			record := coremodel.ImportedRecord{
				SampleID:    sampleID,
				FileID:      sampleID,
				PatientID:   sampleID,
				PatientName: sampleID,
				AnalyteTag:  result.AnalyteTag,
				AnalyteName: result.AnalyteName,
				ResultValue: result.RawValue,
				RawValue:    result.RawValue,
				Interpreted: interpreted,
				Flags:       flags,
				Unit:        result.Unit,
				Meta:        map[string]interface{}{},
			}
			key := runDate + "|" + sampleID + "|" + result.AnalyteTag
			samplesByKey[key] = shimatzuSampleRecord{RunDate: runDate, Record: record}
		}
	}
	samples := make([]shimatzuSampleRecord, 0, len(samplesByKey))
	for _, item := range samplesByKey {
		samples = append(samples, item)
	}
	sort.Slice(samples, func(i, j int) bool {
		if samples[i].RunDate != samples[j].RunDate {
			return samples[i].RunDate < samples[j].RunDate
		}
		if samples[i].Record.SampleID != samples[j].Record.SampleID {
			return samples[i].Record.SampleID < samples[j].Record.SampleID
		}
		return samples[i].Record.AnalyteTag < samples[j].Record.AnalyteTag
	})
	qcRecords := make([]shimatzuQCRecord, 0, len(qcByKey))
	for _, item := range qcByKey {
		qcRecords = append(qcRecords, item)
	}
	sort.Slice(qcRecords, func(i, j int) bool {
		if qcRecords[i].RunDate != qcRecords[j].RunDate {
			return qcRecords[i].RunDate < qcRecords[j].RunDate
		}
		return qcRecords[i].ControlLabel < qcRecords[j].ControlLabel
	})
	analytes := make([]shimatzuAnalyte, 0, len(analytesByTag))
	for _, item := range analytesByTag {
		analytes = append(analytes, item)
	}
	sort.Slice(analytes, func(i, j int) bool { return analytes[i].Tag < analytes[j].Tag })
	return shimatzuImportData{SampleRecords: samples, QCRecords: qcRecords, Analytes: analytes}, nil
}

type parsedResult struct {
	AnalyteTag  string
	AnalyteName string
	RawValue    string
	Unit        string
}

func parseDataRows(raw string) ([]map[string]string, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	inData := false
	var header []string
	rows := []map[string]string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.EqualFold(trimmed, "[Data]") {
			inData = true
			header = nil
			continue
		}
		if !inData {
			continue
		}
		cols := splitColumns(line)
		if len(cols) == 0 {
			continue
		}
		if header == nil {
			header = cols
			continue
		}
		row := map[string]string{}
		for i, name := range header {
			if i < len(cols) {
				row[name] = strings.TrimSpace(cols[i])
				continue
			}
			row[name] = ""
		}
		rows = append(rows, row)
	}
	if len(header) == 0 {
		return nil, fmt.Errorf("no [Data] section found")
	}
	return rows, nil
}

func rowResults(row map[string]string) []parsedResult {
	unit := strings.TrimSpace(row["Unit"])
	keys := []struct {
		Column string
		Tag    string
		Name   string
	}{
		{Column: "Result(TOC)", Tag: "TOC", Name: "TOC"},
		{Column: "Result(TC)", Tag: "TC", Name: "TC"},
		{Column: "Result(IC)", Tag: "IC", Name: "IC"},
		{Column: "Result(POC)", Tag: "POC", Name: "POC"},
		{Column: "Result(NPOC)", Tag: "NPOC", Name: "NPOC"},
		{Column: "Result(TN)", Tag: "TN", Name: "TN"},
	}
	out := make([]parsedResult, 0, len(keys))
	for _, item := range keys {
		value := normalizeNumber(row[item.Column])
		if value == "" {
			continue
		}
		out = append(out, parsedResult{
			AnalyteTag:  item.Tag,
			AnalyteName: item.Name,
			RawValue:    value,
			Unit:        unit,
		})
	}
	return out
}

func buildInterpreted(tag, value, unit string, measuredAt interface{}) string {
	parts := []string{}
	if tag != "" {
		parts = append(parts, "Analit="+tag)
	}
	if value != "" {
		parts = append(parts, "Valoare="+value)
	}
	if strings.TrimSpace(unit) != "" {
		parts = append(parts, "UM="+strings.TrimSpace(unit))
	}
	if text := strings.TrimSpace(fmt.Sprint(measuredAt)); text != "" && text != "<nil>" {
		parts = append(parts, "Data="+text)
	}
	return strings.Join(parts, " · ")
}

func parseDate(value string) string {
	if ts, ok := parseTime(value); ok {
		return ts.Format("2006-01-02")
	}
	return ""
}

func parseTimestamp(value string) string {
	if ts, ok := parseTime(value); ok {
		return ts.UTC().Format(time.RFC3339)
	}
	return ""
}

func parseTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"1/2/2006 3:04:05 PM", "1/2/2006 15:04:05", "01/02/2006 3:04:05 PM"} {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

func isControlSample(value string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	return strings.HasPrefix(value, "PC") || strings.HasPrefix(value, "ETALON")
}

func normalizeSampleID(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func normalizeImportedRecord(record coremodel.ImportedRecord, rules sampleCodeRules) coremodel.ImportedRecord {
	rawSentSampleCode := normalizeSampleID(record.SampleID)
	if raw := strings.TrimSpace(fmt.Sprint(record.Flags["sample_raw"])); raw != "" && raw != "<nil>" {
		rawSentSampleCode = normalizeSampleID(raw)
	}
	parts := normalizeImportedSampleParts(record.SampleID, rules)
	if parts.Normalized == "" {
		return record
	}
	if record.Meta == nil {
		record.Meta = map[string]interface{}{}
	}
	record.Meta["sent_sample_code"] = rawSentSampleCode
	record.SampleID = parts.Normalized
	record.FileID = firstNonEmpty(parts.FileID, parts.Normalized)
	record.PatientID = strings.TrimSpace(parts.SampleCodeID)
	record.PatientName = strings.TrimSpace(parts.SpecimenCode)
	return record
}

type importedSampleParts struct {
	Normalized   string
	FileID       string
	SampleCodeID string
	SpecimenCode string
}

func normalizeImportedSampleParts(value string, rules sampleCodeRules) importedSampleParts {
	value = normalizeSampleID(value)
	if value == "" || isControlSample(value) {
		return importedSampleParts{Normalized: value}
	}
	for _, prefix := range rules.SamplePrefixes {
		prefix = normalizeSampleID(prefix)
		if prefix != "" && strings.HasPrefix(value, prefix) {
			value = strings.TrimPrefix(value, prefix)
			break
		}
	}
	for _, suffix := range rules.SampleSuffixes {
		suffix = normalizeSampleID(suffix)
		if suffix != "" && strings.HasSuffix(value, suffix) {
			value = strings.TrimSuffix(value, suffix)
			break
		}
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return importedSampleParts{}
	}
	out := importedSampleParts{
		Normalized: value,
		FileID:     value,
	}
	separators := rules.Separators
	if len(separators) == 0 {
		separators = []string{"-"}
	}
	for _, separator := range separators {
		if separator == "" || !strings.Contains(value, separator) {
			continue
		}
		parts := strings.Split(value, separator)
		if len(parts) < 1 || len(parts) > 3 {
			continue
		}
		valid := true
		for _, item := range parts {
			item = strings.TrimSpace(item)
			if item == "" || !isDigits(item) {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		for _, other := range separators {
			if other == "" || other == separator {
				continue
			}
			if strings.Contains(value, other) {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		out.FileID = strings.TrimSpace(parts[0])
		if len(parts) > 1 {
			out.SampleCodeID = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			out.SpecimenCode = strings.TrimSpace(parts[2])
		}
		return out
	}
	return out
}

func readStringList(raw interface{}) []string {
	switch typed := raw.(type) {
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		parts := strings.Split(typed, ",")
		out := make([]string, 0, len(parts))
		for _, item := range parts {
			if text := strings.TrimSpace(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func splitColumns(line string) []string {
	parts := strings.Split(line, "\t")
	out := make([]string, len(parts))
	for i, part := range parts {
		out[i] = strings.TrimSpace(strings.ReplaceAll(part, "\u00a0", " "))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

func normalizeNumber(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, ",", ".")
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func effectiveDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now().Format("2006-01-02")
	}
	return value
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return map[string]interface{}{}
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func intSetting(settings map[string]interface{}, key string, fallback int) int {
	raw, ok := settings[key]
	if !ok {
		return fallback
	}
	switch x := raw.(type) {
	case int:
		return x
	case float64:
		return int(x)
	case string:
		if v, err := strconv.Atoi(strings.TrimSpace(x)); err == nil {
			return v
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
