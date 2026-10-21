package siuidata

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (m *Module) uploadHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		failure(w, 405, "POST required")
		return
	}
	_, admin := m.identity(r)
	if !admin {
		failure(w, 403, "administrator required")
		return
	}
	// Large files use HTTPS directly; the WSM adapter has a much smaller body limit.
	if r.TLS == nil {
		failure(w, 400, "upload requires direct HTTPS")
		return
	}
	m.workMu.Lock()
	if m.closing || m.running >= 2 {
		m.workMu.Unlock()
		failure(w, 409, "upload capacity busy; retry later")
		return
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
	m.cfgMu.RLock()
	cfg := m.cfg
	m.cfgMu.RUnlock()
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.ImportMinutes)*time.Minute)
	defer cancel()
	closeBody := context.AfterFunc(ctx, func() { r.Body.Close() })
	defer closeBody()
	stopBody := context.AfterFunc(m.workCtx, func() { r.Body.Close() })
	defer stopBody()
	r = r.WithContext(ctx)
	r.Body = http.MaxBytesReader(w, r.Body, (cfg.MaxUploadMiB<<20)+(1<<20))
	dir := m.rt.ResolvePath(cfg.UploadDir)
	if e := os.MkdirAll(dir, 0700); e != nil {
		failure(w, 500, "cannot create upload staging directory")
		return
	}
	f, e := os.CreateTemp(dir, "catalogue-*.zip")
	if e != nil {
		failure(w, 500, "cannot create upload staging file")
		return
	}
	defer func() {
		f.Close()
		if !transferred {
			os.Remove(f.Name())
		}
	}()
	hash := sha256.New()
	copyFile := func(src io.Reader) error {
		n, e := io.Copy(io.MultiWriter(f, hash), io.LimitReader(src, (cfg.MaxUploadMiB<<20)+1))
		if n > cfg.MaxUploadMiB<<20 {
			return errors.New("ZIP exceeds configured upload limit")
		}
		return e
	}
	filters, filterErr := parseFilters(r.URL.Query())
	if filterErr != nil {
		failure(w, 400, filterErr.Error())
		return
	}
	typ, filterErr := filters.one("type")
	if filterErr != nil {
		failure(w, 400, filterErr.Error())
		return
	}
	sourceName := r.Header.Get("X-Catalogue-Filename")
	media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil {
		failure(w, 400, "invalid Content-Type")
		return
	}
	if media == "multipart/form-data" {
		reader, e := r.MultipartReader()
		if e != nil {
			failure(w, 400, "invalid multipart upload")
			return
		}
		files, fields := 0, 0
		for {
			part, e := reader.NextPart()
			if e == io.EOF {
				break
			}
			if e != nil {
				failure(w, 400, "invalid multipart upload")
				return
			}
			fields++
			if fields > 8 {
				part.Close()
				failure(w, 400, "too many upload fields")
				return
			}
			switch part.FormName() {
			case "type":
				b, e := io.ReadAll(io.LimitReader(part, 17))
				if e != nil || len(b) > 16 {
					part.Close()
					failure(w, 400, "invalid catalogue type")
					return
				}
				if typ != "" && !strings.EqualFold(strings.TrimSpace(typ), strings.TrimSpace(string(b))) {
					part.Close()
					failure(w, 400, "conflicting catalogue types")
					return
				}
				typ = string(b)
			case "file":
				sourceName = part.FileName()
				files++
				if files != 1 {
					part.Close()
					failure(w, 400, "exactly one ZIP file required")
					return
				}
				if e = copyFile(part); e != nil {
					part.Close()
					failure(w, 400, e.Error())
					return
				}
			default:
				part.Close()
				failure(w, 400, "expected only type and file fields")
				return
			}
			part.Close()
		}
		if files != 1 {
			failure(w, 400, "ZIP file required")
			return
		}
	} else if media == "application/zip" || media == "application/octet-stream" {
		if e = copyFile(r.Body); e != nil {
			failure(w, 400, e.Error())
			return
		}
	} else {
		failure(w, 415, "multipart/form-data or application/zip required")
		return
	}
	typ, e = catalogueType(typ)
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	if sourceType, _ := sourceFilename(sourceName); sourceType != "" && sourceType != typ {
		failure(w, 400, "filename does not match catalogue type")
		return
	}
	if sourceName == "" {
		sourceName = "uploaded.zip"
	}
	if e = f.Sync(); e != nil {
		failure(w, 500, "cannot persist staged upload")
		return
	}
	info, e := f.Stat()
	if e != nil {
		failure(w, 500, "cannot inspect upload")
		return
	}
	if _, e = archiveMembers(f, info.Size(), cfg.MaxXMLMiB<<20); e != nil {
		failure(w, 400, e.Error())
		return
	}
	dbctx, dbCancel := context.WithTimeout(ctx, 15*time.Second)
	defer dbCancel()
	db, e := m.database(dbctx)
	if e != nil {
		failure(w, 503, e.Error())
		return
	}
	conn, e := db.Acquire(dbctx)
	if e != nil {
		failure(w, 503, "PostgreSQL busy")
		return
	}
	locked := false
	lockKey := catalogueLockKey(typ)
	if e = conn.QueryRow(dbctx, `SELECT pg_try_advisory_lock(1936288117,$1)`, lockKey).Scan(&locked); e != nil || !locked {
		conn.Release()
		failure(w, 409, "an import of this catalogue type is already running")
		return
	}
	// The advisory lock proves that any earlier unfinished job lost its worker.
	_, e = conn.Exec(dbctx, `UPDATE siui_data_imports SET status='failed',finished_at=now(),error='worker interrupted before activation' WHERE catalogue_type=$1 AND status IN ('queued','parsing','downloading')`, typ)
	id := uuid.NewString()
	asOf := time.Now().In(m.loc).Format("2006-01-02")
	if e == nil {
		_, e = conn.Exec(dbctx, `INSERT INTO siui_data_imports(id,catalogue_type,status,as_of,source_sha256,source_filename,source_date) VALUES($1,$2,'queued',$3,$4,$5,$6)`, id, typ, asOf, hex.EncodeToString(hash.Sum(nil)), path.Base(sourceName), filenameSourceDate(sourceName, typ))
	}
	if e != nil {
		releaseImportConn(conn, lockKey)
		failure(w, 500, "cannot create import job")
		return
	}
	filename := f.Name()
	if e = f.Close(); e != nil {
		releaseImportConn(conn, lockKey)
		failure(w, 500, "cannot close staged archive")
		return
	}
	transferred = true
	go func() {
		defer m.finishWork()
		defer os.Remove(filename)
		defer releaseImportConn(conn, lockKey)
		ctx, cancel := context.WithTimeout(m.workCtx, time.Duration(cfg.ImportMinutes)*time.Minute)
		defer cancel()
		if e := runImport(ctx, conn, id, typ, asOf, filename, cfg.MaxXMLMiB<<20); e != nil {
			m.rt.Logf("siui-data import %s failed: %v", id, e)
			m.markImportFailed(id, e)
		}
	}()
	w.Header().Set("Location", prefix+"imports/"+id)
	output(w, 202, map[string]any{"importId": id, "type": typ, "status": "queued", "statusUrl": prefix + "imports/" + id})
}

