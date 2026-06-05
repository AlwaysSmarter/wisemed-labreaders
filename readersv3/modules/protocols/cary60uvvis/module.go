package cary60uvvis

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
	SaveDailyDetailValue(item coremodel.DailyDetailValue) (coremodel.DailyDetailValue, error)
}

type fileTransportMeta struct {
	ImportDir string
	Pattern   string
}

type Module struct {
	rt      module.Runtime
	mu      sync.Mutex
	running map[string]struct{}
}

func New() module.Module     { return &Module{running: map[string]struct{}{}} }
func (m *Module) ID() string { return "protocol-cary60-uvvis" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.AddMenu(module.MenuEntry{ID: "protocol-cary60", Group: "admin", Label: "Protocol Cary60 UV-VIS", Path: "/settings/protocol/cary60", Order: 45})
	rt.RegisterService("file-importer", m)
	rt.Handle("/api/protocol/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"ok":true,"protocol":"cary60-uvvis"}`))
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
	m.logf(4, "cary60 manual import requested file=%s order_date=%s", path, effectiveDate(orderDate))
	imported, warnings, err := m.importFile(path, orderDate)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ok":         true,
		"file_name":  filepath.Base(path),
		"imported":   imported,
		"warnings":   warnings,
		"protocol":   "CARY60_UVVIS_CSV",
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
		m.rt.Logf("cary60 glob failed: %v", err)
		return
	}
	m.logf(5, "cary60 scan import_dir=%s pattern=%s matched_files=%d", meta.ImportDir, meta.Pattern, len(files))
	for _, path := range files {
		if !m.begin(path) {
			m.logIgnored("file", "already processing", map[string]interface{}{"file": path})
			continue
		}
		m.logf(4, "cary60 picked import file=%s", path)
		func() {
			defer m.end(path)
			if _, _, err := m.importFile(path, ""); err != nil {
				m.rt.Logf("cary60 import failed %s: %v", path, err)
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
	m.logf(4, "cary60 import start file=%s fallback_date=%s", path, effectiveDate(fallbackDate))
	data, err := parseCary60CSV(path, m.logIgnored)
	if err != nil {
		return 0, 0, err
	}
	m.logf(4, "cary60 parse ok file=%s sample_records=%d qc_records=%d analytes=%d daily_details=%d", path, len(data.SampleRecords), len(data.QCRecords), len(data.Analytes), len(data.DailyDetails))
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
		if len(data.DailyDetails) > 0 {
			preview["first_daily_detail"] = data.DailyDetails[0]
		}
		if blob, err := json.Marshal(preview); err == nil {
			m.rt.Logf("cary60 parse preview %s", string(blob))
		}
	}
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
	for _, item := range data.DailyDetails {
		if _, err := store.SaveDailyDetailValue(item); err != nil {
			return 0, 0, err
		}
	}
	roundCache := map[string]int{}
	autoSaveTargets := map[string]*fileimportbase.AutoSaveTarget{}
	imported := 0
	sourceFile := filepath.Base(path)
	for _, item := range data.SampleRecords {
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
			m.rt.Logf("cary60 analyte sync warning %s: %v", path, err)
		}
	}
	if err := fileimportbase.AutoSaveResultsToWiseMED(m.rt, fileimportbase.FlattenAutoSaveTargets(autoSaveTargets)); err != nil {
		warnings++
		m.rt.Logf("cary60 result autosave warning %s: %v", path, err)
	}
	m.logf(4, "cary60 import done file=%s imported=%d warnings=%d analytes_changed=%t", path, imported, warnings, analytesChanged)
	return imported, warnings, nil
}

func (m *Module) ensureAnalyte(known map[string]coremodel.Analyte, item cary60Analyte) (bool, error) {
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
		Description:       "Auto-generated from Cary60 UV-VIS reports",
		ResultType:        "numeric",
		ResultFormatting:  "raw",
		ResultWeighting:   1,
		ResultMeasureUnit: item.Unit,
		ProtocolOptions: map[string]interface{}{
			"worklist_label": defaultCaryWorklistLabel(item.Tag, item.Unit),
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
		m.rt.Logf("cary60 ignored %s", string(blob))
		return
	}
	m.rt.Logf("cary60 ignored kind=%s reason=%s", kind, reason)
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

func (m *Module) ensureQCRecord(store importStore, item cary60QCRecord, runDate, sourceFile string) (coremodel.QCRecord, error) {
	return store.UpsertQCRecord(coremodel.QCRecord{
		RoundNo:      1,
		RunDate:      runDate,
		ControlLabel: item.ControlLabel,
		ControlLevel: firstNonEmpty(item.ControlLevel, "QC"),
		LotNo:        firstNonEmpty(item.LotNo, item.ControlLabel, "-"),
		DiluentInfo:  item.DiluentInfo,
		FileID:       item.FileID,
		Status:       firstNonEmpty(item.Status, "completed"),
		SourceFile:   sourceFile,
	})
}

func (m *Module) ensureQCTarget(store importStore, record cary60QCRecord, item cary60QCResult) error {
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
		Notes:        "Creat automat din import QC Cary60. Definiti media si 1SD in Setari QC.",
	})
	return err
}

