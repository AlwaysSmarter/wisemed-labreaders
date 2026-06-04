package fileimportbase

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

type FileTransportMeta struct {
	ImportDir string
	Pattern   string
}

type SampleCodeRules struct {
	SamplePrefixes []string
	SampleSuffixes []string
	Separators     []string
}

type SampleParts struct {
	Normalized   string
	FileID       string
	SampleCodeID string
	SpecimenCode string
}

type AnalyteDef struct {
	Tag              string
	Code             string
	Name             string
	Description      string
	ResultType       string
	ResultFormatting string
	ResultWeighting  float64
	Unit             string
	ProtocolOptions  map[string]interface{}
}

type SampleRecord struct {
	RunDate string
	Record  coremodel.ImportedRecord
}

type QCRecord struct {
	RunDate      string
	ControlLabel string
	ControlLevel string
	LotNo        string
	FileID       string
	Status       string
	Meta         map[string]interface{}
	Results      []QCResult
}

type QCResult struct {
	AnalyteTag  string
	AnalyteName string
	ResultValue string
	RawValue    string
	Interpreted string
	Unit        string
	Flags       map[string]interface{}
	Meta        map[string]interface{}
}

type ImportData struct {
	SampleRecords []SampleRecord
	QCRecords     []QCRecord
	Analytes      []AnalyteDef
}

type ParserFunc func(path string, rt module.Runtime) (ImportData, error)

type Spec struct {
	ID                 string
	MenuID             string
	MenuLabel          string
	MenuPath           string
	MenuOrder          int
	ProtocolMeta       string
	ResponseProtocol   string
	AnalyteDescription string
	QCTargetNotes      string
	PollSecondsKey     string
	Parse              ParserFunc
}

type Module struct {
	rt      module.Runtime
	spec    Spec
	mu      sync.Mutex
	running map[string]struct{}
}

func New(spec Spec) module.Module {
	return &Module{spec: spec, running: map[string]struct{}{}}
}

func (m *Module) ID() string { return m.spec.ID }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	if m.spec.MenuID != "" {
		rt.AddMenu(module.MenuEntry{
			ID:    m.spec.MenuID,
			Group: "admin",
			Label: m.spec.MenuLabel,
			Path:  m.spec.MenuPath,
			Order: m.spec.MenuOrder,
		})
	}
	rt.RegisterService("file-importer", m)
	rt.Handle("/api/protocol/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"ok":true,"protocol":%q}`, firstNonEmpty(m.spec.ProtocolMeta, m.spec.ID))))
	}))
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	settings := m.rt.ModuleSettings(m.ID())
	pollKey := firstNonEmpty(m.spec.PollSecondsKey, "poll_seconds")
	pollSeconds := IntSetting(settings, pollKey, 2)
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
		"protocol":   firstNonEmpty(m.spec.ResponseProtocol, m.spec.ProtocolMeta, m.spec.ID),
		"order_date": EffectiveDate(orderDate),
	}, nil
}

