package reader

import (
	"fmt"
	"time"

	"wisemed-labreaders/readerslast/generic-test-reader/internal/config"
	"wisemed-labreaders/readerslast/generic-test-reader/internal/model"
)

func (a *App) handleCommand(msg Envelope) {
	if msg.RequestID == "" {
		msg.RequestID = newRequestID()
	}
	command, _ := msg.Payload["command"].(string)
	args, _ := msg.Payload["args"].(map[string]interface{})
	if args == nil {
		args = map[string]interface{}{}
	}

	switch command {
	case "get_status", "reader.status":
		a.respond(msg.RequestID, true, a.StatusSnapshot(), "")
	case "get_stats", "stats.get":
		data, err := a.StatsForDate(strArg(args, "order_date", ""))
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, data, "")
	case "get_stats_series", "stats.series":
		data, err := a.StatsSeries(intArg(args, "series_limit", intArg(args, "limit", 14)))
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, data, "")
	case "get_config", "config.get":
		section := strArg(args, "section", "")
		if section != "" {
			value, err := a.ConfigSection(section)
			if err != nil {
				a.respond(msg.RequestID, false, nil, err.Error())
				return
			}
			a.respond(msg.RequestID, true, map[string]interface{}{"section": section, "config": value}, "")
			return
		}
		cfg, err := a.ConfigSnapshot()
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"config": cfg}, "")
	case "set_config", "config.set":
		section := strArg(args, "section", "")
		if section != "" {
			payload, ok := args["data"]
			if !ok {
				a.respond(msg.RequestID, false, nil, "data is required when section is provided")
				return
			}
			if err := a.UpdateConfigSection(section, payload); err != nil {
				a.respond(msg.RequestID, false, nil, err.Error())
				return
			}
			value, err := a.ConfigSection(section)
			if err != nil {
				a.respond(msg.RequestID, false, nil, err.Error())
				return
			}
			a.respond(msg.RequestID, true, map[string]interface{}{"section": section, "config": value}, "")
			return
		}
		if err := a.UpdateConfig(args); err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		cfg, err := a.ConfigSnapshot()
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"config": cfg}, "")
	case "get_logs", "logs.list":
		limit := intArg(args, "limit", 100)
		items, err := a.ListLogs(limit)
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"logs": items}, "")
	case "read_last_log_lines", "logs.tail":
		limit := intArg(args, "lines", intArg(args, "limit", 100))
		items, err := a.ListLogs(limit)
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{
			"lines": limit,
			"topic": a.logTopic(),
			"logs":  items,
		}, "")
	case "activate_real_time_logs", "logs.activate":
		a.rtLogsMu.Lock()
		a.rtLogs = true
		a.rtLogsMu.Unlock()
		a.respond(msg.RequestID, true, map[string]interface{}{
			"active": true,
			"topic":  a.logTopic(),
		}, "")
	case "deactivate_real_time_logs", "logs.deactivate":
		a.rtLogsMu.Lock()
		a.rtLogs = false
		a.rtLogsMu.Unlock()
		a.respond(msg.RequestID, true, map[string]interface{}{
			"active": false,
			"topic":  a.logTopic(),
		}, "")
	case "activate_real_time_results", "results.activate":
		a.respond(msg.RequestID, true, map[string]interface{}{
			"active": true,
			"topic":  a.resultsTopic(),
		}, "")
	case "deactivate_real_time_results", "results.deactivate":
		a.respond(msg.RequestID, true, map[string]interface{}{
			"active": false,
			"topic":  a.resultsTopic(),
		}, "")
	case "list_analytes", "analytes.list":
		items, err := a.ListAnalytes()
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"analytes": items}, "")
	case "analytes.get":
		var (
			item model.Analyte
			err  error
		)
		if id := int64Arg(args, "id", 0); id > 0 {
			item, err = a.GetAnalyteByID(id)
		} else {
			tag := strArg(args, "tag", "")
			item, err = a.GetAnalyte(tag)
		}
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"analyte": item}, "")
	case "upsert_analyte", "analytes.create", "analytes.update":
		item := analyteFromArgs(args)
		id, err := a.SaveAnalyte(item)
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"id": id, "tag": item.Tag}, "")
	case "delete_analyte", "analytes.delete":
		id := int64Arg(args, "id", 0)
		if err := a.DeleteAnalyte(id); err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"deleted": id}, "")
	case "list_orders", "orders.list":
		roundNo := intArg(args, "round_no", intArg(args, "round_id", 0))
		orderDate := strArg(args, "order_date", "")
		if boolArg(args, "include_analysis", false) {
			items, err := a.store.ListOrderBundles(roundNo, orderDate)
			if err != nil {
				a.respond(msg.RequestID, false, nil, err.Error())
				return
			}
			a.respond(msg.RequestID, true, map[string]interface{}{"orders": items}, "")
			return
		}
		items, err := a.store.ListOrders(roundNo, orderDate)
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"orders": items}, "")
	case "list_order_rounds", "orders.rounds":
		orderDate := strArg(args, "order_date", time.Now().Format("2006-01-02"))
		rounds, err := a.ListRoundNumbers(orderDate)
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		currentRoundNo := 1
		if len(rounds) > 0 {
			currentRoundNo = rounds[len(rounds)-1]
		}
		a.respond(msg.RequestID, true, map[string]interface{}{
			"order_date": orderDate,
			"round_no":   currentRoundNo,
			"rounds":     rounds,
		}, "")
	case "orders.get":
		item, err := a.store.GetOrdersByID(int64(intArg(args, "id", 0)))
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"order": item}, "")
	case "create_order", "update_order", "orders.create", "orders.update":
		orderDate := strArg(args, "order_date", time.Now().Format("2006-01-02"))
		currentRoundNo, err := a.store.CurrentRoundNo(orderDate)
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		order := model.Order{
			RoundNo:      intArg(args, "round_no", intArg(args, "round_id", currentRoundNo)),
			OrderDate:    orderDate,
			SampleID:     strArg(args, "sample_id", ""),
			FileID:       strArg(args, "file_id", ""),
			PatientID:    strArg(args, "patient_id", ""),
			PatientName:  strArg(args, "patient_name", ""),
			RackNo:       intArg(args, "rack_no", 0),
			RackPosition: intArg(args, "rack_position", 0),
			ListPosition: intArg(args, "list_position", 0),
			SampleNo:     intArg(args, "sample_no", 0),
			Status:       strArg(args, "status", "scheduled"),
			SourceFile:   strArg(args, "source_file", ""),
		}
		if a.cfg.Layout.Kind == config.LayoutSimple {
			order.RackNo = 1
			order.RackPosition = 0
		}
		item, err := a.store.UpsertOrder(order)
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"order": item}, "")
	case "delete_order", "orders.delete":
		if err := a.store.DeleteOrder(int64(intArg(args, "id", 0))); err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"deleted": intArg(args, "id", 0)}, "")
	case "list_order_analysis", "order_analysis.list":
		items, err := a.ListOrderAnalyses(int64Arg(args, "order_id", 0))
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"order_analyses": items}, "")
	case "get_order_analysis", "order_analysis.get":
		item, err := a.GetOrderAnalysis(int64Arg(args, "id", 0))
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"order_analysis": item}, "")
	case "create_order_analysis", "update_order_analysis", "order_analysis.create", "order_analysis.update":
		item, err := a.SaveOrderAnalysis(orderAnalysisFromArgs(args))
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"order_analysis": item}, "")
	case "delete_order_analysis", "order_analysis.delete":
		id := int64Arg(args, "id", 0)
		if err := a.DeleteOrderAnalysis(id); err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"deleted": id}, "")
	case "list_results", "results.list":
		items, err := a.store.ListResults(intArg(args, "limit", 100))
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{"results": items}, "")
	case "get_comm_config", "comm.get":
		a.respond(msg.RequestID, true, map[string]interface{}{
			"communication": commConfigPayload(a.cfg),
			"layout":        layoutConfigPayload(a.cfg),
		}, "")
	case "set_comm_config", "comm.set":
		patch := map[string]interface{}{
			"type":     a.cfg.Comm.Type,
			"protocol": a.cfg.Comm.Protocol,
		}
		if v, ok := args["type"].(string); ok && v != "" {
			patch["type"] = v
		}
		if v, ok := args["protocol"].(string); ok && v != "" {
			patch["protocol"] = v
		}
		if err := a.UpdateConfigSection("communication", patch); err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{
			"communication": commConfigPayload(a.cfg),
			"layout":        layoutConfigPayload(a.cfg),
		}, "")
	case "import_file", "imports.run_file":
		path := strArg(args, "path", "")
		if path == "" {
			a.respond(msg.RequestID, false, nil, "path is required")
			return
		}
		summary, err := a.ImportFileNow(path, strArg(args, "order_date", ""))
		if err != nil {
			a.respond(msg.RequestID, false, nil, err.Error())
			return
		}
		a.respond(msg.RequestID, true, map[string]interface{}{
			"imported":  summary.Imported,
			"warnings":  summary.Warnings,
			"protocol":  summary.Protocol,
			"file_name": summary.FileName,
		}, "")
	default:
		a.respond(msg.RequestID, false, nil, fmt.Sprintf("unsupported command %q", command))
	}
}