func (m *Module) ensureQCAnalysis(store importStore, qcRecordID int64, item cary60QCResult, sourceFile string) (coremodel.QCAnalysis, error) {
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

type cary60Analyte struct {
	Tag  string
	Name string
	Unit string
}

type cary60ImportData struct {
	SampleRecords []cary60SampleRecord
	QCRecords     []cary60QCRecord
	Analytes      []cary60Analyte
	DailyDetails  []coremodel.DailyDetailValue
}

type cary60SampleRecord struct {
	RunDate string
	Record  coremodel.ImportedRecord
}

type cary60QCRecord struct {
	RunDate      string
	ControlLabel string
	ControlLevel string
	LotNo        string
	DiluentInfo  string
	FileID       string
	Status       string
	Results      []cary60QCResult
}

type cary60QCResult struct {
	AnalyteTag  string
	AnalyteName string
	ResultValue string
	RawValue    string
	Interpreted string
	Unit        string
	MeasuredAt  string
	Flags       map[string]interface{}
}

type cary60Section struct {
	Method         string
	BatchName      string
	Units          string
	Zero           string
	ReportTime     string
	CollectionTime string
	Header         []string
	Rows           [][]string
}

type cary60Row struct {
	Sample        string
	Concentration float64
	Factor        float64
	Reading       float64
	Flag          string
}

func parseCary60CSV(path string, ignored ...func(kind, reason string, payload map[string]interface{})) (cary60ImportData, error) {
	var logIgnored func(kind, reason string, payload map[string]interface{})
	if len(ignored) > 0 {
		logIgnored = ignored[0]
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cary60ImportData{}, err
	}
	sections := parseCary60Sections(string(raw))
	if len(sections) == 0 {
		return cary60ImportData{}, fmt.Errorf("no Cary60 analysis sections found in %s", path)
	}
	samplesByKey := map[string]cary60SampleRecord{}
	qcByKey := map[string]cary60QCRecord{}
	analytesByTag := map[string]cary60Analyte{}
	dailyDetailsByKey := map[string]coremodel.DailyDetailValue{}
	fileName := filepath.Base(path)
	for _, section := range sections {
		analyte := detectCary60Analyte(path, section)
		if analyte.Tag == "" {
			if logIgnored != nil {
				logIgnored("section", "analyte could not be detected", map[string]interface{}{
					"file":       fileName,
					"method":     strings.TrimSpace(section.Method),
					"batch_name": strings.TrimSpace(section.BatchName),
					"units":      strings.TrimSpace(section.Units),
				})
			}
			continue
		}
		analytesByTag[analyte.Tag] = analyte
		runDate := firstNonEmpty(parseCary60Date(section.CollectionTime), parseCary60Date(section.ReportTime))
		measuredAt := firstNonEmpty(parseCary60Timestamp(section.CollectionTime), parseCary60Timestamp(section.ReportTime))
		if runDate != "" {
			if zero := normalizeNumber(section.Zero); zero != "" {
				key := "zero_report|" + runDate + "|" + analyte.Tag
				dailyDetailsByKey[key] = coremodel.DailyDetailValue{
					DefinitionKey: "zero_report",
					ScopeDate:     runDate,
					RoundNo:       0,
					AnalyteTag:    analyte.Tag,
					ValueText:     zero,
				}
			}
			if units := strings.TrimSpace(section.Units); units != "" {
				key := "concentration_units|" + runDate + "|" + analyte.Tag
				dailyDetailsByKey[key] = coremodel.DailyDetailValue{
					DefinitionKey: "concentration_units",
					ScopeDate:     runDate,
					RoundNo:       0,
					AnalyteTag:    analyte.Tag,
					ValueText:     units,
				}
			}
		}
		domain := detectCary60Domain(section)
		rows := parseCary60SectionRows(section, logIgnored)
		for _, row := range rows {
			sampleID := normalizeCary60SampleID(row.Sample)
			if sampleID == "" {
				if logIgnored != nil {
					logIgnored("row", "missing sample id", map[string]interface{}{
						"file":       fileName,
						"analyte":    analyte.Tag,
						"sample_raw": row.Sample,
					})
				}
				continue
			}
			finalNumeric := row.Concentration
			if row.Factor > 1 {
				finalNumeric = row.Concentration * row.Factor
			}
			finalText := formatDecimal(finalNumeric)
			measuredText := formatDecimal(row.Concentration)
			readingText := formatDecimal(row.Reading)
			flags := map[string]interface{}{
				"source":                 "cary60_uvvis_csv",
				"domain":                 domain,
				"method":                 section.Method,
				"batch_name":             section.BatchName,
				"measurement_unit":       analyte.Unit,
				"measured_concentration": measuredText,
				"final_concentration":    finalText,
				"reading_absorbance":     readingText,
				"zero_absorbance":        normalizeNumber(section.Zero),
				"dilution_factor":        formatDecimal(row.Factor),
				"flag_code":              row.Flag,
				"repeat":                 strings.EqualFold(strings.TrimSpace(row.Flag), "R"),
				"sample_raw":             row.Sample,
				"source_file":            fileName,
			}
			if measuredAt != "" {
				flags["measured_at"] = measuredAt
			}
			interpreted := buildInterpreted(section, domain, measuredText, finalText, readingText, row.Factor)
			if isControlRow(row.Sample) {
				key := strings.ToUpper(strings.TrimSpace(row.Sample)) + "|" + analyte.Tag + "|" + runDate
				result := cary60QCResult{AnalyteTag: analyte.Tag, AnalyteName: analyte.Name, ResultValue: finalText, RawValue: finalText, Interpreted: interpreted, Unit: analyte.Unit, MeasuredAt: measuredAt, Flags: flags}
				existing := qcByKey[key]
				if len(existing.Results) == 0 {
					qcByKey[key] = cary60QCRecord{RunDate: runDate, ControlLabel: sampleID, ControlLevel: "QC", LotNo: sampleID, FileID: fileName, Status: "completed", Results: []cary60QCResult{result}}
				} else {
					existing.Results[0] = chooseBetterQC(existing.Results[0], result)
					qcByKey[key] = existing
				}
				continue
			}
			record := coremodel.ImportedRecord{
				SampleID:    sampleID,
				FileID:      sampleID,
				PatientID:   sampleID,
				PatientName: sampleID,
				AnalyteTag:  analyte.Tag,
				AnalyteName: analyte.Name,
				ResultValue: finalText,
				RawValue:    finalText,
				Interpreted: interpreted,
				Flags:       flags,
				Unit:        analyte.Unit,
				Meta:        map[string]interface{}{},
			}
			key := runDate + "|" + sampleID + "|" + analyte.Tag
			candidate := cary60SampleRecord{RunDate: runDate, Record: record}
			if existing, ok := samplesByKey[key]; ok {
				samplesByKey[key] = chooseBetterSample(existing, candidate)
			} else {
				samplesByKey[key] = candidate
			}
		}
	}
	sampleRecords := make([]cary60SampleRecord, 0, len(samplesByKey))
	for _, item := range samplesByKey {
		sampleRecords = append(sampleRecords, item)
	}
	sort.Slice(sampleRecords, func(i, j int) bool {
		if sampleRecords[i].RunDate != sampleRecords[j].RunDate {
			return sampleRecords[i].RunDate < sampleRecords[j].RunDate
		}
		if sampleRecords[i].Record.SampleID != sampleRecords[j].Record.SampleID {
			return sampleRecords[i].Record.SampleID < sampleRecords[j].Record.SampleID
		}
		return sampleRecords[i].Record.AnalyteTag < sampleRecords[j].Record.AnalyteTag
	})
	qcRecords := make([]cary60QCRecord, 0, len(qcByKey))
	for _, item := range qcByKey {
		qcRecords = append(qcRecords, item)
	}
	sort.Slice(qcRecords, func(i, j int) bool {
		if qcRecords[i].RunDate != qcRecords[j].RunDate {
			return qcRecords[i].RunDate < qcRecords[j].RunDate
		}
		return qcRecords[i].ControlLabel < qcRecords[j].ControlLabel
	})
	analytes := make([]cary60Analyte, 0, len(analytesByTag))
	for _, item := range analytesByTag {
		analytes = append(analytes, item)
	}
	sort.Slice(analytes, func(i, j int) bool { return analytes[i].Tag < analytes[j].Tag })
	dailyDetails := make([]coremodel.DailyDetailValue, 0, len(dailyDetailsByKey))
	for _, item := range dailyDetailsByKey {
		dailyDetails = append(dailyDetails, item)
	}
	sort.Slice(dailyDetails, func(i, j int) bool {
		if dailyDetails[i].ScopeDate != dailyDetails[j].ScopeDate {
			return dailyDetails[i].ScopeDate < dailyDetails[j].ScopeDate
		}
		return dailyDetails[i].AnalyteTag < dailyDetails[j].AnalyteTag
	})
	return cary60ImportData{SampleRecords: sampleRecords, QCRecords: qcRecords, Analytes: analytes, DailyDetails: dailyDetails}, nil
}

func parseCary60Sections(raw string) []cary60Section {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	sections := make([]cary60Section, 0)
	var current *cary60Section
	collectingRows := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		switch {
		case strings.EqualFold(trimmed, "Concentration Analysis Report"):
			if current != nil && len(current.Rows) > 0 {
				sections = append(sections, *current)
			}
			current = &cary60Section{}
			collectingRows = false
		case current == nil:
			continue
		case strings.HasPrefix(trimmed, "Results Flags Legend"):
			if len(current.Rows) > 0 {
				sections = append(sections, *current)
			}
			current = nil
			collectingRows = false
		default:
			if header := parseHeader(trimmed); len(header) > 0 {
				current.Header = header
				collectingRows = true
				continue
			}
			if collectingRows {
				row := splitColumns(line)
				if len(row) > 0 {
					current.Rows = append(current.Rows, row)
				}
				continue
			}
			key, value := parseKeyValue(line)
			switch normalizeToken(key) {
			case "METHOD":
				current.Method = value
			case "BATCH_NAME":
				current.BatchName = value
			case "CONCENTRATION_UNITS":
				current.Units = normalizeUnit(value)
			case "ZERO":
				current.Zero = value
			case "REPORT_TIME":
				current.ReportTime = value
			case "COLLECTION_TIME":
				current.CollectionTime = value
			}
		}
	}
	if current != nil && len(current.Rows) > 0 {
		sections = append(sections, *current)
	}
	return sections
}

