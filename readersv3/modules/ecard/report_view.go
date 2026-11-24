package ecard

import "encoding/json"

// The public response describes the last SDK once. Earlier attempts are kept
// under other_sdks; the internal forensic snapshot remains separately available.
func compactOperationReport(full map[string]interface{}) map[string]interface{} {
	j, _ := full["job"].(Job)
	state, operation := j.State, j.Operation
	if job, ok := full["job"].(map[string]interface{}); ok {
		state, _ = job["state"].(string)
		operation, _ = job["operation"].(string)
	}
	final, _ := full["final_response"].(map[string]interface{})
	sdk, _ := final["sdk_used"].(string)
	request := compactRequest(full["request"], operation)
	if operation == "sign" {
		request["items"] = full["submitted_items"]
	}
	var finalGroup map[string]interface{}
	others := []interface{}{}
	groups, _ := full["sdk_responses"].([]interface{})
	for _, value := range groups {
		group, _ := value.(map[string]interface{})
		if group["sdk"] == sdk {
			finalGroup = group
			continue
		}
		response := compactSDKResponse(group["response"])
		others = append(others, map[string]interface{}{
			"sdk": group["sdk"], "request": compactRequest(group["request"], operation),
			"response": response, "events": compactEvents(group["events"]), "errors": compactErrors(group["errors"], response),
		})
	}
	response := compactSDKResponse(final["response"])
	if terminalState(state) {
		response["ok"] = state == "completed"
		response["data"] = final["data"]
		if state != "completed" && response["error"] == nil {
			code, _ := final["error"].(string)
			if code == "" {
				code = state
			}
			response["error"] = map[string]interface{}{"code": code, "message": errorDescription(code, 0)}
		}
		if operation == "sign" {
			response["data"] = final["results"]
		}
	}
	if !terminalState(state) {
		response = nil
	}
	out := map[string]interface{}{
		"operation_id": full["operation_id"], "state": state, "used_sdk": sdk, "request": request,
		"response": response, "events": compactEvents(finalGroup["events"]),
		"errors": compactErrors(finalGroup["errors"], response), "other_sdks": others,
	}
	if pin, ok := full["pin_request"]; ok {
		out["pin_request"] = pin
	}
	if warning, ok := full["diagnostic_warning"]; ok {
		out["diagnostic_warning"] = warning
	}
	explainErrors(out)
	return out
}

func compactRequest(value interface{}, operation string) map[string]interface{} {
	raw, _ := value.(map[string]interface{})
	if operation == "" {
		operation, _ = raw["operation"].(string)
	}
	out := map[string]interface{}{"operation": operation}
	keys := []string{}
	switch operation {
	case "read":
		keys = []string{"contract", "fields"}
	case "certificates":
		keys = []string{"contract"}
	case "sign":
		keys = []string{"contract", "expected", "encoding", "certificate"}
	case "write":
		keys = []string{"contract", "cid", "fields", "data", "confirm"}
	case "activate", "change-pin", "reset-pin", "enroll":
		keys = []string{"contract", "confirm"}
	}
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			out[key] = value
		}
	}
	return out
}

func compactSDKResponse(value interface{}) map[string]interface{} {
	raw, _ := value.(map[string]interface{})
	ok, _ := raw["ok"].(bool)
	out := map[string]interface{}{"ok": ok, "data": raw["data"], "error": nil}
	if !ok && raw != nil {
		out["error"] = compactError(raw)
	}
	return out
}

func compactError(raw map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, key := range []string{"code", "error_code", "sdk_code", "stage", "message", "detail", "exception_type", "transport_error", "diagnostics"} {
		if value, ok := raw[key]; ok && value != nil && value != "" {
			out[key] = value
		}
	}
	if code, ok := out["code"].(string); ok && out["message"] == nil {
		sdk, _ := out["sdk_code"].(float64)
		out["message"] = errorDescription(code, int(sdk))
	}
	return out
}

func compactErrors(value interface{}, response map[string]interface{}) []interface{} {
	out := []interface{}{}
	seen := map[string]bool{}
	add := func(v map[string]interface{}) {
		if len(v) == 0 {
			return
		}
		encoded, _ := json.Marshal(v)
		if seen[string(encoded)] {
			return
		}
		seen[string(encoded)] = true
		out = append(out, v)
	}
	items, _ := value.([]interface{})
	for _, item := range items {
		raw, _ := item.(map[string]interface{})
		if nested, ok := raw["response"].(map[string]interface{}); ok {
			add(compactError(nested))
		} else if nested, ok := raw["details"].(map[string]interface{}); ok {
			err := compactError(nested)
			if len(err) == 0 {
				err["event"] = raw["event"]
			}
			add(err)
		} else {
			add(compactError(raw))
		}
	}
	if response != nil {
		if err, ok := response["error"].(map[string]interface{}); ok {
			add(err)
		}
	}
	return out
}

func compactEvents(value interface{}) []interface{} {
	out := []interface{}{}
	items, _ := value.([]interface{})
	for _, item := range items {
		raw, _ := item.(map[string]interface{})
		event := map[string]interface{}{"event": raw["event"]}
		if time, ok := raw["time"]; ok {
			event["time"] = time
		}
		if data, ok := raw["data"].(map[string]interface{}); ok {
			detail := map[string]interface{}{}
			for key, value := range data {
				switch key {
				case "response", "fields", "operation", "dll_sha256", "wrapper_sha256":
					continue
				}
				if value != nil && value != "" {
					detail[key] = value
				}
			}
			if reply, ok := data["response"].(map[string]interface{}); ok {
				detail["ok"] = reply["ok"]
				if message, ok := reply["message"].(string); ok && message != "" {
					detail["message"] = message
				}
				if code, ok := reply["code"]; ok {
					detail["code"] = code
				}
				if code, ok := reply["sdk_code"]; ok {
					detail["sdk_code"] = code
				}
			}
			if raw["event"] == "read_result" {
				if fields, ok := data["fields"]; ok {
					detail["fields"] = fields
				}
			}
			if len(detail) > 0 {
				event["data"] = detail
			}
		} else if raw["data"] != nil {
			event["data"] = raw["data"]
		}
		out = append(out, event)
	}
	return out
}
