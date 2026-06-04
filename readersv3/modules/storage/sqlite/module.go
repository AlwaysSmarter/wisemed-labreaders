package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	coremodel "wisemed-labreaders/readersv3/modules/core/model"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct {
	rt     module.Runtime
	dbPath string
	store  *Store
}

type Store struct {
	db       *sql.DB
	jsonPath string
}

type sidecarData struct {
	Analytes  []coremodel.Analyte  `json:"analytes"`
	QCTargets []coremodel.QCTarget `json:"qc_targets"`
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "storage-sqlite" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	settings := rt.ModuleSettings(m.ID())
	if path, _ := settings["path"].(string); path != "" {
		m.dbPath = rt.ResolvePath(path)
	} else {
		m.dbPath = rt.ResolvePath("reader-v3.db")
	}
	store, err := Open(m.dbPath)
	if err != nil {
		return err
	}
	m.store = store
	rt.RegisterService("storage", store)
	rt.RegisterService("storage-meta", map[string]interface{}{
		"driver": "sqlite",
		"path":   m.dbPath,
	})
	rt.Handle("/api/storage/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"ok":true,"module":"storage-sqlite","driver":"sqlite"}`))
	}))
	return nil
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db, jsonPath: path + ".ui.json"}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, explainSQLiteOpenError(path, err)
	}
	if err := store.migrateSidecar(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func explainSQLiteOpenError(path string, err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(message, "disk i/o error") || strings.Contains(message, "(522)") {
		walPath := path + "-wal"
		shmPath := path + "-shm"
		walExists := fileExists(walPath)
		shmExists := fileExists(shmPath)
		if walExists || shmExists {
			hints := []string{}
			if walExists {
				hints = append(hints, filepath.Base(walPath))
			}
			if shmExists {
				hints = append(hints, filepath.Base(shmPath))
			}
			return fmt.Errorf("sqlite open failed: stale WAL/SHM sidecar files detected (%s). Stop the reader, delete these files, then start again", strings.Join(hints, ", "))
		}
		return fmt.Errorf("sqlite open failed with disk I/O error for %s. The database or its sidecar files may be corrupted", filepath.Base(path))
	}
	return err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *Store) init() error {
	stmts := []string{
		`pragma busy_timeout = 5000`,
		`pragma journal_mode = WAL`,
		`pragma synchronous = NORMAL`,
		`create table if not exists rounds (
			id integer primary key autoincrement,
			order_date text not null,
			round_no integer not null,
			created_at text not null,
			unique(order_date, round_no)
		)`,
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
			result_measure_unit text not null default '',
			result_reagents_set text not null default '',
			protocol_options_json text not null default '{}',
			created_at text not null,
			updated_at text not null
		)`,
		`create table if not exists orders (
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
			meta_json text not null default '{}',
			created_at text not null,
			updated_at text not null
		)`,
		`create table if not exists order_analyses (
			id integer primary key autoincrement,
			order_id integer not null,
			analyte_id integer not null default 0,
			analyte_tag text not null,
			analyte_name text not null default '',
			status text not null default 'new',
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
		`create table if not exists qc_targets (
			id integer primary key autoincrement,
			active integer not null default 1,
			analyte_tag text not null,
			analyte_name text not null default '',
			control_level text not null default '',
			lot_no text not null default '',
			expires_at text not null default '',
			unit text not null default '',
			target_mean real not null default 0,
			target_sd real not null default 0,
			target_cv real not null default 0,
			notes text not null default '',
			created_at text not null,
			updated_at text not null,
			unique(analyte_tag, control_level, lot_no)
		)`,
		`create table if not exists qc_records (
			id integer primary key autoincrement,
			round_no integer not null default 1,
			run_date text not null,
			control_label text not null,
			control_level text not null default '',
			lot_no text not null default '',
			diluent_info text not null default '',
			file_id text not null default '',
			status text not null default 'manual',
			source_file text not null default '',
			created_at text not null,
			updated_at text not null
		)`,
		`create table if not exists qc_analyses (
			id integer primary key autoincrement,
			qc_record_id integer not null,
			analyte_id integer not null default 0,
			analyte_tag text not null,
			analyte_name text not null default '',
			control_level text not null default '',
			lot_no text not null default '',
			status text not null default 'completed',
			default_result_id integer not null default 0,
			result_value text not null default '',
			raw_value text not null default '',
			interpreted_value text not null default '',
			numeric_value real,
			unit text not null default '',
			source_file text not null default '',
			flags_json text not null default '{}',
			manual_entered_by text not null default '',
			manual_entered_at text not null default '',
			created_at text not null
		)`,
		`create table if not exists daily_detail_definitions (
			id integer primary key autoincrement,
			key text not null unique,
			label text not null,
			scope text not null default 'day',
			field_type text not null default 'text',
			placeholder text not null default '',
			default_value text not null default '',
			options_json text not null default '[]',
			required integer not null default 0,
			active integer not null default 1,
			source text not null default 'user',
			sort_order integer not null default 0,
			meta_json text not null default '{}',
			created_at text not null,
			updated_at text not null
		)`,
		`create table if not exists daily_detail_values (
			id integer primary key autoincrement,
			definition_key text not null,
			scope_date text not null,
			round_no integer not null default 0,
			analyte_tag text not null default '',
			value_text text not null default '',
			meta_json text not null default '{}',
			created_at text not null,
			updated_at text not null,
			unique(definition_key, scope_date, round_no, analyte_tag)
		)`,
		`create index if not exists idx_rounds_date_round on rounds(order_date, round_no)`,
		`create index if not exists idx_orders_date_round on orders(order_date, round_no)`,
		`create index if not exists idx_order_analyses_order on order_analyses(order_id)`,
		`create index if not exists idx_order_results_analysis on order_analysis_results(order_analysis_id)`,
		`create index if not exists idx_qc_records_run_date on qc_records(run_date)`,
		`create index if not exists idx_qc_analyses_record on qc_analyses(qc_record_id)`,
		`create index if not exists idx_qc_targets_lookup on qc_targets(analyte_tag, control_level, lot_no)`,
		`create index if not exists idx_daily_detail_values_lookup on daily_detail_values(scope_date, round_no, analyte_tag)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`alter table qc_targets add column expires_at text not null default ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`alter table analytes add column protocol_options_json text not null default '{}'`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`alter table orders add column meta_json text not null default '{}'`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`alter table qc_records add column file_id text not null default ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`alter table qc_records add column source_file text not null default ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`alter table qc_analyses add column source_file text not null default ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`alter table qc_analyses add column control_level text not null default ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`alter table qc_analyses add column lot_no text not null default ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`alter table qc_analyses add column numeric_value real`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`alter table qc_analyses add column created_at text not null default ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`alter table qc_analyses add column flags_json text not null default '{}'`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`alter table qc_analyses add column manual_entered_by text not null default ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`alter table qc_analyses add column manual_entered_at text not null default ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`
		update qc_analyses
		set control_level = coalesce((select qr.control_level from qc_records qr where qr.id = qc_analyses.qc_record_id), ''),
		    lot_no = coalesce((select qr.lot_no from qc_records qr where qr.id = qc_analyses.qc_record_id), '')
		where control_level = '' or lot_no = ''
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`update qc_analyses set control_level = '' where control_level <> ''`); err != nil {
		return err
	}
	if hasOldQCResults, err := s.tableExists("qc_analysis_results"); err != nil {
		return err
	} else if hasOldQCResults {
		if _, err := s.db.Exec(`
			update qc_analyses
			set numeric_value = (
				select qar.numeric_value
				from qc_analysis_results qar
				where qar.qc_analysis_id = qc_analyses.id
				order by qar.created_at desc, qar.id desc
				limit 1
			)
			where numeric_value is null
		`); err != nil {
			return err
		}
		if _, err := s.db.Exec(`
			update qc_analyses
			set created_at = coalesce((
				select qar.created_at
				from qc_analysis_results qar
				where qar.qc_analysis_id = qc_analyses.id
				order by qar.created_at desc, qar.id desc
				limit 1
			), '')
			where created_at = ''
		`); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`
		update qc_analyses
		set created_at = coalesce((
			select qr.created_at
			from qc_records qr
			where qr.id = qc_analyses.qc_record_id
		), ?)
		where created_at = ''
	`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return nil
}

func (s *Store) migrateSidecar() error {
	blob, err := os.ReadFile(s.jsonPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(blob) == 0 {
		return nil
	}
	var data sidecarData
	if err := json.Unmarshal(blob, &data); err != nil {
		return err
	}
	if hasAnalytes, err := s.tableExists("analytes"); err != nil {
		return err
	} else if hasAnalytes {
		for _, item := range data.Analytes {
			if _, err := s.SaveAnalyte(item); err != nil {
				return err
			}
		}
	}
	if hasTargets, err := s.tableExists("qc_targets"); err != nil {
		return err
	} else if hasTargets {
		for _, item := range data.QCTargets {
			if _, err := s.SaveQCTarget(item); err != nil {
				return err
			}
		}
	}
	backupPath := s.jsonPath + ".migrated"
	_ = os.Rename(s.jsonPath, backupPath)
	return nil
}

func (s *Store) tableExists(name string) (bool, error) {
	var count int
	if err := s.db.QueryRow(`select count(1) from sqlite_master where type = 'table' and name = ?`, name).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) ListAnalytes() ([]coremodel.Analyte, error) {
	rows, err := s.db.Query(`select id,active,tag,code,name,description,result_type,result_formatting,result_weighting,result_measure_unit,result_reagents_set,protocol_options_json,created_at,updated_at from analytes order by name asc, tag asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []coremodel.Analyte{}
	for rows.Next() {
		var item coremodel.Analyte
		var active int
		var created, updated string
		var protocolOptionsJSON string
		if err := rows.Scan(&item.ID, &active, &item.Tag, &item.Code, &item.Name, &item.Description, &item.ResultType, &item.ResultFormatting, &item.ResultWeighting, &item.ResultMeasureUnit, &item.ResultReagentsSet, &protocolOptionsJSON, &created, &updated); err != nil {
			return nil, err
		}
		item.Active = active == 1
		_ = json.Unmarshal([]byte(protocolOptionsJSON), &item.ProtocolOptions)
		item.CreatedAt, _ = time.Parse(time.RFC3339, created)
		item.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetAnalyteByID(id int64) (coremodel.Analyte, error) {
	var item coremodel.Analyte
	var active int
	var created, updated string
	var protocolOptionsJSON string
	err := s.db.QueryRow(`select id,active,tag,code,name,description,result_type,result_formatting,result_weighting,result_measure_unit,result_reagents_set,protocol_options_json,created_at,updated_at from analytes where id = ?`, id).
		Scan(&item.ID, &active, &item.Tag, &item.Code, &item.Name, &item.Description, &item.ResultType, &item.ResultFormatting, &item.ResultWeighting, &item.ResultMeasureUnit, &item.ResultReagentsSet, &protocolOptionsJSON, &created, &updated)
	if err != nil {
		return coremodel.Analyte{}, err
	}
	item.Active = active == 1
	_ = json.Unmarshal([]byte(protocolOptionsJSON), &item.ProtocolOptions)
	item.CreatedAt, _ = time.Parse(time.RFC3339, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return item, nil
}

func (s *Store) SaveAnalyte(item coremodel.Analyte) (coremodel.Analyte, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	item.Tag = strings.TrimSpace(item.Tag)
	item.Name = strings.TrimSpace(item.Name)
	if item.Tag == "" || item.Name == "" {
		return coremodel.Analyte{}, errors.New("analyte tag and name are required")
	}
	protocolOptionsJSON, _ := json.Marshal(metaOrEmpty(item.ProtocolOptions))
	var existingID int64
	err := s.db.QueryRow(`select id from analytes where tag = ? limit 1`, item.Tag).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return coremodel.Analyte{}, err
	}
	if item.ID <= 0 && existingID > 0 {
		item.ID = existingID
	}
	if item.ID > 0 {
		if existingID > 0 && existingID != item.ID {
			return coremodel.Analyte{}, errors.New("analyte tag must be unique")
		}
		res, err := s.db.Exec(`update analytes set active=?, tag=?, code=?, name=?, description=?, result_type=?, result_formatting=?, result_weighting=?, result_measure_unit=?, result_reagents_set=?, protocol_options_json=?, updated_at=? where id = ?`,
			boolToInt(item.Active), item.Tag, item.Code, item.Name, item.Description, item.ResultType, item.ResultFormatting, item.ResultWeighting, item.ResultMeasureUnit, item.ResultReagentsSet, string(protocolOptionsJSON), now, item.ID)
		if err != nil {
			return coremodel.Analyte{}, err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return coremodel.Analyte{}, sql.ErrNoRows
		}
		return s.GetAnalyteByID(item.ID)
	}
	res, err := s.db.Exec(`insert into analytes(active,tag,code,name,description,result_type,result_formatting,result_weighting,result_measure_unit,result_reagents_set,protocol_options_json,created_at,updated_at) values(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		boolToInt(item.Active), item.Tag, item.Code, item.Name, item.Description, item.ResultType, item.ResultFormatting, item.ResultWeighting, item.ResultMeasureUnit, item.ResultReagentsSet, string(protocolOptionsJSON), now, now)
	if err != nil {
		return coremodel.Analyte{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetAnalyteByID(id)
}

func (s *Store) DeleteAnalyte(id int64) error {
	_, err := s.db.Exec(`delete from analytes where id = ?`, id)
	return err
}

func (s *Store) ListQCTargets() ([]coremodel.QCTarget, error) {
	rows, err := s.db.Query(`select id from qc_targets order by analyte_tag asc, control_level asc, lot_no asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []coremodel.QCTarget{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		item, err := s.GetQCTarget(id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetQCTarget(id int64) (coremodel.QCTarget, error) {
	var item coremodel.QCTarget
	var active int
	var created, updated string
	err := s.db.QueryRow(`select id,active,analyte_tag,analyte_name,control_level,lot_no,expires_at,unit,target_mean,target_sd,target_cv,notes,created_at,updated_at from qc_targets where id = ?`, id).
		Scan(&item.ID, &active, &item.AnalyteTag, &item.AnalyteName, &item.ControlLevel, &item.LotNo, &item.ExpiresAt, &item.Unit, &item.TargetMean, &item.TargetSD, &item.TargetCV, &item.Notes, &created, &updated)
	if err != nil {
		return coremodel.QCTarget{}, err
	}
	item.Active = active == 1
	item.CreatedAt, _ = time.Parse(time.RFC3339, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return item, nil
}

func (s *Store) SaveQCTarget(item coremodel.QCTarget) (coremodel.QCTarget, error) {
	item.AnalyteTag = strings.TrimSpace(item.AnalyteTag)
	if item.AnalyteTag == "" {
		return coremodel.QCTarget{}, errors.New("analyte_tag is required")
	}
	item.LotNo = strings.TrimSpace(item.LotNo)
	if item.LotNo == "" {
		item.LotNo = "-"
	}
	if item.TargetCV == 0 && item.TargetMean != 0 && item.TargetSD != 0 {
		item.TargetCV = abs(item.TargetSD/item.TargetMean) * 100
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var existingID int64
	err := s.db.QueryRow(`select id from qc_targets where analyte_tag = ? and control_level = ? and lot_no = ? limit 1`, item.AnalyteTag, item.ControlLevel, item.LotNo).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return coremodel.QCTarget{}, err
	}
	if item.ID > 0 {
		if existingID > 0 && existingID != item.ID {
			return coremodel.QCTarget{}, errors.New("qc target must be unique for analyte_tag/control_level/lot_no")
		}
		_, err := s.db.Exec(`update qc_targets set active=?,analyte_tag=?,analyte_name=?,control_level=?,lot_no=?,expires_at=?,unit=?,target_mean=?,target_sd=?,target_cv=?,notes=?,updated_at=? where id = ?`,
			boolToInt(item.Active), item.AnalyteTag, item.AnalyteName, item.ControlLevel, item.LotNo, item.ExpiresAt, item.Unit, item.TargetMean, item.TargetSD, item.TargetCV, item.Notes, now, item.ID)
		if err != nil {
			return coremodel.QCTarget{}, err
		}
		return s.GetQCTarget(item.ID)
	}
	if existingID > 0 {
		item.ID = existingID
		return s.SaveQCTarget(item)
	}
	res, err := s.db.Exec(`insert into qc_targets(active,analyte_tag,analyte_name,control_level,lot_no,expires_at,unit,target_mean,target_sd,target_cv,notes,created_at,updated_at) values(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		boolToInt(item.Active), item.AnalyteTag, item.AnalyteName, item.ControlLevel, item.LotNo, item.ExpiresAt, item.Unit, item.TargetMean, item.TargetSD, item.TargetCV, item.Notes, now, now)
	if err != nil {
		return coremodel.QCTarget{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetQCTarget(id)
}

func (s *Store) DeleteQCTarget(id int64) error {
	_, err := s.db.Exec(`delete from qc_targets where id = ?`, id)
	return err
}

func (s *Store) ListQCRecordBundles(runDate string) ([]coremodel.QCRecordBundle, error) {
	if strings.TrimSpace(runDate) == "" {
		runDate = time.Now().Format("2006-01-02")
	}
	return s.ListQCRecordBundlesRange(runDate, runDate)
}

func (s *Store) ListQCRecordBundlesRange(dateFrom, dateTo string) ([]coremodel.QCRecordBundle, error) {
	dateFrom = normalizeDate(dateFrom)
	dateTo = normalizeDate(dateTo)
	if dateFrom == "" {
		dateFrom = time.Now().Format("2006-01-02")
	}
	if dateTo == "" {
		dateTo = dateFrom
	}
	if dateFrom > dateTo {
		dateFrom, dateTo = dateTo, dateFrom
	}
	rows, err := s.db.Query(`
		select qr.id,qr.round_no,qr.run_date,qr.control_label,qr.control_level,qr.lot_no,
		       qr.diluent_info,qr.file_id,qr.status,qr.source_file,qr.created_at,qr.updated_at
		from qc_records qr
		where qr.run_date between ? and ?
		order by qr.run_date desc, qr.created_at desc, qr.id desc`, dateFrom, dateTo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []coremodel.QCRecordBundle{}
	for rows.Next() {
		var record coremodel.QCRecord
		var created, updated string
		if err := rows.Scan(&record.ID, &record.RoundNo, &record.RunDate, &record.ControlLabel, &record.ControlLevel, &record.LotNo, &record.DiluentInfo, &record.FileID, &record.Status, &record.SourceFile, &created, &updated); err != nil {
			return nil, err
		}
		record.CreatedAt, _ = time.Parse(time.RFC3339, created)
		record.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		analyses, err := s.listQCAnalyses(record.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, coremodel.QCRecordBundle{Record: record, Analyses: analyses})
	}
	return out, rows.Err()
}

func (s *Store) listQCAnalyses(recordID int64) ([]coremodel.QCAnalysis, error) {
	rows, err := s.db.Query(`
		select qa.id,qa.qc_record_id,qa.analyte_id,qa.analyte_tag,qa.analyte_name,
		       coalesce(qt.control_level, qr.control_level, qa.control_level) as control_level,
		       coalesce(qa.lot_no, qr.lot_no) as lot_no,
		       qa.status,qa.default_result_id,qa.result_value,qa.raw_value,qa.interpreted_value,qa.numeric_value,qa.unit,qa.source_file,qa.flags_json,qa.created_at
		from qc_analyses qa
		left join qc_records qr on qr.id = qa.qc_record_id
		left join qc_targets qt on qt.active = 1 and qt.analyte_tag = qa.analyte_tag and qt.lot_no = qa.lot_no
		where qa.qc_record_id = ?
		order by qa.created_at desc, qa.id desc`, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []coremodel.QCAnalysis{}
	for rows.Next() {
		var item coremodel.QCAnalysis
		var flagsJSON, created string
		var numeric sql.NullFloat64
		if err := rows.Scan(&item.ID, &item.QCRecordID, &item.AnalyteID, &item.AnalyteTag, &item.AnalyteName, &item.ControlLevel, &item.LotNo, &item.Status, &item.DefaultResultID, &item.ResultValue, &item.RawValue, &item.Interpreted, &numeric, &item.Unit, &item.SourceFile, &flagsJSON, &created); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(flagsJSON), &item.Flags)
		if numeric.Valid {
			value := numeric.Float64
			item.NumericValue = &value
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SaveManualQCRecord(runDate string, analysis coremodel.QCAnalysis, actor string, enteredAt time.Time) error {
	runDate = normalizeDate(runDate)
	controlLabel, _ := analysis.Meta["control_label"].(string)
	controlLevel, _ := analysis.Meta["control_level"].(string)
	lotNo, _ := analysis.Meta["lot_no"].(string)
	if strings.TrimSpace(lotNo) == "" {
		lotNo = strings.TrimSpace(controlLabel)
	}
	if strings.TrimSpace(controlLevel) == "" {
		controlLevel = "QC"
	}
	enteredAt = enteredAt.UTC()
	if analysis.Flags == nil {
		analysis.Flags = map[string]interface{}{}
	}
	analysis.Flags["manual_entry"] = true
	analysis.Flags["manual_entered_by"] = strings.TrimSpace(actor)
	analysis.Flags["manual_entered_at"] = enteredAt.Format(time.RFC3339)
	record, err := s.UpsertQCRecord(coremodel.QCRecord{
		RunDate:      runDate,
		RoundNo:      1,
		ControlLabel: controlLabel,
		ControlLevel: controlLevel,
		LotNo:        lotNo,
		Status:       "manual",
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(analysis.AnalyteTag) != "" {
		if _, err := s.findQCTarget(analysis.AnalyteTag, controlLevel, defaultString(lotNo, "-")); err != nil {
			if _, err := s.SaveQCTarget(coremodel.QCTarget{
				Active:       true,
				AnalyteTag:   analysis.AnalyteTag,
				AnalyteName:  analysis.AnalyteName,
				ControlLevel: controlLevel,
				LotNo:        defaultString(lotNo, "-"),
				Unit:         analysis.Unit,
				TargetMean:   0,
				TargetSD:     0,
				TargetCV:     0,
				Notes:        "Creat automat din inregistrare QC manuala. Definiti media si 1SD in Setari QC.",
			}); err != nil {
				return err
			}
		}
	}
	_, err = s.UpsertQCAnalysis(coremodel.QCAnalysis{
		QCRecordID:   record.ID,
		AnalyteTag:   analysis.AnalyteTag,
		AnalyteName:  analysis.AnalyteName,
		ControlLevel: controlLevel,
		LotNo:        defaultString(lotNo, "-"),
		Status:       "completed",
		ResultValue:  analysis.ResultValue,
		RawValue:     analysis.RawValue,
		Interpreted:  analysis.Interpreted,
		Unit:         analysis.Unit,
		SourceFile:   "manual",
		Flags:        analysis.Flags,
		Meta:         analysis.Meta,
	})
	return err
}

func (s *Store) CurrentRoundNo(orderDate string) (int, error) {
	orderDate = normalizeDate(orderDate)
	var roundNo int
	if err := s.db.QueryRow(`select coalesce(max(round_no), 0) from rounds where order_date = ?`, orderDate).Scan(&roundNo); err != nil {
		return 0, err
	}
	if roundNo <= 0 {
		if _, err := s.db.Exec(`insert or ignore into rounds(order_date, round_no, created_at) values(?,?,?)`, orderDate, 1, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return 0, err
		}
		roundNo = 1
	}
	return roundNo, nil
}

func (s *Store) ListRoundNumbers(orderDate string) ([]int, error) {
	orderDate = normalizeDate(orderDate)
	if _, err := s.CurrentRoundNo(orderDate); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`select distinct round_no from rounds where order_date = ? order by round_no asc`, orderDate)
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

func (s *Store) CreateNextRound(orderDate string) (int, error) {
	orderDate = normalizeDate(orderDate)
	var roundNo int
	if err := s.db.QueryRow(`select coalesce(max(round_no), 0) + 1 from rounds where order_date = ?`, orderDate).Scan(&roundNo); err != nil {
		return 0, err
	}
	if roundNo <= 0 {
		roundNo = 1
	}
	if _, err := s.db.Exec(`insert into rounds(order_date, round_no, created_at) values(?,?,?)`, orderDate, roundNo, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return 0, err
	}
	return roundNo, nil
}

func (s *Store) UpsertOrder(item coremodel.Order) (coremodel.Order, error) {
	item.OrderDate = normalizeDate(item.OrderDate)
	if item.RoundNo <= 0 {
		roundNo, err := s.CurrentRoundNo(item.OrderDate)
		if err != nil {
			return coremodel.Order{}, err
		}
		item.RoundNo = roundNo
	}
	item.SampleID = strings.TrimSpace(item.SampleID)
	if item.SampleID == "" {
		return coremodel.Order{}, errors.New("sample_id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	metaJSON, _ := json.Marshal(metaOrEmpty(item.Meta))
	if _, err := s.db.Exec(`insert or ignore into rounds(order_date, round_no, created_at) values(?,?,?)`, item.OrderDate, item.RoundNo, now); err != nil {
		return coremodel.Order{}, err
	}
	var existingID int64
	err := s.db.QueryRow(`select id from orders where order_date = ? and round_no = ? and sample_id = ? limit 1`, item.OrderDate, item.RoundNo, item.SampleID).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return coremodel.Order{}, err
	}
	if item.ID > 0 {
		existingID = item.ID
	}
	if existingID > 0 {
		_, err = s.db.Exec(`update orders set round_no=?,order_date=?,sample_id=?,file_id=?,patient_id=?,patient_name=?,rack_no=?,rack_position=?,list_position=?,sample_no=?,status=?,source_file=?,meta_json=?,updated_at=? where id = ?`,
			item.RoundNo, item.OrderDate, item.SampleID, item.FileID, item.PatientID, item.PatientName, item.RackNo, item.RackPosition, item.ListPosition, item.SampleNo, defaultString(item.Status, "received"), item.SourceFile, string(metaJSON), now, existingID)
		if err != nil {
			return coremodel.Order{}, err
		}
		return s.GetOrder(existingID)
	}
	res, err := s.db.Exec(`insert into orders(round_no,order_date,sample_id,file_id,patient_id,patient_name,rack_no,rack_position,list_position,sample_no,status,source_file,meta_json,created_at,updated_at) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		item.RoundNo, item.OrderDate, item.SampleID, item.FileID, item.PatientID, item.PatientName, item.RackNo, item.RackPosition, item.ListPosition, item.SampleNo, defaultString(item.Status, "received"), item.SourceFile, string(metaJSON), now, now)
	if err != nil {
		return coremodel.Order{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetOrder(id)
}

func (s *Store) GetOrder(id int64) (coremodel.Order, error) {
	var item coremodel.Order
	var created, updated, metaJSON string
	err := s.db.QueryRow(`select id,round_no,order_date,sample_id,file_id,patient_id,patient_name,rack_no,rack_position,list_position,sample_no,status,source_file,meta_json,created_at,updated_at from orders where id = ?`, id).
		Scan(&item.ID, &item.RoundNo, &item.OrderDate, &item.SampleID, &item.FileID, &item.PatientID, &item.PatientName, &item.RackNo, &item.RackPosition, &item.ListPosition, &item.SampleNo, &item.Status, &item.SourceFile, &metaJSON, &created, &updated)
	if err != nil {
		return coremodel.Order{}, err
	}
	_ = json.Unmarshal([]byte(metaJSON), &item.Meta)
	item.CreatedAt, _ = time.Parse(time.RFC3339, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return item, nil
}

func (s *Store) ListOrders(roundNo int, orderDate string) ([]coremodel.Order, error) {
	orderDate = normalizeDate(orderDate)
	query := `select id from orders where order_date = ?`
	args := []interface{}{orderDate}
	if roundNo > 0 {
		query += ` and round_no = ?`
		args = append(args, roundNo)
	}
	query += ` order by id desc`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []coremodel.Order{}
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

func (s *Store) GetOrderAnalysis(id int64) (coremodel.OrderAnalysis, error) {
	var item coremodel.OrderAnalysis
	var flagsJSON string
	err := s.db.QueryRow(`select id,order_id,analyte_id,analyte_tag,analyte_name,status,default_result_id,result_value,raw_value,interpreted_value,unit,source_file,flags_json from order_analyses where id = ?`, id).
		Scan(&item.ID, &item.OrderID, &item.AnalyteID, &item.AnalyteTag, &item.AnalyteName, &item.Status, &item.DefaultResultID, &item.ResultValue, &item.RawValue, &item.Interpreted, &item.Unit, &item.SourceFile, &flagsJSON)
	if err != nil {
		return coremodel.OrderAnalysis{}, err
	}
	_ = json.Unmarshal([]byte(flagsJSON), &item.Flags)
	return item, nil
}

func (s *Store) ListOrderAnalyses(orderID int64) ([]coremodel.OrderAnalysis, error) {
	rows, err := s.db.Query(`select id from order_analyses where order_id = ? order by id asc`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []coremodel.OrderAnalysis{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		item, err := s.GetOrderAnalysis(id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SaveOrderAnalysis(item coremodel.OrderAnalysis) (coremodel.OrderAnalysis, error) {
	if item.OrderID <= 0 {
		return coremodel.OrderAnalysis{}, errors.New("order_id is required")
	}
	item.AnalyteTag = strings.TrimSpace(item.AnalyteTag)
	if item.AnalyteTag == "" {
		return coremodel.OrderAnalysis{}, errors.New("analyte_tag is required")
	}
	if item.Status == "" {
		item.Status = "completed"
	}
	if item.AnalyteID <= 0 {
		var analyteID int64
		_ = s.db.QueryRow(`select id from analytes where tag = ? limit 1`, item.AnalyteTag).Scan(&analyteID)
		item.AnalyteID = analyteID
	}
	flagsJSON, _ := json.Marshal(metaOrEmpty(item.Flags))
	var existingID int64
	err := s.db.QueryRow(`select id from order_analyses where order_id = ? and analyte_tag = ? limit 1`, item.OrderID, item.AnalyteTag).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return coremodel.OrderAnalysis{}, err
	}
	if item.ID > 0 {
		existingID = item.ID
	}
	if existingID > 0 {
		_, err = s.db.Exec(`update order_analyses set analyte_id=?,analyte_tag=?,analyte_name=?,status=?,default_result_id=?,result_value=?,raw_value=?,interpreted_value=?,unit=?,source_file=?,flags_json=? where id = ?`,
			item.AnalyteID, item.AnalyteTag, item.AnalyteName, item.Status, item.DefaultResultID, item.ResultValue, item.RawValue, item.Interpreted, item.Unit, item.SourceFile, string(flagsJSON), existingID)
		if err != nil {
			return coremodel.OrderAnalysis{}, err
		}
		return s.GetOrderAnalysis(existingID)
	}
	res, err := s.db.Exec(`insert into order_analyses(order_id,analyte_id,analyte_tag,analyte_name,status,default_result_id,result_value,raw_value,interpreted_value,unit,source_file,flags_json) values(?,?,?,?,?,?,?,?,?,?,?,?)`,
		item.OrderID, item.AnalyteID, item.AnalyteTag, item.AnalyteName, item.Status, item.DefaultResultID, item.ResultValue, item.RawValue, item.Interpreted, item.Unit, item.SourceFile, string(flagsJSON))
	if err != nil {
		return coremodel.OrderAnalysis{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetOrderAnalysis(id)
}

func (s *Store) DeleteOrderAnalysis(id int64) error {
	if _, err := s.db.Exec(`delete from order_analysis_results where order_analysis_id = ?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`delete from order_analyses where id = ?`, id)
	return err
}

func (s *Store) GetOrderAnalysisResult(id int64) (coremodel.OrderAnalysisResult, error) {
	var item coremodel.OrderAnalysisResult
	var flagsJSON, created string
	err := s.db.QueryRow(`select id,order_analysis_id,result_value,raw_value,interpreted_value,unit,source_file,flags_json,created_at from order_analysis_results where id = ?`, id).
		Scan(&item.ID, &item.OrderAnalysisID, &item.ResultValue, &item.RawValue, &item.Interpreted, &item.Unit, &item.SourceFile, &flagsJSON, &created)
	if err != nil {
		return coremodel.OrderAnalysisResult{}, err
	}
	_ = json.Unmarshal([]byte(flagsJSON), &item.Flags)
	item.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return item, nil
}

func (s *Store) ListResultsForAnalysis(orderAnalysisID int64) ([]coremodel.OrderAnalysisResult, error) {
	rows, err := s.db.Query(`select id from order_analysis_results where order_analysis_id = ? order by id desc`, orderAnalysisID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []coremodel.OrderAnalysisResult{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		item, err := s.GetOrderAnalysisResult(id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpsertOrderResultForAnalysis(orderAnalysisID int64, resultValue, rawValue, interpreted, unit, sourceFile string, flags map[string]interface{}) (coremodel.OrderAnalysisResult, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	flagsJSON, _ := json.Marshal(metaOrEmpty(flags))
	res, err := s.db.Exec(`insert into order_analysis_results(order_analysis_id,result_value,raw_value,interpreted_value,unit,source_file,flags_json,created_at) values(?,?,?,?,?,?,?,?)`,
		orderAnalysisID, resultValue, rawValue, interpreted, unit, sourceFile, string(flagsJSON), now)
	if err != nil {
		return coremodel.OrderAnalysisResult{}, false, err
	}
	resultID, _ := res.LastInsertId()
	if err := s.SetDefaultResult(orderAnalysisID, resultID, "individual"); err != nil {
		return coremodel.OrderAnalysisResult{}, false, err
	}
	result, err := s.GetOrderAnalysisResult(resultID)
	return result, true, err
}

func (s *Store) SetDefaultResult(orderAnalysisID, resultID int64, repeatMode string) error {
	result, err := s.GetOrderAnalysisResult(resultID)
	if err != nil {
		return err
	}
	if result.OrderAnalysisID != orderAnalysisID {
		return errors.New("result does not belong to order analysis")
	}
	if normalizeRepeatModeForResults(repeatMode) != "grouped" {
		return s.applyDefaultOrderResult(orderAnalysisID, resultID, result)
	}
	analysis, err := s.GetOrderAnalysis(orderAnalysisID)
	if err != nil {
		return err
	}
	order, err := s.GetOrder(analysis.OrderID)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(`select oar.id, oar.order_analysis_id
		from order_analysis_results oar
		join order_analyses oa on oa.id = oar.order_analysis_id
		join orders o on o.id = oa.order_id
		where o.sample_id = ? and o.order_date = ? and oar.created_at = ?
		order by oar.order_analysis_id asc, oar.id asc`, order.SampleID, order.OrderDate, result.CreatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	defer rows.Close()
	type selection struct {
		resultID        int64
		orderAnalysisID int64
	}
	selected := []selection{}
	for rows.Next() {
		var item selection
		if err := rows.Scan(&item.resultID, &item.orderAnalysisID); err != nil {
			return err
		}
		selected = append(selected, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(selected) == 0 {
		return s.applyDefaultOrderResult(orderAnalysisID, resultID, result)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, item := range selected {
		targetResult, getErr := s.GetOrderAnalysisResult(item.resultID)
		if getErr != nil {
			err = getErr
			return err
		}
		flagsJSON, _ := json.Marshal(metaOrEmpty(targetResult.Flags))
		if _, execErr := tx.Exec(`update order_analyses set default_result_id=?,result_value=?,raw_value=?,interpreted_value=?,unit=?,source_file=?,flags_json=? where id = ?`,
			item.resultID, targetResult.ResultValue, targetResult.RawValue, targetResult.Interpreted, targetResult.Unit, targetResult.SourceFile, string(flagsJSON), item.orderAnalysisID); execErr != nil {
			err = execErr
			return err
		}
	}
	err = tx.Commit()
	return err
}

func (s *Store) applyDefaultOrderResult(orderAnalysisID, resultID int64, result coremodel.OrderAnalysisResult) error {
	flagsJSON, _ := json.Marshal(metaOrEmpty(result.Flags))
	_, err := s.db.Exec(`update order_analyses set default_result_id=?,result_value=?,raw_value=?,interpreted_value=?,unit=?,source_file=?,flags_json=? where id = ?`,
		resultID, result.ResultValue, result.RawValue, result.Interpreted, result.Unit, result.SourceFile, string(flagsJSON), orderAnalysisID)
	return err
}

func normalizeRepeatModeForResults(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "grouped", "grupat", "group", "batch":
		return "grouped"
	default:
		return "individual"
	}
}

func (s *Store) ListOrderBundles(roundNo int, orderDate string) ([]coremodel.OrderBundle, error) {
	orders, err := s.ListOrders(roundNo, orderDate)
	if err != nil {
		return nil, err
	}
	out := make([]coremodel.OrderBundle, 0, len(orders))
	for _, order := range orders {
		analyses, err := s.ListOrderAnalyses(order.ID)
		if err != nil {
			return nil, err
		}
		bundle := coremodel.OrderBundle{Order: order, Analyses: make([]coremodel.OrderAnalysisBundle, 0, len(analyses))}
		for _, analysis := range analyses {
			results, err := s.ListResultsForAnalysis(analysis.ID)
			if err != nil {
				return nil, err
			}
			bundle.Analyses = append(bundle.Analyses, coremodel.OrderAnalysisBundle{
				Analysis: analysis,
				Results:  results,
			})
		}
		out = append(out, bundle)
	}
	return out, nil
}

func (s *Store) RecordImportedResult(orderDate string, roundNo int, rec coremodel.ImportedRecord, sourceFile string) (coremodel.Order, coremodel.OrderAnalysis, coremodel.OrderAnalysisResult, error) {
	order, err := s.UpsertOrder(coremodel.Order{
		RoundNo:      roundNo,
		OrderDate:    orderDate,
		SampleID:     defaultString(rec.SampleID, rec.FileID),
		FileID:       rec.FileID,
		PatientID:    rec.PatientID,
		PatientName:  rec.PatientName,
		RackNo:       rec.RackNo,
		RackPosition: rec.RackPosition,
		ListPosition: rec.ListPosition,
		SampleNo:     rec.SampleNo,
		Status:       "received",
		SourceFile:   sourceFile,
		Meta:         metaOrEmpty(rec.Meta),
	})
	if err != nil {
		return coremodel.Order{}, coremodel.OrderAnalysis{}, coremodel.OrderAnalysisResult{}, err
	}
	analysis, err := s.SaveOrderAnalysis(coremodel.OrderAnalysis{
		OrderID:     order.ID,
		AnalyteTag:  rec.AnalyteTag,
		AnalyteName: rec.AnalyteName,
		Status:      "completed",
		ResultValue: rec.ResultValue,
		RawValue:    rec.RawValue,
		Interpreted: rec.Interpreted,
		Unit:        rec.Unit,
		SourceFile:  sourceFile,
		Flags:       rec.Flags,
	})
	if err != nil {
		return coremodel.Order{}, coremodel.OrderAnalysis{}, coremodel.OrderAnalysisResult{}, err
	}
	result, _, err := s.UpsertOrderResultForAnalysis(analysis.ID, rec.ResultValue, rec.RawValue, rec.Interpreted, rec.Unit, sourceFile, rec.Flags)
	return order, analysis, result, err
}

func (s *Store) CurrentQCRoundNo(runDate string) (int, error) {
	return 1, nil
}

func (s *Store) ListQCRecords(roundNo int, runDate string) ([]coremodel.QCRecord, error) {
	runDate = normalizeDate(runDate)
	query := `select id,round_no,run_date,control_label,control_level,lot_no,diluent_info,file_id,status,source_file,created_at,updated_at from qc_records where run_date = ?`
	args := []interface{}{runDate}
	if roundNo > 0 {
		query += ` and round_no = ?`
		args = append(args, roundNo)
	}
	query += ` order by id asc`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []coremodel.QCRecord{}
	for rows.Next() {
		var item coremodel.QCRecord
		var created, updated string
		if err := rows.Scan(&item.ID, &item.RoundNo, &item.RunDate, &item.ControlLabel, &item.ControlLevel, &item.LotNo, &item.DiluentInfo, &item.FileID, &item.Status, &item.SourceFile, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339, created)
		item.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetQCRecord(id int64) (coremodel.QCRecord, error) {
	var item coremodel.QCRecord
	var created, updated string
	err := s.db.QueryRow(`select id,round_no,run_date,control_label,control_level,lot_no,diluent_info,file_id,status,source_file,created_at,updated_at from qc_records where id = ?`, id).
		Scan(&item.ID, &item.RoundNo, &item.RunDate, &item.ControlLabel, &item.ControlLevel, &item.LotNo, &item.DiluentInfo, &item.FileID, &item.Status, &item.SourceFile, &created, &updated)
	if err != nil {
		return coremodel.QCRecord{}, err
	}
	item.CreatedAt, _ = time.Parse(time.RFC3339, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return item, nil
}

func (s *Store) UpsertQCRecord(item coremodel.QCRecord) (coremodel.QCRecord, error) {
	item.RunDate = normalizeDate(item.RunDate)
	item.ControlLabel = strings.TrimSpace(item.ControlLabel)
	if item.ControlLabel == "" {
		return coremodel.QCRecord{}, errors.New("control_label is required")
	}
	if item.RoundNo <= 0 {
		item.RoundNo = 1
	}
	item.LotNo = defaultString(strings.TrimSpace(item.LotNo), "-")
	item.Status = defaultString(item.Status, "completed")
	now := time.Now().UTC().Format(time.RFC3339)
	var existingID int64
	err := s.db.QueryRow(`select id from qc_records where run_date = ? and control_label = ? and control_level = ? and lot_no = ? limit 1`,
		item.RunDate, item.ControlLabel, item.ControlLevel, item.LotNo).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return coremodel.QCRecord{}, err
	}
	if item.ID <= 0 && existingID > 0 {
		item.ID = existingID
	}
	if item.ID > 0 {
		_, err := s.db.Exec(`update qc_records set round_no=?,run_date=?,control_label=?,control_level=?,lot_no=?,diluent_info=?,file_id=?,status=?,source_file=?,updated_at=? where id = ?`,
			item.RoundNo, item.RunDate, item.ControlLabel, item.ControlLevel, item.LotNo, item.DiluentInfo, item.FileID, item.Status, item.SourceFile, now, item.ID)
		if err != nil {
			return coremodel.QCRecord{}, err
		}
		return s.GetQCRecord(item.ID)
	}
	res, err := s.db.Exec(`insert into qc_records(round_no,run_date,control_label,control_level,lot_no,diluent_info,file_id,status,source_file,created_at,updated_at) values(?,?,?,?,?,?,?,?,?,?,?)`,
		item.RoundNo, item.RunDate, item.ControlLabel, item.ControlLevel, item.LotNo, item.DiluentInfo, item.FileID, item.Status, item.SourceFile, now, now)
	if err != nil {
		return coremodel.QCRecord{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetQCRecord(id)
}

func (s *Store) GetQCAnalysis(id int64) (coremodel.QCAnalysis, error) {
	var item coremodel.QCAnalysis
	var flagsJSON, created string
	var numeric sql.NullFloat64
	err := s.db.QueryRow(`
		select qa.id,qa.qc_record_id,qa.analyte_id,qa.analyte_tag,qa.analyte_name,
		       coalesce(qt.control_level, qr.control_level, qa.control_level) as control_level,
		       coalesce(qa.lot_no, qr.lot_no) as lot_no,
		       qa.status,qa.default_result_id,qa.result_value,qa.raw_value,qa.interpreted_value,qa.numeric_value,qa.unit,qa.source_file,qa.flags_json,qa.created_at
		from qc_analyses qa
		left join qc_records qr on qr.id = qa.qc_record_id
		left join qc_targets qt on qt.active = 1 and qt.analyte_tag = qa.analyte_tag and qt.lot_no = qa.lot_no
		where qa.id = ?`, id).
		Scan(&item.ID, &item.QCRecordID, &item.AnalyteID, &item.AnalyteTag, &item.AnalyteName, &item.ControlLevel, &item.LotNo, &item.Status, &item.DefaultResultID, &item.ResultValue, &item.RawValue, &item.Interpreted, &numeric, &item.Unit, &item.SourceFile, &flagsJSON, &created)
	if err != nil {
		return coremodel.QCAnalysis{}, err
	}
	_ = json.Unmarshal([]byte(flagsJSON), &item.Flags)
	if numeric.Valid {
		value := numeric.Float64
		item.NumericValue = &value
	}
	item.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return item, nil
}

func (s *Store) ListQCAnalyses(recordID int64) ([]coremodel.QCAnalysis, error) {
	rows, err := s.db.Query(`select id from qc_analyses where qc_record_id = ? order by id asc`, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []coremodel.QCAnalysis{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		item, err := s.GetQCAnalysis(id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpsertQCAnalysis(item coremodel.QCAnalysis) (coremodel.QCAnalysis, error) {
	if item.QCRecordID <= 0 {
		return coremodel.QCAnalysis{}, errors.New("qc_record_id is required")
	}
	item.AnalyteTag = strings.TrimSpace(item.AnalyteTag)
	if item.AnalyteTag == "" {
		return coremodel.QCAnalysis{}, errors.New("analyte_tag is required")
	}
	item.Status = defaultString(item.Status, "completed")
	if item.AnalyteID <= 0 {
		var analyteID int64
		_ = s.db.QueryRow(`select id from analytes where tag = ? limit 1`, item.AnalyteTag).Scan(&analyteID)
		item.AnalyteID = analyteID
	}
	item.ControlLevel = ""
	if item.QCRecordID > 0 && strings.TrimSpace(item.LotNo) == "" {
		record, err := s.GetQCRecord(item.QCRecordID)
		if err == nil {
			if strings.TrimSpace(item.LotNo) == "" {
				item.LotNo = record.LotNo
			}
		}
	}
	flagsJSON, _ := json.Marshal(metaOrEmpty(item.Flags))
	numericValue, hasNumeric := parseNumericQCValue(item.RawValue, item.ResultValue)
	createdAt := time.Now().UTC().Format(time.RFC3339)
	if measuredAt, ok := parseMeasuredAt(item.Flags); ok {
		createdAt = measuredAt
	}
	manualEnteredBy, _ := item.Flags["manual_entered_by"].(string)
	manualEnteredAt, _ := item.Flags["manual_entered_at"].(string)
	if item.ID > 0 {
		_, err := s.db.Exec(`update qc_analyses set analyte_id=?,analyte_tag=?,analyte_name=?,control_level=?,lot_no=?,status=?,default_result_id=?,result_value=?,raw_value=?,interpreted_value=?,numeric_value=?,unit=?,source_file=?,flags_json=?,manual_entered_by=?,manual_entered_at=?,created_at=? where id = ?`,
			item.AnalyteID, item.AnalyteTag, item.AnalyteName, "", defaultString(item.LotNo, "-"), item.Status, item.DefaultResultID, item.ResultValue, item.RawValue, item.Interpreted, nullableFloatArg(numericValue, hasNumeric), item.Unit, item.SourceFile, string(flagsJSON), manualEnteredBy, manualEnteredAt, createdAt, item.ID)
		if err != nil {
			return coremodel.QCAnalysis{}, err
		}
		return s.GetQCAnalysis(item.ID)
	}
	res, err := s.db.Exec(`insert into qc_analyses(qc_record_id,analyte_id,analyte_tag,analyte_name,control_level,lot_no,status,default_result_id,result_value,raw_value,interpreted_value,numeric_value,unit,source_file,flags_json,manual_entered_by,manual_entered_at,created_at) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		item.QCRecordID, item.AnalyteID, item.AnalyteTag, item.AnalyteName, "", defaultString(item.LotNo, "-"), item.Status, item.DefaultResultID, item.ResultValue, item.RawValue, item.Interpreted, nullableFloatArg(numericValue, hasNumeric), item.Unit, item.SourceFile, string(flagsJSON), manualEnteredBy, manualEnteredAt, createdAt)
	if err != nil {
		return coremodel.QCAnalysis{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetQCAnalysis(id)
}

func (s *Store) DeleteQCAnalysis(id int64) error {
	_, err := s.db.Exec(`delete from qc_analyses where id = ?`, id)
	return err
}

func (s *Store) ListQCRoundNumbers(runDate string) ([]int, error) {
	return []int{1}, nil
}

func (s *Store) CreateNextQCRound(runDate string) (int, error) {
	return 1, nil
}

func (s *Store) DashboardSnapshot(limit int) (map[string]interface{}, error) {
	if limit <= 0 {
		limit = 14
	}
	today := time.Now().Format("2006-01-02")
	var withoutResult, withResult int
	if err := s.db.QueryRow(`
		select
			coalesce(sum(case when coalesce(oa.result_value,'') = '' and coalesce(oa.raw_value,'') = '' then 1 else 0 end), 0),
			coalesce(sum(case when coalesce(oa.result_value,'') <> '' or coalesce(oa.raw_value,'') <> '' then 1 else 0 end), 0)
		from order_analyses oa
		join orders o on o.id = oa.order_id
		where o.order_date = ?`, today).Scan(&withoutResult, &withResult); err != nil {
		return nil, err
	}
	var qcResults, qcNumericResults, qcOutside2SD, qcOutside3SD int
	if err := s.db.QueryRow(`
		select
			coalesce(count(*), 0),
			coalesce(sum(case when qa.numeric_value is not null then 1 else 0 end), 0),
			coalesce(sum(case when qa.numeric_value is not null and qt.target_sd > 0 and abs(qa.numeric_value - qt.target_mean) > (2 * qt.target_sd) then 1 else 0 end), 0),
			coalesce(sum(case when qa.numeric_value is not null and qt.target_sd > 0 and abs(qa.numeric_value - qt.target_mean) > (3 * qt.target_sd) then 1 else 0 end), 0)
		from qc_analyses qa
		join qc_records qr on qr.id = qa.qc_record_id
		left join qc_targets qt on qt.active = 1 and qt.analyte_tag = qa.analyte_tag and qt.lot_no = qa.lot_no
		where qr.run_date = ?`, today).Scan(&qcResults, &qcNumericResults, &qcOutside2SD, &qcOutside3SD); err != nil {
		return nil, err
	}
	qcToday := map[string]interface{}{
		"results":         qcResults,
		"numeric_results": qcNumericResults,
		"outside_2sd":     qcOutside2SD,
		"outside_3sd":     qcOutside3SD,
	}
	rows, err := s.db.Query(`
		with recursive days(day, idx) as (
			select date(?), 0
			union all
			select date(day, '-1 day'), idx + 1 from days where idx + 1 < ?
		)
		select
			day,
			coalesce((select count(*) from orders o where o.order_date = day), 0) as orders_count,
			coalesce((select count(*) from order_analyses oa join orders o on o.id = oa.order_id where o.order_date = day), 0) as analyses_count,
			coalesce((select count(*) from order_analyses oa join orders o on o.id = oa.order_id where o.order_date = day and (coalesce(oa.result_value,'') <> '' or coalesce(oa.raw_value,'') <> '')), 0) as analyses_with_result
		from days
		order by day asc`, today, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	series := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var day string
		var ordersCount, analysesCount, analysesWithResult int
		if err := rows.Scan(&day, &ordersCount, &analysesCount, &analysesWithResult); err != nil {
			return nil, err
		}
		series = append(series, map[string]interface{}{
			"day":                  day,
			"orders":               ordersCount,
			"analyses":             analysesCount,
			"analyses_with_result": analysesWithResult,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"today": map[string]interface{}{
			"without_result": withoutResult,
			"with_result":    withResult,
		},
		"qc_today": qcToday,
		"series":   series,
	}, nil
}

func (s *Store) ListLogs(limit int) ([]coremodel.EventLog, error) {
	if limit <= 0 {
		limit = 40
	}
	rows, err := s.db.Query(`
		select id, level, event_type, message, created_at from (
			select
				(1000000000 + qar.id) as id,
				'info' as level,
				'result' as event_type,
				'Rezultat primit: ' || coalesce(oa.analyte_tag,'-') || ' / ' || coalesce(o.sample_id,'-') || ' = ' || coalesce(nullif(qar.result_value,''), nullif(qar.raw_value,''), '-') as message,
				qar.created_at as created_at
			from order_analysis_results qar
			join order_analyses oa on oa.id = qar.order_analysis_id
			join orders o on o.id = oa.order_id
			union all
			select
				(2000000000 + qa.id) as id,
				case when qr.status = 'manual' then 'warning' else 'info' end as level,
				'qc' as event_type,
				'QC ' || case when qr.status = 'manual' then 'manual' else 'importat' end || ': ' || coalesce(qa.analyte_tag,'-') || ' / ' || coalesce(qa.lot_no,'-') || ' = ' || coalesce(nullif(qa.result_value,''), nullif(qa.raw_value,''), '-') as message,
				coalesce(nullif(qa.created_at,''), qr.created_at) as created_at
			from qc_analyses qa
			join qc_records qr on qr.id = qa.qc_record_id
		)
		order by datetime(created_at) desc
		limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]coremodel.EventLog, 0, limit)
	for rows.Next() {
		var item coremodel.EventLog
		var created string
		if err := rows.Scan(&item.ID, &item.Level, &item.EventType, &item.Message, &created); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) QCPerformance(analyteTag, controlLevel, lotNo, dateFrom, dateTo string, limit int) (map[string]interface{}, error) {
	analyteTag = strings.TrimSpace(analyteTag)
	if analyteTag == "" {
		return nil, errors.New("analyte_tag is required")
	}
	if limit <= 0 {
		limit = 500
	}
	query := `
		select q.run_date, q.control_label, coalesce(t.control_level, a.control_level, q.control_level), a.lot_no, a.numeric_value, a.created_at, t.target_mean, t.target_sd, t.target_cv
		from qc_analyses a
		join qc_records q on q.id = a.qc_record_id
		left join qc_targets t on t.active = 1 and t.analyte_tag = a.analyte_tag and t.lot_no = a.lot_no
		where a.analyte_tag = ? and a.numeric_value is not null`
	args := []interface{}{analyteTag}
	if strings.TrimSpace(controlLevel) != "" {
		query += ` and coalesce(t.control_level, a.control_level, q.control_level) = ?`
		args = append(args, strings.TrimSpace(controlLevel))
	}
	if strings.TrimSpace(lotNo) != "" {
		query += ` and a.lot_no = ?`
		args = append(args, strings.TrimSpace(lotNo))
	}
	if strings.TrimSpace(dateFrom) != "" {
		query += ` and q.run_date >= ?`
		args = append(args, normalizeDate(dateFrom))
	}
	if strings.TrimSpace(dateTo) != "" {
		query += ` and q.run_date <= ?`
		args = append(args, normalizeDate(dateTo))
	}
	query += ` order by q.run_date asc, a.created_at asc, a.id asc`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := make([]map[string]interface{}, 0)
	values := make([]float64, 0)
	targetMean := 0.0
	targetSD := 0.0
	targetCV := 0.0
	for rows.Next() {
		var runDate, controlLabel, level, lot, createdAt string
		var numeric float64
		var mean, sd, cv sql.NullFloat64
		if err := rows.Scan(&runDate, &controlLabel, &level, &lot, &numeric, &createdAt, &mean, &sd, &cv); err != nil {
			return nil, err
		}
		if mean.Valid {
			targetMean = mean.Float64
		}
		if sd.Valid {
			targetSD = sd.Float64
		}
		if cv.Valid {
			targetCV = cv.Float64
		}
		values = append(values, numeric)
		points = append(points, map[string]interface{}{
			"index":         len(points) + 1,
			"run_date":      runDate,
			"control_label": controlLabel,
			"control_level": level,
			"lot_no":        lot,
			"created_at":    createdAt,
			"value":         numeric,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(points) > limit {
		points = points[len(points)-limit:]
		values = values[len(values)-limit:]
		for i := range points {
			points[i]["index"] = i + 1
		}
	}
	mean, sd, cv := basicStats(values)
	median := median(values)
	minValue, maxValue := minMax(values)
	outliers := outlierCount(values, mean, sd)
	repeatability := repeatability(values)
	robustMean := mean
	if len(values) > 10 {
		robustMean = mean
	}
	westgard := classifyWestgard(points, targetMean, targetSD)
	issues := validateWestgardDataset(points, targetMean, targetSD, westgard)
	return map[string]interface{}{
		"analyte_tag":            analyteTag,
		"control_level":          controlLevel,
		"lot_no":                 lotNo,
		"date_from":              strings.TrimSpace(dateFrom),
		"date_to":                strings.TrimSpace(dateTo),
		"count":                  len(values),
		"mean":                   mean,
		"sd":                     sd,
		"cv":                     cv,
		"median":                 median,
		"min":                    minValue,
		"max":                    maxValue,
		"outliers":               outliers,
		"repeatability":          repeatability,
		"target_mean":            targetMean,
		"target_sd":              targetSD,
		"target_cv":              targetCV,
		"robust_mean":            robustMean,
		"use_own_mean":           len(values) > 10,
		"points":                 points,
		"westgard":               westgard,
		"is_valid":               len(issues) == 0,
		"validation_issues":      issues,
		"validation_issue_count": len(issues),
	}, nil
}

func (s *Store) findQCTarget(analyteTag, controlLevel, lotNo string) (coremodel.QCTarget, error) {
	query := `select id from qc_targets where analyte_tag = ?`
	args := []interface{}{strings.TrimSpace(analyteTag)}
	if strings.TrimSpace(controlLevel) != "" {
		query += ` and control_level = ?`
		args = append(args, strings.TrimSpace(controlLevel))
	}
	if strings.TrimSpace(lotNo) != "" {
		query += ` and lot_no = ?`
		args = append(args, strings.TrimSpace(lotNo))
	}
	query += ` order by id desc limit 1`
	var id int64
	if err := s.db.QueryRow(query, args...).Scan(&id); err != nil {
		return coremodel.QCTarget{}, err
	}
	return s.GetQCTarget(id)
}

func (s *Store) ListDailyDetailDefinitions() ([]coremodel.DailyDetailDefinition, error) {
	rows, err := s.db.Query(`select id,key,label,scope,field_type,placeholder,default_value,options_json,required,active,source,sort_order,meta_json,created_at,updated_at from daily_detail_definitions order by sort_order asc, label asc, key asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []coremodel.DailyDetailDefinition{}
	for rows.Next() {
		var item coremodel.DailyDetailDefinition
		var optionsJSON, metaJSON, created, updated string
		var required, active int
		if err := rows.Scan(&item.ID, &item.Key, &item.Label, &item.Scope, &item.FieldType, &item.Placeholder, &item.DefaultValue, &optionsJSON, &required, &active, &item.Source, &item.SortOrder, &metaJSON, &created, &updated); err != nil {
			return nil, err
		}
		item.Required = required == 1
		item.Active = active == 1
		_ = json.Unmarshal([]byte(optionsJSON), &item.Options)
		_ = json.Unmarshal([]byte(metaJSON), &item.Meta)
		item.CreatedAt, _ = time.Parse(time.RFC3339, created)
		item.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetDailyDetailDefinition(id int64) (coremodel.DailyDetailDefinition, error) {
	var item coremodel.DailyDetailDefinition
	var optionsJSON, metaJSON, created, updated string
	var required, active int
	err := s.db.QueryRow(`select id,key,label,scope,field_type,placeholder,default_value,options_json,required,active,source,sort_order,meta_json,created_at,updated_at from daily_detail_definitions where id = ?`, id).
		Scan(&item.ID, &item.Key, &item.Label, &item.Scope, &item.FieldType, &item.Placeholder, &item.DefaultValue, &optionsJSON, &required, &active, &item.Source, &item.SortOrder, &metaJSON, &created, &updated)
	if err != nil {
		return coremodel.DailyDetailDefinition{}, err
	}
	item.Required = required == 1
	item.Active = active == 1
	_ = json.Unmarshal([]byte(optionsJSON), &item.Options)
	_ = json.Unmarshal([]byte(metaJSON), &item.Meta)
	item.CreatedAt, _ = time.Parse(time.RFC3339, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return item, nil
}

func (s *Store) SaveDailyDetailDefinition(item coremodel.DailyDetailDefinition) (coremodel.DailyDetailDefinition, error) {
	item.Key = strings.TrimSpace(item.Key)
	item.Label = strings.TrimSpace(item.Label)
	item.Scope = normalizeDailyDetailScope(item.Scope)
	item.FieldType = defaultString(strings.TrimSpace(item.FieldType), "text")
	item.Source = defaultString(strings.TrimSpace(item.Source), "user")
	if item.Key == "" || item.Label == "" {
		return coremodel.DailyDetailDefinition{}, errors.New("daily detail key and label are required")
	}
	optionsJSON, _ := json.Marshal(item.Options)
	metaJSON, _ := json.Marshal(metaOrEmpty(item.Meta))
	now := time.Now().UTC().Format(time.RFC3339)
	var existingID int64
	err := s.db.QueryRow(`select id from daily_detail_definitions where key = ? limit 1`, item.Key).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return coremodel.DailyDetailDefinition{}, err
	}
	if item.ID > 0 {
		if existingID > 0 && existingID != item.ID {
			return coremodel.DailyDetailDefinition{}, errors.New("daily detail key must be unique")
		}
		_, err = s.db.Exec(`update daily_detail_definitions set key=?,label=?,scope=?,field_type=?,placeholder=?,default_value=?,options_json=?,required=?,active=?,source=?,sort_order=?,meta_json=?,updated_at=? where id = ?`,
			item.Key, item.Label, item.Scope, item.FieldType, item.Placeholder, item.DefaultValue, string(optionsJSON), boolToInt(item.Required), boolToInt(item.Active), item.Source, item.SortOrder, string(metaJSON), now, item.ID)
		if err != nil {
			return coremodel.DailyDetailDefinition{}, err
		}
		return s.GetDailyDetailDefinition(item.ID)
	}
	if existingID > 0 {
		item.ID = existingID
		return s.SaveDailyDetailDefinition(item)
	}
	res, err := s.db.Exec(`insert into daily_detail_definitions(key,label,scope,field_type,placeholder,default_value,options_json,required,active,source,sort_order,meta_json,created_at,updated_at) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		item.Key, item.Label, item.Scope, item.FieldType, item.Placeholder, item.DefaultValue, string(optionsJSON), boolToInt(item.Required), boolToInt(item.Active), item.Source, item.SortOrder, string(metaJSON), now, now)
	if err != nil {
		return coremodel.DailyDetailDefinition{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetDailyDetailDefinition(id)
}

func (s *Store) DeleteDailyDetailDefinition(id int64) error {
	def, err := s.GetDailyDetailDefinition(id)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`delete from daily_detail_values where definition_key = ?`, def.Key); err != nil {
		return err
	}
	_, err = s.db.Exec(`delete from daily_detail_definitions where id = ?`, id)
	return err
}

func (s *Store) ListDailyDetailValues(scopeDate string, roundNo int) ([]coremodel.DailyDetailValue, error) {
	scopeDate = normalizeDate(scopeDate)
	query := `select id,definition_key,scope_date,round_no,analyte_tag,value_text,meta_json,created_at,updated_at from daily_detail_values where scope_date = ?`
	args := []interface{}{scopeDate}
	if roundNo > 0 {
		query += ` and (round_no = 0 or round_no = ?)`
		args = append(args, roundNo)
	}
	query += ` order by definition_key asc, analyte_tag asc, round_no asc, id asc`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []coremodel.DailyDetailValue{}
	for rows.Next() {
		var item coremodel.DailyDetailValue
		var metaJSON, created, updated string
		if err := rows.Scan(&item.ID, &item.DefinitionKey, &item.ScopeDate, &item.RoundNo, &item.AnalyteTag, &item.ValueText, &metaJSON, &created, &updated); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(metaJSON), &item.Meta)
		item.CreatedAt, _ = time.Parse(time.RFC3339, created)
		item.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SaveDailyDetailValue(item coremodel.DailyDetailValue) (coremodel.DailyDetailValue, error) {
	item.DefinitionKey = strings.TrimSpace(item.DefinitionKey)
	item.ScopeDate = normalizeDate(item.ScopeDate)
	item.AnalyteTag = strings.TrimSpace(item.AnalyteTag)
	item.RoundNo = normalizeDailyDetailRound(item.RoundNo)
	if item.DefinitionKey == "" {
		return coremodel.DailyDetailValue{}, errors.New("definition_key is required")
	}
	metaJSON, _ := json.Marshal(metaOrEmpty(item.Meta))
	now := time.Now().UTC().Format(time.RFC3339)
	var existingID int64
	err := s.db.QueryRow(`select id from daily_detail_values where definition_key = ? and scope_date = ? and round_no = ? and analyte_tag = ? limit 1`, item.DefinitionKey, item.ScopeDate, item.RoundNo, item.AnalyteTag).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return coremodel.DailyDetailValue{}, err
	}
	if item.ID > 0 {
		existingID = item.ID
	}
	if existingID > 0 {
		_, err = s.db.Exec(`update daily_detail_values set value_text=?,meta_json=?,updated_at=? where id = ?`,
			item.ValueText, string(metaJSON), now, existingID)
		if err != nil {
			return coremodel.DailyDetailValue{}, err
		}
		return s.GetDailyDetailValue(existingID)
	}
	res, err := s.db.Exec(`insert into daily_detail_values(definition_key,scope_date,round_no,analyte_tag,value_text,meta_json,created_at,updated_at) values(?,?,?,?,?,?,?,?)`,
		item.DefinitionKey, item.ScopeDate, item.RoundNo, item.AnalyteTag, item.ValueText, string(metaJSON), now, now)
	if err != nil {
		return coremodel.DailyDetailValue{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetDailyDetailValue(id)
}

func (s *Store) GetDailyDetailValue(id int64) (coremodel.DailyDetailValue, error) {
	var item coremodel.DailyDetailValue
	var metaJSON, created, updated string
	err := s.db.QueryRow(`select id,definition_key,scope_date,round_no,analyte_tag,value_text,meta_json,created_at,updated_at from daily_detail_values where id = ?`, id).
		Scan(&item.ID, &item.DefinitionKey, &item.ScopeDate, &item.RoundNo, &item.AnalyteTag, &item.ValueText, &metaJSON, &created, &updated)
	if err != nil {
		return coremodel.DailyDetailValue{}, err
	}
	_ = json.Unmarshal([]byte(metaJSON), &item.Meta)
	item.CreatedAt, _ = time.Parse(time.RFC3339, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return item, nil
}

func normalizeDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now().Format("2006-01-02")
	}
	return value
}

func normalizeDailyDetailScope(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "day", "zi":
		return "day"
	case "day_analyte", "zi_analiza", "day-analysis":
		return "day_analyte"
	case "day_round", "zi_runda", "day-round":
		return "day_round"
	case "day_round_analyte", "zi_runda_analiza", "day-round-analysis":
		return "day_round_analyte"
	default:
		return "day"
	}
}

func normalizeDailyDetailRound(roundNo int) int {
	if roundNo < 0 {
		return 0
	}
	return roundNo
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func metaOrEmpty(value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	return value
}

func parseNumericQCValue(rawValue, resultValue string) (float64, bool) {
	for _, candidate := range []string{rawValue, resultValue} {
		candidate = strings.TrimSpace(strings.ReplaceAll(candidate, ",", "."))
		if candidate == "" {
			continue
		}
		if parsed, err := strconv.ParseFloat(candidate, 64); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func nullableFloatArg(value float64, ok bool) interface{} {
	if !ok {
		return nil
	}
	return value
}

func basicStats(values []float64) (mean, sd, cv float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	var sum float64
	for _, value := range values {
		sum += value
	}
	mean = sum / float64(len(values))
	if len(values) > 1 {
		var variance float64
		for _, value := range values {
			diff := value - mean
			variance += diff * diff
		}
		sd = abs(variance / float64(len(values)-1))
		sd = sqrt(sd)
	}
	if mean != 0 {
		cv = abs(sd/mean) * 100
	}
	return mean, sd, cv
}

func parseMeasuredAt(flags map[string]interface{}) (string, bool) {
	if flags == nil {
		return "", false
	}
	raw := strings.TrimSpace(fmt.Sprint(flags["measured_at"]))
	if raw == "" {
		return "", false
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts.UTC().Format(time.RFC3339), true
	}
	return "", false
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 0 {
		return (cp[mid-1] + cp[mid]) / 2
	}
	return cp[mid]
}

func minMax(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	minValue, maxValue := values[0], values[0]
	for _, value := range values[1:] {
		if value < minValue {
			minValue = value
		}
		if value > maxValue {
			maxValue = value
		}
	}
	return minValue, maxValue
}

func outlierCount(values []float64, mean, sd float64) int {
	if len(values) == 0 || sd <= 0 {
		return 0
	}
	count := 0
	for _, value := range values {
		if math.Abs(value-mean) > 3*sd {
			count++
		}
	}
	return count
}

func repeatability(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	total := 0.0
	for i := 1; i < len(values); i++ {
		total += math.Abs(values[i] - values[i-1])
	}
	return total / float64(len(values)-1)
}

func classifyWestgard(points []map[string]interface{}, targetMean, targetSD float64) map[string]interface{} {
	out := map[string]interface{}{
		"1_2s":  0,
		"1_3s":  0,
		"2_2s":  0,
		"r_4s":  0,
		"4_1s":  0,
		"10x":   0,
		"7t":    0,
		"rules": []map[string]interface{}{},
	}
	if len(points) == 0 || targetSD <= 0 {
		return out
	}
	zscores := make([]float64, len(points))
	values := make([]float64, len(points))
	for i, point := range points {
		value := point["value"].(float64)
		values[i] = value
		z := (value - targetMean) / targetSD
		zscores[i] = z
		point["zscore"] = z
		rules := make([]string, 0)
		if math.Abs(z) > 2 {
			out["1_2s"] = out["1_2s"].(int) + 1
			rules = append(rules, "1_2s")
		}
		if math.Abs(z) > 3 {
			out["1_3s"] = out["1_3s"].(int) + 1
			rules = append(rules, "1_3s")
		}
		if i >= 1 && sameSide(zscores[i], zscores[i-1]) && math.Abs(zscores[i]) > 2 && math.Abs(zscores[i-1]) > 2 {
			out["2_2s"] = out["2_2s"].(int) + 1
			rules = append(rules, "2_2s")
		}
		if i >= 1 && math.Abs(zscores[i]-zscores[i-1]) > 4 {
			out["r_4s"] = out["r_4s"].(int) + 1
			rules = append(rules, "R_4s")
		}
		if i >= 3 && sameSide(zscores[i], zscores[i-1], zscores[i-2], zscores[i-3]) &&
			math.Abs(zscores[i]) > 1 && math.Abs(zscores[i-1]) > 1 && math.Abs(zscores[i-2]) > 1 && math.Abs(zscores[i-3]) > 1 {
			out["4_1s"] = out["4_1s"].(int) + 1
			rules = append(rules, "4_1s")
		}
		if i >= 9 && sameSide(zscores[i-9:i+1]...) {
			out["10x"] = out["10x"].(int) + 1
			rules = append(rules, "10x")
		}
		if i >= 6 && monotonic(values[i-6:i+1]) {
			out["7t"] = out["7t"].(int) + 1
			rules = append(rules, "7T")
		}
		point["westgard_rules"] = rules
		if len(rules) > 0 {
			out["rules"] = append(out["rules"].([]map[string]interface{}), map[string]interface{}{
				"index":         point["index"],
				"run_date":      point["run_date"],
				"control_label": point["control_label"],
				"value":         point["value"],
				"zscore":        point["zscore"],
				"rules":         rules,
			})
		}
	}
	return out
}

func validateWestgardDataset(points []map[string]interface{}, targetMean, targetSD float64, westgard map[string]interface{}) []string {
	issues := make([]string, 0)
	if len(points) == 0 {
		return []string{"Nu exista citiri numerice pentru filtrul selectat."}
	}
	if targetSD <= 0 {
		issues = append(issues, "Target SD nu este definit in Setari QC pentru lotul selectat.")
	}
	if targetMean == 0 {
		issues = append(issues, "Target mean nu este definit in Setari QC pentru lotul selectat.")
	}
	if rules, ok := westgard["rules"].([]map[string]interface{}); ok && len(rules) > 0 {
		for _, rule := range rules {
			issues = append(issues, fmt.Sprintf("%s %v la %s (%v)", strings.Join(toStringSlice(rule["rules"]), ", "), rule["value"], rule["run_date"], rule["control_label"]))
		}
	}
	return issues
}

func sameSide(values ...float64) bool {
	if len(values) == 0 || values[0] == 0 {
		return false
	}
	sign := values[0] > 0
	for _, value := range values[1:] {
		if value == 0 || (value > 0) != sign {
			return false
		}
	}
	return true
}

func monotonic(values []float64) bool {
	if len(values) < 2 {
		return false
	}
	inc := true
	dec := true
	for i := 1; i < len(values); i++ {
		if values[i] <= values[i-1] {
			inc = false
		}
		if values[i] >= values[i-1] {
			dec = false
		}
	}
	return inc || dec
}

func toStringSlice(value interface{}) []string {
	items, ok := value.([]string)
	if ok {
		return items
	}
	raw, ok := value.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

func sqrt(value float64) float64 {
	if value <= 0 {
		return 0
	}
	guess := value
	for i := 0; i < 12; i++ {
		guess = 0.5 * (guess + value/guess)
	}
	return guess
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
