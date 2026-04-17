package reader

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"wisemed-labreaders/readerslast/generic-test-reader/internal/config"
	"wisemed-labreaders/readerslast/generic-test-reader/internal/model"
)

var errImportDeferred = errors.New("import deferred until file becomes stable")

func effectiveImportOrderDate(orderDate string) string {
	orderDate = strings.TrimSpace(orderDate)
	if orderDate == "" {
		return time.Now().Format("2006-01-02")
	}
	return orderDate
}

func (a *App) fileLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(a.cfg.Comm.File.PollSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.scanImportDir()
		}
	}
}

func (a *App) scanImportDir() {
	if a.cfg.Comm.Type != config.CommTypeFile {
		return
	}
	pattern := filepath.Join(a.cfg.Comm.File.ImportDir, a.cfg.Comm.File.Pattern)
	files, err := filepath.Glob(pattern)
	if err != nil {
		a.logEvent("error", "file_scan_failed", "glob failed", map[string]interface{}{"error": err.Error(), "pattern": pattern})
		return
	}
	if len(files) > 0 {
		log.Printf("file scan found %d candidate files in %s", len(files), a.cfg.Comm.File.ImportDir)
	}
	for _, path := range files {
		_, _ = a.processImportCandidate(path, false, "")
	}
}

func (a *App) processImportCandidate(path string, force bool, orderDate string) (ImportSummary, error) {
	summary := ImportSummary{
		FileName:  filepath.Base(path),
		Manual:    force,
		OrderDate: effectiveImportOrderDate(orderDate),
	}
	if !a.beginImport(path) {
		return summary, nil
	}
	defer a.endImport(path)
	imported, err := a.importFile(path, force, orderDate)
	if err != nil {
		if errors.Is(err, errImportDeferred) {
			a.logEvent("info", "file_import_deferred", "waiting for file to stabilize before import", map[string]interface{}{
				"path":           path,
				"stable_wait_ms": a.cfg.Comm.File.StableWaitMS,
			})
			return summary, nil
		}
		log.Printf("import failed for %s: %v", path, err)
		a.logEvent("error", "file_import_failed", "import failed", map[string]interface{}{"path": path, "error": err.Error()})
		if archiveErr := a.archiveFile(path, a.cfg.Comm.File.FailedDir); archiveErr != nil {
			log.Printf("failed to archive %s to failed dir: %v", path, archiveErr)
			a.logEvent("error", "file_archive_failed", "failed to archive failed import", map[string]interface{}{"path": path, "error": archiveErr.Error(), "target_dir": a.cfg.Comm.File.FailedDir})
		}
		return summary, err
	}
	summary = imported
	if archiveErr := a.archiveFile(path, a.cfg.Comm.File.ProcessedDir); archiveErr != nil {
		log.Printf("failed to archive %s to processed dir: %v", path, archiveErr)
		a.logEvent("error", "file_archive_failed", "failed to archive processed import", map[string]interface{}{"path": path, "error": archiveErr.Error(), "target_dir": a.cfg.Comm.File.ProcessedDir})
		return summary, archiveErr
	}
	a.logEvent("info", "file_import_completed", "import completed", map[string]interface{}{
		"path":      path,
		"imported":  summary.Imported,
		"warnings":  summary.Warnings,
		"protocol":  summary.Protocol,
		"manual":    summary.Manual,
		"file_name": summary.FileName,
	})
	return summary, nil
}

func (a *App) beginImport(path string) bool {
	a.importMu.Lock()
	defer a.importMu.Unlock()
	if _, ok := a.importInFlight[path]; ok {
		return false
	}
	a.importInFlight[path] = struct{}{}
	return true
}

func (a *App) endImport(path string) {
	a.importMu.Lock()
	defer a.importMu.Unlock()
	delete(a.importInFlight, path)
}