func parseCary60SectionRows(section cary60Section, logIgnored func(kind, reason string, payload map[string]interface{})) []cary60Row {
	index := map[string]int{}
	for i, col := range section.Header {
		index[normalizeToken(col)] = i
	}
	rows := make([]cary60Row, 0, len(section.Rows))
	for _, row := range section.Rows {
		sample := valueAt(index, row, "SAMPLE")
		if strings.TrimSpace(sample) == "" {
			if logIgnored != nil {
				logIgnored("row", "empty sample column", map[string]interface{}{
					"method":     strings.TrimSpace(section.Method),
					"batch_name": strings.TrimSpace(section.BatchName),
					"row":        row,
				})
			}
			continue
		}
		concentration, ok := parseNumber(valueAt(index, row, "CONCENTRATION_MG_L", "CONCENTRATION_UG_L", "CONCENTRATION_UGAL_L", "CONCENTRATION_UGFE_L"))
		if !ok {
			if logIgnored != nil {
				logIgnored("row", "missing numeric concentration", map[string]interface{}{
					"method":        strings.TrimSpace(section.Method),
					"batch_name":    strings.TrimSpace(section.BatchName),
					"sample":        strings.TrimSpace(sample),
					"concentration": valueAt(index, row, "CONCENTRATION_MG_L", "CONCENTRATION_UG_L", "CONCENTRATION_UGAL_L", "CONCENTRATION_UGFE_L"),
				})
			}
			continue
		}
		factor := 1.0
		if parsed, ok := parseNumber(valueAt(index, row, "FACTOR")); ok && parsed > 0 {
			factor = parsed
		}
		reading, _ := parseNumber(valueAt(index, row, "READINGS"))
		rows = append(rows, cary60Row{Sample: sample, Concentration: concentration, Factor: factor, Reading: reading, Flag: strings.TrimSpace(valueAt(index, row, "F"))})
	}
	return rows
}

