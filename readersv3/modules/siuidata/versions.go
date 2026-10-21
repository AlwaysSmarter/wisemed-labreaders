package siuidata

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func (m *Module) versionsHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request, db *pgxpool.Pool) {

	f, e := parseFilters(r.URL.Query())
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	limit, start, e := pagination(f)
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	args := []any{}
	where := "true"
	categories := f["partnerCategory"]
	if len(categories) == 0 {
		categories = f["type"]
	}
	for i, v := range categories {
		categories[i], e = knownPartnerCategory(v)
		if e != nil {
			failure(w, 400, e.Error())
			return
		}
	}
	if len(categories) > 0 {
		args = append(args, categories)
		where = "i.catalogue_type=ANY($1::text[])"
	}
	tx, e := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		failure(w, 503, "PostgreSQL busy")
		return
	}
	defer tx.Rollback(context.Background())
	var total int64
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM siui_data_imports i WHERE "+where, args...).Scan(&total); e != nil {
		failure(w, 503, "cannot count versions")
		return
	}
	args = append(args, limit, start)
	rows, e := tx.Query(ctx, fmt.Sprintf(`SELECT jsonb_build_object('importId',i.id,'type',i.catalogue_type,'sourceDate',i.source_date,'sourceFilename',i.source_filename,'sourceKind',i.source_kind,'status',i.status,'state',CASE WHEN i.status IN ('ready','unchanged') THEN 'success' WHEN i.status='failed' THEN 'fail' ELSE 'running' END,'createdAt',i.created_at,'finishedAt',i.finished_at,'asOf',i.as_of,'records',i.record_count,'services',i.service_count,'drugs',i.drug_count,'physicians',i.physician_count,'sourceDates',i.source_dates,'drugProjectionReady',i.drug_projection_version>0,'statistics',i.statistics,'error',i.error,'active',EXISTS(SELECT 1 FROM siui_data_active a WHERE a.import_id=i.id)) FROM siui_data_imports i WHERE %s ORDER BY created_at DESC,id LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args)), args...)
	if e != nil {
		failure(w, 503, "cannot read import versions")
		return
	}
	defer rows.Close()
	items := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			break
		}
		items = append(items, b)
	}
	if e != nil || rows.Err() != nil {
		failure(w, 503, "cannot read import versions")
		return
	}
	rows.Close()
	// Read across the complete history, independent of the displayed page limit.
	checks, e := tx.Query(ctx, `SELECT catalogue_type,max(created_at) FROM siui_data_imports WHERE source_kind='wsm' AND status IN ('ready','unchanged') GROUP BY catalogue_type`)
	if e != nil {
		failure(w, 503, "cannot read last catalogue checks")
		return
	}
	defer checks.Close()
	lastCheckedAt := map[string]time.Time{}
	for checks.Next() {
		var category string
		var checked time.Time
		if e = checks.Scan(&category, &checked); e != nil {
			break
		}
		lastCheckedAt[category] = checked
	}
	if e != nil || checks.Err() != nil {
		failure(w, 503, "cannot read last catalogue checks")
		return
	}
	listOutput(w, 200, items, total, map[string]any{"limit": limit, "start": start, "lastCheckedAt": lastCheckedAt})
}
