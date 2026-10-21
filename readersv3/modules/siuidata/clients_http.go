package siuidata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
)

func (m *Module) clientsHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !m.internalBrowser(r) {
		failure(w, 403, "local administrator browser session required")
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		if r.TLS == nil {
			failure(w, 400, "HTTPS required")
			return
		}
		if r.Header.Get("X-SIUI-Internal") != "1" {
			failure(w, 403, "internal request header required")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host != r.Host || u.Scheme != "https" {
				failure(w, 403, "origin rejected")
				return
			}
		}
	}
	switch r.URL.Path {
	case "/siui-data-clients":
		if r.Method != "GET" {
			failure(w, 405, "GET required")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, clientsPage)
	case "/siui-data-clients/data":
		if r.Method != "GET" {
			failure(w, 405, "GET required")
			return
		}
		m.clientOverviewHTTP(w, r)
	case "/siui-data-clients/import":
		m.clientImportHTTP(w, r)
	case "/siui-data-clients/keys":
		m.clientKeysHTTP(w, r)
	case "/siui-data-clients/token":
		if r.Method != "POST" {
			failure(w, 405, "POST required")
			return
		}
		subject, admin := m.identity(r)
		m.issueTokenHTTP(w, r, subject, admin)
	default:
		failure(w, 404, "internal route not found")
	}
}