func detectCary60Analyte(path string, section cary60Section) cary60Analyte {
	source := strings.ToLower(path + " " + section.Method + " " + section.BatchName)
	switch {
	case strings.Contains(source, "aluminiu") || strings.Contains(filepath.Base(strings.ToLower(path)), "al-"):
		return cary60Analyte{Tag: "ALUMINIU", Name: "Aluminiu", Unit: firstNonEmpty(section.Units, "ug/L")}
	case strings.Contains(source, "bor"):
		return cary60Analyte{Tag: "BOR", Name: "Bor", Unit: firstNonEmpty(section.Units, "mg/L")}
	case strings.Contains(source, "fier"):
		return cary60Analyte{Tag: "FIER", Name: "Fier", Unit: firstNonEmpty(section.Units, "ug/L")}
	case strings.Contains(source, "sulfat"):
		return cary60Analyte{Tag: "SULFATI", Name: "Sulfati", Unit: firstNonEmpty(section.Units, "mg/L")}
	default:
		return cary60Analyte{}
	}
}

func defaultCaryWorklistLabel(tag, unit string) string {
	switch strings.ToUpper(strings.TrimSpace(tag)) {
	case "ALUMINIU":
		return "0-50 / 100-500 " + firstNonEmpty(unit, "ug/L")
	case "BOR":
		return "0-0.2 / 0.2-1 " + firstNonEmpty(unit, "mg/L")
	case "FIER":
		return "Concentratie / " + firstNonEmpty(unit, "ug/L")
	case "SULFATI":
		return "Concentratie / " + firstNonEmpty(unit, "mg/L")
	default:
		return strings.TrimSpace(unit)
	}
}

