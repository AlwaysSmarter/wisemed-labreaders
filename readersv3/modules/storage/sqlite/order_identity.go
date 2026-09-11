package sqlite

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	model "wisemed-labreaders/readersv3/modules/core/model"
)

// Matching/sending keeps an identity stable for the duration of the operation.
func (s *Store) BeginOrderIdentityUse() func() {
	s.identityMu.RLock()
	return s.identityMu.RUnlock
}

func (s *Store) CorrectOrderID(change model.OrderIDChange, actor string) (model.Order, error) {
	change.NewID = strings.TrimSpace(change.NewID)
	change.Reason = strings.TrimSpace(change.Reason)
	if change.OrderID <= 0 || change.NewID == "" || len(change.NewID) > 200 || len(change.Reason) > 1000 || change.NewID == change.ExpectedID || change.Confirmation != "deacord" || strings.TrimSpace(actor) == "" || strings.ContainsFunc(change.NewID, unicode.IsControl) {
		return model.Order{}, model.ErrOrderIdentityInvalid
	}
	s.identityMu.Lock()
	defer s.identityMu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return model.Order{}, err
	}
	defer tx.Rollback()
	var oldID, date, metaJSON string
	var round int
	if err = tx.QueryRow(`select sample_id,order_date,round_no,meta_json from orders where id=?`, change.OrderID).Scan(&oldID, &date, &round, &metaJSON); err != nil {
		return model.Order{}, err
	}
	meta := map[string]interface{}{}
	if err = json.Unmarshal([]byte(metaJSON), &meta); err != nil {
		return model.Order{}, err
	}
	revision := identityRevision(meta)
	if oldID != change.ExpectedID || revision != change.ExpectedRevision {
		return model.Order{}, model.ErrOrderIdentityConflict
	}
	var collision int
	if err = tx.QueryRow(`select count(*) from orders where order_date=? and round_no=? and (sample_id=? or json_extract(meta_json, '$.id_correction.original_sample_id')=?) and id<>?`, date, round, change.NewID, change.NewID, change.OrderID).Scan(&collision); err != nil {
		return model.Order{}, err
	}
	if collision > 0 {
		return model.Order{}, model.ErrOrderIdentityConflict
	}
	original := oldID
	originalSample := oldID
	if raw, ok := meta["sent_sample_code"].(string); ok && raw != "" {
		original = raw
	}
	if previous, ok := meta["id_correction"].(map[string]interface{}); ok {
		if value, ok := previous["original_id"].(string); ok {
			original = value
		}
		if value, ok := previous["original_sample_id"].(string); ok {
			originalSample = value
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	correction := map[string]interface{}{"original_id": original, "original_sample_id": originalSample, "previous_id": oldID, "new_id": change.NewID, "actor": actor, "reason": change.Reason, "changed_at": now}
	history, _ := meta["id_correction_history"].([]interface{})
	history = append(history, correction)
	// Clear the old patient/file association and send state, never the measurements.
	for key := range meta {
		if strings.HasPrefix(key, "sync_") || strings.HasPrefix(key, "sample_code_") || strings.HasPrefix(key, "wisemed_") || key == "file_id" {
			delete(meta, key)
		}
	}
	meta["id_correction"] = correction
	meta["id_correction_history"] = history
	meta["id_revision"] = revision + 1
	meta["wisemed_match_flag"] = 1
	meta["wisemed_send_status"] = "pending"
	encoded, err := json.Marshal(meta)
	if err != nil {
		return model.Order{}, err
	}
	if _, err = tx.Exec(`update orders set sample_id=?,file_id=?,patient_id='',patient_name='',status='received',meta_json=?,updated_at=? where id=?`, change.NewID, change.NewID, string(encoded), now, change.OrderID); err != nil {
		return model.Order{}, err
	}
	rows, err := tx.Query(`select id,flags_json from order_analyses where order_id=?`, change.OrderID)
	if err != nil {
		return model.Order{}, err
	}
	type update struct {
		id    int64
		flags string
	}
	updates := []update{}
	for rows.Next() {
		var id int64
		var raw string
		if err = rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return model.Order{}, err
		}
		flags := map[string]interface{}{}
		if err = json.Unmarshal([]byte(raw), &flags); err != nil {
			rows.Close()
			return model.Order{}, err
		}
		for key := range flags {
			if strings.HasPrefix(key, "wisemed_") {
				delete(flags, key)
			}
		}
		flags["wisemed_match_flag"] = 1
		flags["wisemed_send_status"] = "pending"
		b, e := json.Marshal(flags)
		if e != nil {
			rows.Close()
			return model.Order{}, e
		}
		updates = append(updates, update{id, string(b)})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return model.Order{}, err
	}
	for _, u := range updates {
		if _, err = tx.Exec(`update order_analyses set wisemed_sm_id='',wisemed_fsm_id='',flags_json=? where id=?`, u.flags, u.id); err != nil {
			return model.Order{}, err
		}
	}
	audit, _ := json.Marshal(map[string]interface{}{"order_id": change.OrderID, "revision": revision + 1, "change": correction})
	message := fmt.Sprintf("ID modificat de %s, valoarea initiala: %s; %s -> %s", actor, original, oldID, change.NewID)
	if change.Reason != "" {
		message += " (" + change.Reason + ")"
	}
	if _, err = tx.Exec(`insert into audit_logs(level,event_type,actor,message,meta_json,created_at) values('info','order-id-change',?,?,?,?)`, actor, message, string(audit), now); err != nil {
		return model.Order{}, err
	}
	if err = tx.Commit(); err != nil {
		return model.Order{}, err
	}
	return s.GetOrder(change.OrderID)
}
func identityRevision(meta map[string]interface{}) int {
	switch n := meta["id_revision"].(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

// Reject outdated copies (for example a sync started before an ID correction).
func checkOrderIdentity(current, proposed map[string]interface{}) error {
	if identityRevision(current) != identityRevision(proposed) {
		return model.ErrOrderIdentityConflict
	}
	return nil
}
