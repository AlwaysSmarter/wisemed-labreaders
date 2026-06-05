package resultsync

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	coremodel "wisemed-labreaders/readersv3/modules/core/model"

	"wisemed-labreaders/readersv3/core/module"
)

type orderStore interface {
	ListOrderBundles(roundNo int, orderDate string) ([]coremodel.OrderBundle, error)
	ListRoundNumbers(orderDate string) ([]int, error)
	UpsertOrder(item coremodel.Order) (coremodel.Order, error)
	SaveOrderAnalysis(item coremodel.OrderAnalysis) (coremodel.OrderAnalysis, error)
}

type wiseMedLookupService interface {
	Settings() map[string]string
	HasEquipmentID() bool
	FetchFileForAnalyzer(fileID, equipmentID string) (map[string]interface{}, error)
}

type QCCodeRule struct {
	Prefix    string `json:"prefix" yaml:"prefix"`
	KeepAsLot bool   `json:"keep_as_lot" yaml:"keep_as_lot"`
}

type processedCode struct {
	RawSampleID        string
	NormalizedSampleID string
	FileID             string
	SampleCodeID       string
	SpecimenID         string
	IsQC               bool
	QCPrefix           string
	QCLotNo            string
	Valid              bool
	Reason             string
}

type syncSnapshot struct {
	Running        bool                   `json:"running"`
	Enabled        bool                   `json:"enabled"`
	IntervalMinute int                    `json:"interval_minutes"`
	LastRunAt      string                 `json:"last_run_at,omitempty"`
	NextRunAt      string                 `json:"next_run_at,omitempty"`
	LastError      string                 `json:"last_error,omitempty"`
	LastSummary    map[string]interface{} `json:"last_summary,omitempty"`
}

type Module struct {
	rt module.Runtime

	mu           sync.RWMutex
	running      bool
	lastRunAt    time.Time
	nextRunAt    time.Time
	lastError    string
	lastSummary  map[string]interface{}
	lastSettings syncSettings
}

type syncSettings struct {
	Enabled         bool
	IntervalMinutes int
	SamplePrefixes  []string
	SampleSuffixes  []string
	Separators      []string
	QCPrefixes      []QCCodeRule
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "result-sync" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.RegisterService("result-sync", m)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	settings := m.readSettings()
	m.setSettings(settings)
	if !settings.Enabled {
		<-ctx.Done()
		return nil
	}
	interval := time.Duration(settings.IntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	m.setNextRun(time.Now().Add(interval))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	m.runOnce(context.Background())
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.runOnce(context.Background())
			m.setNextRun(time.Now().Add(interval))
		}
	}
}

func (m *Module) Status() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := syncSnapshot{
		Running:        m.running,
		Enabled:        m.lastSettings.Enabled,
		IntervalMinute: m.lastSettings.IntervalMinutes,
		LastError:      m.lastError,
	}
	if !m.lastRunAt.IsZero() {
		out.LastRunAt = m.lastRunAt.UTC().Format(time.RFC3339)
	}
	if !m.nextRunAt.IsZero() {
		out.NextRunAt = m.nextRunAt.UTC().Format(time.RFC3339)
	}
	if len(m.lastSummary) > 0 {
		out.LastSummary = cloneMap(m.lastSummary)
	}
	raw, _ := json.Marshal(out)
	resp := map[string]interface{}{}
	_ = json.Unmarshal(raw, &resp)
	return resp
}

func (m *Module) SettingsPayload() map[string]interface{} {
	settings := m.readSettings()
	qcRules := make([]map[string]interface{}, 0, len(settings.QCPrefixes))
	for _, item := range settings.QCPrefixes {
		qcRules = append(qcRules, map[string]interface{}{
			"prefix":      item.Prefix,
			"keep_as_lot": item.KeepAsLot,
		})
	}
	return map[string]interface{}{
		"enabled":          settings.Enabled,
		"interval_minutes": settings.IntervalMinutes,
		"sample_prefixes":  settings.SamplePrefixes,
		"sample_suffixes":  settings.SampleSuffixes,
		"separators":       settings.Separators,
		"qc_prefixes":      qcRules,
	}
}

func (m *Module) RunNow() (map[string]interface{}, error) {
	return m.runOnce(context.Background())
}

