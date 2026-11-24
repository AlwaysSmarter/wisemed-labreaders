package ecard

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"wisemed-labreaders/readersv3/shared/apibridge"
)

func terminalState(state string) bool {
	return state != "uploading" && state != "queued" && state != "running" && state != "waiting_pin"
}

func (m *Module) reportHTTP(w http.ResponseWriter, r *http.Request, owner string, j Job) {
	report, err := m.operationReport(owner, j)
	if err != nil {
		fail(w, 500, err)
		return
	}
	if r.URL.Query().Get("diagnostics") != "full" {
		report = compactOperationReport(report)
	}
	// Parts bypass respond's JSON normalization; give them the same error
	// envelopes as the ordinary HTTP and WSS responses before hashing.
	if raw, e := json.Marshal(report); e == nil {
		var normalized map[string]interface{}
		if json.Unmarshal(raw, &normalized) == nil {
			explainErrors(normalized)
			report = normalized
		}
	}
	if r.URL.Query().Has("part") {
		if !terminalState(j.State) {
			fail(w, 409, errors.New("wait for terminal state before downloading report parts"))
			return
		}
		part, err := strconv.Atoi(r.URL.Query().Get("part"))
		if err != nil || part < 0 {
			fail(w, 400, errors.New("invalid report part"))
			return
		}
		raw, err := json.Marshal(report)
		if err != nil {
			fail(w, 500, err)
			return
		}
		const chunk = 512 * 1024
		count := (len(raw) + chunk - 1) / chunk
		if part >= count {
			fail(w, 416, errors.New("report part out of range"))
			return
		}
		end := (part + 1) * chunk
		if end > len(raw) {
			end = len(raw)
		}
		sum := sha256.Sum256(raw)
		respond(w, 200, map[string]interface{}{"operation_id": j.ID, "part": part, "parts": count, "total_bytes": len(raw), "sha256": hex.EncodeToString(sum[:]), "encoding": "base64-json-utf8", "data_base64": base64.StdEncoding.EncodeToString(raw[part*chunk : end])})
		return
	}
	if _, remote := apibridge.PrincipalFrom(r.Context()); remote {
		raw, err := json.Marshal(report)
		if err != nil {
			fail(w, 500, err)
			return
		}
		if len(raw) > 3<<20 {
			respond(w, 413, map[string]interface{}{"error": "complete_report_exceeds_WSM_limit", "report_parts_path": "/api/ecard/jobs/" + j.ID + "/report?part=0", "results_path": "/api/ecard/jobs/" + j.ID + "/results", "message": "Retrieve report parts over the same authenticated transport and reconstruct the complete JSON using its SHA-256. No report was truncated."})
			return
		}
	}
	respond(w, 200, report)
}

