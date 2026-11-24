package ecard

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"time"
)

type store struct {
	db  *sql.DB
	box cipher.AEAD
}

func openStore(dir string) (*store, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	kp := filepath.Join(dir, "payload.key")
	key, e := os.ReadFile(kp)
	if os.IsNotExist(e) {
		key = make([]byte, 32)
		if _, e = rand.Read(key); e != nil {
			return nil, e
		}
		f, err := os.OpenFile(kp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return nil, err
		}
		_, e = f.Write(key)
		ce := f.Close()
		if e == nil {
			e = ce
		}
	}
	if e != nil {
		return nil, e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	box, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	db, e := sql.Open("sqlite", filepath.Join(dir, "jobs.db"))
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &store{db: db, box: box}
	_, e = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
 CREATE TABLE IF NOT EXISTS jobs(id TEXT PRIMARY KEY,owner TEXT NOT NULL,operation TEXT NOT NULL,state TEXT NOT NULL,sdk TEXT NOT NULL DEFAULT '',total INTEGER NOT NULL DEFAULT 0,completed INTEGER NOT NULL DEFAULT 0,bytes INTEGER NOT NULL DEFAULT 0,created INTEGER NOT NULL,error TEXT NOT NULL DEFAULT '',request BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS events(id INTEGER PRIMARY KEY AUTOINCREMENT,owner TEXT NOT NULL,payload BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS job_events(job TEXT NOT NULL,id INTEGER NOT NULL,owner TEXT NOT NULL,payload BLOB NOT NULL,PRIMARY KEY(job,id));
 CREATE TABLE IF NOT EXISTS operation_reports(job TEXT PRIMARY KEY,owner TEXT NOT NULL,payload BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS sdk_attempts(job TEXT NOT NULL,n INTEGER NOT NULL,payload BLOB NOT NULL,PRIMARY KEY(job,n));
 CREATE TABLE IF NOT EXISTS call_history(id TEXT PRIMARY KEY,owner TEXT NOT NULL,created INTEGER NOT NULL,payload BLOB NOT NULL);
 CREATE INDEX IF NOT EXISTS call_history_owner_created ON call_history(owner,created);
 CREATE TABLE IF NOT EXISTS batch_keys(id TEXT PRIMARY KEY, owner TEXT NOT NULL, fingerprint TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS items(job TEXT NOT NULL,n INTEGER NOT NULL,id TEXT NOT NULL,payload BLOB NOT NULL,result BLOB,PRIMARY KEY(job,n),UNIQUE(job,id));
 UPDATE jobs SET state='interrupted',error='service_restarted_no_replay' WHERE state IN ('queued','running','waiting_pin');`)
	if e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
func (s *store) seal(v interface{}, aad string) ([]byte, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return nil, e
	}
	n := make([]byte, s.box.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return nil, e
	}
	return s.box.Seal(n, n, b, []byte(aad)), nil
}
func (s *store) open(b []byte, aad string, v interface{}) error {
	n := s.box.NonceSize()
	if len(b) < n {
		return errors.New("invalid encrypted payload")
	}
	p, e := s.box.Open(nil, b[:n], b[n:], []byte(aad))
	if e != nil {
		return e
	}
	return json.Unmarshal(p, v)
}
func (s *store) job(id, owner string) (Job, error) {
	j := Job{}
	e := s.db.QueryRow(`SELECT id,state,operation,sdk,total,completed,created,error FROM jobs WHERE id=? AND owner=?`, id, owner).Scan(&j.ID, &j.State, &j.Operation, &j.SDK, &j.Total, &j.Completed, &j.Created, &j.Error)
	return j, e
}
func (s *store) state(id, state, code string) error {
	_, e := s.db.Exec(`UPDATE jobs SET state=?,error=? WHERE id=?`, state, code, id)
	return e
}
func (s *store) cleanup(hours int) error {
	_, e := s.db.Exec(`DELETE FROM items WHERE job IN (SELECT id FROM jobs WHERE created<? AND state NOT IN ('running','waiting_pin','queued')); DELETE FROM jobs WHERE created<? AND state NOT IN ('running','waiting_pin','queued'); DELETE FROM sdk_attempts WHERE job NOT IN (SELECT id FROM jobs); DELETE FROM job_events WHERE job NOT IN (SELECT id FROM jobs); DELETE FROM operation_reports WHERE job NOT IN (SELECT id FROM jobs)`, time.Now().Add(-time.Duration(hours)*time.Hour).Unix(), time.Now().Add(-time.Duration(hours)*time.Hour).Unix())
	return e
}
