package ecard

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"time"
)

func (m *Module) saveSDKAttempt(id string, n int, sdk string, req Request, reply Reply, callErr error, retry bool) error {
	entry := map[string]interface{}{"time": time.Now().UTC().Format(time.RFC3339Nano), "sdk": sdk, "operation": req.Operation, "fields": req.Fields, "response": reply, "try_next_sdk": retry}
	if callErr != nil {
		entry["transport_error"] = callErr.Error()
	}
	// Apply the same redaction as HTTP call history before persisting diagnostics.
	raw, e := json.Marshal(entry)
	if e != nil {
		return e
	}
	var safe interface{}
	if e = json.Unmarshal(raw, &safe); e != nil {
		return e
	}
	safe = cleanHistory(safe)
	blob, e := m.store.seal(safe, fmt.Sprintf("%s:sdk:%d", id, n))
	if e != nil {
		return e
	}
	_, e = m.store.db.Exec(`INSERT INTO sdk_attempts(job,n,payload) VALUES(?,?,?)`, id, n, blob)
	if e != nil {
		return e
	}
	var owner string
	if e = m.store.db.QueryRow(`SELECT owner FROM jobs WHERE id=?`, id).Scan(&owner); e != nil {
		return e
	}
	m.emitEvent(owner, id, sdk, "sdk_attempt_finished", safe)
	historyID := uuid.NewString()
	logEntry := map[string]interface{}{"time": entry["time"], "method": "SDK", "path": "/api/ecard/jobs/" + id, "status": reply.Code, "request": map[string]interface{}{"sdk": sdk, "operation": req.Operation, "fields": req.Fields}, "response": safe}
	blob, e = m.store.seal(logEntry, historyID)
	if e != nil {
		return e
	}
	_, e = m.store.db.Exec(`INSERT INTO call_history(id,owner,created,payload) VALUES(?,?,?,?)`, historyID, owner, time.Now().UnixNano(), blob)
	return e
}
func (m *Module) sdkAttempts(id string) ([]interface{}, error) {
	rows, e := m.store.db.Query(`SELECT n,payload FROM sdk_attempts WHERE job=? ORDER BY n`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []interface{}{}
	for rows.Next() {
		var n int
		var blob []byte
		var v interface{}
		if e = rows.Scan(&n, &blob); e != nil {
			return nil, e
		}
		if e = m.store.open(blob, fmt.Sprintf("%s:sdk:%d", id, n), &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