func (m *Module) finishWork() { m.workMu.Lock(); m.running--; m.workMu.Unlock(); m.workWG.Done() }
func releaseImportConn(conn *pgxpool.Conn, key int32) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var unlocked bool
	if e := conn.QueryRow(ctx, `SELECT pg_advisory_unlock(1936288117,$1)`, key).Scan(&unlocked); e != nil || !unlocked {
		conn.Conn().Close(ctx)
	}
	conn.Release()
}

func archiveMembers(f *os.File, size, maxXML int64) ([]*zip.File, error) {
	z, e := zip.NewReader(f, size)
	if e != nil {
		return nil, errors.New("invalid ZIP archive")
	}
	if len(z.File) > 128 {
		return nil, errors.New("ZIP contains too many entries (maximum 128)")
	}
	var members []*zip.File
	var total uint64
	for _, entry := range z.File {
		if entry.FileInfo().IsDir() || strings.HasPrefix(entry.Name, "__MACOSX/") || path.Base(entry.Name) == ".DS_Store" {
			continue
		}
		if path.IsAbs(entry.Name) || strings.Contains(entry.Name, "\\") || path.Clean(entry.Name) != entry.Name || strings.HasPrefix(entry.Name, "../") {
			return nil, errors.New("invalid ZIP entry path")
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name), ".xml") {
			return nil, errors.New("ZIP may contain only XML catalogue files")
		}
		if entry.UncompressedSize64 > uint64(maxXML)-total {
			return nil, errors.New("expanded XML exceeds configured limit")
		}
		total += entry.UncompressedSize64
		members = append(members, entry)
	}
	if len(members) == 0 {
		return nil, errors.New("ZIP contains no XML catalogue files")
	}
	return members, nil
}

