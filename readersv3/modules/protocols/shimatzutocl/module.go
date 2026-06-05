package shimatzutocl

import (
	"context"
	"encoding/json"
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
	"wisemed-labreaders/readersv3/modules/protocols/fileimportbase"
)

type analyteStore interface {
	ListAnalytes() ([]coremodel.Analyte, error)
	SaveAnalyte(item coremodel.Analyte) (coremodel.Analyte, error)
}

type wiseMedSyncService interface {
	SetupComplete() bool
	EnsureEquipmentOnline(reader map[string]interface{}) (map[string]interface{}, error)
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
	m.logf(4, "shimatzu tocl manual import requested file=%s order_date=%s", path, effectiveDate(orderDate))
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
	m.logf(5, "shimatzu tocl scan import_dir=%s pattern=%s matched_files=%d", meta.ImportDir, meta.Pattern, len(files))
	for _, path := range files {
		if !m.begin(path) {
			m.logIgnored("file", "already processing", map[string]interface{}{"file": path})
			continue
		}
		m.logf(4, "shimatzu tocl picked import file=%s", path)
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
	m.logf(4, "shimatzu tocl import start file=%s fallback_date=%s", path, effectiveDate(fallbackDate))
	data, err := parseShimatzuTOCL(path, m.logIgnored)
	if err != nil {
		return 0, 0, err
	}
	m.logf(4, "shimatzu tocl parse ok file=%s sample_records=%d qc_records=%d analytes=%d", path, len(data.SampleRecords), len(data.QCRecords), len(data.Analytes))
	if m.verboseLevel() >= 5 {
		preview := map[string]interface{}{}
		if len(data.SampleRecords) > 0 {
			preview["first_sample_record"] = data.SampleRecords[0]
		}
		if len(data.QCRecords) > 0 {
			preview["first_qc_record"] = data.QCRecords[0]
		}
		if len(data.Analytes) > 0 {
			limit := len(data.Analytes)
			if limit > 5 {
				limit = 5
			}
			preview["analyte_preview"] = data.Analytes[:limit]
		}
		if blob, err := json.Marshal(preview); err == nil {
			m.rt.Logf("shimatzu tocl parse preview %s", string(blob))
		}
	}
	rules := m.sampleCodeRules()
	knownAnalytes, err := m.listExistingAnalytes()
	if err != nil {
		return 0, 0, err
	}
	analytesChanged := false
	for _, analyte := range data.Analytes {
		changed, err := m.ensureAnalyte(knownAnalytes, analyte)
		if err != nil {
			return 0, 0, err
		}
		analytesChanged = analytesChanged || changed
	}
	roundCache := map[string]int{}
	autoSaveTargets := map[string]*fileimportbase.AutoSaveTarget{}
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
		order, _, _, err := store.RecordImportedResult(runDate, roundNo, item.Record, sourceFile)
		if err != nil {
			return imported, 0, err
		}
		fileimportbase.CollectAutoSaveTarget(autoSaveTargets, runDate, roundNo, order.ID)
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
	warnings := 0
	if analytesChanged {
		if err := m.syncAnalytesToWiseMED(); err != nil {
			warnings++
			m.rt.Logf("shimatzu tocl analyte sync warning %s: %v", path, err)
		}
	}
	if err := fileimportbase.AutoSaveResultsToWiseMED(m.rt, fileimportbase.FlattenAutoSaveTargets(autoSaveTargets)); err != nil {
		warnings++
		m.rt.Logf("shimatzu tocl result autosave warning %s: %v", path, err)
	}
	m.logf(4, "shimatzu tocl import done file=%s imported=%d warnings=%d analytes_changed=%t", path, imported, warnings, analytesChanged)
	return imported, warnings, nil
}

func (m *Module) ensureAnalyte(known map[string]coremodel.Analyte, item shimatzuAnalyte) (bool, error) {
	service, ok := m.rt.Service("storage")
	if !ok {
		return false, errors.New("storage service unavailable")
	}
	store, ok := service.(analyteStore)
	if !ok {
		return false, errors.New("analyte store unavailable")
	}
	target := coremodel.Analyte{
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
	}
	existing, ok := known[strings.TrimSpace(target.Tag)]
	if ok && analytesEquivalent(existing, target) {
		return false, nil
	}
	saved, err := store.SaveAnalyte(target)
	if err != nil {
		return false, err
	}
	known[strings.TrimSpace(saved.Tag)] = saved
	return true, nil
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

func (m *Module) listExistingAnalytes() (map[string]coremodel.Analyte, error) {
	service, ok := m.rt.Service("storage")
	if !ok {
		return nil, errors.New("storage service unavailable")
	}
	store, ok := service.(analyteStore)
	if !ok {
		return nil, errors.New("analyte store unavailable")
	}
	items, err := store.ListAnalytes()
	if err != nil {
		return nil, err
	}
	out := make(map[string]coremodel.Analyte, len(items))
	for _, item := range items {
		out[strings.TrimSpace(item.Tag)] = item
	}
	return out, nil
}

func analytesEquivalent(existing, target coremodel.Analyte) bool {
	if existing.Active != target.Active ||
		strings.TrimSpace(existing.Tag) != strings.TrimSpace(target.Tag) ||
		strings.TrimSpace(existing.Code) != strings.TrimSpace(target.Code) ||
		strings.TrimSpace(existing.Name) != strings.TrimSpace(target.Name) ||
		strings.TrimSpace(existing.Description) != strings.TrimSpace(target.Description) ||
		strings.TrimSpace(existing.ResultType) != strings.TrimSpace(target.ResultType) ||
		strings.TrimSpace(existing.ResultFormatting) != strings.TrimSpace(target.ResultFormatting) ||
		existing.ResultWeighting != target.ResultWeighting ||
		strings.TrimSpace(existing.ResultMeasureUnit) != strings.TrimSpace(target.ResultMeasureUnit) {
		return false
	}
	leftJSON, _ := json.Marshal(existing.ProtocolOptions)
	rightJSON, _ := json.Marshal(target.ProtocolOptions)
	return string(leftJSON) == string(rightJSON)
}

func (m *Module) syncAnalytesToWiseMED() error {
	service, ok := m.rt.Service("wisemed-api")
	if !ok {
		return nil
	}
	api, ok := service.(wiseMedSyncService)
	if !ok || !api.SetupComplete() {
		return nil
	}
	_, err := api.EnsureEquipmentOnline(nil)
	return err
}

func (m *Module) verboseLevel() int {
	raw := strings.TrimSpace(fmt.Sprint(m.rt.ModuleSettings("logging")["verbose_level"]))
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 1
	}
	if value > 5 {
		return 5
	}
	return value
}

func (m *Module) logf(level int, format string, args ...interface{}) {
	if m.verboseLevel() >= level {
		m.rt.Logf(format, args...)
	}
}

func (m *Module) logIgnored(kind, reason string, payload map[string]interface{}) {
	if m.verboseLevel() < 5 {
		return
	}
	entry := map[string]interface{}{
		"kind":   kind,
		"reason": reason,
	}
	for key, value := range payload {
		entry[key] = value
	}
	if blob, err := json.Marshal(entry); err == nil {
		m.rt.Logf("shimatzu tocl ignored %s", string(blob))
		return
	}
	m.rt.Logf("shimatzu tocl ignored kind=%s reason=%s", kind, reason)
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

func parseShimatzuTOCL(path string, ignored ...func(kind, reason string, payload map[string]interface{})) (shimatzuImportData, error) {
	var logIgnored func(kind, reason string, payload map[string]interface{})
	if len(ignored) > 0 {
		logIgnored = ignored[0]
	}
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
		sampleID := fileimportbase.PreferredSampleCode(row["Sample ID"], row["Sample Name"])
		sampleRaw := fileimportbase.PreferredRawSampleCode(row["Sample ID"], row["Sample Name"])
		if sampleID == "" {
			if logIgnored != nil {
				logIgnored("row", "missing sample id", map[string]interface{}{
					"sample_id":   strings.TrimSpace(row["Sample ID"]),
					"sample_name": strings.TrimSpace(row["Sample Name"]),
					"measured_at": strings.TrimSpace(row["Date / Time"]),
				})
			}
			continue
		}
		runDate := parseDate(row["Date / Time"])
		results := rowResults(row)
		if len(results) == 0 {
			if logIgnored != nil {
				logIgnored("row", "no analyte result columns with numeric value", map[string]interface{}{
					"sample_id":   sampleID,
					"sample_name": strings.TrimSpace(row["Sample Name"]),
					"analysis":    strings.TrimSpace(row["Anal."]),
					"unit":        strings.TrimSpace(row["Unit"]),
				})
			}
			continue
		}
		for _, result := range results {
			analytesByTag[result.AnalyteTag] = shimatzuAnalyte{
				Tag:  result.AnalyteTag,
				Name: result.AnalyteName,
				Unit: result.Unit,
			}
			flags := map[string]interface{}{
				"source":       "shimatzu_tocl_txt",
				"sample_name":  strings.TrimSpace(row["Sample Name"]),
				"sample_raw":   strings.TrimSpace(sampleRaw),
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
