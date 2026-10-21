package siuidata

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Publish the pair as one version, never replacing the other source with a
// partially copied local file. Original member names preserve provenance.
func (m *Module) stagePhysiciansLocal(ctx context.Context, c settings, source localCandidate, id string, conn *pgxpool.Conn) (string, error) {
	if source.companion == nil {
		return "", errors.New("local physician import requires CNAS and OPSNAJ files with the same filename date")
	}
	dir := m.rt.ResolvePath(c.UploadDir)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	tmp, e := os.CreateTemp(dir, "physicians-*.zip")
	if e != nil {
		return "", e
	}
	defer tmp.Close()
	h := sha256.New()
	z := zip.NewWriter(io.MultiWriter(tmp, h))
	defer z.Close()
	var total int64
	for _, part := range []localCandidate{source, *source.companion} {
		e = func() error {
			f, e := os.Open(part.path)
			if e != nil {
				return e
			}
			defer f.Close()
			before, e := f.Stat()
			if e != nil {
				return e
			}
			if !os.SameFile(before, part.info) || before.Size() != part.info.Size() || !before.ModTime().Equal(part.info.ModTime()) {
				return errors.New("physician source changed before staging")
			}
			w, e := z.Create(filepath.Base(part.path))
			if e != nil {
				return e
			}
			n, e := io.Copy(w, io.LimitReader(contextReader{ctx, f}, (c.MaxXMLMiB<<20)-total+1))
			total += n
			if e != nil {
				return e
			}
			if total > c.MaxXMLMiB<<20 {
				return errors.New("physician XML exceeds configured limit")
			}
			after, e := os.Stat(part.path)
			if e != nil {
				return e
			}
			if n != before.Size() || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
				return errors.New("physician source changed during staging")
			}
			return nil
		}()
		if e != nil {
			return tmp.Name(), e
		}
	}
	if e = z.Close(); e != nil {
		return tmp.Name(), e
	}
	if e = tmp.Sync(); e != nil {
		return tmp.Name(), e
	}
	_, e = conn.Exec(ctx, `UPDATE siui_data_imports SET source_sha256=$2 WHERE id=$1`, id, hex.EncodeToString(h.Sum(nil)))
	return tmp.Name(), e
}