func (a *App) importFile(path string, force bool, orderDate string) (ImportSummary, error) {
	summary := ImportSummary{
		FileName:  filepath.Base(path),
		Manual:    force,
		OrderDate: effectiveImportOrderDate(orderDate),
	}
	st, err := os.Stat(path)
	if err != nil {
		return summary, err
	}
	stableWait := time.Duration(a.cfg.Comm.File.StableWaitMS) * time.Millisecond
	if !force && time.Since(st.ModTime()) < stableWait {
		return summary, errImportDeferred
	}
	a.logEvent("info", "file_import_started", "import started", map[string]interface{}{
		"path":           path,
		"file_name":      filepath.Base(path),
		"manual":         force,
		"order_date":     summary.OrderDate,
		"stable_wait_ms": a.cfg.Comm.File.StableWaitMS,
		"size_bytes":     st.Size(),
	})
	log.Printf("importing file %s", path)
	if isIRBiotyperProtocol(a.cfg.Comm.Protocol) || looksLikeIRBiotyperFile(path) {
		summary.Protocol = "IRBIOTYPER"
		imported, warnings, err := a.importIRBiotyperFile(path, summary.OrderDate)
		summary.Imported = imported
		summary.Warnings = warnings
		return summary, err
	}
	summary.Protocol = "GENERIC"
	records, err := parseImportFile(path, a.cfg.Comm.Protocol)
	if err != nil {
		return summary, err
	}
	a.logEvent("info", "file_import_parsed", "import file parsed", map[string]interface{}{
		"path":       path,
		"file_name":  filepath.Base(path),
		"records":    len(records),
		"protocol":   summary.Protocol,
		"manual":     force,
		"order_date": summary.OrderDate,
	})
	log.Printf("parsed %d records from %s", len(records), path)
	for i, rec := range records {
		log.Printf("parsed record %d/%d: %s", i+1, len(records), mustJSON(map[string]interface{}{
			"sample_id":     rec.SampleID,
			"file_id":       rec.FileID,
			"patient_id":    rec.PatientID,
			"patient_name":  rec.PatientName,
			"analyte_tag":   rec.AnalyteTag,
			"analyte_name":  rec.AnalyteName,
			"result_value":  rec.ResultValue,
			"raw_value":     rec.RawValue,
			"flags":         rec.Flags,
			"unit":          rec.Unit,
			"rack_no":       rec.RackNo,
			"rack_position": rec.RackPosition,
			"list_position": rec.ListPosition,
			"sample_no":     rec.SampleNo,
			"meta":          rec.Meta,
		}))
		a.logEvent("debug", "file_import_row", "parsed import row", map[string]interface{}{
			"path":          path,
			"row_index":     i + 1,
			"sample_id":     rec.SampleID,
			"analyte_tag":   rec.AnalyteTag,
			"result_value":  rec.ResultValue,
			"rack_no":       rec.RackNo,
			"rack_position": rec.RackPosition,
			"sample_no":     rec.SampleNo,
		})
	}
	roundNo, err := a.store.CurrentRoundNo(summary.OrderDate)
	if err != nil {
		return summary, err
	}
	imported := 0
	for _, rec := range records {
		order, analysis, result, err := a.store.RecordImportedResult(summary.OrderDate, roundNo, a.cfg.Layout.Kind, rec, filepath.Base(path))
		if err != nil {
			return summary, err
		}
		imported++
		log.Printf("imported record: %s", mustJSON(map[string]interface{}{
			"source_file": filepath.Base(path),
			"round_no":    roundNo,
			"order": map[string]interface{}{
				"id":            order.ID,
				"round_no":      order.RoundNo,
				"sample_id":     order.SampleID,
				"file_id":       order.FileID,
				"patient_id":    order.PatientID,
				"patient_name":  order.PatientName,
				"rack_no":       order.RackNo,
				"rack_position": order.RackPosition,
				"list_position": order.ListPosition,
				"sample_no":     order.SampleNo,
				"status":        order.Status,
			},
			"analysis": map[string]interface{}{
				"id":           analysis.ID,
				"analyte_tag":  analysis.AnalyteTag,
				"analyte_name": analysis.AnalyteName,
				"status":       analysis.Status,
			},
			"result": map[string]interface{}{
				"id":                result.ID,
				"order_analysis_id": result.OrderAnalysisID,
				"result_value":      result.ResultValue,
				"raw_value":         result.RawValue,
				"interpreted_value": result.Interpreted,
				"unit":              result.Unit,
				"flags":             result.Flags,
				"meta":              result.Meta,
			},
		}))
		a.logEvent("info", "result_imported", "imported result from file", map[string]interface{}{
			"path":     path,
			"sample":   order.SampleID,
			"analyte":  analysis.AnalyteTag,
			"result":   result.ResultValue,
			"round_no": order.RoundNo,
		})
		a.sendResultEvent("result_available", map[string]interface{}{
			"source_file": filepath.Base(path),
			"round_no":    order.RoundNo,
			"order":       order,
			"analysis":    analysis,
			"result":      result,
		})
	}
	a.sendLogEvent("tick", map[string]interface{}{
		"mode":        a.cfg.Comm.Type,
		"source_file": filepath.Base(path),
		"imported":    imported,
	})
	log.Printf("imported %d records from %s", imported, path)
	if imported == 0 {
		return summary, fmt.Errorf("no records imported from %s", path)
	}
	summary.Imported = imported
	return summary, nil
}