func (m *Module) operationReport(owner string, j Job) (map[string]interface{}, error) {
	// Do not let callers accidentally bypass owner checks when reusing this helper.
	if _, err := m.store.job(j.ID, owner); err != nil {
		return nil, errors.New("job not found")
	}
	var cached []byte
	if err := m.store.db.QueryRow(`SELECT payload FROM operation_reports WHERE job=? AND owner=?`, j.ID, owner).Scan(&cached); err == nil {
		var out map[string]interface{}
		err = m.store.open(cached, "ecard-report:"+owner+":"+j.ID, &out)
		return out, err
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var requestBlob []byte
	if err := m.store.db.QueryRow(`SELECT request FROM jobs WHERE id=? AND owner=?`, j.ID, owner).Scan(&requestBlob); err != nil {
		return nil, err
	}
	var request interface{}
	if err := m.store.open(requestBlob, j.ID, &request); err != nil {
		return nil, err
	}
	request = cleanHistory(request)
	attempts, err := m.sdkAttempts(j.ID)
	if err != nil {
		return nil, err
	}
	groups := []interface{}{}
	bySDK := map[string]map[string]interface{}{}
	group := func(sdk string) map[string]interface{} {
		if g := bySDK[sdk]; g != nil {
			return g
		}
		g := map[string]interface{}{"sdk": sdk, "request": request, "response": nil, "events": []interface{}{}, "errors": []interface{}{}}
		bySDK[sdk] = g
		groups = append(groups, g)
		return g
	}
	for _, item := range attempts {
		a, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		sdk, _ := a["sdk"].(string)
		g := group(sdk)
		g["response"] = a["response"]
		g["try_next_sdk"] = a["try_next_sdk"]
		if response, ok := a["response"].(map[string]interface{}); ok && response["ok"] != true {
			g["errors"] = append(g["errors"].([]interface{}), response)
		}
		if transport, ok := a["transport_error"]; ok {
			g["errors"] = append(g["errors"].([]interface{}), map[string]interface{}{"transport_error": transport})
		}
	}
	events := []interface{}{}
	started, finished := false, false
	rows, err := m.eventRows(owner, j.ID, 0)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var eventOwner string
		var raw []byte
		var event map[string]interface{}
		if err = rows.Scan(&id, &eventOwner, &raw); err != nil {
			break
		}
		if err = m.store.open(raw, "ecard-event:"+eventOwner, &event); err != nil {
			break
		}
		if event["job"] != j.ID {
			continue
		}
		event["id"] = id
		events = append(events, event)
		kind, _ := event["event"].(string)
		data, _ := event["data"].(map[string]interface{})
		if (kind == "operation_started" || kind == "operation_accepted" || (kind == "operation_finished" && data["not_started"] == true)) && data["report_version"] == float64(1) {
			started = true
		}
		if kind == "operation_finished" {
			finished = true
		}
		sdk, _ := event["source"].(string)
		if sdk == "worker" {
			continue
		}
		if sdk == "pcsc" {
			sdk, _ = data["sdk"].(string)
			if sdk == "" {
				continue
			}
		}
		g := group(sdk)
		g["events"] = append(g["events"].([]interface{}), event)
		if strings.HasSuffix(kind, "_failed") || (data["sdk_code"] != nil && data["sdk_code"] != float64(0)) {
			g["errors"] = append(g["errors"].([]interface{}), map[string]interface{}{"event_id": id, "event": kind, "details": data})
		}
		if kind == "call_finished" || kind == "host_start_failed" {
			if response, ok := data["response"].(map[string]interface{}); ok {
				g["response"] = response
				if response["ok"] != true {
					g["errors"] = append(g["errors"].([]interface{}), map[string]interface{}{"event_id": id, "response": response})
				}
			}
			if transport, ok := data["transport_error"]; ok {
				g["errors"] = append(g["errors"].([]interface{}), map[string]interface{}{"event_id": id, "transport_error": transport})
			}
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	items := []interface{}{}
	submitted := []interface{}{}
	rows, err = m.store.db.Query(`SELECT n,payload,result FROM items WHERE job=? ORDER BY n`, j.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var n int
		var raw []byte
		var payload []byte
		var value interface{}
		if err = rows.Scan(&n, &payload, &raw); err != nil {
			break
		}
		if j.Operation == "sign" && len(payload) > 0 {
			var input interface{}
			if err = m.store.open(payload, j.ID+":"+strconv.Itoa(n), &input); err != nil {
				break
			}
			submitted = append(submitted, cleanHistory(input))
		}
		if raw == nil {
			continue
		}
		if err = m.store.open(raw, j.ID+":result:"+strconv.Itoa(n), &value); err != nil {
			break
		}
		items = append(items, value)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	sdk := j.SDK
	if len(groups) > 0 {
		sdk, _ = groups[len(groups)-1].(map[string]interface{})["sdk"].(string)
	}
	finalEvents := []interface{}{}
	var response interface{}
	finalErrors := []interface{}{}
	if g := bySDK[sdk]; g != nil {
		finalEvents = g["events"].([]interface{})
		response = g["response"]
		finalErrors = g["errors"].([]interface{})
	}
	final := map[string]interface{}{"sdk_used": sdk, "state": j.State, "ok": j.State == "completed", "response": response, "data": items, "errors": finalErrors}
	if j.Operation != "sign" {
		if reply, ok := response.(map[string]interface{}); ok {
			final["data"] = reply["data"]
		}
	}
	final["results"] = items
	if j.Error != "" {
		final["error"] = j.Error
	}
	out := map[string]interface{}{"report_version": 1, "operation_id": j.ID, "job": j, "request": request, "complete": terminalState(j.State), "events_complete": started && (!terminalState(j.State) || finished), "final_response": final, "final_response_events": finalEvents, "sdk_responses": groups, "events": events}
	if j.Operation == "sign" {
		out["submitted_items"] = submitted
		for _, g := range bySDK {
			g["submitted_items_reference"] = "/submitted_items"
		}
	}
	if !started {
		out["diagnostic_warning"] = "Legacy operation: only events still retained by the previous version can be recovered; missing events cannot be reconstructed."
	}
	if a := m.active[j.ID]; !terminalState(j.State) && a != nil && a.prompt != "" {
		out["pin_request"] = map[string]string{"prompt": a.prompt, "challenge": a.challenge}
	}
	if terminalState(j.State) && j.State != "completed" && response == nil {
		final["message"] = fmt.Sprintf("Operation ended with state %s; inspect job.error and events", j.State)
	}
	if terminalState(j.State) {
		blob, err := m.store.seal(out, "ecard-report:"+owner+":"+j.ID)
		if err != nil {
			return nil, err
		}
		if _, err = m.store.db.Exec(`INSERT OR IGNORE INTO operation_reports(job,owner,payload) VALUES(?,?,?)`, j.ID, owner, blob); err != nil {
			return nil, err
		}
	}
	return out, nil
}