func detectCary60Domain(section cary60Section) string {
	source := strings.ToLower(section.Method + " " + section.BatchName)
	switch {
	case strings.Contains(source, "domeniumic") || strings.Contains(source, "dmic"):
		return "mic"
	case strings.Contains(source, "domeniu100-500") || strings.Contains(source, "dmare"):
		if strings.Contains(source, "dilutie") {
			return "mare_dilutie"
		}
		return "mare"
	case strings.Contains(source, "dilutie"):
		return "dilutie"
	default:
		return "implicit"
	}
}

func buildInterpreted(section cary60Section, domain, measured, final, reading string, factor float64) string {
	parts := []string{}
	if domain != "" {
		parts = append(parts, "Domeniu="+domain)
	}
	if measured != "" {
		parts = append(parts, "Masurat="+measured)
	}
	if factor > 1 {
		parts = append(parts, "Fdil="+formatDecimal(factor))
	}
	if final != "" && final != measured {
		parts = append(parts, "Final="+final)
	}
	if reading != "" {
		parts = append(parts, "A="+reading)
	}
	if zero := normalizeNumber(section.Zero); zero != "" {
		parts = append(parts, "Martor="+zero)
	}
	return strings.Join(parts, " · ")
}

func chooseBetterSample(existing, candidate cary60SampleRecord) cary60SampleRecord {
	if boolFlag(candidate.Record.Flags, "repeat") && !boolFlag(existing.Record.Flags, "repeat") {
		return candidate
	}
	if score(candidate.Record.Flags) > score(existing.Record.Flags) {
		return candidate
	}
	return existing
}