func (a *App) importIRBiotyperFile(path, orderDate string) (int, int, error) {
	rows, err := parseIRBiotyperRows(path)
	if err != nil {
		return 0, 0, err
	}
	a.logEvent("info", "file_import_parsed", "IR Biotyper file parsed", map[string]interface{}{
		"path":       path,
		"file_name":  filepath.Base(path),
		"rows":       len(rows),
		"protocol":   "IRBIOTYPER",
		"order_date": effectiveImportOrderDate(orderDate),
	})
	log.Printf("parsed %d IR Biotyper rows from %s", len(rows), path)
	mapper, err := loadIRBiotyperMapper(a.cfg)
	if err != nil {
		return 0, 0, err
	}
	imported := 0
	warnings := 0
	for i, row := range rows {
		log.Printf("parsed irbt row %d/%d: %s", i+1, len(rows), mustJSON(row))
		outcome, err := a.processIRBiotyperRow(filepath.Base(path), effectiveImportOrderDate(orderDate), row, mapper)
		if err != nil {
			warnings++
			log.Printf("IRBT row skipped sample=%s: %v", row.IsolateIdentifier, err)
			a.logEvent("warning", "IRBT_IMPORT", "IR Biotyper row skipped", map[string]interface{}{
				"sample_id":    row.IsolateIdentifier,
				"model_key":    row.ModelKey,
				"best_value":   row.BestValue,
				"rating":       row.Rating,
				"scorePercent": row.ScorePercent,
				"error":        err.Error(),
			})
			continue
		}
		a.logEvent("debug", "file_import_row", "parsed IR Biotyper row", map[string]interface{}{
			"path":          path,
			"row_index":     i + 1,
			"sample_id":     row.IsolateIdentifier,
			"model_key":     row.ModelKey,
			"best_value":    row.BestValue,
			"rating":        row.Rating,
			"score_percent": row.ScorePercent,
		})
		imported++
		log.Printf("imported irbt row: %s", mustJSON(outcome))
	}
	a.sendLogEvent("tick", map[string]interface{}{
		"mode":        a.cfg.Comm.Type,
		"source_file": filepath.Base(path),
		"imported":    imported,
		"warnings":    warnings,
		"protocol":    "IRBIOTYPER",
	})
	if imported == 0 {
		return imported, warnings, fmt.Errorf("no IR Biotyper rows imported from %s", path)
	}
	log.Printf("imported %d IR Biotyper rows from %s", imported, path)
	return imported, warnings, nil
}

func parseImportFile(path, protocol string) ([]model.ImportedRecord, error) {
	log.Printf("parseImportFile: path=%s protocol=%s", path, protocol)
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Printf("parseImportFile: read failed path=%s err=%v", path, err)
		return nil, err
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		log.Printf("parseImportFile: empty file path=%s", path)
		return nil, errors.New("empty import file")
	}
	if isIRBiotyperProtocol(protocol) || looksLikeIRBiotyperCSV(trimmed) {
		log.Printf("parseImportFile: detected IR Biotyper format path=%s", path)
		return parseIRBiotyperCSV(raw)
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		log.Printf("parseImportFile: detected JSON payload path=%s", path)
		return parseJSONRecords(raw)
	}
	log.Printf("parseImportFile: detected generic CSV payload path=%s", path)
	return parseCSVRecords(raw)
}

func looksLikeIRBiotyperCSV(raw string) bool {
	lines := splitNonEmptyLines(raw)
	for _, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lower, "#") {
			continue
		}
		return strings.Contains(lower, "isolateidentifier") &&
			strings.Contains(lower, "modelkey") &&
			strings.Contains(lower, "bestvalue")
	}
	return false
}

func looksLikeIRBiotyperFile(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return looksLikeIRBiotyperCSV(string(raw))
}

