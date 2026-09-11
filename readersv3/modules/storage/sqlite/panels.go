package sqlite

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	model "wisemed-labreaders/readersv3/modules/core/model"
)

// RecordImportedPanel commits one complete measurement, including its history,
// atomically. panel_id separates repeats even when they arrive in the same second.
func (s *Store) RecordImportedPanel(date string, records []model.ImportedRecord, source string) (int64, error) {
	if len(records) == 0 || strings.TrimSpace(records[0].SampleID) == "" {
		return 0, errors.New("empty panel")
	}
	sample := records[0].SampleID
	seen := map[string]bool{}
	for _, r := range records {
		if r.SampleID != sample || r.AnalyteTag == "" || seen[r.AnalyteTag] || strings.TrimSpace(r.ResultValue) == "" {
			return 0, errors.New("invalid panel")
		}
		seen[r.AnalyteTag] = true
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return 0, err
	}
	panelID := hex.EncodeToString(nonce[:])
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	date = normalizeDate(date)
	var round int
	if err = tx.QueryRow(`select coalesce(max(round_no),1) from rounds where order_date=?`, date).Scan(&round); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`insert or ignore into rounds(order_date,round_no,created_at) values(?,?,?)`, date, round, now); err != nil {
		return 0, err
	}
	var orderID int64
	err = tx.QueryRow(`select id from orders where order_date=? and round_no=? and sample_id=? limit 1`, date, round, sample).Scan(&orderID)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	meta, err := json.Marshal(metaOrEmpty(records[0].Meta))
	if err != nil {
		return 0, err
	}
	if orderID == 0 {
		lookupErr := tx.QueryRow(`select id from orders where order_date=? and round_no=? and json_extract(meta_json, '$.id_correction.original_sample_id')=? limit 1`, date, round, sample).Scan(&orderID)
		if lookupErr != nil && lookupErr != sql.ErrNoRows {
			return 0, lookupErr
		}
	}
	if orderID == 0 {
		res, e := tx.Exec(`insert into orders(round_no,order_date,sample_id,file_id,status,source_file,meta_json,created_at,updated_at) values(?,?,?,?,'received',?,?,?,?)`, round, date, sample, records[0].FileID, source, string(meta), now, now)
		if e != nil {
			return 0, e
		}
		orderID, _ = res.LastInsertId()
	} else {
		if _, err = tx.Exec(`update orders set status='received',source_file=?,updated_at=? where id=?`, source, now, orderID); err != nil {
			return 0, err
		}
	}
	for _, r := range records {
		flags := mergeMeta(r.Flags, map[string]interface{}{"panel_id": panelID})
		flagsJSON, e := json.Marshal(flags)
		if e != nil {
			return 0, e
		}
		var analysisID int64
		err = tx.QueryRow(`select id from order_analyses where order_id=? and analyte_tag=? limit 1`, orderID, r.AnalyteTag).Scan(&analysisID)
		if err != nil && err != sql.ErrNoRows {
			return 0, err
		}
		if analysisID == 0 {
			res, e := tx.Exec(`insert into order_analyses(order_id,analyte_id,analyte_tag,analyte_name) values(?,coalesce((select id from analytes where tag=? limit 1),0),?,?)`, orderID, r.AnalyteTag, r.AnalyteTag, r.AnalyteName)
			if e != nil {
				return 0, e
			}
			analysisID, _ = res.LastInsertId()
		}
		res, e := tx.Exec(`insert into order_analysis_results(order_analysis_id,result_value,raw_value,interpreted_value,source_result_value,source_raw_value,source_interpreted_value,unit,source_file,flags_json,created_at) values(?,?,?,?,?,?,?,?,?,?,?)`, analysisID, r.ResultValue, r.RawValue, r.Interpreted, r.ResultValue, r.RawValue, r.Interpreted, r.Unit, source, string(flagsJSON), now)
		if e != nil {
			return 0, e
		}
		resultID, _ := res.LastInsertId()
		if _, err = tx.Exec(`update order_analyses set status='completed',default_result_id=?,result_value=?,raw_value=?,interpreted_value=?,source_result_value=?,source_raw_value=?,source_interpreted_value=?,unit=?,source_file=?,flags_json=? where id=?`, resultID, r.ResultValue, r.RawValue, r.Interpreted, r.ResultValue, r.RawValue, r.Interpreted, r.Unit, source, string(flagsJSON), analysisID); err != nil {
			return 0, err
		}
	}
	return orderID, tx.Commit()
}
