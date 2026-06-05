package fileimportbase

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
	"wisemed-labreaders/readersv3/modules/wisemedapi"
)

type analyteStore interface {
	ListAnalytes() ([]coremodel.Analyte, error)
	SaveAnalyte(item coremodel.Analyte) (coremodel.Analyte, error)
}

type importStore interface {
	CurrentRoundNo(orderDate string) (int, error)
	RecordImportedResult(orderDate string, roundNo int, rec coremodel.ImportedRecord, sourceFile string) (coremodel.Order, coremodel.OrderAnalysis, coremodel.OrderAnalysisResult, error)
	ListOrderBundles(roundNo int, orderDate string) ([]coremodel.OrderBundle, error)
	ListQCRecords(roundNo int, runDate string) ([]coremodel.QCRecord, error)
	ListQCAnalyses(recordID int64) ([]coremodel.QCAnalysis, error)
	ListQCTargets() ([]coremodel.QCTarget, error)
	SaveQCTarget(item coremodel.QCTarget) (coremodel.QCTarget, error)
	UpsertQCRecord(item coremodel.QCRecord) (coremodel.QCRecord, error)
	UpsertQCAnalysis(item coremodel.QCAnalysis) (coremodel.QCAnalysis, error)
}

type FileTransportMeta struct {
	ImportDir    string
	ProcessedDir string
	FailedDir    string
	ExportDir    string
	Pattern      string
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
type AfterImportFunc func(path string, data ImportData, rt module.Runtime) error

type wiseMedSyncService interface {
	SetupComplete() bool
	EnsureEquipmentOnline(reader map[string]interface{}) (map[string]interface{}, error)
}

type wiseMedResultsService interface {
	SetupComplete() bool
	SaveFileServiceResults(fileID string, entries []wisemedapi.ServiceResultEntry) (map[string]interface{}, error)
}

type resultSyncService interface {
	RunOrders(orderIDs []int64, roundNo int, orderDate string) (map[string]interface{}, error)
}

type AutoSaveTarget struct {
	OrderDate string
	RoundNo   int
	OrderIDs  []int64
}

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
	AfterImport        AfterImportFunc
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
	meta := m.fileTransport()
	message := fmt.Sprintf("%s file import watcher active import_dir=%s processed_dir=%s failed_dir=%s pattern=%s poll_seconds=%d", m.spec.ID, meta.ImportDir, meta.ProcessedDir, meta.FailedDir, meta.Pattern, pollSeconds)
	m.rt.Logf(message)
	fmt.Println(message)
	m.scanImportDir()
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
	m.logf(4, "%s manual import requested file=%s order_date=%s", m.spec.ID, path, EffectiveDate(orderDate))
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
	m.logf(5, "%s scan import_dir=%s pattern=%s matched_files=%d", m.spec.ID, meta.ImportDir, meta.Pattern, len(files))
	for _, path := range files {
		if !m.begin(path) {
			m.logIgnored("file", "already processing", map[string]interface{}{"file": path})
			continue
		}
		m.logf(4, "%s picked import file=%s", m.spec.ID, path)
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
	m.logf(4, "%s import start file=%s fallback_date=%s", m.spec.ID, path, EffectiveDate(fallbackDate))
	data, err := m.spec.Parse(path, m.rt)
	if err != nil {
		return 0, 0, err
	}
	m.logf(4, "%s parse ok file=%s sample_records=%d qc_records=%d analytes=%d", m.spec.ID, path, len(data.SampleRecords), len(data.QCRecords), len(data.Analytes))
	m.logParsedPreview(path, data)
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
	autoSaveTargets := map[string]*AutoSaveTarget{}
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
		order, _, _, err := store.RecordImportedResult(runDate, roundNo, item.Record, sourceFile)
		if err != nil {
			return imported, 0, err
		}
		CollectAutoSaveTarget(autoSaveTargets, runDate, roundNo, order.ID)
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
	warnings := 0
	if m.spec.AfterImport != nil {
		if err := m.spec.AfterImport(path, data, m.rt); err != nil {
			warnings++
			m.rt.Logf("%s after-import warning %s: %v", m.spec.ID, path, err)
		}
	}
	if analytesChanged {
		if err := m.syncAnalytesToWiseMED(); err != nil {
			warnings++
			m.rt.Logf("%s analyte sync warning %s: %v", m.spec.ID, path, err)
		}
	}
	if err := AutoSaveResultsToWiseMED(m.rt, FlattenAutoSaveTargets(autoSaveTargets)); err != nil {
		warnings++
		m.rt.Logf("%s result autosave warning %s: %v", m.spec.ID, path, err)
	}
	m.logf(4, "%s import done file=%s imported=%d warnings=%d analytes_changed=%t", m.spec.ID, path, imported, warnings, analytesChanged)
	return imported, warnings, nil
}

func (m *Module) ensureAnalyte(known map[string]coremodel.Analyte, item AnalyteDef) (bool, error) {
	service, ok := m.rt.Service("storage")
	if !ok {
		return false, errors.New("storage service unavailable")
	}
	store, ok := service.(analyteStore)
	if !ok {
		return false, errors.New("analyte store unavailable")
	}
	target := m.normalizeAnalyte(item)
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
	if value, _ := raw["processed_dir"].(string); value != "" {
		meta.ProcessedDir = value
	}
	if value, _ := raw["failed_dir"].(string); value != "" {
		meta.FailedDir = value
	}
	if value, _ := raw["export_dir"].(string); value != "" {
		meta.ExportDir = value
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

func (m *Module) normalizeAnalyte(item AnalyteDef) coremodel.Analyte {
	resultType := firstNonEmpty(item.ResultType, "numeric")
	resultFormatting := firstNonEmpty(item.ResultFormatting, "raw")
	resultWeighting := item.ResultWeighting
	if resultWeighting == 0 {
		resultWeighting = 1
	}
	return coremodel.Analyte{
		Active:            true,
		Tag:               strings.TrimSpace(item.Tag),
		Code:              firstNonEmpty(item.Code, item.Tag),
		Name:              firstNonEmpty(item.Name, item.Tag),
		Description:       firstNonEmpty(item.Description, m.spec.AnalyteDescription, "Auto-generated from file imports"),
		ResultType:        resultType,
		ResultFormatting:  resultFormatting,
		ResultWeighting:   resultWeighting,
		ResultMeasureUnit: item.Unit,
		ProtocolOptions:   cloneMap(item.ProtocolOptions),
	}
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
	return mapsEquivalent(existing.ProtocolOptions, target.ProtocolOptions)
}

func mapsEquivalent(left, right map[string]interface{}) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
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

func AutoSaveResultsToWiseMED(rt module.Runtime, targets []AutoSaveTarget) error {
	if !autoConfirmWiseMEDEnabled(rt) || len(targets) == 0 {
		return nil
	}
	store := autoSaveOrderStore(rt)
	if store == nil {
		return errors.New("storage service unavailable")
	}
	syncer := autoSaveResultSync(rt)
	if syncer == nil {
		return errors.New("result-sync service unavailable")
	}
	api := autoSaveWiseMEDAPI(rt)
	if api == nil || !api.SetupComplete() {
		return errors.New("wisemed-api service unavailable or setup incomplete")
	}
	for _, target := range targets {
		if len(target.OrderIDs) == 0 {
			continue
		}
		rt.Logf("wisemed autosave: sync start order_date=%s round_no=%d order_ids=%v", target.OrderDate, target.RoundNo, target.OrderIDs)
		if _, err := syncer.RunOrders(target.OrderIDs, target.RoundNo, target.OrderDate); err != nil {
			return err
		}
		bundles, err := store.ListOrderBundles(target.RoundNo, target.OrderDate)
		if err != nil {
			return err
		}
		selected := filterOrderBundlesByIDs(bundles, target.OrderIDs)
		if len(selected) == 0 {
			continue
		}
		if _, err := saveOrderBundlesToWiseMED(api, selected, rt); err != nil {
			return err
		}
	}
	return nil
}

func SaveOrderBundlesToWiseMED(rt module.Runtime, bundles []coremodel.OrderBundle) (map[string]interface{}, error) {
	api := autoSaveWiseMEDAPI(rt)
	if api == nil || !api.SetupComplete() {
		return nil, errors.New("wisemed-api service unavailable or setup incomplete")
	}
	return saveOrderBundlesToWiseMED(api, bundles, rt)
}

func autoConfirmWiseMEDEnabled(rt module.Runtime) bool {
	return boolString(asString(rt.ModuleSettings("results")["auto_confirm_wisemed"]))
}

func autoSaveOrderStore(rt module.Runtime) importStore {
	service, ok := rt.Service("storage")
	if !ok {
		return nil
	}
	store, _ := service.(importStore)
	return store
}

func autoSaveWiseMEDAPI(rt module.Runtime) wiseMedResultsService {
	service, ok := rt.Service("wisemed-api")
	if !ok {
		return nil
	}
	api, _ := service.(wiseMedResultsService)
	return api
}

func autoSaveResultSync(rt module.Runtime) resultSyncService {
	service, ok := rt.Service("result-sync")
	if !ok {
		return nil
	}
	syncer, _ := service.(resultSyncService)
	return syncer
}

func saveOrderBundlesToWiseMED(api wiseMedResultsService, bundles []coremodel.OrderBundle, rt module.Runtime) (map[string]interface{}, error) {
	saved := 0
	skipped := 0
	files := make([]map[string]interface{}, 0, len(bundles))
	for _, bundle := range bundles {
		fileID := strings.TrimSpace(bundle.Order.FileID)
		if fileID == "" {
			fileID = strings.TrimSpace(asString(bundle.Order.Meta["file_id"]))
		}
		if fileID == "" {
			skipped++
			files = append(files, map[string]interface{}{
				"order_id": bundle.Order.ID,
				"status":   "skipped",
				"reason":   "missing_file_id",
			})
			continue
		}
		entries := make([]wisemedapi.ServiceResultEntry, 0, len(bundle.Analyses))
		for _, item := range bundle.Analyses {
			fsmID := strings.TrimSpace(item.Analysis.WiseMEDFSMID)
			if fsmID == "" {
				continue
			}
			result := strings.TrimSpace(item.Analysis.ResultValue)
			if result == "" {
				result = strings.TrimSpace(item.Analysis.RawValue)
			}
			if result == "" {
				continue
			}
			entries = append(entries, wisemedapi.ServiceResultEntry{
				FSMID:          fsmID,
				Result:         result,
				Interpretation: strings.TrimSpace(item.Analysis.Interpreted),
				Conclusion:     extractConclusion(item.Analysis.Flags),
			})
		}
		if len(entries) == 0 {
			skipped++
			files = append(files, map[string]interface{}{
				"order_id": bundle.Order.ID,
				"file_id":  fileID,
				"status":   "skipped",
				"reason":   "no_entries",
			})
			continue
		}
		rt.Logf("wisemed autosave: patch results order_id=%d file_id=%s entries=%d", bundle.Order.ID, fileID, len(entries))
		resp, err := api.SaveFileServiceResults(fileID, entries)
		if err != nil {
			return map[string]interface{}{
				"saved_orders":   saved,
				"skipped_orders": skipped,
				"files":          files,
			}, err
		}
		saved++
		files = append(files, map[string]interface{}{
			"order_id": bundle.Order.ID,
			"file_id":  fileID,
			"status":   "saved",
			"entries":  len(entries),
			"response": resp,
		})
	}
	return map[string]interface{}{
		"saved_orders":   saved,
		"skipped_orders": skipped,
		"files":          files,
	}, nil
}

func extractConclusion(flags map[string]interface{}) string {
	if len(flags) == 0 {
		return ""
	}
	for _, key := range []string{"conclusion", "final_conclusion", "result_conclusion"} {
		if value := strings.TrimSpace(asString(flags[key])); value != "" {
			return value
		}
	}
	return ""
}

func CollectAutoSaveTarget(items map[string]*AutoSaveTarget, orderDate string, roundNo int, orderID int64) {
	if orderID <= 0 {
		return
	}
	key := fmt.Sprintf("%s|%d", orderDate, roundNo)
	target := items[key]
	if target == nil {
		target = &AutoSaveTarget{OrderDate: orderDate, RoundNo: roundNo}
		items[key] = target
	}
	for _, existing := range target.OrderIDs {
		if existing == orderID {
			return
		}
	}
	target.OrderIDs = append(target.OrderIDs, orderID)
}

func FlattenAutoSaveTargets(items map[string]*AutoSaveTarget) []AutoSaveTarget {
	if len(items) == 0 {
		return nil
	}
	out := make([]AutoSaveTarget, 0, len(items))
	for _, item := range items {
		if item == nil || len(item.OrderIDs) == 0 {
			continue
		}
		sort.Slice(item.OrderIDs, func(i, j int) bool { return item.OrderIDs[i] < item.OrderIDs[j] })
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OrderDate == out[j].OrderDate {
			return out[i].RoundNo < out[j].RoundNo
		}
		return out[i].OrderDate < out[j].OrderDate
	})
	return out
}

func filterOrderBundlesByIDs(items []coremodel.OrderBundle, ids []int64) []coremodel.OrderBundle {
	if len(ids) == 0 {
		return nil
	}
	allowed := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	out := make([]coremodel.OrderBundle, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.Order.ID]; ok {
			out = append(out, item)
		}
	}
	return out
}

func asString(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func boolString(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
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

func (m *Module) logParsedPreview(path string, data ImportData) {
	if m.verboseLevel() < 5 {
		return
	}
	preview := map[string]interface{}{
		"file":           filepath.Base(path),
		"sample_records": minInt(len(data.SampleRecords), 2),
		"qc_records":     minInt(len(data.QCRecords), 2),
		"analytes":       minInt(len(data.Analytes), 5),
	}
	if len(data.SampleRecords) > 0 {
		preview["first_sample_record"] = data.SampleRecords[0]
	}
	if len(data.QCRecords) > 0 {
		preview["first_qc_record"] = data.QCRecords[0]
	}
	if len(data.Analytes) > 0 {
		limit := minInt(len(data.Analytes), 5)
		preview["analyte_preview"] = data.Analytes[:limit]
	}
	blob, err := json.Marshal(preview)
	if err == nil {
		m.rt.Logf("%s parse preview %s", m.spec.ID, string(blob))
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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
	blob, err := json.Marshal(entry)
	if err == nil {
		m.rt.Logf("%s ignored %s", m.spec.ID, string(blob))
	}
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

func PreferredSampleCode(sampleID, sampleName string) string {
	if raw := PreferredRawSampleCode(sampleID, sampleName); raw != "" {
		return NormalizeSampleID(raw)
	}
	return ""
}

func PreferredRawSampleCode(sampleID, sampleName string) string {
	sampleID = strings.TrimSpace(sampleID)
	normalizedID := NormalizeSampleID(sampleID)
	if normalizedID != "" && normalizedID != "UNTITLED" && normalizedID != "UNDEFINED" {
		return sampleID
	}
	sampleName = strings.TrimSpace(sampleName)
	normalizedName := NormalizeSampleID(sampleName)
	if normalizedName != "" && normalizedName != "UNTITLED" && normalizedName != "UNDEFINED" {
		return sampleName
	}
	return sampleID
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