func parseJSONRecords(raw []byte) ([]model.ImportedRecord, error) {
	log.Printf("parseJSONRecords: starting unmarshal bytes=%d", len(raw))
	var payload interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		log.Printf("parseJSONRecords: unmarshal failed err=%v", err)
		return nil, err
	}
	var items []interface{}
	switch x := payload.(type) {
	case []interface{}:
		items = x
	case map[string]interface{}:
		for _, key := range []string{"results", "items", "data"} {
			if arr, ok := x[key].([]interface{}); ok {
				items = arr
				break
			}
		}
		if items == nil {
			items = []interface{}{x}
		}
	default:
		log.Printf("parseJSONRecords: unsupported payload type=%T", payload)
		return nil, fmt.Errorf("unsupported json payload")
	}
	log.Printf("parseJSONRecords: normalized items=%d", len(items))
	out := []model.ImportedRecord{}
	for idx, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			log.Printf("parseJSONRecords: skip item=%d reason=not_object type=%T", idx+1, item)
			continue
		}
		rec := model.ImportedRecord{
			SampleID:     stringField(m, "sample_id", "patient_id", "fid"),
			FileID:       stringField(m, "file_id", "fisa_id"),
			PatientID:    stringField(m, "patient_id"),
			PatientName:  stringField(m, "patient_name", "name"),
			AnalyteTag:   strings.ToUpper(stringField(m, "analyte_tag", "tag")),
			AnalyteName:  stringField(m, "analyte_name", "name"),
			ResultValue:  stringField(m, "result_value", "result", "rez"),
			RawValue:     stringField(m, "raw_value", "result", "rez"),
			Unit:         stringField(m, "unit"),
			RackNo:       intField(m, "rack_no"),
			RackPosition: intField(m, "rack_position", "position"),
			ListPosition: intField(m, "list_position"),
			SampleNo:     intField(m, "sample_no"),
			Meta:         m,
		}
		if rec.SampleID == "" || rec.AnalyteTag == "" || rec.ResultValue == "" {
			log.Printf("parseJSONRecords: skip item=%d sample_id=%q analyte_tag=%q result_value=%q", idx+1, rec.SampleID, rec.AnalyteTag, rec.ResultValue)
			continue
		}
		log.Printf("parseJSONRecords: accept item=%d sample_id=%q analyte_tag=%q result_value=%q rack_no=%d rack_position=%d sample_no=%d", idx+1, rec.SampleID, rec.AnalyteTag, rec.ResultValue, rec.RackNo, rec.RackPosition, rec.SampleNo)
		out = append(out, rec)
	}
	if len(out) == 0 {
		log.Printf("parseJSONRecords: no valid rows found")
		return nil, errors.New("no valid result rows found in json file")
	}
	log.Printf("parseJSONRecords: parsed valid records=%d", len(out))
	return out, nil
}

func parseCSVRecords(raw []byte) ([]model.ImportedRecord, error) {
	rows, err := parseDelimitedRows(raw)
	if err != nil {
		log.Printf("parseCSVRecords: parseDelimitedRows failed err=%v", err)
		return nil, err
	}
	if len(rows) < 2 {
		log.Printf("parseCSVRecords: insufficient rows count=%d", len(rows))
		return nil, errors.New("csv requires header and at least one data row")
	}
	header := rows[0]
	log.Printf("parseCSVRecords: header=%s", mustJSON(header))
	index := map[string]int{}
	for i, col := range header {
		index[strings.ToLower(strings.TrimSpace(col))] = i
	}
	log.Printf("parseCSVRecords: index=%s", mustJSON(index))
	out := []model.ImportedRecord{}
	for rowIdx, row := range rows[1:] {
		log.Printf("parseCSVRecords: row=%d raw=%s", rowIdx+1, mustJSON(row))
		rec := model.ImportedRecord{
			SampleID:     csvValue(index, row, "sample_id", "patient_id", "fid"),
			FileID:       csvValue(index, row, "file_id", "fisa_id"),
			PatientID:    csvValue(index, row, "patient_id"),
			PatientName:  csvValue(index, row, "patient_name", "name"),
			AnalyteTag:   strings.ToUpper(csvValue(index, row, "analyte_tag", "tag")),
			AnalyteName:  csvValue(index, row, "analyte_name"),
			ResultValue:  csvValue(index, row, "result_value", "result", "rez"),
			RawValue:     csvValue(index, row, "raw_value", "result", "rez"),
			Unit:         csvValue(index, row, "unit"),
			RackNo:       csvInt(index, row, "rack_no"),
			RackPosition: csvInt(index, row, "rack_position", "position"),
			ListPosition: csvInt(index, row, "list_position"),
			SampleNo:     csvInt(index, row, "sample_no"),
			Meta:         map[string]interface{}{},
		}
		if rec.SampleID == "" || rec.AnalyteTag == "" || rec.ResultValue == "" {
			log.Printf("parseCSVRecords: skip row=%d sample_id=%q analyte_tag=%q result_value=%q", rowIdx+1, rec.SampleID, rec.AnalyteTag, rec.ResultValue)
			continue
		}
		log.Printf("parseCSVRecords: accept row=%d sample_id=%q analyte_tag=%q result_value=%q rack_no=%d rack_position=%d sample_no=%d", rowIdx+1, rec.SampleID, rec.AnalyteTag, rec.ResultValue, rec.RackNo, rec.RackPosition, rec.SampleNo)
		out = append(out, rec)
	}
	if len(out) == 0 {
		log.Printf("parseCSVRecords: no valid rows found")
		return nil, errors.New("no valid result rows found in csv file")
	}
	log.Printf("parseCSVRecords: parsed valid records=%d", len(out))
	return out, nil
}

