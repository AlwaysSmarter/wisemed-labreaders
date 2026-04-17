package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"wisemed-labreaders/readerslast/generic-test-reader/internal/model"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) init() error {
	pragmas := []string{
		`pragma busy_timeout = 5000`,
		`pragma journal_mode = WAL`,
		`pragma synchronous = NORMAL`,
		`pragma foreign_keys = ON`,
	}
	for _, stmt := range pragmas {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	stmts := []string{
		`create table if not exists analytes (
			id integer primary key autoincrement,
			active integer not null default 1,
			tag text not null unique,
			code text not null default '',
			name text not null,
			description text not null default '',
			result_type text not null default 'numeric',
			result_formatting text not null default 'raw',
			result_weighting real not null default 1,
			transformation_json text not null default '[]',
			result_measure_unit text not null default '',
			result_reagents_set text not null default '',
			protocol_options_json text not null default '{}',
			created_at text not null,
			updated_at text not null
		)`,
		`create table if not exists orders (
			id integer primary key autoincrement,
			round_no integer not null default 0,
			order_date text not null,
			sample_id text not null,
			file_id text not null default '',
			patient_id text not null default '',
			patient_name text not null default '',
			rack_no integer not null default 0,
			rack_position integer not null default 0,
			list_position integer not null default 0,
			sample_no integer not null default 0,
			status text not null default 'new',
			source_file text not null default '',
			created_at text not null,
			updated_at text not null
		)`,
		`create table if not exists rounds (
			id integer primary key autoincrement,
			order_date text not null,
			round_no integer not null,
			created_at text not null,
			unique(order_date, round_no)
		)`,
		`create table if not exists order_analyses (
			id integer primary key autoincrement,
			order_id integer not null,
			analyte_id integer not null default 0,
			analyte_tag text not null,
			analyte_name text not null default '',
			status text not null default 'new',
			requested_at text not null default '',
			received_at text not null default '',
			default_result_id integer not null default 0,
			result_value text not null default '',
			raw_value text not null default '',
			interpreted_value text not null default '',
			unit text not null default '',
			source_file text not null default '',
			flags_json text not null default '{}'
		)`,
		`create table if not exists order_analysis_results (
			id integer primary key autoincrement,
			order_analysis_id integer not null,
			result_value text not null default '',
			raw_value text not null default '',
			interpreted_value text not null default '',
			unit text not null default '',
			source_file text not null default '',
			flags_json text not null default '{}',
			created_at text not null
		)`,
		`create table if not exists event_logs (
			id integer primary key autoincrement,
			level text not null,
			event_type text not null,
			message text not null,
			payload_json text not null default '{}',
			created_at text not null
		)`,
		`create index if not exists idx_orders_round_no on orders(round_no)`,
		`create index if not exists idx_orders_sample on orders(sample_id)`,
		`create index if not exists idx_rounds_date_round on rounds(order_date, round_no)`,
		`create index if not exists idx_order_analyses_order on order_analyses(order_id)`,
		`create index if not exists idx_results_analysis on order_analysis_results(order_analysis_id)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	if err := s.migrateAwayMetaJSON(); err != nil {
		return err
	}
	if err := s.ensureOrderAnalysesDefaultColumns(); err != nil {
		return err
	}
	if err := s.migrateOrdersSchema(); err != nil {
		return err
	}
	if err := s.ensureRoundsSeeded(); err != nil {
		return err
	}
	return nil
}

func (s *Store) migrateAwayMetaJSON() error {
	migrations := []struct {
		table     string
		temp      string
		createSQL string
		copySQL   string
	}{
		{
			table: "orders",
			temp:  "orders_v2",
			createSQL: `create table orders_v2 (
				id integer primary key autoincrement,
				round_no integer not null default 0,
				order_date text not null,
				sample_id text not null,
				file_id text not null default '',
				patient_id text not null default '',
				patient_name text not null default '',
				rack_no integer not null default 0,
				rack_position integer not null default 0,
				list_position integer not null default 0,
				sample_no integer not null default 0,
				status text not null default 'new',
				source_file text not null default '',
				created_at text not null,
				updated_at text not null
			)`,
			copySQL: `insert into orders_v2(id,round_no,order_date,sample_id,file_id,patient_id,patient_name,rack_no,rack_position,list_position,sample_no,status,source_file,created_at,updated_at)
				select id,coalesce(round_no,0),order_date,sample_id,file_id,patient_id,patient_name,rack_no,rack_position,list_position,sample_no,status,source_file,created_at,updated_at from orders`,
		},
		{
			table: "order_analyses",
			temp:  "order_analyses_v2",
			createSQL: `create table order_analyses_v2 (
				id integer primary key autoincrement,
				order_id integer not null,
				analyte_id integer not null default 0,
				analyte_tag text not null,
				analyte_name text not null default '',
				status text not null default 'new',
				requested_at text not null default '',
				received_at text not null default '',
				default_result_id integer not null default 0,
				result_value text not null default '',
				raw_value text not null default '',
				interpreted_value text not null default '',
				unit text not null default '',
				source_file text not null default '',
				flags_json text not null default '{}'
			)`,
			copySQL: `insert into order_analyses_v2(id,order_id,analyte_id,analyte_tag,analyte_name,status,requested_at,received_at,default_result_id,result_value,raw_value,interpreted_value,unit,source_file,flags_json)
				select id,order_id,0,analyte_tag,analyte_name,status,requested_at,received_at,0,'','','','','','{}' from order_analyses`,
		},
		{
			table: "order_analysis_results",
			temp:  "order_analysis_results_v2",
			createSQL: `create table order_analysis_results_v2 (
				id integer primary key autoincrement,
				order_analysis_id integer not null,
				result_value text not null default '',
				raw_value text not null default '',
				interpreted_value text not null default '',
				unit text not null default '',
				source_file text not null default '',
				flags_json text not null default '{}',
				created_at text not null
			)`,
			copySQL: `insert into order_analysis_results_v2(id,order_analysis_id,result_value,raw_value,interpreted_value,unit,source_file,flags_json,created_at)
				select id,order_analysis_id,result_value,raw_value,interpreted_value,unit,source_file,flags_json,created_at from order_analysis_results`,
		},
	}

	for _, migration := range migrations {
		hasMeta, err := s.tableHasColumn(migration.table, "meta_json")
		if err != nil {
			return err
		}
		if !hasMeta {
			continue
		}
		if err := s.rebuildTableWithoutMeta(migration.table, migration.temp, migration.createSQL, migration.copySQL); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) tableHasColumn(table, column string) (bool, error) {
	rows, err := s.db.Query(fmt.Sprintf("pragma table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (s *Store) rebuildTableWithoutMeta(table, temp, createSQL, copySQL string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(createSQL); err != nil {
		return err
	}
	if _, err := tx.Exec(copySQL); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf("drop table %s", table)); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf("alter table %s rename to %s", temp, table)); err != nil {
		return err
	}
	switch table {
	case "orders":
		if _, err := tx.Exec(`create index if not exists idx_orders_round_no on orders(round_no)`); err != nil {
			return err
		}
		if _, err := tx.Exec(`create index if not exists idx_orders_sample on orders(sample_id)`); err != nil {
			return err
		}
	case "order_analyses":
		if _, err := tx.Exec(`create index if not exists idx_order_analyses_order on order_analyses(order_id)`); err != nil {
			return err
		}
	case "order_analysis_results":
		if _, err := tx.Exec(`create index if not exists idx_results_analysis on order_analysis_results(order_analysis_id)`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ensureOrderAnalysesDefaultColumns() error {
	columns := []struct {
		name    string
		sqlType string
		def     string
	}{
		{"analyte_id", "integer", "0"},
		{"default_result_id", "integer", "0"},
		{"result_value", "text", "''"},
		{"raw_value", "text", "''"},
		{"interpreted_value", "text", "''"},
		{"unit", "text", "''"},
		{"source_file", "text", "''"},
		{"flags_json", "text", "'{}'"},
	}
	for _, column := range columns {
		has, err := s.tableHasColumn("order_analyses", column.name)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		stmt := fmt.Sprintf("alter table order_analyses add column %s %s not null default %s", column.name, column.sqlType, column.def)
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrateOrdersSchema() error {
	hasRoundNo, err := s.tableHasColumn("orders", "round_no")
	if err != nil {
		return err
	}
	hasRoundID, err := s.tableHasColumn("orders", "round_id")
	if err != nil {
		return err
	}
	if hasRoundID || !hasRoundNo {
		if err := s.rebuildOrdersWithoutRoundID(); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`update orders set round_no = case when round_no <= 0 then 1 else round_no end`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`create index if not exists idx_orders_round_no on orders(round_no)`); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureRoundsSeeded() error {
	if _, err := s.db.Exec(`insert or ignore into rounds(order_date, round_no, created_at)
		select order_date, round_no, min(created_at)
		from orders
		where trim(order_date) <> '' and round_no > 0
		group by order_date, round_no`); err != nil {
		return err
	}
	return nil
}

func (s *Store) rebuildOrdersWithoutRoundID() error {
	hasRoundID, err := s.tableHasColumn("orders", "round_id")
	if err != nil {
		return err
	}
	selectRoundNo := "coalesce(round_no, 1)"
	if hasRoundID {
		selectRoundNo = "coalesce((select rounds.round_no from rounds where rounds.id = orders.round_id), round_no, 1)"
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`create table orders_v3 (
		id integer primary key autoincrement,
		round_no integer not null default 1,
		order_date text not null,
		sample_id text not null,
		file_id text not null default '',
		patient_id text not null default '',
		patient_name text not null default '',
		rack_no integer not null default 0,
		rack_position integer not null default 0,
		list_position integer not null default 0,
		sample_no integer not null default 0,
		status text not null default 'new',
		source_file text not null default '',
		created_at text not null,
		updated_at text not null
	)`); err != nil {
		return err
	}
	copySQL := fmt.Sprintf(`insert into orders_v3(id,round_no,order_date,sample_id,file_id,patient_id,patient_name,rack_no,rack_position,list_position,sample_no,status,source_file,created_at,updated_at)
		select id,%s,order_date,sample_id,file_id,patient_id,patient_name,rack_no,rack_position,list_position,sample_no,status,source_file,created_at,updated_at from orders`, selectRoundNo)
	if _, err := tx.Exec(copySQL); err != nil {
		return err
	}
	if _, err := tx.Exec(`drop table orders`); err != nil {
		return err
	}
	if _, err := tx.Exec(`alter table orders_v3 rename to orders`); err != nil {
		return err
	}
	if _, err := tx.Exec(`create index if not exists idx_orders_round_no on orders(round_no)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`create index if not exists idx_orders_sample on orders(sample_id)`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AppendEvent(level, eventType, message string, payload map[string]interface{}) error {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	raw, _ := json.Marshal(payload)
	_, err := s.db.Exec(`insert into event_logs(level,event_type,message,payload_json,created_at) values(?,?,?,?,?)`,
		level, eventType, message, string(raw), time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) ListEvents(limit int) ([]model.EventLog, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`select id,level,event_type,message,payload_json,created_at from event_logs order by id desc limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.EventLog{}
	for rows.Next() {
		var item model.EventLog
		var payloadJSON, created string
		if err := rows.Scan(&item.ID, &item.Level, &item.EventType, &item.Message, &payloadJSON, &created); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(payloadJSON), &item.Payload)
		item.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpsertAnalyte(a model.Analyte) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	trans, _ := json.Marshal(a.Transformation)
	protocol, _ := json.Marshal(a.ProtocolOptions)
	a.Tag = strings.TrimSpace(a.Tag)
	a.Name = strings.TrimSpace(a.Name)
	if a.Tag == "" || a.Name == "" {
		return 0, errors.New("analyte tag and name are required")
	}

	var existingID int64
	err := s.db.QueryRow(`select id from analytes where tag = ? limit 1`, a.Tag).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if a.ID > 0 {
		if existingID > 0 && existingID != a.ID {
			return 0, errors.New("analyte tag must be unique")
		}
		res, err := s.db.Exec(`update analytes set
			active=?, tag=?, code=?, name=?, description=?, result_type=?, result_formatting=?, result_weighting=?,
			transformation_json=?, result_measure_unit=?, result_reagents_set=?, protocol_options_json=?, updated_at=?
			where id = ?`,
			boolToInt(a.Active), a.Tag, a.Code, a.Name, a.Description, a.ResultType, a.ResultFormatting, a.ResultWeighting,
			string(trans), a.ResultMeasureUnit, a.ResultReagentsSet, string(protocol), now, a.ID)
		if err != nil {
			return 0, err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return 0, sql.ErrNoRows
		}
		return a.ID, nil
	}
	if existingID > 0 {
		return 0, errors.New("analyte tag must be unique")
	}
	res, err := s.db.Exec(`insert into analytes(active,tag,code,name,description,result_type,result_formatting,result_weighting,transformation_json,result_measure_unit,result_reagents_set,protocol_options_json,created_at,updated_at)
	values(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		boolToInt(a.Active), a.Tag, a.Code, a.Name, a.Description, a.ResultType, a.ResultFormatting, a.ResultWeighting,
		string(trans), a.ResultMeasureUnit, a.ResultReagentsSet, string(protocol), now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (s *Store) DeleteAnalyte(id int64) error {
	_, err := s.db.Exec(`delete from analytes where id = ?`, id)
	return err
}

func (s *Store) GetAnalyte(tag string) (model.Analyte, error) {
	rows, err := s.db.Query(`select id,active,tag,code,name,description,result_type,result_formatting,result_weighting,transformation_json,result_measure_unit,result_reagents_set,protocol_options_json,created_at,updated_at from analytes where tag = ? limit 1`, tag)
	if err != nil {
		return model.Analyte{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return model.Analyte{}, sql.ErrNoRows
	}
	var a model.Analyte
	var active int
	var transJSON, protocolJSON, created, updated string
	if err := rows.Scan(&a.ID, &active, &a.Tag, &a.Code, &a.Name, &a.Description, &a.ResultType, &a.ResultFormatting, &a.ResultWeighting, &transJSON, &a.ResultMeasureUnit, &a.ResultReagentsSet, &protocolJSON, &created, &updated); err != nil {
		return model.Analyte{}, err
	}
	a.Active = active == 1
	_ = json.Unmarshal([]byte(transJSON), &a.Transformation)
	_ = json.Unmarshal([]byte(protocolJSON), &a.ProtocolOptions)
	a.CreatedAt, _ = time.Parse(time.RFC3339, created)
	a.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return a, nil
}

func (s *Store) GetAnalyteByID(id int64) (model.Analyte, error) {
	rows, err := s.db.Query(`select id,active,tag,code,name,description,result_type,result_formatting,result_weighting,transformation_json,result_measure_unit,result_reagents_set,protocol_options_json,created_at,updated_at from analytes where id = ? limit 1`, id)
	if err != nil {
		return model.Analyte{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return model.Analyte{}, sql.ErrNoRows
	}
	var a model.Analyte
	var active int
	var transJSON, protocolJSON, created, updated string
	if err := rows.Scan(&a.ID, &active, &a.Tag, &a.Code, &a.Name, &a.Description, &a.ResultType, &a.ResultFormatting, &a.ResultWeighting, &transJSON, &a.ResultMeasureUnit, &a.ResultReagentsSet, &protocolJSON, &created, &updated); err != nil {
		return model.Analyte{}, err
	}
	a.Active = active == 1
	_ = json.Unmarshal([]byte(transJSON), &a.Transformation)
	_ = json.Unmarshal([]byte(protocolJSON), &a.ProtocolOptions)
	a.CreatedAt, _ = time.Parse(time.RFC3339, created)
	a.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return a, nil
}

func (s *Store) GetAnalyteByCode(code string) (model.Analyte, error) {
	rows, err := s.db.Query(`select id,active,tag,code,name,description,result_type,result_formatting,result_weighting,transformation_json,result_measure_unit,result_reagents_set,protocol_options_json,created_at,updated_at from analytes where code = ? limit 1`, code)
	if err != nil {
		return model.Analyte{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return model.Analyte{}, sql.ErrNoRows
	}
	var a model.Analyte
	var active int
	var transJSON, protocolJSON, created, updated string
	if err := rows.Scan(&a.ID, &active, &a.Tag, &a.Code, &a.Name, &a.Description, &a.ResultType, &a.ResultFormatting, &a.ResultWeighting, &transJSON, &a.ResultMeasureUnit, &a.ResultReagentsSet, &protocolJSON, &created, &updated); err != nil {
		return model.Analyte{}, err
	}
	a.Active = active == 1
	_ = json.Unmarshal([]byte(transJSON), &a.Transformation)
	_ = json.Unmarshal([]byte(protocolJSON), &a.ProtocolOptions)
	a.CreatedAt, _ = time.Parse(time.RFC3339, created)
	a.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return a, nil
}

func (s *Store) ListAnalytes() ([]model.Analyte, error) {
	rows, err := s.db.Query(`select id,active,tag,code,name,description,result_type,result_formatting,result_weighting,transformation_json,result_measure_unit,result_reagents_set,protocol_options_json,created_at,updated_at from analytes order by name asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Analyte{}
	for rows.Next() {
		var a model.Analyte
		var active int
		var transJSON, protocolJSON, created, updated string
		if err := rows.Scan(&a.ID, &active, &a.Tag, &a.Code, &a.Name, &a.Description, &a.ResultType, &a.ResultFormatting, &a.ResultWeighting, &transJSON, &a.ResultMeasureUnit, &a.ResultReagentsSet, &protocolJSON, &created, &updated); err != nil {
			return nil, err
		}
		a.Active = active == 1
		_ = json.Unmarshal([]byte(transJSON), &a.Transformation)
		_ = json.Unmarshal([]byte(protocolJSON), &a.ProtocolOptions)
		a.CreatedAt, _ = time.Parse(time.RFC3339, created)
		a.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) CurrentRoundNo(orderDate string) (int, error) {
	orderDate = normalizeOrderDate(orderDate)
	var roundNo int
	err := s.db.QueryRow(`select coalesce(max(round_no), 0) from rounds where order_date = ?`, orderDate).Scan(&roundNo)
	if err != nil {
		return 0, err
	}
	if roundNo <= 0 {
		if err := s.EnsureRound(orderDate, 1); err != nil {
			return 0, err
		}
		roundNo = 1
	}
	return roundNo, nil
}

func (s *Store) NextRoundNo(orderDate string) (int, error) {
	orderDate = normalizeOrderDate(orderDate)
	var roundNo int
	err := s.db.QueryRow(`select coalesce(max(round_no), 0) + 1 from rounds where order_date = ?`, orderDate).Scan(&roundNo)
	if err != nil {
		return 0, err
	}
	if roundNo <= 0 {
		roundNo = 1
	}
	return roundNo, nil
}

func (s *Store) EnsureRound(orderDate string, roundNo int) error {
	orderDate = normalizeOrderDate(orderDate)
	if roundNo <= 0 {
		roundNo = 1
	}
	_, err := s.db.Exec(`insert or ignore into rounds(order_date, round_no, created_at) values(?,?,?)`,
		orderDate, roundNo, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) CreateNextRound(orderDate string) (int, error) {
	orderDate = normalizeOrderDate(orderDate)
	nextRoundNo, err := s.NextRoundNo(orderDate)
	if err != nil {
		return 0, err
	}
	if err := s.EnsureRound(orderDate, nextRoundNo); err != nil {
		return 0, err
	}
	return nextRoundNo, nil
}

func (s *Store) DeleteRoundNo(roundNo int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`delete from order_analysis_results where order_analysis_id in (select id from order_analyses where order_id in (select id from orders where round_no = ?))`, roundNo); err != nil {
		return err
	}
	if _, err := tx.Exec(`delete from order_analyses where order_id in (select id from orders where round_no = ?)`, roundNo); err != nil {
		return err
	}
	if _, err := tx.Exec(`delete from orders where round_no = ?`, roundNo); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpsertOrder(order model.Order) (model.Order, error) {
	order.OrderDate = normalizeOrderDate(order.OrderDate)
	if order.RoundNo == 0 {
		roundNo, err := s.CurrentRoundNo(order.OrderDate)
		if err != nil {
			return model.Order{}, err
		}
		order.RoundNo = roundNo
	}
	if order.SampleID == "" {
		return model.Order{}, errors.New("order.sample_id is required")
	}
	if err := s.EnsureRound(order.OrderDate, order.RoundNo); err != nil {
		return model.Order{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var existingID int64
	err := s.db.QueryRow(`select id from orders where round_no = ? and sample_id = ? limit 1`, order.RoundNo, order.SampleID).Scan(&existingID)
	if err == sql.ErrNoRows {
		res, err := s.db.Exec(`insert into orders(round_no,order_date,sample_id,file_id,patient_id,patient_name,rack_no,rack_position,list_position,sample_no,status,source_file,created_at,updated_at)
			values(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			order.RoundNo, order.OrderDate, order.SampleID, order.FileID,
			order.PatientID, order.PatientName, order.RackNo, order.RackPosition, order.ListPosition, order.SampleNo, defaultString(order.Status, "new"), order.SourceFile,
			now, now)
		if err != nil {
			return model.Order{}, err
		}
		id, _ := res.LastInsertId()
		return s.GetOrder(id)
	}
	if err != nil {
		return model.Order{}, err
	}
	_, err = s.db.Exec(`update orders set round_no=?,order_date=?,file_id=?,patient_id=?,patient_name=?,rack_no=?,rack_position=?,list_position=?,sample_no=?,status=?,source_file=?,updated_at=? where id = ?`,
		order.RoundNo, order.OrderDate, order.FileID, order.PatientID, order.PatientName,
		order.RackNo, order.RackPosition, order.ListPosition, order.SampleNo, defaultString(order.Status, "new"), order.SourceFile, now, existingID)
	if err != nil {
		return model.Order{}, err
	}
	return s.GetOrder(existingID)
}

func (s *Store) GetOrder(id int64) (model.Order, error) {
	var item model.Order
	var created, updated string
	err := s.db.QueryRow(`select id,round_no,order_date,sample_id,file_id,patient_id,patient_name,rack_no,rack_position,list_position,sample_no,status,source_file,created_at,updated_at from orders where id = ?`, id).
		Scan(&item.ID, &item.RoundNo, &item.OrderDate, &item.SampleID, &item.FileID, &item.PatientID, &item.PatientName, &item.RackNo, &item.RackPosition, &item.ListPosition, &item.SampleNo, &item.Status, &item.SourceFile, &created, &updated)
	if err != nil {
		return item, err
	}
	item.CreatedAt, _ = time.Parse(time.RFC3339, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return item, nil
}

func (s *Store) ListOrders(roundNo int, orderDate string) ([]model.Order, error) {
	query := `select id from orders`
	args := []interface{}{}
	conditions := []string{}
	if roundNo > 0 {
		conditions = append(conditions, `round_no = ?`)
		args = append(args, roundNo)
	}
	if strings.TrimSpace(orderDate) != "" {
		conditions = append(conditions, `order_date = ?`)
		args = append(args, strings.TrimSpace(orderDate))
	}
	if len(conditions) > 0 {
		query += ` where ` + strings.Join(conditions, ` and `)
	}
	query += ` order by id desc`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Order{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		item, err := s.GetOrder(id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListOrderBundles(roundNo int, orderDate string) ([]model.OrderBundle, error) {
	orders, err := s.ListOrders(roundNo, orderDate)
	if err != nil {
		return nil, err
	}
	out := make([]model.OrderBundle, 0, len(orders))
	for _, order := range orders {
		analyses, err := s.ListAnalysesForOrder(order.ID)
		if err != nil {
			return nil, err
		}
		bundle := model.OrderBundle{Order: order, Analyses: make([]model.OrderAnalysisBundle, 0, len(analyses))}
		for _, analysis := range analyses {
			results, err := s.ListResultsForAnalysis(analysis.ID)
			if err != nil {
				return nil, err
			}
			bundle.Analyses = append(bundle.Analyses, model.OrderAnalysisBundle{
				Analysis: analysis,
				Results:  results,
			})
		}
		out = append(out, bundle)
	}
	return out, nil
}

func (s *Store) ListRoundNumbers(orderDate string) ([]int, error) {
	orderDate = normalizeOrderDate(orderDate)
	if err := s.EnsureRound(orderDate, 1); err != nil {
		return nil, err
	}
	query := `select distinct round_no from rounds where round_no > 0`
	args := []interface{}{}
	if strings.TrimSpace(orderDate) != "" {
		query += ` and order_date = ?`
		args = append(args, orderDate)
	}
	query += ` order by round_no asc`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int{}
	for rows.Next() {
		var roundNo int
		if err := rows.Scan(&roundNo); err != nil {
			return nil, err
		}
		out = append(out, roundNo)
	}
	return out, rows.Err()
}

func (s *Store) LatestOrderDate() (string, error) {
	var orderDate string
	err := s.db.QueryRow(`select coalesce(max(order_date), '') from rounds`).Scan(&orderDate)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(orderDate) == "" {
		err = s.db.QueryRow(`select coalesce(max(order_date), '') from orders`).Scan(&orderDate)
		if err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(orderDate), nil
}

func (s *Store) GetOrdersByID(id int64) (model.Order, error) {
	return s.GetOrder(id)
}

func (s *Store) FindOrderBySampleID(sampleID string) (model.Order, error) {
	var id int64
	err := s.db.QueryRow(`select id from orders where sample_id = ? order by id desc limit 1`, sampleID).Scan(&id)
	if err != nil {
		return model.Order{}, err
	}
	return s.GetOrder(id)
}

func (s *Store) EnsureImportedOrder(orderDate string, roundNo int, layoutKind, sampleID, fileID, patientID, patientName, sourceFile string) (model.Order, error) {
	orderDate = normalizeOrderDate(orderDate)
	rec, err := s.normalizeImportedRecord(orderDate, roundNo, layoutKind, model.ImportedRecord{
		SampleID:    sampleID,
		FileID:      fileID,
		PatientID:   patientID,
		PatientName: patientName,
	})
	if err != nil {
		return model.Order{}, err
	}
	return s.UpsertOrder(model.Order{
		RoundNo:      roundNo,
		OrderDate:    orderDate,
		SampleID:     defaultString(rec.SampleID, rec.FileID),
		FileID:       rec.FileID,
		PatientID:    rec.PatientID,
		PatientName:  rec.PatientName,
		RackNo:       rec.RackNo,
		RackPosition: rec.RackPosition,
		SampleNo:     rec.SampleNo,
		Status:       "received",
		SourceFile:   sourceFile,
	})
}

func (s *Store) DeleteOrder(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`delete from order_analysis_results where order_analysis_id in (select id from order_analyses where order_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`delete from order_analyses where order_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`delete from orders where id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RecordImportedResult(orderDate string, roundNo int, layoutKind string, rec model.ImportedRecord, sourceFile string) (model.Order, model.OrderAnalysis, model.OrderAnalysisResult, error) {
	orderDate = normalizeOrderDate(orderDate)
	rec, err := s.normalizeImportedRecord(orderDate, roundNo, layoutKind, rec)
	if err != nil {
		return model.Order{}, model.OrderAnalysis{}, model.OrderAnalysisResult{}, err
	}
	order, err := s.UpsertOrder(model.Order{
		RoundNo:      roundNo,
		OrderDate:    orderDate,
		SampleID:     defaultString(rec.SampleID, rec.FileID),
		FileID:       rec.FileID,
		PatientID:    rec.PatientID,
		PatientName:  rec.PatientName,
		RackNo:       rec.RackNo,
		RackPosition: rec.RackPosition,
		SampleNo:     rec.SampleNo,
		Status:       "received",
		SourceFile:   sourceFile,
	})
	if err != nil {
		return model.Order{}, model.OrderAnalysis{}, model.OrderAnalysisResult{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var analysisID int64
	analyteID := int64(0)
	if analyte, err := s.GetAnalyte(rec.AnalyteTag); err == nil {
		analyteID = analyte.ID
	}
	err = s.db.QueryRow(`select id from order_analyses where order_id = ? and analyte_tag = ? limit 1`, order.ID, rec.AnalyteTag).Scan(&analysisID)
	if err == sql.ErrNoRows {
		res, err := s.db.Exec(`insert into order_analyses(order_id,analyte_id,analyte_tag,analyte_name,status,requested_at,received_at) values(?,?,?,?,?,?,?)`,
			order.ID, analyteID, rec.AnalyteTag, rec.AnalyteName, "received", "", now)
		if err != nil {
			return model.Order{}, model.OrderAnalysis{}, model.OrderAnalysisResult{}, err
		}
		analysisID, _ = res.LastInsertId()
	} else if err != nil {
		return model.Order{}, model.OrderAnalysis{}, model.OrderAnalysisResult{}, err
	} else {
		_, err = s.db.Exec(`update order_analyses set analyte_id=?, analyte_name=?,status='received',received_at=? where id = ?`, analyteID, rec.AnalyteName, now, analysisID)
		if err != nil {
			return model.Order{}, model.OrderAnalysis{}, model.OrderAnalysisResult{}, err
		}
	}

	flagsJSON, _ := json.Marshal(metaOrEmpty(rec.Flags))
	res, err := s.db.Exec(`insert into order_analysis_results(order_analysis_id,result_value,raw_value,interpreted_value,unit,source_file,flags_json,created_at) values(?,?,?,?,?,?,?,?)`,
		analysisID, rec.ResultValue, rec.RawValue, rec.ResultValue, rec.Unit, sourceFile, string(flagsJSON), now)
	if err != nil {
		return model.Order{}, model.OrderAnalysis{}, model.OrderAnalysisResult{}, err
	}
	resultID, _ := res.LastInsertId()
	analysis, err := s.GetAnalysis(analysisID)
	if err != nil {
		return model.Order{}, model.OrderAnalysis{}, model.OrderAnalysisResult{}, err
	}
	result, err := s.GetResult(resultID)
	return order, analysis, result, err
}

func (s *Store) normalizeImportedRecord(orderDate string, roundNo int, layoutKind string, rec model.ImportedRecord) (model.ImportedRecord, error) {
	orderDate = normalizeOrderDate(orderDate)
	next, err := s.nextRackPosition(roundNo)
	if err != nil {
		return rec, err
	}
	nextSampleNo, err := s.nextSampleNo(orderDate)
	if err != nil {
		return rec, err
	}
	if layoutKind == "simple_list" {
		rec.RackNo = 1
		rec.RackPosition = 0
		if rec.SampleNo <= 0 {
			rec.SampleNo = nextSampleNo
		}
		rec.ListPosition = 0
		return rec, nil
	}
	if rec.RackNo <= 0 {
		rec.RackNo = 1
	}
	if rec.RackPosition <= 0 {
		rec.RackPosition = next
	}
	if rec.SampleNo <= 0 {
		rec.SampleNo = nextSampleNo
	}
	rec.ListPosition = 0
	return rec, nil
}

func (s *Store) nextRackPosition(roundNo int) (int, error) {
	var next int
	err := s.db.QueryRow(`select coalesce(max(rack_position), 0) + 1 from orders where round_no = ?`, roundNo).Scan(&next)
	if err != nil {
		return 0, err
	}
	if next <= 0 {
		next = 1
	}
	return next, nil
}

func (s *Store) nextSampleNo(orderDate string) (int, error) {
	orderDate = normalizeOrderDate(orderDate)
	var next int
	err := s.db.QueryRow(`select coalesce(max(sample_no), 0) + 1 from orders where order_date = ?`, orderDate).Scan(&next)
	if err != nil {
		return 0, err
	}
	if next <= 0 {
		next = 1
	}
	return next, nil
}

func normalizeOrderDate(orderDate string) string {
	orderDate = strings.TrimSpace(orderDate)
	if orderDate != "" {
		return orderDate
	}
	return time.Now().Format("2006-01-02")
}

func (s *Store) GetAnalysis(id int64) (model.OrderAnalysis, error) {
	var item model.OrderAnalysis
	var req, recv, flagsJSON string
	err := s.db.QueryRow(`select id,order_id,analyte_id,analyte_tag,analyte_name,status,requested_at,received_at,default_result_id,result_value,raw_value,interpreted_value,unit,source_file,flags_json from order_analyses where id = ?`, id).
		Scan(&item.ID, &item.OrderID, &item.AnalyteID, &item.AnalyteTag, &item.AnalyteName, &item.Status, &req, &recv, &item.DefaultResultID, &item.ResultValue, &item.RawValue, &item.Interpreted, &item.Unit, &item.SourceFile, &flagsJSON)
	if err != nil {
		return item, err
	}
	if item.AnalyteID > 0 {
		if analyte, err := s.GetAnalyte(item.AnalyteTag); err == nil {
			item.AnalyteDescription = analyte.Description
		}
	}
	_ = json.Unmarshal([]byte(flagsJSON), &item.Flags)
	if req != "" {
		item.RequestedAt, _ = time.Parse(time.RFC3339, req)
	}
	if recv != "" {
		item.ReceivedAt, _ = time.Parse(time.RFC3339, recv)
	}
	return item, nil
}

func (s *Store) ListAnalysesForOrder(orderID int64) ([]model.OrderAnalysis, error) {
	rows, err := s.db.Query(`select id from order_analyses where order_id = ? order by id asc`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.OrderAnalysis{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		item, err := s.GetAnalysis(id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpsertOrderAnalysis(item model.OrderAnalysis) (model.OrderAnalysis, error) {
	if item.OrderID <= 0 {
		return model.OrderAnalysis{}, errors.New("order_id is required")
	}
	item.AnalyteTag = strings.TrimSpace(item.AnalyteTag)
	if item.AnalyteTag == "" {
		return model.OrderAnalysis{}, errors.New("analyte_tag is required")
	}
	if item.Status == "" {
		item.Status = "scheduled"
	}
	analyteID := item.AnalyteID
	if analyteID <= 0 {
		if analyte, err := s.GetAnalyte(item.AnalyteTag); err == nil {
			analyteID = analyte.ID
			if strings.TrimSpace(item.AnalyteName) == "" {
				item.AnalyteName = analyte.Name
			}
		}
	}
	flagsJSON, _ := json.Marshal(metaOrEmpty(item.Flags))
	requestedAt := ""
	if !item.RequestedAt.IsZero() {
		requestedAt = item.RequestedAt.UTC().Format(time.RFC3339)
	}
	receivedAt := ""
	if !item.ReceivedAt.IsZero() {
		receivedAt = item.ReceivedAt.UTC().Format(time.RFC3339)
	}
	if item.ID > 0 {
		_, err := s.db.Exec(`update order_analyses
			set order_id=?, analyte_id=?, analyte_tag=?, analyte_name=?, status=?, requested_at=?, received_at=?, default_result_id=?, result_value=?, raw_value=?, interpreted_value=?, unit=?, source_file=?, flags_json=?
			where id = ?`,
			item.OrderID, analyteID, item.AnalyteTag, item.AnalyteName, item.Status, requestedAt, receivedAt, item.DefaultResultID, item.ResultValue, item.RawValue, item.Interpreted, item.Unit, item.SourceFile, string(flagsJSON), item.ID)
		if err != nil {
			return model.OrderAnalysis{}, err
		}
		return s.GetAnalysis(item.ID)
	}
	res, err := s.db.Exec(`insert into order_analyses(order_id, analyte_id, analyte_tag, analyte_name, status, requested_at, received_at, default_result_id, result_value, raw_value, interpreted_value, unit, source_file, flags_json)
		values(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		item.OrderID, analyteID, item.AnalyteTag, item.AnalyteName, item.Status, requestedAt, receivedAt, item.DefaultResultID, item.ResultValue, item.RawValue, item.Interpreted, item.Unit, item.SourceFile, string(flagsJSON))
	if err != nil {
		return model.OrderAnalysis{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetAnalysis(id)
}

func (s *Store) EnsureOrderAnalysis(orderID int64, analyteTag, analyteName, status string) (model.OrderAnalysis, error) {
	if analyteTag == "" {
		return model.OrderAnalysis{}, errors.New("analyte_tag is required")
	}
	if status == "" {
		status = "completed"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	analyteID := int64(0)
	if analyte, err := s.GetAnalyte(analyteTag); err == nil {
		analyteID = analyte.ID
	}
	var analysisID int64
	err := s.db.QueryRow(`select id from order_analyses where order_id = ? and analyte_tag = ? limit 1`, orderID, analyteTag).Scan(&analysisID)
	if err == sql.ErrNoRows {
		res, err := s.db.Exec(`insert into order_analyses(order_id,analyte_id,analyte_tag,analyte_name,status,requested_at,received_at) values(?,?,?,?,?,?,?)`,
			orderID, analyteID, analyteTag, analyteName, status, "", now)
		if err != nil {
			return model.OrderAnalysis{}, err
		}
		analysisID, _ = res.LastInsertId()
		return s.GetAnalysis(analysisID)
	}
	if err != nil {
		return model.OrderAnalysis{}, err
	}
	if _, err := s.db.Exec(`update order_analyses set analyte_id=?, analyte_name=?,status=?,received_at=? where id = ?`, analyteID, analyteName, status, now, analysisID); err != nil {
		return model.OrderAnalysis{}, err
	}
	return s.GetAnalysis(analysisID)
}

func (s *Store) DeleteOrderAnalysis(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`delete from order_analysis_results where order_analysis_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`delete from order_analyses where id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetResult(id int64) (model.OrderAnalysisResult, error) {
	var item model.OrderAnalysisResult
	var flagsJSON, created string
	err := s.db.QueryRow(`select id,order_analysis_id,result_value,raw_value,interpreted_value,unit,source_file,flags_json,created_at from order_analysis_results where id = ?`, id).
		Scan(&item.ID, &item.OrderAnalysisID, &item.ResultValue, &item.RawValue, &item.Interpreted, &item.Unit, &item.SourceFile, &flagsJSON, &created)
	if err != nil {
		return item, err
	}
	_ = json.Unmarshal([]byte(flagsJSON), &item.Flags)
	item.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return item, nil
}

func (s *Store) UpsertResultForAnalysis(orderAnalysisID int64, resultValue, rawValue, interpreted, unit, sourceFile string, flags map[string]interface{}) (model.OrderAnalysisResult, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	flagsJSON, _ := json.Marshal(metaOrEmpty(flags))
	var resultID int64
	err := s.db.QueryRow(`select id from order_analysis_results where order_analysis_id = ? order by id desc limit 1`, orderAnalysisID).Scan(&resultID)
	if err == sql.ErrNoRows {
		res, err := s.db.Exec(`insert into order_analysis_results(order_analysis_id,result_value,raw_value,interpreted_value,unit,source_file,flags_json,created_at) values(?,?,?,?,?,?,?,?)`,
			orderAnalysisID, resultValue, rawValue, interpreted, unit, sourceFile, string(flagsJSON), now)
		if err != nil {
			return model.OrderAnalysisResult{}, false, err
		}
		resultID, _ = res.LastInsertId()
		if err := s.SetDefaultResultForAnalysis(orderAnalysisID, resultID); err != nil {
			return model.OrderAnalysisResult{}, false, err
		}
		result, err := s.GetResult(resultID)
		return result, true, err
	}
	if err != nil {
		return model.OrderAnalysisResult{}, false, err
	}
	if _, err := s.db.Exec(`update order_analysis_results set result_value=?,raw_value=?,interpreted_value=?,unit=?,source_file=?,flags_json=?,created_at=? where id = ?`,
		resultValue, rawValue, interpreted, unit, sourceFile, string(flagsJSON), now, resultID); err != nil {
		return model.OrderAnalysisResult{}, false, err
	}
	var defaultResultID int64
	if err := s.db.QueryRow(`select default_result_id from order_analyses where id = ?`, orderAnalysisID).Scan(&defaultResultID); err == nil && defaultResultID == resultID {
		if err := s.SetDefaultResultForAnalysis(orderAnalysisID, resultID); err != nil {
			return model.OrderAnalysisResult{}, false, err
		}
	}
	result, err := s.GetResult(resultID)
	return result, false, err
}

func (s *Store) ListResultsForAnalysis(orderAnalysisID int64) ([]model.OrderAnalysisResult, error) {
	rows, err := s.db.Query(`select id from order_analysis_results where order_analysis_id = ? order by id desc`, orderAnalysisID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.OrderAnalysisResult{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		item, err := s.GetResult(id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SetDefaultResultForAnalysis(orderAnalysisID, resultID int64) error {
	result, err := s.GetResult(resultID)
	if err != nil {
		return err
	}
	if result.OrderAnalysisID != orderAnalysisID {
		return errors.New("result does not belong to analysis")
	}
	flagsJSON, _ := json.Marshal(metaOrEmpty(result.Flags))
	_, err = s.db.Exec(`update order_analyses set default_result_id=?, result_value=?, raw_value=?, interpreted_value=?, unit=?, source_file=?, flags_json=? where id = ?`,
		resultID, result.ResultValue, result.RawValue, result.Interpreted, result.Unit, result.SourceFile, string(flagsJSON), orderAnalysisID)
	return err
}

func (s *Store) ListResults(limit int) ([]model.OrderAnalysisResult, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`select id from order_analysis_results order by id desc limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.OrderAnalysisResult{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		item, err := s.GetResult(id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) TodayResultSummary() (int, int, error) {
	today := time.Now().Format("2006-01-02")
	var withResult int
	if err := s.db.QueryRow(`
		select count(a.id)
		from orders o
		join order_analyses a on a.order_id = o.id
		where substr(o.order_date,1,10) = ? and a.default_result_id > 0`, today).Scan(&withResult); err != nil {
		return 0, 0, err
	}
	var total int
	if err := s.db.QueryRow(`
		select count(a.id)
		from orders o
		join order_analyses a on a.order_id = o.id
		where substr(o.order_date,1,10) = ?`, today).Scan(&total); err != nil {
		return 0, 0, err
	}
	return total - withResult, withResult, nil
}

func (s *Store) DailySeries(limit int) ([]model.DashboardSeriesPoint, error) {
	if limit <= 0 {
		limit = 14
	}
	rows, err := s.db.Query(`
		with order_days as (
			select substr(order_date,1,10) as day, count(*) as orders
			from orders
			group by substr(order_date,1,10)
		),
		analysis_days as (
			select substr(o.order_date,1,10) as day,
			       count(a.id) as analyses,
			       sum(case when a.default_result_id > 0 then 1 else 0 end) as analyses_with_result
			from orders o
			left join order_analyses a on a.order_id = o.id
			group by substr(o.order_date,1,10)
		)
		select coalesce(o.day,a.day) as day,
		       coalesce(o.orders,0),
		       coalesce(a.analyses,0),
		       coalesce(a.analyses_with_result,0)
		from order_days o
		full join analysis_days a on a.day = o.day
		order by day desc
		limit ?`, limit)
	if err != nil {
		rows, err = s.db.Query(`
			select day, max(orders), max(analyses), max(analyses_with_result)
			from (
				select substr(order_date,1,10) as day, count(*) as orders, 0 as analyses, 0 as analyses_with_result
				from orders group by substr(order_date,1,10)
				union all
				select substr(o.order_date,1,10) as day, 0 as orders, count(a.id) as analyses, sum(case when a.default_result_id > 0 then 1 else 0 end) as analyses_with_result
				from orders o left join order_analyses a on a.order_id = o.id
				group by substr(o.order_date,1,10)
			)
			group by day
			order by day desc
			limit ?`, limit)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()
	out := []model.DashboardSeriesPoint{}
	for rows.Next() {
		var p model.DashboardSeriesPoint
		if err := rows.Scan(&p.Day, &p.Orders, &p.Analyses, &p.AnalysesWithResult); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

func (s *Store) StatsForDate(orderDate string) (map[string]interface{}, error) {
	orderDate = normalizeOrderDate(orderDate)
	stats := map[string]interface{}{}
	var analytes int
	if err := s.db.QueryRow(`select count(*) from analytes`).Scan(&analytes); err != nil {
		return nil, err
	}
	stats["analytes"] = analytes
	var events int
	if err := s.db.QueryRow(`select count(*) from event_logs`).Scan(&events); err != nil {
		return nil, err
	}
	stats["events"] = events
	var orders int
	if err := s.db.QueryRow(`select count(*) from orders where substr(order_date,1,10) = ?`, orderDate).Scan(&orders); err != nil {
		return nil, err
	}
	stats["orders"] = orders
	var results int
	if err := s.db.QueryRow(`
		select count(r.id)
		from orders o
		join order_analyses a on a.order_id = o.id
		join order_analysis_results r on r.order_analysis_id = a.id
		where substr(o.order_date,1,10) = ?`, orderDate).Scan(&results); err != nil {
		return nil, err
	}
	stats["results"] = results
	return stats, nil
}

func (s *Store) Stats() map[string]interface{} {
	stats := map[string]interface{}{}
	for key, query := range map[string]string{
		"analytes": "select count(*) from analytes",
		"orders":   "select count(*) from orders",
		"results":  "select count(*) from order_analysis_results",
		"events":   "select count(*) from event_logs",
	} {
		var n int
		_ = s.db.QueryRow(query).Scan(&n)
		stats[key] = n
	}
	return stats
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func metaOrEmpty(v map[string]interface{}) map[string]interface{} {
	if v == nil {
		return map[string]interface{}{}
	}
	return v
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func (s *Store) String() string {
	return fmt.Sprintf("sqlite-store")
}
