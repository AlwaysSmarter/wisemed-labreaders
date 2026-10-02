package siui

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var operationIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

type Job struct {
	ID          string            `json:"id"`
	OperationID string            `json:"operation_id"`
	Status      string            `json:"status"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
	Error       string            `json:"error,omitempty"`
	Result      *ValidationResult `json:"result,omitempty"`
}
type work struct {
	ID, XML string
	Config  settings
}

func openJobs(path string) (*sql.DB, error) {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	file, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	file.Close()
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	_, e = db.Exec(`PRAGMA busy_timeout=5000; CREATE TABLE IF NOT EXISTS siui_jobs (id TEXT PRIMARY KEY, owner TEXT NOT NULL, operation_id TEXT NOT NULL, digest TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, error TEXT NOT NULL DEFAULT '', result TEXT NOT NULL DEFAULT '', UNIQUE(owner,operation_id));`)
	if e == nil {
		_, e = db.Exec(`UPDATE siui_jobs SET status='unknown', error='Process stopped before completion; reconcile with CNAS before resubmission', updated_at=? WHERE status IN ('queued','running')`, time.Now().UTC().Format(time.RFC3339))
	}
	if e != nil {
		db.Close()
		return nil, e
	}
	return db, nil
}
func readJob(db *sql.DB, owner, id string) (Job, error) {
	var j Job
	var result string
	e := db.QueryRow(`SELECT id,operation_id,status,created_at,updated_at,error,result FROM siui_jobs WHERE owner=? AND id=?`, owner, id).Scan(&j.ID, &j.OperationID, &j.Status, &j.CreatedAt, &j.UpdatedAt, &j.Error, &result)
	if e == nil && result != "" {
		e = json.Unmarshal([]byte(result), &j.Result)
	}
	return j, e
}
func insertJob(db *sql.DB, owner, op, xml string) (Job, bool, error) {
	if !operationIDPattern.MatchString(op) {
		return Job{}, false, errors.New("operation_id is required (1-128 identifier characters)")
	}
	hash := sha256.Sum256([]byte(xml))
	digest := hex.EncodeToString(hash[:])
	random := make([]byte, 16)
	if _, e := rand.Read(random); e != nil {
		return Job{}, false, e
	}
	id := hex.EncodeToString(random)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, e := db.Exec(`INSERT INTO siui_jobs(id,owner,operation_id,digest,status,created_at,updated_at) VALUES(?,?,?,?,'queued',?,?) ON CONFLICT(owner,operation_id) DO NOTHING`, id, owner, op, digest, now, now)
	if e != nil {
		return Job{}, false, e
	}
	count, e := result.RowsAffected()
	if e != nil {
		return Job{}, false, e
	}
	if count == 0 {
		var existing string
		e = db.QueryRow(`SELECT id,digest FROM siui_jobs WHERE owner=? AND operation_id=?`, owner, op).Scan(&id, &existing)
		if e != nil {
			return Job{}, false, e
		}
		if existing != digest {
			return Job{}, false, errors.New("operation_id already belongs to a different request")
		}
	}
	j, e := readJob(db, owner, id)
	return j, count == 1, e
}