func parseIRBiotyperCSV(raw []byte) ([]model.ImportedRecord, error) {
	lines := splitNonEmptyLines(string(raw))
	log.Printf("parseIRBiotyperCSV: lines=%d", len(lines))
	if len(lines) < 4 {
		log.Printf("parseIRBiotyperCSV: insufficient lines=%d", len(lines))
		return nil, errors.New("ir biotyper csv requires metadata lines, header and at least one data row")
	}

	runMeta := map[string]interface{}{}
	metaIdx := 0
	if strings.HasPrefix(lines[0], "#") {
		runMeta["server_version"] = strings.TrimPrefix(lines[0], "#")
		metaIdx++
	}
	if len(lines) > 1 && strings.HasPrefix(lines[1], "#") {
		runMeta["run_uuid"] = strings.TrimPrefix(lines[1], "#")
		metaIdx++
	}
	if len(lines) > 2 && strings.HasPrefix(lines[2], "#") {
		runMeta["run_name"] = strings.TrimPrefix(lines[2], "#")
		metaIdx++
	}

	rows, err := parseDelimitedRows([]byte(strings.Join(lines[metaIdx:], "\n")))
	if err != nil {
		log.Printf("parseIRBiotyperCSV: parseDelimitedRows failed err=%v", err)
		return nil, err
	}
	if len(rows) < 2 {
		log.Printf("parseIRBiotyperCSV: insufficient rows count=%d", len(rows))
		return nil, errors.New("ir biotyper csv requires header and at least one data row")
	}

	header := rows[0]
	log.Printf("parseIRBiotyperCSV: header=%s", mustJSON(header))
	index := map[string]int{}
	for i, col := range header {
		index[strings.ToLower(strings.TrimSpace(col))] = i
	}
	log.Printf("parseIRBiotyperCSV: index=%s", mustJSON(index))

	required := []string{"isolateidentifier", "modelkey", "bestvalue"}
	for _, key := range required {
		if _, ok := index[key]; !ok {
			log.Printf("parseIRBiotyperCSV: missing required column=%s", key)
			return nil, fmt.Errorf("ir biotyper csv missing required column %s", key)
		}
	}

	out := []model.ImportedRecord{}
	for rowIdx, row := range rows[1:] {
		log.Printf("parseIRBiotyperCSV: row=%d raw=%s", rowIdx+1, mustJSON(row))
		sampleID := csvValue(index, row, "isolateidentifier")
		modelKey := csvValue(index, row, "modelkey")
		bestValue := csvValue(index, row, "bestvalue")
		if sampleID == "" || modelKey == "" || bestValue == "" {
			log.Printf("parseIRBiotyperCSV: skip row=%d sample_id=%q model_key=%q best_value=%q", rowIdx+1, sampleID, modelKey, bestValue)
			continue
		}
		recordMeta := cloneMap(runMeta)
		recordMeta["source"] = "ir_biotyper"
		recordMeta["isolate_identifier"] = sampleID
		recordMeta["model_key"] = modelKey
		recordMeta["rating"] = csvValue(index, row, "rating")
		recordMeta["score_percent"] = csvValue(index, row, "scorepercent")

		flags := map[string]interface{}{}
		if rating := csvValue(index, row, "rating"); rating != "" {
			flags["rating"] = rating
		}
		if score := csvValue(index, row, "scorepercent"); score != "" {
			flags["score_percent"] = score
		}

		out = append(out, model.ImportedRecord{
			SampleID:    sampleID,
			FileID:      sampleID,
			AnalyteTag:  normalizeAnalyteTag(modelKey),
			AnalyteName: modelKey,
			ResultValue: bestValue,
			RawValue:    bestValue,
			Flags:       flags,
			Meta:        recordMeta,
		})
		log.Printf("parseIRBiotyperCSV: accept row=%d sample_id=%q analyte_tag=%q result_value=%q", rowIdx+1, sampleID, normalizeAnalyteTag(modelKey), bestValue)
	}
	if len(out) == 0 {
		log.Printf("parseIRBiotyperCSV: no valid rows found")
		return nil, errors.New("no valid result rows found in ir biotyper csv")
	}
	log.Printf("parseIRBiotyperCSV: parsed valid records=%d", len(out))
	return out, nil
}

type irbtRow struct {
	IsolateIdentifier string `json:"isolateIdentifier"`
	ModelKey          string `json:"modelKey"`
	BestValue         string `json:"bestValue"`
	Rating            string `json:"rating"`
	ScorePercent      string `json:"scorePercent"`
	RunName           string `json:"runName,omitempty"`
	RunUUID           string `json:"runUuid,omitempty"`
	ServerVersion     string `json:"serverVersion,omitempty"`
}