func (m *Module) RunOrders(orderIDs []int64, roundNo int, orderDate string) (map[string]interface{}, error) {
	return m.runOrders(context.Background(), orderIDs, roundNo, orderDate)
}

func (m *Module) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastRunAt = time.Time{}
	m.nextRunAt = time.Time{}
	m.lastError = ""
	m.lastSummary = nil
}

func (m *Module) runOnce(ctx context.Context) (map[string]interface{}, error) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil, fmt.Errorf("result sync already running")
	}
	m.running = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	store := m.orderStore()
	if store == nil {
		err := fmt.Errorf("storage service unavailable")
		m.finishRun(err, map[string]interface{}{"processed": 0, "errors": 1})
		return nil, err
	}
	wiseMED := m.wiseMED()
	if wiseMED == nil {
		err := fmt.Errorf("wisemed api service unavailable")
		m.finishRun(err, map[string]interface{}{"processed": 0, "errors": 1})
		return nil, err
	}
	settings := m.readSettings()
	m.setSettings(settings)
	today := time.Now().Format("2006-01-02")
	rounds, err := store.ListRoundNumbers(today)
	if err != nil {
		m.finishRun(err, map[string]interface{}{"processed": 0, "errors": 1})
		return nil, err
	}
	if len(rounds) == 0 {
		summary := map[string]interface{}{"date": today, "processed": 0, "matched": 0, "qc": 0, "invalid": 0, "lookup_errors": 0}
		m.finishRun(nil, summary)
		return summary, nil
	}

	summary := newSummary(today)
	equipmentID := strings.TrimSpace(wiseMED.Settings()["echipament_id"])
	m.rt.Logf("result-sync: scheduled run start date=%s rounds=%d equipment_id=%s", today, len(rounds), equipmentID)
	for _, roundNo := range rounds {
		bundles, err := store.ListOrderBundles(roundNo, today)
		if err != nil {
			m.finishRun(err, summary)
			return nil, err
		}
		summary["orders"] = summary["orders"].(int) + len(bundles)
		for _, bundle := range bundles {
			select {
			case <-ctx.Done():
				err := ctx.Err()
				m.finishRun(err, summary)
				return nil, err
			default:
			}
			order := bundle.Order
			result, updatedOrder, updatedAnalyses := m.processOrder(settings, order, bundle.Analyses, equipmentID, wiseMED)
			recordSummaryStatus(summary, result)
			updatedOrder.Meta = mergeMeta(updatedOrder.Meta, result)
			if _, err := store.UpsertOrder(updatedOrder); err != nil {
				m.rt.Logf("result-sync: failed to update order id=%d: %v", order.ID, err)
			}
			for _, analysis := range updatedAnalyses {
				if _, err := store.SaveOrderAnalysis(analysis); err != nil {
					m.rt.Logf("result-sync: failed to update order analysis id=%d tag=%s: %v", analysis.ID, analysis.AnalyteTag, err)
				}
			}
		}
	}
	m.rt.Logf("result-sync: scheduled run done date=%s processed=%d matched=%d file_found=%d qc=%d invalid=%d lookup_errors=%d", today, summary["processed"], summary["matched"], summary["file_found"], summary["qc"], summary["invalid"], summary["lookup_errors"])
	m.finishRun(nil, summary)
	return summary, nil
}