func commConfigPayload(cfg *config.Config) map[string]interface{} {
	return map[string]interface{}{
		"type":           cfg.Comm.Type,
		"protocol":       cfg.Comm.Protocol,
		"protocol_extra": cfg.Comm.ProtocolExtra,
		"file": map[string]interface{}{
			"import_dir":     cfg.Comm.File.ImportDir,
			"export_dir":     cfg.Comm.File.ExportDir,
			"processed_dir":  cfg.Comm.File.ProcessedDir,
			"failed_dir":     cfg.Comm.File.FailedDir,
			"pattern":        cfg.Comm.File.Pattern,
			"poll_seconds":   cfg.Comm.File.PollSeconds,
			"stable_wait_ms": cfg.Comm.File.StableWaitMS,
			"archive_mode":   cfg.Comm.File.ArchiveMode,
		},
		"serial": map[string]interface{}{
			"port":      cfg.Comm.Serial.Port,
			"baud":      cfg.Comm.Serial.Baud,
			"parity":    cfg.Comm.Serial.Parity,
			"data_bits": cfg.Comm.Serial.DataBits,
			"stop_bits": cfg.Comm.Serial.StopBits,
		},
		"network": map[string]interface{}{
			"host": cfg.Comm.Network.Host,
			"port": cfg.Comm.Network.Port,
			"mode": cfg.Comm.Network.Mode,
		},
	}
}