func parseIRBiotyperRows(path string) ([]irbtRow, error) {
	log.Printf("parseIRBiotyperRows: path=%s", path)
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Printf("parseIRBiotyperRows: read failed err=%v", err)
		return nil, err
	}
	lines := splitNonEmptyLines(string(raw))
	log.Printf("parseIRBiotyperRows: lines=%d", len(lines))
	if len(lines) < 4 {
		log.Printf("parseIRBiotyperRows: insufficient lines=%d", len(lines))
		return nil, errors.New("ir biotyper csv requires metadata lines, header and at least one data row")
	}
	runName, runUUID, serverVersion := "", "", ""
	metaIdx := 0
	if strings.HasPrefix(lines[0], "#") {
		serverVersion = strings.TrimPrefix(lines[0], "#")
		metaIdx++
	}
	if len(lines) > 1 && strings.HasPrefix(lines[1], "#") {
		runUUID = strings.TrimPrefix(lines[1], "#")
		metaIdx++
	}
	if len(lines) > 2 && strings.HasPrefix(lines[2], "#") {
		runName = strings.TrimPrefix(lines[2], "#")
		metaIdx++
	}
	rows, err := parseDelimitedRows([]byte(strings.Join(lines[metaIdx:], "\n")))
	if err != nil {
		log.Printf("parseIRBiotyperRows: parseDelimitedRows failed err=%v", err)
		return nil, err
	}
	if len(rows) < 2 {
		log.Printf("parseIRBiotyperRows: insufficient rows count=%d", len(rows))
		return nil, errors.New("ir biotyper csv requires header and at least one data row")
	}
	header := rows[0]
	log.Printf("parseIRBiotyperRows: header=%s", mustJSON(header))
	index := map[string]int{}
	for i, col := range header {
		index[strings.ToLower(strings.TrimSpace(col))] = i
	}
	log.Printf("parseIRBiotyperRows: index=%s", mustJSON(index))
	required := []string{"isolateidentifier", "modelkey", "bestvalue"}
	for _, key := range required {
		if _, ok := index[key]; !ok {
			log.Printf("parseIRBiotyperRows: missing required column=%s", key)
			return nil, fmt.Errorf("ir biotyper csv missing required column %s", key)
		}
	}
	out := make([]irbtRow, 0, len(rows)-1)
	for rowIdx, row := range rows[1:] {
		log.Printf("parseIRBiotyperRows: row=%d raw=%s", rowIdx+1, mustJSON(row))
		item := irbtRow{
			IsolateIdentifier: csvValue(index, row, "isolateidentifier"),
			ModelKey:          csvValue(index, row, "modelkey"),
			BestValue:         csvValue(index, row, "bestvalue"),
			Rating:            csvValue(index, row, "rating"),
			ScorePercent:      csvValue(index, row, "scorepercent"),
			RunName:           runName,
			RunUUID:           runUUID,
			ServerVersion:     serverVersion,
		}
		if item.IsolateIdentifier == "" || item.ModelKey == "" || item.BestValue == "" {
			log.Printf("parseIRBiotyperRows: skip row=%d isolateIdentifier=%q modelKey=%q bestValue=%q", rowIdx+1, item.IsolateIdentifier, item.ModelKey, item.BestValue)
			continue
		}
		log.Printf("parseIRBiotyperRows: accept row=%d isolateIdentifier=%q modelKey=%q bestValue=%q rating=%q scorePercent=%q", rowIdx+1, item.IsolateIdentifier, item.ModelKey, item.BestValue, item.Rating, item.ScorePercent)
		out = append(out, item)
	}
	if len(out) == 0 {
		log.Printf("parseIRBiotyperRows: no valid rows found")
		return nil, errors.New("no valid result rows found in ir biotyper csv")
	}
	log.Printf("parseIRBiotyperRows: parsed valid rows=%d", len(out))
	return out, nil
}

type irbtMapper struct {
	entries map[string]string
}

func loadIRBiotyperMapper(cfg *config.Config) (*irbtMapper, error) {
	mappingPath := ""
	if v, ok := cfg.Comm.ProtocolExtra["mapping_file"].(string); ok {
		mappingPath = strings.TrimSpace(v)
	}
	if mappingPath == "" {
		mappingPath = filepath.Join(filepath.Dir(cfg.ConfigPath()), "irbt-mapping.json")
	}
	raw, err := os.ReadFile(mappingPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &irbtMapper{entries: map[string]string{}}, nil
		}
		return nil, fmt.Errorf("read IRBT mapping file: %w", err)
	}
	entries := map[string]string{}
	var arr []map[string]string
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, item := range arr {
			key := irbtMapKey(item["modelKey"], item["bestValue"])
			tag := strings.TrimSpace(item["analyte_tag"])
			if key != "" && tag != "" {
				entries[key] = tag
			}
		}
		return &irbtMapper{entries: entries}, nil
	}
	var obj map[string]string
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("parse IRBT mapping file: %w", err)
	}
	for key, tag := range obj {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(tag) == "" {
			continue
		}
		entries[normalizeToken(key)] = strings.TrimSpace(tag)
	}
	return &irbtMapper{entries: entries}, nil
}