func (m *Module) runOrders(ctx context.Context, orderIDs []int64, roundNo int, orderDate string) (map[string]interface{}, error) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil, fmt.Errorf("result sync already running")
	}
	m.running = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	store := m.orderStore()
	if store == nil {
		err := fmt.Errorf("storage service unavailable")
		m.finishRun(err, map[string]interface{}{"processed": 0, "errors": 1})
		return nil, err
	}
	wiseMED := m.wiseMED()
	if wiseMED == nil {
		err := fmt.Errorf("wisemed api service unavailable")
		m.finishRun(err, map[string]interface{}{"processed": 0, "errors": 1})
		return nil, err
	}
	settings := m.readSettings()
	m.setSettings(settings)
	orderDate = firstNonEmpty(strings.TrimSpace(orderDate), time.Now().Format("2006-01-02"))
	bundles, err := store.ListOrderBundles(roundNo, orderDate)
	if err != nil {
		m.finishRun(err, map[string]interface{}{"processed": 0, "errors": 1})
		return nil, err
	}
	selected := filterBundlesByOrderIDs(bundles, orderIDs)
	summary := newSummary(orderDate)
	summary["orders"] = len(selected)
	summary["mode"] = "manual"
	summary["round_no"] = roundNo
	summary["selected_order_ids"] = normalizeOrderIDs(orderIDs)
	equipmentID := strings.TrimSpace(wiseMED.Settings()["echipament_id"])
	m.rt.Logf("result-sync: manual run start date=%s round_no=%d selected_orders=%d equipment_id=%s", orderDate, roundNo, len(selected), equipmentID)
	for _, bundle := range selected {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			m.finishRun(err, summary)
			return nil, err
		default:
		}
		order := bundle.Order
		result, updatedOrder, updatedAnalyses := m.processOrder(settings, order, bundle.Analyses, equipmentID, wiseMED)
		recordSummaryStatus(summary, result)
		updatedOrder.Meta = mergeMeta(updatedOrder.Meta, result)
		if _, err := store.UpsertOrder(updatedOrder); err != nil {
			m.rt.Logf("result-sync: failed to update order id=%d: %v", order.ID, err)
		}
		for _, analysis := range updatedAnalyses {
			if _, err := store.SaveOrderAnalysis(analysis); err != nil {
				m.rt.Logf("result-sync: failed to update order analysis id=%d tag=%s: %v", analysis.ID, analysis.AnalyteTag, err)
			}
		}
	}
	m.rt.Logf("result-sync: manual run done date=%s round_no=%d processed=%d matched=%d file_found=%d qc=%d invalid=%d lookup_errors=%d", orderDate, roundNo, summary["processed"], summary["matched"], summary["file_found"], summary["qc"], summary["invalid"], summary["lookup_errors"])
	m.finishRun(nil, summary)
	return summary, nil
}

func (m *Module) processOrder(settings syncSettings, order coremodel.Order, analyses []coremodel.OrderAnalysisBundle, equipmentID string, wiseMED wiseMedLookupService) (map[string]interface{}, coremodel.Order, []coremodel.OrderAnalysis) {
	rawSampleID := strings.TrimSpace(order.SampleID)
	info := postprocessSampleCode(rawSampleID, settings)
	meta := map[string]interface{}{
		"sample_code_raw":         info.RawSampleID,
		"sample_code_normalized":  info.NormalizedSampleID,
		"sample_code_file_id":     info.FileID,
		"sample_code_id":          info.SampleCodeID,
		"sample_code_specimen_id": info.SpecimenID,
		"sample_code_is_qc":       info.IsQC,
		"sample_code_reason":      info.Reason,
		"sample_code_updated_at":  time.Now().UTC().Format(time.RFC3339),
	}
	if info.QCPrefix != "" {
		meta["sample_code_qc_prefix"] = info.QCPrefix
	}
	if info.QCLotNo != "" {
		meta["sample_code_qc_lot_no"] = info.QCLotNo
	}
	if info.IsQC {
		meta["sync_status"] = "qc"
		meta["sync_message"] = "sample code matched configured QC prefix"
		m.logProcessOutcome(order, meta)
		return meta, order, nil
	}
	if !info.Valid {
		meta["sync_status"] = "invalid_code"
		meta["sync_message"] = "sample code could not be normalized to <file>-<sample>-<specimen>"
		m.logProcessOutcome(order, meta)
		return meta, order, nil
	}
	if strings.TrimSpace(equipmentID) == "" {
		meta["sync_status"] = "lookup_error"
		meta["sync_message"] = "missing echipament_id in WiseMED settings"
		m.logProcessOutcome(order, meta)
		return meta, order, nil
	}
	m.rt.Logf("result-sync: lookup fileforanalyzer order_id=%d file_id=%s equipment_id=%s sample_id=%s specimen_id=%s", order.ID, info.FileID, equipmentID, info.SampleCodeID, info.SpecimenID)
	resp, err := wiseMED.FetchFileForAnalyzer(info.FileID, equipmentID)
	if err != nil {
		meta["sync_status"] = "lookup_error"
		meta["sync_message"] = err.Error()
		m.logProcessOutcome(order, meta)
		return meta, order, nil
	}
	meta["sync_lookup"] = resp
	applyWiseMEDOrderDetails(&order, resp, nil)
	if matched, candidate := matchWiseMEDFile(resp, info, analyses); matched {
		meta["sync_status"] = "matched"
		meta["sync_message"] = "WiseMED file/probe/specimen matched"
		meta["sync_match"] = candidate
		meta["sync_result_summary"] = summarizeAnalyses(analyses)
		updatedAnalyses := applyWiseMEDMatch(&order, analyses, resp, candidate)
		m.logProcessOutcome(order, meta)
		return meta, order, updatedAnalyses
	}
	meta["sync_status"] = "file_found"
	meta["sync_message"] = "WiseMED file loaded, but no exact probe/specimen match was found"
	meta["sync_result_summary"] = summarizeAnalyses(analyses)
	updatedAnalyses := applyWiseMEDTestsOnly(analyses, resp)
	m.logProcessOutcome(order, meta)
	return meta, order, updatedAnalyses
}

