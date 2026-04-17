package reader

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"wisemed-labreaders/readerslast/generic-test-reader/internal/config"
	"wisemed-labreaders/readerslast/generic-test-reader/internal/model"
)

type ImportSummary struct {
	FileName  string
	Imported  int
	Warnings  int
	Protocol  string
	Manual    bool
	OrderDate string
}

func (a *App) StatusSnapshot() map[string]interface{} {
	wsConnected, analyzerConnected := a.connectionState()
	return map[string]interface{}{
		"reader": map[string]interface{}{
			"id":                a.cfg.Reader.ID,
			"client_id":         a.cfg.Reader.ClientID,
			"label":             a.cfg.Reader.Label,
			"medical_unit_id":   a.cfg.Reader.MedicalUnitID,
			"equipment_id":      a.cfg.Reader.EquipmentID,
			"equipment_type_id": a.cfg.Reader.EquipmentTypeID,
			"analyzer_name":     a.cfg.Reader.AnalyzerName,
			"analyzer_code":     a.cfg.Reader.AnalyzerCode,
		},
		"communication": a.cfg.Comm,
		"layout":        a.cfg.Layout,
		"db_path":       a.cfg.DBPath(),
		"stats":         a.store.Stats(),
		"connections": map[string]interface{}{
			"wisemed_ws_connected": wsConnected,
			"analyzer_connected":   analyzerConnected,
		},
	}
}

func (a *App) DashboardSnapshot(limit int) (map[string]interface{}, error) {
	withoutResult, withResult, err := a.store.TodayResultSummary()
	if err != nil {
		return nil, err
	}
	series, err := a.store.DailySeries(limit)
	if err != nil {
		return nil, err
	}
	wsConnected, analyzerConnected := a.connectionState()
	return map[string]interface{}{
		"stats":       a.store.Stats(),
		"today":       map[string]interface{}{"without_result": withoutResult, "with_result": withResult},
		"series":      series,
		"connections": map[string]interface{}{"wisemed_ws_connected": wsConnected, "analyzer_connected": analyzerConnected},
		"reader": map[string]interface{}{
			"id":                a.cfg.Reader.ID,
			"client_id":         a.cfg.Reader.ClientID,
			"label":             a.cfg.Reader.Label,
			"medical_unit_id":   a.cfg.Reader.MedicalUnitID,
			"equipment_id":      a.cfg.Reader.EquipmentID,
			"equipment_type_id": a.cfg.Reader.EquipmentTypeID,
			"analyzer_name":     a.cfg.Reader.AnalyzerName,
			"analyzer_code":     a.cfg.Reader.AnalyzerCode,
		},
	}, nil
}

func (a *App) StatsForDate(orderDate string) (map[string]interface{}, error) {
	if strings.TrimSpace(orderDate) == "" {
		orderDate = time.Now().Format("2006-01-02")
	}
	stats, err := a.store.StatsForDate(orderDate)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"order_date": orderDate,
		"stats":      stats,
	}, nil
}

func (a *App) StatsSeries(limit int) (map[string]interface{}, error) {
	series, err := a.store.DailySeries(limit)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"series": series,
	}, nil
}

func (a *App) ConfigSnapshot() (*config.Config, error) {
	return a.cfg.Clone()
}

func (a *App) ConfigSection(section string) (interface{}, error) {
	return a.cfg.Section(section)
}

func (a *App) UpdateConfig(patch map[string]interface{}) error {
	return a.cfg.Update(patch)
}

func (a *App) UpdateConfigSection(section string, value interface{}) error {
	return a.cfg.UpdateSection(section, value)
}

func (a *App) ListLogs(limit int) ([]model.EventLog, error) {
	items, err := a.store.ListEvents(limit)
	if err != nil {
		return nil, err
	}
	return sanitizeEventLogs(items), nil
}

func (a *App) ListAnalytes() ([]model.Analyte, error) {
	return a.store.ListAnalytes()
}

func (a *App) GetAnalyte(tag string) (model.Analyte, error) {
	return a.store.GetAnalyte(tag)
}

func (a *App) GetAnalyteByID(id int64) (model.Analyte, error) {
	return a.store.GetAnalyteByID(id)
}

func (a *App) SaveAnalyte(item model.Analyte) (int64, error) {
	return a.store.UpsertAnalyte(item)
}

func (a *App) DeleteAnalyte(id int64) error {
	return a.store.DeleteAnalyte(id)
}

func (a *App) ListOrderBundles(roundNo int, orderDate string) ([]model.OrderBundle, error) {
	return a.store.ListOrderBundles(roundNo, orderDate)
}

func (a *App) ListOrders(roundNo int, orderDate string) ([]model.Order, error) {
	return a.store.ListOrders(roundNo, orderDate)
}

func (a *App) UpsertOrder(order model.Order) (model.Order, error) {
	return a.store.UpsertOrder(order)
}

func (a *App) ListOrderAnalyses(orderID int64) ([]model.OrderAnalysis, error) {
	return a.store.ListAnalysesForOrder(orderID)
}

func (a *App) GetOrderAnalysis(id int64) (model.OrderAnalysis, error) {
	return a.store.GetAnalysis(id)
}

func (a *App) SaveOrderAnalysis(item model.OrderAnalysis) (model.OrderAnalysis, error) {
	return a.store.UpsertOrderAnalysis(item)
}