func (m *Module) scanImportDir() {
	meta := m.fileTransport()
	if meta.ImportDir == "" || meta.Pattern == "" {
		return
	}
	files, err := filepath.Glob(filepath.Join(meta.ImportDir, meta.Pattern))
	if err != nil {
		m.rt.Logf("%s glob failed: %v", m.spec.ID, err)
		return
	}
	for _, path := range files {
		if !m.begin(path) {
			continue
		}
		func() {
			defer m.end(path)
			if _, _, err := m.importFile(path, ""); err != nil {
				m.rt.Logf("%s import failed %s: %v", m.spec.ID, path, err)
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
	data, err := m.spec.Parse(path, m.rt)
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
		item.Record = NormalizeImportedRecord(item.Record, rules)
		runDate := EffectiveDate(firstNonEmpty(item.RunDate, fallbackDate))
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
		runDate := EffectiveDate(firstNonEmpty(record.RunDate, fallbackDate))
		for _, result := range record.Results {
			savedRecord, err := m.ensureQCRecord(store, record, runDate, sourceFile)
			if err != nil {
				return imported, 0, err
			}
			if err := m.ensureQCTarget(store, record, result); err != nil {
				return imported, 0, err
			}
			if _, err := m.ensureQCAnalysis(store, savedRecord.ID, record, result, sourceFile); err != nil {
				return imported, 0, err
			}
			imported++
		}
	}
	return imported, 0, nil
}

func (m *Module) ensureAnalyte(item AnalyteDef) error {
	service, ok := m.rt.Service("storage")
	if !ok {
		return errors.New("storage service unavailable")
	}
	store, ok := service.(analyteStore)
	if !ok {
		return errors.New("analyte store unavailable")
	}
	resultType := firstNonEmpty(item.ResultType, "numeric")
	resultFormatting := firstNonEmpty(item.ResultFormatting, "raw")
	resultWeighting := item.ResultWeighting
	if resultWeighting == 0 {
		resultWeighting = 1
	}
	_, err := store.SaveAnalyte(coremodel.Analyte{
		Active:            true,
		Tag:               item.Tag,
		Code:              firstNonEmpty(item.Code, item.Tag),
		Name:              firstNonEmpty(item.Name, item.Tag),
		Description:       firstNonEmpty(item.Description, m.spec.AnalyteDescription, "Auto-generated from file imports"),
		ResultType:        resultType,
		ResultFormatting:  resultFormatting,
		ResultWeighting:   resultWeighting,
		ResultMeasureUnit: item.Unit,
		ProtocolOptions:   cloneMap(item.ProtocolOptions),
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

func (m *Module) fileTransport() FileTransportMeta {
	service, ok := m.rt.Service("transport-file")
	if !ok {
		return FileTransportMeta{}
	}
	raw, _ := service.(map[string]interface{})
	meta := FileTransportMeta{}
	if value, _ := raw["import_dir"].(string); value != "" {
		meta.ImportDir = value
	}
	if value, _ := raw["pattern"].(string); value != "" {
		meta.Pattern = value
	}
	return meta
}

func (m *Module) sampleCodeRules() SampleCodeRules {
	settings := m.rt.ModuleSettings("result-sync")
	return SampleCodeRules{
		SamplePrefixes: ReadStringList(settings["sample_prefixes"]),
		SampleSuffixes: ReadStringList(settings["sample_suffixes"]),
		Separators:     ReadStringList(settings["separators"]),
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

func (m *Module) ensureQCRecord(store importStore, item QCRecord, runDate, sourceFile string) (coremodel.QCRecord, error) {
	return store.UpsertQCRecord(coremodel.QCRecord{
		RoundNo:      1,
		RunDate:      runDate,
		ControlLabel: item.ControlLabel,
		ControlLevel: firstNonEmpty(item.ControlLevel, "QC"),
		LotNo:        firstNonEmpty(item.LotNo, item.ControlLabel, "-"),
		FileID:       item.FileID,
		Status:       firstNonEmpty(item.Status, "completed"),
		SourceFile:   sourceFile,
		Meta:         cloneMap(item.Meta),
	})
}

func (m *Module) ensureQCTarget(store importStore, record QCRecord, item QCResult) error {
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
		Notes:        firstNonEmpty(m.spec.QCTargetNotes, "Creat automat din import local. Definiti media si 1SD in Setari QC."),
	})
	return err
}

func (m *Module) ensureQCAnalysis(store importStore, qcRecordID int64, record QCRecord, item QCResult, sourceFile string) (coremodel.QCAnalysis, error) {
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
					ControlLevel:    firstNonEmpty(existing.ControlLevel, firstNonEmpty(record.ControlLevel, "QC")),
					LotNo:           firstNonEmpty(existing.LotNo, firstNonEmpty(record.LotNo, "-")),
					Status:          "completed",
					DefaultResultID: existing.DefaultResultID,
					ResultValue:     item.ResultValue,
					RawValue:        item.RawValue,
					Interpreted:     item.Interpreted,
					Unit:            item.Unit,
					SourceFile:      sourceFile,
					Flags:           cloneMap(item.Flags),
					Meta:            cloneMap(item.Meta),
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
		Meta:        cloneMap(item.Meta),
	})
}

func BuildInterpreted(tag, value, unit string, measuredAt interface{}) string {
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

func ParseDate(value string) string {
	if ts, ok := ParseTime(value); ok {
		return ts.Format("2006-01-02")
	}
	return ""
}

func ParseTimestamp(value string) string {
	if ts, ok := ParseTime(value); ok {
		return ts.UTC().Format(time.RFC3339)
	}
	return ""
}

func ParseTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"02/01/2006 15:04:05",
		"02/01/2006 15:04",
		"1/2/2006 3:04:05 PM",
		"1/2/2006 15:04:05",
		"01/02/2006 3:04:05 PM",
		"1/2/2006",
		"01/02/2006",
		"1/2/2006 3:04 PM",
		"2006/01/02 15:04:05",
	} {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

func NormalizeSampleID(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func NormalizeImportedRecord(record coremodel.ImportedRecord, rules SampleCodeRules) coremodel.ImportedRecord {
	rawSentSampleCode := NormalizeSampleID(record.SampleID)
	if raw := strings.TrimSpace(fmt.Sprint(record.Flags["sample_raw"])); raw != "" && raw != "<nil>" {
		rawSentSampleCode = NormalizeSampleID(raw)
	}
	parts := NormalizeImportedSampleParts(record.SampleID, rules)
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

func NormalizeImportedSampleParts(value string, rules SampleCodeRules) SampleParts {
	value = NormalizeSampleID(value)
	if value == "" {
		return SampleParts{Normalized: value}
	}
	for _, prefix := range rules.SamplePrefixes {
		prefix = NormalizeSampleID(prefix)
		if prefix != "" && strings.HasPrefix(value, prefix) {
			value = strings.TrimPrefix(value, prefix)
			break
		}
	}
	for _, suffix := range rules.SampleSuffixes {
		suffix = NormalizeSampleID(suffix)
		if suffix != "" && strings.HasSuffix(value, suffix) {
			value = strings.TrimSuffix(value, suffix)
			break
		}
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return SampleParts{}
	}
	out := SampleParts{
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

func ReadStringList(raw interface{}) []string {
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

func SplitColumns(line string, separator string) []string {
	parts := strings.Split(line, separator)
	out := make([]string, len(parts))
	for i, part := range parts {
		out[i] = strings.TrimSpace(strings.ReplaceAll(part, "\u00a0", " "))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

func NormalizeNumber(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, ",", ".")
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func EffectiveDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now().Format("2006-01-02")
	}
	return value
}

func CloneMap(src map[string]interface{}) map[string]interface{} { return cloneMap(src) }

func IntSetting(settings map[string]interface{}, key string, fallback int) int {
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

func SortImportData(data *ImportData) {
	sort.Slice(data.SampleRecords, func(i, j int) bool {
		if data.SampleRecords[i].RunDate != data.SampleRecords[j].RunDate {
			return data.SampleRecords[i].RunDate < data.SampleRecords[j].RunDate
		}
		if data.SampleRecords[i].Record.SampleID != data.SampleRecords[j].Record.SampleID {
			return data.SampleRecords[i].Record.SampleID < data.SampleRecords[j].Record.SampleID
		}
		return data.SampleRecords[i].Record.AnalyteTag < data.SampleRecords[j].Record.AnalyteTag
	})
	sort.Slice(data.QCRecords, func(i, j int) bool {
		if data.QCRecords[i].RunDate != data.QCRecords[j].RunDate {
			return data.QCRecords[i].RunDate < data.QCRecords[j].RunDate
		}
		return data.QCRecords[i].ControlLabel < data.QCRecords[j].ControlLabel
	})
	sort.Slice(data.Analytes, func(i, j int) bool { return data.Analytes[i].Tag < data.Analytes[j].Tag })
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