func (m *Module) logProcessOutcome(order coremodel.Order, meta map[string]interface{}) {
	m.rt.Logf(
		"result-sync: order id=%d sample_id=%s file_id=%s normalized=%s status=%s message=%s",
		order.ID,
		strings.TrimSpace(order.SampleID),
		asString(meta["sample_code_file_id"]),
		asString(meta["sample_code_normalized"]),
		asString(meta["sync_status"]),
		asString(meta["sync_message"]),
	)
}

func (m *Module) finishRun(err error, summary map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastRunAt = time.Now()
	if err != nil {
		m.lastError = err.Error()
	} else {
		m.lastError = ""
	}
	m.lastSummary = cloneMap(summary)
}

func (m *Module) setNextRun(ts time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextRunAt = ts
}

func (m *Module) setSettings(settings syncSettings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSettings = settings
}

func (m *Module) readSettings() syncSettings {
	raw := m.rt.ModuleSettings(m.ID())
	settings := syncSettings{
		Enabled:         true,
		IntervalMinutes: 5,
		SamplePrefixes:  readStringList(raw["sample_prefixes"]),
		SampleSuffixes:  readStringList(raw["sample_suffixes"]),
		Separators:      readStringList(raw["separators"]),
		QCPrefixes:      readQCPrefixes(raw["qc_prefixes"]),
	}
	if enabled, ok := raw["enabled"].(bool); ok {
		settings.Enabled = enabled
	} else if text := strings.TrimSpace(asString(raw["enabled"])); text != "" {
		settings.Enabled = parseBool(text)
	}
	if n := parseInt(raw["interval_minutes"], 5); n > 0 {
		settings.IntervalMinutes = n
	}
	if len(settings.Separators) == 0 {
		settings.Separators = []string{"-"}
	}
	return settings
}

func (m *Module) orderStore() orderStore {
	service, ok := m.rt.Service("storage")
	if !ok {
		return nil
	}
	store, _ := service.(orderStore)
	return store
}

func (m *Module) wiseMED() wiseMedLookupService {
	service, ok := m.rt.Service("wisemed-api")
	if !ok {
		return nil
	}
	api, _ := service.(wiseMedLookupService)
	return api
}

func postprocessSampleCode(raw string, settings syncSettings) processedCode {
	out := processedCode{RawSampleID: strings.TrimSpace(raw)}
	if out.RawSampleID == "" {
		out.Reason = "empty"
		return out
	}
	for _, item := range settings.QCPrefixes {
		prefix := strings.TrimSpace(item.Prefix)
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(out.RawSampleID), strings.ToUpper(prefix)) {
			out.IsQC = true
			out.Valid = false
			out.QCPrefix = prefix
			if item.KeepAsLot {
				out.QCLotNo = prefix
			} else {
				out.QCLotNo = strings.TrimSpace(strings.TrimPrefix(out.RawSampleID, prefix))
			}
			out.Reason = "qc_prefix"
			return out
		}
	}
	normalized := out.RawSampleID
	for _, prefix := range settings.SamplePrefixes {
		if strings.HasPrefix(strings.ToUpper(normalized), strings.ToUpper(prefix)) {
			normalized = normalized[len(prefix):]
			break
		}
	}
	for _, suffix := range settings.SampleSuffixes {
		if strings.HasSuffix(strings.ToUpper(normalized), strings.ToUpper(suffix)) {
			normalized = normalized[:len(normalized)-len(suffix)]
			break
		}
	}
	normalized = strings.TrimSpace(normalized)
	out.NormalizedSampleID = normalized
	if normalized == "" {
		out.Reason = "empty_after_trim"
		return out
	}
	if isDigits(normalized) {
		out.FileID = normalized
		out.Valid = true
		out.Reason = "file_only"
		return out
	}
	for _, separator := range settings.Separators {
		if separator == "" || !strings.Contains(normalized, separator) {
			continue
		}
		parts := strings.Split(normalized, separator)
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
		for _, other := range settings.Separators {
			if other == "" || other == separator {
				continue
			}
			if strings.Contains(normalized, other) {
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
			out.SpecimenID = strings.TrimSpace(parts[2])
		}
		out.Valid = out.FileID != ""
		if out.Valid {
			out.Reason = "ok"
			return out
		}
	}
	out.Reason = "separator_mismatch"
	return out
}