func (m *Module) clientImportHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		failure(w, 405, "POST required")
		return
	}
	m.workMu.Lock()
	if m.closing || m.running >= 2 {
		m.workMu.Unlock()
		failure(w, 409, "import capacity busy")
		return
	}
	m.running++
	m.workWG.Add(1)
	m.workMu.Unlock()
	defer m.finishWork()
	m.cfgMu.RLock()
	cfg := m.cfg
	m.cfgMu.RUnlock()
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.ImportMinutes)*time.Minute)
	defer cancel()
	stopBody := context.AfterFunc(ctx, func() { r.Body.Close() })
	defer stopBody()
	stop := context.AfterFunc(m.workCtx, cancel)
	defer stop()
	maxUpload := min(cfg.MaxUploadMiB<<20, int64(64<<20))
	maxXML := min(cfg.MaxXMLMiB<<20, int64(128<<20))
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	reader, e := r.MultipartReader()
	if e != nil {
		failure(w, 400, "multipart category and XML/ZIP file required")
		return
	}
	dir := m.rt.ResolvePath(cfg.UploadDir)
	if e = os.MkdirAll(dir, 0700); e != nil {
		failure(w, 500, "cannot create private staging directory")
		return
	}
	f, e := os.CreateTemp(dir, "personalization-*")
	if e != nil {
		failure(w, 500, "cannot create private staging file")
		return
	}
	defer f.Close()
	defer os.Remove(f.Name())
	category, name := "", ""
	files, fields := 0, 0
	for {
		part, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			failure(w, 400, "invalid multipart body")
			return
		}
		fields++
		if fields > 3 {
			part.Close()
			failure(w, 400, "too many fields")
			return
		}
		switch part.FormName() {
		case "category":
			b, e := io.ReadAll(io.LimitReader(part, 17))
			if e != nil || len(b) > 16 || category != "" {
				part.Close()
				failure(w, 400, "invalid category")
				return
			}
			category = string(b)
		case "file":
			files++
			if files != 1 {
				part.Close()
				failure(w, 400, "exactly one file required")
				return
			}
			name = filepath.Base(part.FileName())
			n, e := io.Copy(f, io.LimitReader(contextReader{ctx, part}, maxUpload+1))
			if e != nil || n > maxUpload {
				part.Close()
				failure(w, 400, "invalid or oversized personalization upload")
				return
			}
		default:
			part.Close()
			failure(w, 400, "expected category and file only")
			return
		}
		part.Close()
	}
	category, e = knownPartnerCategory(category)
	if e != nil || category == "PHYS" || files != 1 {
		failure(w, 400, "valid provider category and one file required")
		return
	}
	if _, e = f.Seek(0, io.SeekStart); e != nil {
		failure(w, 500, "cannot read staged file")
		return
	}
	type member struct {
		name string
		open func() (io.ReadCloser, error)
	}
	members := []member{}
	magic := make([]byte, 2)
	f.ReadAt(magic, 0)
	info, e := f.Stat()
	if e != nil {
		failure(w, 500, "cannot inspect file")
		return
	}
	if string(magic) == "PK" {
		entries, e := archiveMembers(f, info.Size(), maxXML)
		if e != nil {
			failure(w, 400, e.Error())
			return
		}
		for _, entry := range entries {
			members = append(members, member{filepath.Base(entry.Name), entry.Open})
		}
	} else {
		if info.Size() > maxXML {
			failure(w, 400, "personalization XML too large")
			return
		}
		members = append(members, member{name, func() (io.ReadCloser, error) { return io.NopCloser(io.NewSectionReader(f, 0, info.Size())), nil }})
	}
	db, e := m.database(ctx)
	if e != nil {
		failure(w, 503, "PostgreSQL unavailable")
		return
	}
	tx, e := db.Begin(ctx)
	if e != nil {
		failure(w, 503, "PostgreSQL busy")
		return
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1936288117,5)`); e != nil {
		failure(w, 503, "personalization import busy")
		return
	}
	results := []map[string]any{}
	var expanded int64
	for _, member := range members {
		src, e := member.open()
		if e != nil {
			failure(w, 400, "cannot read archive member")
			return
		}
		limited := &io.LimitedReader{R: src, N: maxXML - expanded + 1}
		doc, e := parseClientDocument(ctx, limited, category, member.name, m.loc)
		src.Close()
		used := maxXML - expanded + 1 - limited.N
		expanded += used
		if e != nil || expanded > maxXML {
			message := "expanded personalization limit exceeded"
			if e != nil {
				message = e.Error()
			}
			failure(w, 400, message)
			return
		}
		id, created, e := saveClientDocument(ctx, tx, doc)
		if e != nil {
			failure(w, 409, "personalization rejected: "+safeImportError(e))
			return
		}
		results = append(results, map[string]any{"id": id, "cui": doc.CUI, "category": category, "issueDate": doc.Effective, "created": created})
	}
	if e = tx.Commit(ctx); e != nil {
		failure(w, 500, "cannot commit personalizations")
		return
	}
	output(w, 200, map[string]any{"imports": results})
}
func safeImportError(e error) string {
	var pe interface{ SQLState() string }
	if errors.As(e, &pe) {
		return "invalid or conflicting document data"
	}
	return e.Error()
}

func queryDay(raw string, loc *time.Location) (string, error) {
	if raw == "" {
		return time.Now().In(loc).Format("2006-01-02"), nil
	}
	if len(raw) != 10 {
		return "", errors.New("atDate must be YYYY-MM-DD")
	}
	if _, e := time.Parse("2006-01-02", raw); e != nil {
		return "", errors.New("invalid atDate")
	}
	return raw, nil
}
func (m *Module) clientOverviewHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	f, e := parseFilters(r.URL.Query())
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	raw, e := f.one("atDate")
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	day, e := queryDay(raw, m.loc)
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	cui, e := f.one("cui")
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	db, e := m.database(ctx)
	if e != nil {
		failure(w, 503, "PostgreSQL unavailable")
		return
	}
	tx, e := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		failure(w, 503, "PostgreSQL busy")
		return
	}
	defer tx.Rollback(context.Background())
	if cui == "" {
		limit, start, e := pagination(f)
		if e != nil {
			failure(w, 400, e.Error())
			return
		}
		q, e := f.one("q")
		if e != nil {
			failure(w, 400, e.Error())
			return
		}
		pattern := likePattern(q)
		activeCategory, e := f.one("activeCategory")
		if e != nil {
			failure(w, 400, e.Error())
			return
		}
		if activeCategory != "" {
			activeCategory, e = knownPartnerCategory(activeCategory)
			if e != nil || activeCategory == "PHYS" {
				failure(w, 400, "invalid active contract category")
				return
			}
		}
		predicate := `(s.cui LIKE $1 OR siui_data_drug_fold(s.provider_name) LIKE $1) AND ($3::text='' OR EXISTS(
          SELECT 1 FROM siui_client_snapshots p JOIN siui_client_contracts c ON c.snapshot_id=p.id
          WHERE p.cui=s.cui AND c.category=$3 AND c.document->>'kind'='contract' AND c.valid_from<=$2::date AND (c.valid_to IS NULL OR c.valid_to>=$2::date)
          AND p.id=(SELECT n.id FROM siui_client_snapshots n WHERE n.cui=p.cui AND n.category=p.category AND n.provider_code=p.provider_code AND n.effective_at<(($2::date+1)::timestamp AT TIME ZONE 'Europe/Bucharest') ORDER BY n.effective_at DESC LIMIT 1)))`
		var total int64
		if e = tx.QueryRow(ctx, "SELECT count(DISTINCT s.cui) FROM siui_client_snapshots s WHERE "+predicate, pattern, day, activeCategory).Scan(&total); e != nil {
			failure(w, 503, "cannot count clients")
			return
		}
		rows, e := tx.Query(ctx, "SELECT s.cui,(array_agg(s.provider_name ORDER BY s.effective_at DESC))[1],max(s.effective_at),count(*),array_agg(DISTINCT s.category) FROM siui_client_snapshots s WHERE "+predicate+" GROUP BY s.cui ORDER BY s.cui LIMIT $4 OFFSET $5", pattern, day, activeCategory, limit, start)
		if e != nil {
			failure(w, 503, "cannot read clients")
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var cui, name string
			var at time.Time
			var count int64
			var categories []string
			if e = rows.Scan(&cui, &name, &at, &count, &categories); e != nil {
				break
			}
			items = append(items, map[string]any{"cui": cui, "name": name, "latestIssueDate": at, "versions": count, "categories": categories})
		}
		if e != nil || rows.Err() != nil {
			failure(w, 503, "cannot read clients")
			return
		}
		listOutput(w, 200, items, total, map[string]any{"limit": limit, "start": start})
		return
	}
	cui, e = normalizeCUI(cui)
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	rows, e := tx.Query(ctx, `WITH latest AS (`+clientLatestSQL+`) SELECT s.id::text,s.category,s.provider_code,s.provider_name,s.effective_at,s.statistics,
 COALESCE((SELECT jsonb_agg(c.document||jsonb_build_object('activeAtDate',c.valid_from<=$2::date AND (c.valid_to IS NULL OR c.valid_to>=$2::date),'normalizedCategory',c.category) ORDER BY c.ordinal) FROM siui_client_contracts c WHERE c.snapshot_id=s.id),'[]'::jsonb),
 COALESCE((SELECT jsonb_agg(jsonb_build_object('stencil',e.stencil,'name',e.name,'validFrom',e.valid_from,'validTo',e.valid_to,'activeAtDate',e.valid_from<=$2::date AND (e.valid_to IS NULL OR e.valid_to>=$2::date),'specialities',COALESCE((SELECT jsonb_agg(sp.attributes||jsonb_build_object('activeAtDate',sp.valid_from<=$2::date AND (sp.valid_to IS NULL OR sp.valid_to>=$2::date)) ORDER BY sp.ordinal) FROM siui_client_specialties sp WHERE sp.snapshot_id=e.snapshot_id AND sp.employee_ordinal=e.ordinal),'[]'::jsonb)) ORDER BY e.ordinal) FROM siui_client_employees e WHERE e.snapshot_id=s.id),'[]'::jsonb)
 FROM latest l JOIN siui_client_snapshots s ON s.id=l.id ORDER BY s.category,s.provider_code`, cui, day)
	if e != nil {
		failure(w, 503, "cannot read client profile")
		return
	}
	defer rows.Close()
	profiles := []map[string]any{}
	for rows.Next() {
		var id, category, code, name string
		var at time.Time
		var stats map[string]int
		var contracts, employees json.RawMessage
		if e = rows.Scan(&id, &category, &code, &name, &at, &stats, &contracts, &employees); e != nil {
			break
		}
		profiles = append(profiles, map[string]any{"id": id, "category": category, "providerCode": code, "providerName": name, "issueDate": at, "statistics": stats, "contracts": contracts, "employees": employees})
	}
	if e != nil || rows.Err() != nil {
		failure(w, 503, "cannot read client profile")
		return
	}
	rows.Close()
	codes, e := clientSpecialties(ctx, tx, cui, day)
	if e != nil {
		failure(w, 503, "cannot derive client profile")
		return
	}
	history, e := tx.Query(ctx, `SELECT id::text,category,provider_code,effective_at,imported_at,source_filename,statistics FROM siui_client_snapshots WHERE cui=$1 ORDER BY effective_at DESC,category,provider_code LIMIT 100`, cui)
	if e != nil {
		failure(w, 503, "cannot read personalization history")
		return
	}
	defer history.Close()
	versions := []map[string]any{}
	for history.Next() {
		var id, category, code, name string
		var at, imported time.Time
		var stats map[string]int
		if e = history.Scan(&id, &category, &code, &at, &imported, &name, &stats); e != nil {
			break
		}
		versions = append(versions, map[string]any{"id": id, "category": category, "providerCode": code, "issueDate": at, "importedAt": imported, "filename": name, "statistics": stats})
	}
	if e != nil || history.Err() != nil {
		failure(w, 503, "cannot read history")
		return
	}
	output(w, 200, map[string]any{"cui": cui, "atDate": day, "profiles": profiles, "clinicalSpecialities": codes, "versions": versions})
}
