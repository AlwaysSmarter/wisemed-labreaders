package siui

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"wisemed-labreaders/readersv3/shared/history"
)

var downloadStorageMu sync.RWMutex

func datedDownloadPath(database, id string, at time.Time) string {
	return filepath.Join(filepath.Dir(database), "siui-downloads", at.Format("2006"), at.Format("01"), at.Format("2006-01-02"), id)
}
func existingDownloadPath(database, id string) string {
	root := filepath.Join(filepath.Dir(database), "siui-downloads")
	if !history.ValidID(id) {
		return filepath.Join(root, "invalid")
	}
	old := filepath.Join(root, id)
	if _, err := os.Stat(old); err == nil {
		return old
	}
	matches, _ := filepath.Glob(filepath.Join(root, "*", "*", "*", id))
	if len(matches) == 1 {
		return matches[0]
	}
	return old
}

type storageTotals struct {
	Bytes          int64    `json:"bytes"`
	Files          int      `json:"files"`
	ArchivedBytes  int64    `json:"archived_bytes"`
	ArchivedFiles  int      `json:"archived_files"`
	ProtectedBytes int64    `json:"protected_bytes"`
	RemovedBytes   int64    `json:"removed_bytes"`
	RemovedFiles   int      `json:"removed_files"`
	Errors         []string `json:"errors"`
}

func (m *Module) downloadableArchive(id string, size int64) (string, bool) {
	var manifest string
	err := m.db.QueryRow(`SELECT o.manifest FROM siui_history_outbox o JOIN siui_history_calls_local c ON c.id=o.call_id WHERE o.call_id=? AND o.status='archived' AND c.status IN ('completed','failed','unknown') AND NOT EXISTS (SELECT 1 FROM siui_wm_callbacks w WHERE w.call_id=o.call_id AND w.status<>'delivered')`, id).Scan(&manifest)
	if err != nil {
		return "", false
	}
	var rec history.Record
	if json.Unmarshal([]byte(manifest), &rec) != nil {
		return "", false
	}
	for _, a := range rec.Artifacts {
		if a.Name == "download" && a.Size == size && len(a.SHA256) == 64 {
			return a.SHA256, true
		}
	}
	return "", false
}
func (m *Module) downloadStorageHTTP(w http.ResponseWriter, r *http.Request, admin bool) {
	if !admin {
		fail(w, 403, "administrator required")
		return
	}
	if r.Method != "GET" && r.Method != "POST" {
		fail(w, 405, "GET or POST required")
		return
	}
	select {
	case <-m.ready:
	default:
		fail(w, 503, "SIUI starting")
		return
	}
	clean := r.Method == "POST"
	if clean {
		var in struct {
			Confirm bool `json:"confirm"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in) != nil || !in.Confirm {
			fail(w, 400, "confirm=true required")
			return
		}
	}
	if !downloadStorageMu.TryLock() {
		fail(w, 409, "files currently in use; retry shortly")
		return
	}
	defer downloadStorageMu.Unlock()
	m.mu.Lock()
	database := m.cfg.Database
	m.mu.Unlock()
	root := filepath.Join(filepath.Dir(database), "siui-downloads")
	totals := storageTotals{Errors: []string{}}
	var migrations []func()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		id := d.Name()
		totals.Files++
		totals.Bytes += info.Size()
		hash, eligible := m.downloadableArchive(id, info.Size())
		if eligible {
			totals.ArchivedFiles++
			totals.ArchivedBytes += info.Size()
		} else {
			totals.ProtectedBytes += info.Size()
		}
		if clean && eligible {
			actual, e := hashHistoryFile(path, "download")
			if e != nil || actual.SHA256 != hash {
				totals.Errors = append(totals.Errors, id+": checksum mismatch or file unreadable")
				return nil
			}
			if _, ok := m.downloadableArchive(id, info.Size()); !ok {
				return nil
			}
			if e = os.Remove(path); e != nil {
				totals.Errors = append(totals.Errors, id+": "+e.Error())
				return nil
			}
			totals.RemovedFiles++
			totals.RemovedBytes += info.Size()
			return nil
		}
		// Migrate legacy flat files lazily; readers use the same lock and support both layouts.
		if filepath.Dir(path) == root && history.ValidID(id) {
			migrations = append(migrations, func() {
				at := info.ModTime()
				var created string
				if m.db.QueryRow(`SELECT created_at FROM siui_history_calls_local WHERE id=?`, id).Scan(&created) == nil {
					if t, e := time.Parse(time.RFC3339Nano, created); e == nil {
						at = t.UTC()
					}
				}
				dst := datedDownloadPath(database, id, at)
				if _, e := os.Lstat(dst); errors.Is(e, os.ErrNotExist) {
					if e = os.MkdirAll(filepath.Dir(dst), 0700); e == nil {
						e = os.Rename(path, dst)
					}
					if e != nil {
						totals.Errors = append(totals.Errors, id+": migration failed")
					}
				}
			})
		}
		return nil
	})
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	for _, migrate := range migrations {
		migrate()
	}
	respond(w, 200, map[string]any{"success": true, "storage": totals, "percent_archived": percentBytes(totals.ArchivedBytes, totals.Bytes), "root": root})
}
func percentBytes(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) * 100 / float64(total)
}
