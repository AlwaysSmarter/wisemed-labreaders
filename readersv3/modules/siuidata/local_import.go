package siuidata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var localFilename = regexp.MustCompile(`(?i)^Nomenclatoare(Clinice|Paraclinice|Farmacii|PHM)_?([0-9]{8})\.xml(?:\.zip)?$`)

func sourceFilename(name string) (typ, day string) {
	parts := localFilename.FindStringSubmatch(filepath.Base(name))
	if p := physicianFilename.FindStringSubmatch(filepath.Base(name)); p != nil {
		if d, e := time.Parse("20060102", p[2]); e == nil {
			return "PHYS", d.Format("2006-01-02")
		}
	}
	if parts == nil {
		return "", ""
	}
	d, e := time.Parse("20060102", parts[2])
	if e != nil {
		return "", ""
	}
	typ = "CLIN"
	if strings.EqualFold(parts[1], "Farmacii") || strings.EqualFold(parts[1], "PHM") {
		typ = "PHM"
	}
	if strings.EqualFold(parts[1], "Paraclinice") {
		typ = "PARA"
	}
	return typ, d.Format("2006-01-02")
}
func filenameSourceDate(name, typ string) *string {
	kind, day := sourceFilename(name)
	if kind != typ || day == "" {
		return nil
	}
	return &day
}

type localCandidate struct {
	path, typ, day, signature string
	info                      os.FileInfo
	companion                 *localCandidate
}
type localScanner struct{ observed, attempted map[string]string }

func newLocalScanner() *localScanner { return &localScanner{map[string]string{}, map[string]string{}} }