func chooseBetterQC(existing, candidate cary60QCResult) cary60QCResult {
	if boolFlag(candidate.Flags, "repeat") && !boolFlag(existing.Flags, "repeat") {
		return candidate
	}
	if score(candidate.Flags) > score(existing.Flags) {
		return candidate
	}
	return existing
}

func score(flags map[string]interface{}) int {
	switch strings.ToLower(fmt.Sprint(flags["domain"])) {
	case "mare_dilutie":
		return 3
	case "mare":
		return 2
	case "mic":
		return 1
	default:
		return 0
	}
}

func boolFlag(flags map[string]interface{}, key string) bool {
	if flags == nil {
		return false
	}
	switch x := flags[key].(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(strings.TrimSpace(x), "true")
	default:
		return false
	}
}

func splitColumns(line string) []string {
	parts := strings.Split(line, "\t")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.ReplaceAll(part, "\u00a0", " "))
		if len(out) == 0 || part != "" || strings.Contains(line, "\t\t") {
			out = append(out, part)
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

func parseHeader(line string) []string {
	cols := splitColumns(line)
	if len(cols) >= 3 && strings.EqualFold(strings.TrimSpace(cols[0]), "Sample") {
		return cols
	}
	return nil
}

func parseKeyValue(line string) (string, string) {
	cols := splitColumns(line)
	if len(cols) < 2 {
		return "", ""
	}
	return cols[0], cols[1]
}

func valueAt(index map[string]int, row []string, keys ...string) string {
	for _, key := range keys {
		if idx, ok := index[key]; ok && idx >= 0 && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
	}
	return ""
}

func parseCary60Date(value string) string {
	if ts, ok := parseCary60Time(value); ok {
		return ts.Format("2006-01-02")
	}
	return ""
}

func parseCary60Timestamp(value string) string {
	if ts, ok := parseCary60Time(value); ok {
		return ts.UTC().Format(time.RFC3339)
	}
	return ""
}

func parseCary60Time(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"1/2/2006 3:04:05 PM", "1/2/2006 15:04:05", "01/02/2006 3:04:05 PM"} {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

func parseNumber(value string) (float64, bool) {
	value = normalizeNumber(value)
	if value == "" || value == "-" {
		return 0, false
	}
	v, err := strconv.ParseFloat(value, 64)
	return v, err == nil
}

func normalizeNumber(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, ",", ".")
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func normalizeUnit(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(value), "�", "u"), "µ", "u")
}

func isControlRow(value string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(value)), "PC")
}

func normalizeCary60SampleID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if isControlRow(value) {
		return strings.ToUpper(value)
	}
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "")
	return strings.ToUpper(value)
}

func formatDecimal(value float64) string {
	text := strconv.FormatFloat(value, 'f', 4, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" {
		return "0"
	}
	return text
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeToken(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "_", "-", "_", "/", "_", "\\", "_", "(", "", ")", "", ".", "", "�", "U", "µ", "U")
	value = replacer.Replace(value)
	for strings.Contains(value, "__") {
		value = strings.ReplaceAll(value, "__", "_")
	}
	return strings.Trim(value, "_")
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