func runImport(ctx context.Context, conn *pgxpool.Conn, id, typ, asOf, filename string, maxXML int64) error {
	_, e := conn.Exec(ctx, `UPDATE siui_data_imports SET status='parsing' WHERE id=$1`, id)
	if e != nil {
		return e
	}
	f, e := os.Open(filename)
	if e != nil {
		return e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return e
	}
	type xmlMember struct {
		Name string
		Open func() (io.ReadCloser, error)
	}
	var members []xmlMember
	if strings.HasSuffix(filename, ".xml") {
		if info.Size() > maxXML {
			return errors.New("XML exceeds configured limit")
		}
		members = append(members, xmlMember{path.Base(filename), func() (io.ReadCloser, error) { return io.NopCloser(io.NewSectionReader(f, 0, info.Size())), nil }})
	} else {
		entries, e := archiveMembers(f, info.Size(), maxXML)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			members = append(members, xmlMember{entry.Name, entry.Open})
		}
	}
	tx, e := conn.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		tx.Rollback(c)
	}()
	if _, e = tx.Exec(ctx, `SET LOCAL statement_timeout=0`); e != nil {
		return e
	}
	var ordinal, expanded int64
	seenMain := false
	issueDate := ""
	sourceDates := map[string]string{}
	sourcesSeen := map[string]bool{}
	for _, member := range members {
		sourceName := ""
		if typ == "PHYS" {
			sourceName = physicianSource(member.Name)
			if sourceName == "" || sourcesSeen[sourceName] {
				return errors.New("PHYS requires distinct MediciCNAS_YYYYMMDD.xml / MediciOPSNAJ_YYYYMMDD.xml members")
			}
			sourcesSeen[sourceName] = true
		}
		r, e := member.Open()
		if e != nil {
			return e
		}
		limited := &io.LimitedReader{R: r, N: maxXML - expanded + 1}
		source := newCatalogueRows(ctx, limited, models[typ], asOf, &ordinal)
		_, e = tx.CopyFrom(ctx, pgx.Identifier{"siui_data_records"}, []string{"import_id", "ordinal", "kind", "container", "code", "name_normalized", "attributes", "payload", "valid_from", "valid_to", "active", "source"}, pgx.CopyFromFunc(func() ([]any, error) {
			if !source.Next() {
				return nil, source.Err()
			}
			v, e := source.Values()
			if e != nil {
				return nil, e
			}
			return append(append([]any{pgtype.UUID{Bytes: uuid.MustParse(id), Valid: true}}, v...), sourceName), nil
		}))
		r.Close()
		expanded += source.bytes.n
		if limited.N <= 0 || expanded > maxXML {
			return errors.New("expanded XML exceeds configured limit")
		}
		if e != nil {
			return fmt.Errorf("parse %s: %w", member.Name, e)
		}
		if typ == "PHYS" && !source.seen[models[typ].Container] {
			return fmt.Errorf("%s has no Physicians container", member.Name)
		}
		seenMain = seenMain || source.seen[models[typ].Container]
		if source.issueDate != "" {
			if issueDate != "" && issueDate != source.issueDate && typ != "PHYS" {
				return errors.New("XML files have different issue dates")
			}
			if source.issueDate > issueDate {
				issueDate = source.issueDate
			}
			if sourceName != "" {
				sourceDates[sourceName] = source.issueDate
			}
		}
	}
	if !seenMain {
		return errors.New("archive does not contain the main catalogue for the selected type")
	}
	if _, e = tx.Exec(ctx, `ANALYZE siui_data_records`); e != nil {
		return e
	}
	var physicians int64
	if typ == "PHYS" {
		if !sourcesSeen["CNAS"] || !sourcesSeen["OPSNAJ"] {
			return errors.New("PHYS import requires both CNAS and OPSNAJ files to publish a complete snapshot")
		}
		physicians, e = buildPhysicians(ctx, tx, id)
		if e != nil {
			return e
		}
	}
	count, e := buildServices(ctx, tx, id, typ)
	if e != nil {
		return e
	}
	drugs, e := buildDrugs(ctx, tx, id, typ)
	if e != nil {
		return fmt.Errorf("drug graph: %w", e)
	}
	var declaredDate *string
	if e = tx.QueryRow(ctx, `SELECT source_date::text FROM siui_data_imports WHERE id=$1`, id).Scan(&declaredDate); e != nil {
		return e
	}
	if declaredDate != nil && issueDate != "" && *declaredDate != issueDate && typ != "PHYS" {
		return errors.New("filename date does not match XML issueDate")
	}
	if _, e = tx.Exec(ctx, `UPDATE siui_data_imports SET status='ready',finished_at=now(),record_count=$2,service_count=$3,drug_count=$5,physician_count=$6,source_dates=$7,drug_projection_version=1,source_date=COALESCE(source_date,NULLIF($4,'')::date),statistics=(SELECT COALESCE(jsonb_object_agg(kind,counts),'{}'::jsonb) FROM (SELECT kind,jsonb_build_object('total',count(*),'active',count(*) FILTER(WHERE active),'invalidPeriods',count(*) FILTER(WHERE valid_to<valid_from)) AS counts FROM siui_data_records WHERE import_id=$1 GROUP BY kind) c) WHERE id=$1`, id, ordinal, count, issueDate, drugs, physicians, sourceDates); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO siui_data_active(catalogue_type,import_id) VALUES($1,$2) ON CONFLICT(catalogue_type) DO UPDATE SET import_id=excluded.import_id`, typ, id); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