func (m *irbtMapper) Resolve(modelKey, bestValue string) (string, bool) {
	if m == nil {
		return "", false
	}
	tag, ok := m.entries[irbtMapKey(modelKey, bestValue)]
	if !ok || strings.TrimSpace(tag) == "" {
		return "", false
	}
	return strings.TrimSpace(tag), true
}

func irbtMapKey(modelKey, bestValue string) string {
	return normalizeToken(strings.TrimSpace(modelKey) + "||" + strings.TrimSpace(bestValue))
}

func (a *App) processIRBiotyperRow(sourceFile, orderDate string, row irbtRow, mapper *irbtMapper) (map[string]interface{}, error) {
	sampleID := strings.TrimSpace(row.IsolateIdentifier)
	order, err := a.store.FindOrderBySampleID(sampleID)
	if errors.Is(err, sql.ErrNoRows) {
		orderDate = effectiveImportOrderDate(orderDate)
		roundNo, roundErr := a.store.CurrentRoundNo(orderDate)
		if roundErr != nil {
			return nil, roundErr
		}
		order, err = a.store.EnsureImportedOrder(orderDate, roundNo, a.cfg.Layout.Kind, sampleID, sampleID, "", "", sourceFile)
	}
	if err != nil {
		return nil, fmt.Errorf("ensure order for sample_id=%s: %w", row.IsolateIdentifier, err)
	}
	tupleCode := irbtMapKey(row.ModelKey, row.BestValue)
	analyteTag, mapped, analyteCreated, err := a.resolveIRBTAnalyte(row, tupleCode, mapper)
	if err != nil {
		return nil, err
	}
	analysis, err := a.store.EnsureOrderAnalysis(order.ID, analyteTag, row.ModelKey, "completed")
	if err != nil {
		return nil, err
	}
	rawValue := mustJSON(map[string]interface{}{
		"modelKey":     row.ModelKey,
		"bestValue":    row.BestValue,
		"rating":       row.Rating,
		"scorePercent": row.ScorePercent,
	})
	interpreted := "low_confidence"
	if strings.EqualFold(strings.TrimSpace(row.Rating), "A") {
		interpreted = "valid"
	}
	flags := map[string]interface{}{
		"rating":       row.Rating,
		"scorePercent": row.ScorePercent,
	}
	result, created, err := a.store.UpsertResultForAnalysis(analysis.ID, row.BestValue, rawValue, interpreted, "", sourceFile, flags)
	if err != nil {
		return nil, err
	}
	payload := map[string]interface{}{
		"sample_id":       row.IsolateIdentifier,
		"order_id":        order.ID,
		"round_no":        order.RoundNo,
		"modelKey":        row.ModelKey,
		"bestValue":       row.BestValue,
		"rating":          row.Rating,
		"scorePercent":    row.ScorePercent,
		"analyte_tag":     analyteTag,
		"analyte_code":    tupleCode,
		"mapping_found":   mapped,
		"analyte_created": analyteCreated,
		"order_analysis":  analysis.ID,
		"result_id":       result.ID,
		"result_created":  created,
		"source_file":     sourceFile,
	}
	level := "info"
	message := "IR Biotyper row imported"
	if analyteCreated {
		level = "warning"
		message = "IR Biotyper row imported and analyte auto-created"
	} else if !mapped {
		level = "warning"
		message = "IR Biotyper row imported using analyte code lookup"
	}
	a.logEvent(level, "IRBT_IMPORT", message, payload)
	a.sendResultEvent("result_available", map[string]interface{}{
		"source_file": sourceFile,
		"round_no":    order.RoundNo,
		"order":       order,
		"analysis":    analysis,
		"result":      result,
	})
	return map[string]interface{}{
		"order": map[string]interface{}{
			"id":        order.ID,
			"round_no":  order.RoundNo,
			"sample_id": order.SampleID,
		},
		"analysis": map[string]interface{}{
			"id":          analysis.ID,
			"analyte_tag": analysis.AnalyteTag,
			"status":      analysis.Status,
		},
		"result": map[string]interface{}{
			"id":                result.ID,
			"created":           created,
			"result_value":      result.ResultValue,
			"raw_value":         result.RawValue,
			"interpreted_value": result.Interpreted,
			"flags":             result.Flags,
		},
		"mapping_found":   mapped,
		"analyte_created": analyteCreated,
	}, nil
}

func (a *App) resolveIRBTAnalyte(row irbtRow, tupleCode string, mapper *irbtMapper) (string, bool, bool, error) {
	if tag, mapped := mapper.Resolve(row.ModelKey, row.BestValue); mapped {
		if err := a.ensureIRBTAnalyte(tupleCode, tag, row); err != nil {
			return "", false, false, err
		}
		return tag, true, false, nil
	}
	if analyte, err := a.store.GetAnalyteByCode(tupleCode); err == nil {
		return analyte.Tag, false, false, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", false, false, err
	}
	tag := generateIRBTAnalyteTag(row.ModelKey, row.BestValue)
	if err := a.ensureIRBTAnalyte(tupleCode, tag, row); err != nil {
		return "", false, false, err
	}
	return tag, false, true, nil
}

