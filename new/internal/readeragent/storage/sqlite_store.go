package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
	"wisemed-labreaders/new/internal/shared/protocol"
)

type SQLiteStore struct {
	db *sql.DB
}

var ErrNotFound = errors.New("not found")

type Analyte struct {
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

type CommunicationConfig struct {
	AnalyzerCode string                 `json:"analyzer_code"`
	Transport    string                 `json:"transport"`
	Mode         string                 `json:"mode"`
	Settings     map[string]interface{} `json:"settings"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type CommEvent struct {
	ID        int64                  `json:"id"`
	EventType string                 `json:"event_type"`
	Payload   map[string]interface{} `json:"payload"`
	CreatedAt time.Time              `json:"created_at"`
}

func Open(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) migrate() error {
	stmts := []string{
		`create table if not exists analytes (
			tag text primary key,
			name text not null,
			updated_at text not null
		)`,
		`create table if not exists result_outbox (
			ref_id text primary key,
			patient_id text not null,
			sample_id text not null,
			analyte_tag text not null,
			result_value text not null,
			unit text not null,
			meta_json text not null,
			produced_at text not null,
			sent_at text
		)`,
		`create table if not exists comm_events (
			id integer primary key autoincrement,
			event_type text not null,
			payload_json text not null,
			created_at text not null
		)`,
		`create table if not exists comm_config (
			analyzer_code text primary key,
			transport text not null,
			mode text not null,
			settings_json text not null,
			updated_at text not null
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) UpsertAnalytes(items []Analyte) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	stmt, err := tx.Prepare(`insert into analytes(tag,name,updated_at) values(?,?,?)
		on conflict(tag) do update set name=excluded.name, updated_at=excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, a := range items {
		if a.Tag == "" || a.Name == "" {
			return errors.New("invalid analyte item")
		}
		if _, err = stmt.Exec(a.Tag, a.Name, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListAnalytes() ([]Analyte, error) {
	rows, err := s.db.Query(`select name, tag from analytes order by name asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := []Analyte{}
	for rows.Next() {
		var a Analyte
		if err := rows.Scan(&a.Name, &a.Tag); err != nil {
			return nil, err
		}
		res = append(res, a)
	}
	return res, nil
}

func (s *SQLiteStore) EnqueueResult(it protocol.ResultOutboxItemWire) error {
	meta, _ := json.Marshal(it.Meta)
	_, err := s.db.Exec(`insert or replace into result_outbox(ref_id,patient_id,sample_id,analyte_tag,result_value,unit,meta_json,produced_at,sent_at)
		values(?,?,?,?,?,?,?,?,null)`,
		it.RefID, it.PatientID, it.SampleID, it.AnalyteTag, it.ResultValue, it.Unit, string(meta), it.ProducedAt.Format(time.RFC3339Nano))
	return err
}

func (s *SQLiteStore) PendingResults(limit int) ([]protocol.ResultOutboxItemWire, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`select ref_id,patient_id,sample_id,analyte_tag,result_value,unit,meta_json,produced_at from result_outbox where sent_at is null order by produced_at asc limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []protocol.ResultOutboxItemWire{}
	for rows.Next() {
		var it protocol.ResultOutboxItemWire
		var metaJSON string
		var produced string
		if err := rows.Scan(&it.RefID, &it.PatientID, &it.SampleID, &it.AnalyteTag, &it.ResultValue, &it.Unit, &metaJSON, &produced); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(metaJSON), &it.Meta)
		t, err := time.Parse(time.RFC3339Nano, produced)
		if err != nil {
			t = time.Now().UTC()
		}
		it.ProducedAt = t
		items = append(items, it)
	}
	return items, nil
}

func (s *SQLiteStore) MarkResultsSent(refIDs []string) error {
	if len(refIDs) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`update result_outbox set sent_at=? where ref_id=?`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, id := range refIDs {
		if _, err := stmt.Exec(now, id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) AppendEvent(eventType string, payload interface{}) error {
	raw, _ := json.Marshal(payload)
	_, err := s.db.Exec(`insert into comm_events(event_type,payload_json,created_at) values(?,?,?)`, eventType, string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *SQLiteStore) ListEvents(limit int) ([]CommEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.Query(`select id,event_type,payload_json,created_at from comm_events order by id desc limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := make([]CommEvent, 0, limit)
	for rows.Next() {
		var ev CommEvent
		var payloadJSON string
		var created string
		if err := rows.Scan(&ev.ID, &ev.EventType, &payloadJSON, &created); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(payloadJSON), &ev.Payload)
		if t, err := time.Parse(time.RFC3339Nano, created); err == nil {
			ev.CreatedAt = t
		}
		res = append(res, ev)
	}
	return res, nil
}

func (s *SQLiteStore) UpsertCommunicationConfig(cfg CommunicationConfig) error {
	if cfg.AnalyzerCode == "" || cfg.Transport == "" || cfg.Mode == "" {
		return errors.New("invalid communication config")
	}
	raw, _ := json.Marshal(cfg.Settings)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`insert into comm_config(analyzer_code,transport,mode,settings_json,updated_at) values(?,?,?,?,?)
		on conflict(analyzer_code) do update set transport=excluded.transport, mode=excluded.mode, settings_json=excluded.settings_json, updated_at=excluded.updated_at`,
		cfg.AnalyzerCode, cfg.Transport, cfg.Mode, string(raw), now)
	return err
}

func (s *SQLiteStore) GetCommunicationConfig(analyzerCode string) (*CommunicationConfig, error) {
	row := s.db.QueryRow(`select analyzer_code,transport,mode,settings_json,updated_at from comm_config where analyzer_code=?`, analyzerCode)
	var cfg CommunicationConfig
	var settingsJSON string
	var updated string
	if err := row.Scan(&cfg.AnalyzerCode, &cfg.Transport, &cfg.Mode, &settingsJSON, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(settingsJSON), &cfg.Settings)
	t, err := time.Parse(time.RFC3339Nano, updated)
	if err == nil {
		cfg.UpdatedAt = t
	}
	return &cfg, nil
}

func (s *SQLiteStore) DebugStats() map[string]interface{} {
	out := map[string]interface{}{}
	var pending int
	_ = s.db.QueryRow(`select count(*) from result_outbox where sent_at is null`).Scan(&pending)
	var analytes int
	_ = s.db.QueryRow(`select count(*) from analytes`).Scan(&analytes)
	var commConfigured int
	_ = s.db.QueryRow(`select count(*) from comm_config`).Scan(&commConfigured)
	out["pending_results"] = pending
	out["configured_analytes"] = analytes
	out["configured_comm"] = commConfigured
	return out
}

func (s *SQLiteStore) SeedDemoResult() (string, error) {
	ref := fmt.Sprintf("r-%d", time.Now().UnixNano())
	err := s.EnqueueResult(protocol.ResultOutboxItemWire{
		RefID:       ref,
		PatientID:   "P-DEMO",
		SampleID:    "S-DEMO",
		AnalyteTag:  "GLU",
		ResultValue: "NEG",
		Unit:        "",
		Meta:        map[string]interface{}{"source": "demo"},
		ProducedAt:  time.Now().UTC(),
	})
	return ref, err
}