func layoutConfigPayload(cfg *config.Config) map[string]interface{} {
	return map[string]interface{}{
		"kind":               cfg.Layout.Kind,
		"racks_count":        cfg.Layout.RacksCount,
		"positions_per_rack": cfg.Layout.PositionsPerRack,
	}
}

func analyteFromArgs(args map[string]interface{}) model.Analyte {
	return model.Analyte{
		ID:                int64Arg(args, "id", 0),
		Active:            boolArg(args, "active", true),
		Tag:               strArg(args, "tag", ""),
		Code:              strArg(args, "code", ""),
		Name:              strArg(args, "name", ""),
		Description:       strArg(args, "description", ""),
		ResultType:        strArg(args, "result_type", "numeric"),
		ResultFormatting:  strArg(args, "result_formatting", "raw"),
		ResultWeighting:   floatArg(args, "result_weighting", 1),
		ResultMeasureUnit: strArg(args, "result_measure_unit", ""),
		ResultReagentsSet: strArg(args, "result_reagents_set", ""),
	}
}

func orderAnalysisFromArgs(args map[string]interface{}) model.OrderAnalysis {
	return model.OrderAnalysis{
		ID:              int64Arg(args, "id", 0),
		OrderID:         int64Arg(args, "order_id", 0),
		AnalyteID:       int64Arg(args, "analyte_id", 0),
		AnalyteTag:      strArg(args, "analyte_tag", ""),
		AnalyteName:     strArg(args, "analyte_name", ""),
		Status:          strArg(args, "status", "scheduled"),
		DefaultResultID: int64Arg(args, "default_result_id", 0),
		ResultValue:     strArg(args, "result_value", ""),
		RawValue:        strArg(args, "raw_value", ""),
		Interpreted:     strArg(args, "interpreted_value", ""),
		Unit:            strArg(args, "unit", ""),
		SourceFile:      strArg(args, "source_file", ""),
	}
}

func strArg(args map[string]interface{}, key, fallback string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

func intArg(args map[string]interface{}, key string, fallback int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return fallback
}

func int64Arg(args map[string]interface{}, key string, fallback int64) int64 {
	switch v := args[key].(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	}
	return fallback
}

func boolArg(args map[string]interface{}, key string, fallback bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return fallback
}

func floatArg(args map[string]interface{}, key string, fallback float64) float64 {
	if v, ok := args[key].(float64); ok {
		return v
	}
	return fallback
}