func (a *App) ensureIRBTAnalyte(tupleCode, tag string, row irbtRow) error {
	_, err := a.store.UpsertAnalyte(model.Analyte{
		Active:           true,
		Tag:              tag,
		Code:             tupleCode,
		Name:             fmt.Sprintf("%s | %s", strings.TrimSpace(row.ModelKey), strings.TrimSpace(row.BestValue)),
		Description:      "Auto-generated from IR Biotyper tuple",
		ResultType:       "text",
		ResultFormatting: "raw",
		ProtocolOptions: map[string]interface{}{
			"source":     "ir_biotyper",
			"modelKey":   row.ModelKey,
			"bestValue":  row.BestValue,
			"tuple_code": tupleCode,
		},
	})
	return err
}

func generateIRBTAnalyteTag(modelKey, bestValue string) string {
	tag := "IRBT_" + normalizeAnalyteTag(modelKey+"_"+bestValue)
	tag = strings.Trim(tag, "_")
	if tag == "IRBT" || tag == "IRBT_" {
		return "IRBT_AUTO"
	}
	return tag
}

func parseDelimitedRows(raw []byte) ([][]string, error) {
	text := string(raw)
	delimiter := sniffDelimiter(text)
	log.Printf("parseDelimitedRows: delimiter=%q bytes=%d", string(delimiter), len(raw))
	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma = delimiter
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		log.Printf("parseDelimitedRows: csv read failed err=%v", err)
		return nil, err
	}
	log.Printf("parseDelimitedRows: raw rows=%d", len(rows))
	filtered := make([][]string, 0, len(rows))
	for idx, row := range rows {
		if len(row) == 0 {
			log.Printf("parseDelimitedRows: skip row=%d reason=empty_row", idx+1)
			continue
		}
		if len(row) == 1 && strings.TrimSpace(row[0]) == "" {
			log.Printf("parseDelimitedRows: skip row=%d reason=blank_row", idx+1)
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(row[0]), "#") {
			log.Printf("parseDelimitedRows: skip row=%d reason=comment raw=%s", idx+1, mustJSON(row))
			continue
		}
		log.Printf("parseDelimitedRows: keep row=%d raw=%s", idx+1, mustJSON(row))
		filtered = append(filtered, row)
	}
	log.Printf("parseDelimitedRows: filtered rows=%d", len(filtered))
	return filtered, nil
}

func sniffDelimiter(text string) rune {
	for _, line := range splitNonEmptyLines(text) {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.Count(line, ";") > strings.Count(line, ",") {
			return ';'
		}
		break
	}
	return ','
}

func splitNonEmptyLines(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func isIRBiotyperProtocol(protocol string) bool {
	switch normalizeToken(protocol) {
	case "IRBIOTYPER", "IR_BIOTYPER":
		return true
	default:
		return false
	}
}

var nonAlphaNum = regexp.MustCompile(`[^A-Z0-9]+`)

func normalizeAnalyteTag(name string) string {
	token := normalizeToken(name)
	token = strings.Trim(token, "_")
	if token == "" {
		return "IR_BIOTYPER"
	}
	return token
}

func normalizeToken(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	return nonAlphaNum.ReplaceAllString(value, "_")
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mustJSON(value interface{}) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"marshal_error":%q}`, err.Error())
	}
	return string(raw)
}

func (a *App) archiveFile(src, dstDir string) error {
	if a.cfg.Comm.File.ArchiveMode == "none" {
		return os.Remove(src)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(dstDir, filepath.Base(src))
	if err := os.Rename(src, target); err == nil {
		return nil
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, raw, 0o644); err != nil {
		return err
	}
	if a.cfg.Comm.File.ArchiveMode == "move" {
		return os.Remove(src)
	}
	return nil
}

func stringField(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func intField(m map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch x := v.(type) {
			case float64:
				return int(x)
			case int:
				return x
			}
		}
	}
	return 0
}

func csvValue(index map[string]int, row []string, keys ...string) string {
	for _, key := range keys {
		if idx, ok := index[key]; ok && idx < len(row) {
			if s := strings.TrimSpace(row[idx]); s != "" {
				return s
			}
		}
	}
	return ""
}

func csvInt(index map[string]int, row []string, keys ...string) int {
	for _, key := range keys {
		if idx, ok := index[key]; ok && idx < len(row) {
			var n int
			if _, err := fmt.Sscanf(strings.TrimSpace(row[idx]), "%d", &n); err == nil {
				return n
			}
		}
	}
	return 0
}