func matchWiseMEDFile(payload map[string]interface{}, info processedCode, analyses []coremodel.OrderAnalysisBundle) (bool, map[string]interface{}) {
	candidates := collectWiseMEDCandidates(payload)
	for _, item := range candidates {
		fileID := firstNonEmpty(item["fisa_id"], item["file_id"], item["o_file_id"])
		sampleID := firstNonEmpty(item["flsm_cod_proba_id"], item["sample_id"], item["cod_proba_id"])
		specimenID := firstNonEmpty(item["flsm_cod_proba_esantion_id"], item["specimen_id"], item["cod_proba_esantion_id"])
		if info.FileID != "" && fileID != "" && fileID != info.FileID {
			continue
		}
		if strings.TrimSpace(info.SampleCodeID) == "" && strings.TrimSpace(info.SpecimenID) == "" {
			if fileOnlyTestMatch(item, info, analyses) {
				return true, map[string]interface{}{
					"fisa_id":   firstNonEmpty(item["fisa_id"], item["o_file_id"], item["file_id"]),
					"file_id":   firstNonEmpty(item["file_id"], item["o_file_id"], item["fisa_id"]),
					"o_tests":   item["o_tests"],
					"match_via": "file_id+test_code",
				}
			}
			continue
		}
		if info.SampleCodeID != "" && sampleID != "" && sampleID != info.SampleCodeID {
			continue
		}
		if info.SpecimenID != "" && specimenID != "" && specimenID != info.SpecimenID {
			continue
		}
		return true, item
	}
	if fileID := firstNonEmpty(payload["fisa_id"], payload["file_id"], payload["o_file_id"]); info.FileID != "" && fileID == info.FileID {
		if fileOnlyTestMatch(payload, info, analyses) {
			return true, map[string]interface{}{
				"fisa_id":   firstNonEmpty(payload["fisa_id"], payload["o_file_id"], payload["file_id"]),
				"file_id":   firstNonEmpty(payload["file_id"], payload["o_file_id"], payload["fisa_id"]),
				"o_tests":   payload["o_tests"],
				"match_via": "file_id+test_code",
			}
		}
		return true, map[string]interface{}{
			"fisa_id": payload["fisa_id"],
			"file_id": firstNonEmpty(payload["file_id"], payload["o_file_id"]),
		}
	}
	if fileOnlyTestMatch(payload, info, analyses) {
		return true, map[string]interface{}{
			"fisa_id":   firstNonEmpty(payload["fisa_id"], payload["o_file_id"], payload["file_id"]),
			"file_id":   firstNonEmpty(payload["file_id"], payload["o_file_id"], payload["fisa_id"]),
			"o_tests":   payload["o_tests"],
			"match_via": "file_id+test_code",
		}
	}
	return false, nil
}

func fileOnlyTestMatch(payload map[string]interface{}, info processedCode, analyses []coremodel.OrderAnalysisBundle) bool {
	if strings.TrimSpace(info.FileID) == "" || strings.TrimSpace(info.SampleCodeID) != "" || strings.TrimSpace(info.SpecimenID) != "" {
		return false
	}
	fileID := firstNonEmpty(payload["fisa_id"], payload["file_id"], payload["o_file_id"])
	if fileID == "" || fileID != info.FileID {
		return false
	}
	localTags := make(map[string]struct{}, len(analyses))
	for _, item := range analyses {
		tag := strings.ToUpper(strings.TrimSpace(item.Analysis.AnalyteTag))
		if tag != "" {
			localTags[tag] = struct{}{}
		}
	}
	if len(localTags) == 0 {
		return false
	}
	tests, _ := payload["o_tests"].([]interface{})
	for _, raw := range tests {
		test, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(asString(test["t_tag"])))
		if key == "" {
			continue
		}
		if _, ok := localTags[key]; ok {
			return true
		}
	}
	return false
}