func sanitizeEventLogs(items []model.EventLog) []model.EventLog {
	for i := range items {
		if !isTransportEventLog(items[i].EventType, items[i].Message) {
			continue
		}
		items[i].Payload = summarizeTransportLogPayload(items[i].Payload)
	}
	return items
}

func isTransportEventLog(eventType, message string) bool {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "ws_rx", "ws_tx", "ws_ping", "ws_pong":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "received ws message", "sent ws message":
		return true
	}
	return false
}

func summarizeTransportLogPayload(payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return map[string]interface{}{"truncated": true}
	}
	out := map[string]interface{}{
		"truncated": true,
	}
	if msgType, ok := payload["type"].(string); ok && strings.TrimSpace(msgType) != "" {
		out["type"] = msgType
	}
	if requestID, ok := payload["request_id"].(string); ok && strings.TrimSpace(requestID) != "" {
		out["request_id"] = requestID
	}
	if correlationID, ok := payload["correlation_id"].(string); ok && strings.TrimSpace(correlationID) != "" {
		out["correlation_id"] = correlationID
	}
	if target, ok := payload["target"]; ok {
		out["target"] = target
	}
	if nested, ok := payload["payload"].(map[string]interface{}); ok {
		out["payload_keys"] = sortedPayloadKeys(nested)
		out["payload_size"] = len(nested)
	}
	if keys, ok := payload["payload_keys"]; ok {
		out["payload_keys"] = keys
	}
	if size, ok := payload["payload_size"]; ok {
		out["payload_size"] = size
	}
	if raw, err := json.Marshal(payload); err == nil {
		out["raw_size_bytes"] = len(raw)
	}
	return out
}

func sortedPayloadKeys(payload map[string]interface{}) []string {
	if len(payload) == 0 {
		return nil
	}
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (a *App) DeleteOrderAnalysis(id int64) error {
	return a.store.DeleteOrderAnalysis(id)
}

func (a *App) SetDefaultResult(orderAnalysisID, resultID int64) error {
	return a.store.SetDefaultResultForAnalysis(orderAnalysisID, resultID)
}

func (a *App) TodayResultSummary() (int, int, error) {
	return a.store.TodayResultSummary()
}

func (a *App) DailySeries(limit int) ([]model.DashboardSeriesPoint, error) {
	return a.store.DailySeries(limit)
}

func (a *App) LatestOrderDate() (string, error) {
	return a.store.LatestOrderDate()
}

func (a *App) ListRoundNumbers(orderDate string) ([]int, error) {
	return a.store.ListRoundNumbers(orderDate)
}

func (a *App) CreateNextRound(orderDate string) (int, error) {
	return a.store.CreateNextRound(orderDate)
}

func (a *App) ImportFile(path string) error {
	_, err := a.processImportCandidate(path, false, "")
	return err
}

func (a *App) ImportFileNow(path, orderDate string) (ImportSummary, error) {
	return a.processImportCandidate(path, true, orderDate)
}

func (a *App) ExportOrdersCSV(orderIDs []int64, orderDate string) (string, int, error) {
	selected := map[int64]struct{}{}
	for _, id := range orderIDs {
		if id > 0 {
			selected[id] = struct{}{}
		}
	}
	items, err := a.store.ListOrderBundles(0, orderDate)
	if err != nil {
		return "", 0, err
	}
	var rows [][]string
	rows = append(rows, []string{"isolateIdentifier", "modelKey", "bestValue", "rating", "scorePercent"})
	exported := 0
	for _, bundle := range items {
		if len(selected) > 0 {
			if _, ok := selected[bundle.Order.ID]; !ok {
				continue
			}
		}
		for _, analysis := range bundle.Analyses {
			rows = append(rows, []string{
				chooseString(bundle.Order.SampleID, bundle.Order.FileID),
				strings.TrimSpace(chooseString(analysis.Analysis.AnalyteName, analysis.Analysis.AnalyteTag)),
				"",
				"",
				"",
			})
			exported++
		}
	}
	if exported == 0 {
		return "", 0, fmt.Errorf("no selected orders available for export")
	}
	if err := os.MkdirAll(a.cfg.Comm.File.ExportDir, 0o755); err != nil {
		return "", 0, err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.WriteAll(rows); err != nil {
		return "", 0, err
	}
	filename := fmt.Sprintf("%s-worklist-%s%s", sanitizeExportName(a.cfg.Reader.AnalyzerCode), time.Now().Format("20060102-150405"), configuredFileExtension(a.cfg.Comm.File.Pattern))
	fullPath := filepath.Join(a.cfg.Comm.File.ExportDir, filename)
	if err := os.WriteFile(fullPath, buf.Bytes(), 0o644); err != nil {
		return "", 0, err
	}
	a.logEvent("info", "worklist_exported", "worklist exported to CSV", map[string]interface{}{
		"path":         fullPath,
		"order_date":   orderDate,
		"rows":         exported,
		"selected_ids": orderIDs,
	})
	return fullPath, exported, nil
}

func sanitizeExportName(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "reader"
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", "_", "-")
	return replacer.Replace(v)
}

func chooseString(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func configuredFileExtension(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ".csv"
	}
	ext := filepath.Ext(strings.ReplaceAll(pattern, "*", "x"))
	if ext == "" || ext == "." {
		return ".csv"
	}
	return strings.ToLower(ext)
}