func (m *Module) watchLocal(ctx context.Context) {
	scanner := newLocalScanner()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	next := time.Time{}
	lastError := ""
	for {
		m.cfgMu.RLock()
		c := m.cfg
		m.cfgMu.RUnlock()
		if c.LocalDir != "" && !time.Now().Before(next) {
			e := scanner.scan(ctx, m, c)
			if e != nil && e.Error() != lastError {
				m.rt.Logf("siui-data local import: %v", e)
				lastError = e.Error()
			}
			if e == nil {
				lastError = ""
			}
			next = time.Now().Add(time.Duration(c.LocalPollSeconds) * time.Second)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *localScanner) scan(ctx context.Context, m *Module, c settings) error {
	dir := m.rt.ResolvePath(c.LocalDir)
	entries, e := os.ReadDir(dir)
	if e != nil {
		return errors.New("cannot read configured local import directory")
	}
	// The SIUI physician export is commonly kept beside the other catalogues
	// in this named subdirectory; do not recursively traverse arbitrary trees.
	type localEntry struct {
		entry os.DirEntry
		dir   string
	}
	files := []localEntry{}
	for _, entry := range entries {
		files = append(files, localEntry{entry, dir})
	}
	physicianDir := filepath.Join(dir, "getPhysicianCatalogues")
	if info, e := os.Lstat(physicianDir); e == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		nested, e := os.ReadDir(physicianDir)
		if e != nil {
			return errors.New("cannot read local getPhysicianCatalogues directory")
		}
		for _, entry := range nested {
			files = append(files, localEntry{entry, physicianDir})
		}
	}
	latest := map[string]localCandidate{}
	physicianFiles := map[string]map[string]localCandidate{}
	current := map[string]string{}
	for _, file := range files {
		entry := file.entry
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(strings.ToLower(entry.Name()), ".xml") {
			continue
		}
		typ, day := sourceFilename(entry.Name())
		if typ == "" {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		path := filepath.Join(file.dir, entry.Name())
		signature := fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
		current[path] = signature
		candidate := localCandidate{path: path, typ: typ, day: day, signature: signature, info: info}
		if typ == "PHYS" {
			if physicianFiles[day] == nil {
				physicianFiles[day] = map[string]localCandidate{}
			}
			physicianFiles[day][physicianSource(entry.Name())] = candidate
			continue
		}
		if prior, ok := latest[typ]; !ok || candidate.day > prior.day {
			latest[typ] = candidate
		}
	}
	for day, files := range physicianFiles {
		cn, cnOK := files["CNAS"]
		op, opOK := files["OPSNAJ"]
		if !cnOK || !opOK {
			continue
		}
		cn.companion = &op
		cn.signature += "|" + op.signature
		current[cn.path] = cn.signature
		if prev, ok := latest["PHYS"]; !ok || day > prev.day {
			latest["PHYS"] = cn
		}
	}
	for _, candidate := range latest {
		// Require unchanged size/mtime across two polls before opening the file.
		if s.observed[candidate.path] != candidate.signature || s.attempted[candidate.path] == candidate.signature {
			continue
		}
		started, e := m.startLocalImport(ctx, c, candidate)
		if e != nil {
			return e
		}
		if started {
			s.attempted[candidate.path] = candidate.signature
		}
	}
	s.observed = current
	return nil
}

func (m *Module) startLocalImport(ctx context.Context, c settings, source localCandidate) (bool, error) {
	m.workMu.Lock()
	if m.closing || m.running >= 2 {
		m.workMu.Unlock()
		return false, nil
	}
	m.running++
	m.workWG.Add(1)
	m.workMu.Unlock()
	transferred := false
	defer func() {
		if !transferred {
			m.finishWork()
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	db, e := m.database(ctx)
	if e != nil {
		return false, e
	}
	conn, e := db.Acquire(ctx)
	if e != nil {
		return false, e
	}
	key := catalogueLockKey(source.typ)
	var locked bool
	if e = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(1936288117,$1)`, key).Scan(&locked); e != nil || !locked {
		conn.Release()
		return false, e
	}
	defer func() {
		if !transferred {
			releaseImportConn(conn, key)
		}
	}()
	var newest *string
	if e = conn.QueryRow(ctx, `SELECT max(source_date)::text FROM siui_data_imports WHERE catalogue_type=$1 AND status='ready'`, source.typ).Scan(&newest); e != nil {
		return false, e
	}
	if newest != nil && source.day <= *newest {
		return false, nil
	}
	_, e = conn.Exec(ctx, `UPDATE siui_data_imports SET status='failed',finished_at=now(),error='worker interrupted before activation' WHERE catalogue_type=$1 AND status IN ('queued','parsing','downloading')`, source.typ)
	id := uuid.NewString()
	asOf := time.Now().In(m.loc).Format("2006-01-02")
	if e == nil {
		_, e = conn.Exec(ctx, `INSERT INTO siui_data_imports(id,catalogue_type,status,as_of,source_date,source_filename,source_kind) VALUES($1,$2,'queued',$3,$4,$5,'local')`, id, source.typ, asOf, source.day, filepath.Base(source.path))
	}
	if e != nil {
		return false, e
	}
	transferred = true
	go func() {
		defer m.finishWork()
		defer releaseImportConn(conn, key)
		ctx, cancel := context.WithTimeout(m.workCtx, time.Duration(c.ImportMinutes)*time.Minute)
		defer cancel()
		filename, e := m.stageLocal(ctx, c, source, id, conn)
		if filename != "" {
			defer os.Remove(filename)
		}
		if e == nil {
			e = runImport(ctx, conn, id, source.typ, asOf, filename, c.MaxXMLMiB<<20)
		}
		if e != nil {
			m.rt.Logf("siui-data local import %s failed: %v", id, e)
			m.markImportFailed(id, e)
		}
	}()
	return true, nil
}

func (m *Module) markImportFailed(id string, reason error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m.dbMu.Lock()
	db := m.db
	m.dbMu.Unlock()
	if db == nil {
		return
	}
	message := reason.Error()
	if len(message) > 2000 {
		message = message[:2000]
	}
	_, _ = db.Exec(ctx, `UPDATE siui_data_imports SET status='failed',finished_at=now(),error=$2 WHERE id=$1 AND status<>'ready'`, id, message)
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(p)
}
func (m *Module) stageLocal(ctx context.Context, c settings, source localCandidate, id string, conn *pgxpool.Conn) (string, error) {
	if source.typ == "PHYS" {
		return m.stagePhysiciansLocal(ctx, c, source, id, conn)
	}
	f, e := os.Open(source.path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	before, e := f.Stat()
	if e != nil {
		return "", e
	}
	if before.Size() != source.info.Size() || !before.ModTime().Equal(source.info.ModTime()) || !os.SameFile(before, source.info) {
		return "", errors.New("local source changed before import")
	}
	if before.Size() > c.MaxXMLMiB<<20 {
		return "", errors.New("local XML exceeds configured limit")
	}
	dir := m.rt.ResolvePath(c.UploadDir)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	tmp, e := os.CreateTemp(dir, "local-*.xml")
	if e != nil {
		return "", e
	}
	defer tmp.Close()
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(contextReader{ctx, f}, (c.MaxXMLMiB<<20)+1))
	if e == nil && n > c.MaxXMLMiB<<20 {
		e = errors.New("local XML exceeds configured limit")
	}
	if e != nil {
		return tmp.Name(), e
	}
	after, e := os.Stat(source.path)
	if e != nil {
		return tmp.Name(), e
	}
	if n != before.Size() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || !os.SameFile(before, after) {
		return tmp.Name(), errors.New("local source changed during staging; write a completed file with atomic rename")
	}
	if e = tmp.Sync(); e != nil {
		return tmp.Name(), e
	}
	if e = tmp.Close(); e != nil {
		return tmp.Name(), e
	}
	_, e = conn.Exec(ctx, `UPDATE siui_data_imports SET source_sha256=$2 WHERE id=$1`, id, hex.EncodeToString(h.Sum(nil)))
	return tmp.Name(), e
}