func applyWiseMEDMatch(order *coremodel.Order, analyses []coremodel.OrderAnalysisBundle, payload map[string]interface{}, candidate map[string]interface{}) []coremodel.OrderAnalysis {
	applyWiseMEDOrderDetails(order, payload, candidate)
	updated := applyWiseMEDTestsOnly(analyses, candidate)
	if len(updated) == 0 {
		return applyWiseMEDTestsOnly(analyses, payload)
	}
	return updated
}

func applyWiseMEDOrderDetails(order *coremodel.Order, payload map[string]interface{}, candidate map[string]interface{}) {
	if order != nil {
		if value := strings.TrimSpace(firstNonEmpty(payload["o_patient_id"], candidate["o_patient_id"])); value != "" {
			order.PatientID = value
		}
		if value := strings.TrimSpace(firstNonEmpty(payload["o_patient_name"], candidate["o_patient_name"])); value != "" {
			order.PatientName = value
		}
		if value := strings.TrimSpace(firstNonEmpty(payload["o_file_id"], candidate["o_file_id"])); value != "" {
			order.FileID = value
		}
		if order.Meta == nil {
			order.Meta = map[string]interface{}{}
		}
		if value := strings.TrimSpace(firstNonEmpty(payload["o_patient_birthday"], candidate["o_patient_birthday"])); value != "" {
			order.Meta["wisemed_patient_birthday"] = value
		}
		if value := strings.TrimSpace(firstNonEmpty(payload["o_patient_sex"], candidate["o_patient_sex"])); value != "" {
			order.Meta["wisemed_patient_sex"] = value
		}
		if value := strings.TrimSpace(firstNonEmpty(payload["o_file_code"], candidate["o_file_code"])); value != "" {
			order.Meta["wisemed_file_code"] = value
		}
		if value := strings.TrimSpace(firstNonEmpty(payload["o_file_code2"], candidate["o_file_code2"])); value != "" {
			order.Meta["wisemed_file_code2"] = value
		}
	}
}

func collectWiseMEDTestsByTag(payload map[string]interface{}) map[string]map[string]interface{} {
	out := map[string]map[string]interface{}{}
	tests, _ := payload["o_tests"].([]interface{})
	for _, raw := range tests {
		test, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		tag := strings.ToUpper(strings.TrimSpace(asString(test["t_tag"])))
		if tag == "" {
			continue
		}
		out[tag] = test
	}
	return out
}

func applyWiseMEDTestsOnly(analyses []coremodel.OrderAnalysisBundle, payload map[string]interface{}) []coremodel.OrderAnalysis {
	testsByTag := collectWiseMEDTestsByTag(payload)
	if len(testsByTag) == 0 {
		return nil
	}
	updated := make([]coremodel.OrderAnalysis, 0, len(analyses))
	for _, item := range analyses {
		tag := strings.ToUpper(strings.TrimSpace(item.Analysis.AnalyteTag))
		if tag == "" {
			continue
		}
		test, ok := testsByTag[tag]
		if !ok {
			continue
		}
		next := item.Analysis
		if value := strings.TrimSpace(asString(test["t_name"])); value != "" {
			next.AnalyteName = value
		}
		next.WiseMEDSMID = strings.TrimSpace(asString(test["t_sm_id"]))
		next.WiseMEDFSMID = strings.TrimSpace(asString(test["t_fsm_id"]))
		updated = append(updated, next)
	}
	return updated
}

func collectWiseMEDCandidates(value interface{}) []map[string]interface{} {
	out := []map[string]interface{}{}
	seen := map[string]struct{}{}
	var walk func(item interface{})
	walk = func(item interface{}) {
		switch typed := item.(type) {
		case map[string]interface{}:
			if len(typed) > 0 {
				if _, hasFile := typed["fisa_id"]; hasFile {
					key := mustJSON(typed)
					if _, ok := seen[key]; !ok {
						seen[key] = struct{}{}
						out = append(out, typed)
					}
				}
				if _, hasFile := typed["file_id"]; hasFile {
					key := mustJSON(typed)
					if _, ok := seen[key]; !ok {
						seen[key] = struct{}{}
						out = append(out, typed)
					}
				}
			}
			for _, nested := range typed {
				walk(nested)
			}
		case []interface{}:
			for _, nested := range typed {
				walk(nested)
			}
		}
	}
	walk(value)
	return out
}

func summarizeAnalyses(items []coremodel.OrderAnalysisBundle) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]interface{}{
			"analyte_tag":       item.Analysis.AnalyteTag,
			"analyte_name":      item.Analysis.AnalyteName,
			"result_value":      item.Analysis.ResultValue,
			"raw_value":         item.Analysis.RawValue,
			"interpreted_value": item.Analysis.Interpreted,
			"unit":              item.Analysis.Unit,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return asString(out[i]["analyte_tag"]) < asString(out[j]["analyte_tag"])
	})
	return out
}

func mergeMeta(base map[string]interface{}, next map[string]interface{}) map[string]interface{} {
	out := cloneMap(base)
	if out == nil {
		out = map[string]interface{}{}
	}
	for key, value := range next {
		out[key] = value
	}
	return out
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	out := make(map[string]interface{}, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func newSummary(orderDate string) map[string]interface{} {
	return map[string]interface{}{
		"date":          orderDate,
		"processed":     0,
		"matched":       0,
		"file_found":    0,
		"qc":            0,
		"invalid":       0,
		"lookup_errors": 0,
		"orders":        0,
	}
}

func recordSummaryStatus(summary map[string]interface{}, result map[string]interface{}) {
	summary["processed"] = summary["processed"].(int) + 1
	switch result["sync_status"] {
	case "matched":
		summary["matched"] = summary["matched"].(int) + 1
	case "file_found":
		summary["file_found"] = summary["file_found"].(int) + 1
	case "qc":
		summary["qc"] = summary["qc"].(int) + 1
	case "invalid_code":
		summary["invalid"] = summary["invalid"].(int) + 1
	case "lookup_error":
		summary["lookup_errors"] = summary["lookup_errors"].(int) + 1
	}
}

func filterBundlesByOrderIDs(items []coremodel.OrderBundle, orderIDs []int64) []coremodel.OrderBundle {
	if len(orderIDs) == 0 {
		return items
	}
	selected := make(map[int64]struct{}, len(orderIDs))
	for _, id := range orderIDs {
		if id > 0 {
			selected[id] = struct{}{}
		}
	}
	if len(selected) == 0 {
		return items
	}
	out := make([]coremodel.OrderBundle, 0, len(items))
	for _, item := range items {
		if _, ok := selected[item.Order.ID]; ok {
			out = append(out, item)
		}
	}
	return out
}

func normalizeOrderIDs(items []int64) []int64 {
	if len(items) == 0 {
		return nil
	}
	out := make([]int64, 0, len(items))
	for _, id := range items {
		if id > 0 {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func readStringList(raw interface{}) []string {
	switch typed := raw.(type) {
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(asString(item)); text != "" {
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

func readQCPrefixes(raw interface{}) []QCCodeRule {
	switch typed := raw.(type) {
	case []interface{}:
		out := make([]QCCodeRule, 0, len(typed))
		for _, item := range typed {
			if row, ok := item.(map[string]interface{}); ok {
				prefix := strings.TrimSpace(asString(row["prefix"]))
				if prefix == "" {
					continue
				}
				out = append(out, QCCodeRule{
					Prefix:    prefix,
					KeepAsLot: parseBool(asString(row["keep_as_lot"])),
				})
			}
		}
		return out
	case string:
		parts := strings.Split(typed, ",")
		out := make([]QCCodeRule, 0, len(parts))
		for _, item := range parts {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			rule := QCCodeRule{Prefix: item}
			if strings.Contains(item, ":") {
				chunks := strings.SplitN(item, ":", 2)
				rule.Prefix = strings.TrimSpace(chunks[0])
				rule.KeepAsLot = parseBool(chunks[1])
			}
			if rule.Prefix != "" {
				out = append(out, rule)
			}
		}
		return out
	default:
		return nil
	}
}

func parseInt(value interface{}, def int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return n
		}
	}
	return def
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on", "da":
		return true
	default:
		return false
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

func firstNonEmpty(items ...interface{}) string {
	for _, item := range items {
		if text := strings.TrimSpace(asString(item)); text != "" {
			return text
		}
	}
	return ""
}

func asString(value interface{}) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(value)
	}
}

func mustJSON(value interface{}) string {
	blob, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(blob)
}
