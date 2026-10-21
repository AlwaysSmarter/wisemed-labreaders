package siuidata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func likePattern(s string) string {
	return "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(normalizeText(s)) + "%"
}

func (m *Module) queryHTTP(w http.ResponseWriter, r *http.Request, route string) {
	if r.Method != "GET" && r.Method != "HEAD" && !(r.Method == "DELETE" && strings.HasPrefix(route, "imports/")) {
		failure(w, 405, "method not supported")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	db, e := m.database(ctx)
	if e != nil {
		failure(w, 503, e.Error())
		return
	}
	if route == "versions" {
		m.versionsHTTP(ctx, w, r, db)
		return
	}
	if route == "status" {
		rows, e := db.Query(ctx, `SELECT a.catalogue_type,i.id::text,i.as_of::text,i.service_count,i.record_count,i.finished_at,i.source_sha256,i.source_date::text,i.drug_count,i.drug_projection_version,i.physician_count,i.source_dates FROM siui_data_active a JOIN siui_data_imports i ON i.id=a.import_id ORDER BY a.catalogue_type`)
		if e != nil {
			failure(w, 503, "cannot read catalogue status")
			return
		}
		defer rows.Close()
		types := map[string]any{}
		for rows.Next() {
			var typ, id, asOf, hash string
			var count, records, drugs int64
			var projection int
			var physicians int64
			var sourceDates map[string]string
			var at time.Time
			var sourceDate *string
			if e = rows.Scan(&typ, &id, &asOf, &count, &records, &at, &hash, &sourceDate, &drugs, &projection, &physicians, &sourceDates); e != nil {
				break
			}
			types[typ] = map[string]any{"ready": true, "importId": id, "asOf": asOf, "services": count, "drugs": drugs, "physicians": physicians, "sourceDates": sourceDates, "drugProjectionReady": projection > 0, "records": records, "importedAt": at, "sourceDate": sourceDate, "sourceSha256": hash, "stale": asOf != time.Now().In(m.loc).Format("2006-01-02")}
		}
		if e != nil || rows.Err() != nil {
			failure(w, 503, "cannot read catalogue status")
			return
		}
		m.metricsMu.RLock()
		metrics := m.metrics
		m.metricsMu.RUnlock()
		state := map[string]any{"ready": len(types) > 0, "catalogues": types, "process": metrics, "requests": m.requests.Load(), "uptimeSeconds": time.Since(m.started).Seconds(), "storage": "postgresql"}
		if clin, ok := types["CLIN"].(map[string]any); ok {
			for k, v := range clin {
				state[k] = v
			}
		}
		output(w, 200, state)
		return
	}
	if strings.HasPrefix(route, "imports/") {
		id := strings.TrimPrefix(route, "imports/")
		if _, e = uuid.Parse(id); e != nil {
			failure(w, 400, "invalid import id")
			return
		}
		if r.Method == "DELETE" {
			_, admin := m.identity(r)
			if !admin {
				failure(w, 403, "administrator required")
				return
			}
			result, e := db.Exec(ctx, `DELETE FROM siui_data_imports i WHERE i.id=$1 AND status IN ('ready','failed','unchanged') AND NOT EXISTS(SELECT 1 FROM siui_data_active a WHERE a.import_id=i.id)`, id)
			if e != nil {
				failure(w, 409, "cannot delete an active or busy import")
				return
			}
			if result.RowsAffected() == 0 {
				failure(w, 409, "import missing, active or still running")
				return
			}
			output(w, 200, map[string]any{})
			return
		}
		var data []byte
		e = db.QueryRow(ctx, `SELECT jsonb_build_object('importId',id,'type',catalogue_type,'status',status,'state',CASE WHEN status IN ('ready','unchanged') THEN 'success' WHEN status='failed' THEN 'fail' ELSE 'running' END,'createdAt',created_at,'finishedAt',finished_at,'sourceDate',source_date,'sourceFilename',source_filename,'sourceKind',source_kind,'statistics',statistics,'asOf',as_of,'records',record_count,'services',service_count,'drugs',drug_count,'physicians',physician_count,'sourceDates',source_dates,'drugProjectionReady',drug_projection_version>0,'sourceSha256',source_sha256,'error',error) FROM siui_data_imports WHERE id=$1`, id).Scan(&data)
		if errors.Is(e, pgx.ErrNoRows) {
			failure(w, 404, "import not found")
			return
		}
		if e != nil {
			failure(w, 503, "cannot read import")
			return
		}
		output(w, 200, json.RawMessage(data))
		return
	}

	f, e := parseFilters(r.URL.Query())
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	physicianRoute := route == "physicians" || route == "simple-physicians" || strings.HasPrefix(route, "physicians/") || route == "physician-catalogues" || strings.HasPrefix(route, "physician-records/")
	types := f["type"]
	if len(types) == 0 {
		types = f["partnerCategory"]
	}
	if len(types) == 0 {
		if physicianRoute {
			types = []string{"PHYS"}
		} else {
			types = []string{"CLIN"}
		}
	}
	if physicianRoute {
		for _, v := range types {
			if strings.ToUpper(v) != "PHYS" {
				failure(w, 400, "physician endpoints require type=PHYS")
				return
			}
		}
	}
	for i, value := range types {
		types[i], e = catalogueType(value)
		if e != nil {
			failure(w, 400, e.Error())
			return
		}
	}
	tx, e := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		failure(w, 503, "PostgreSQL busy")
		return
	}
	defer tx.Rollback(context.Background())
	target, _ := r.Context().Value(targetProfileKey{}).(*targetProfile)
	if target != nil {
		target.Specialties, e = clientSpecialties(ctx, tx, target.CUI, target.Day)
		if e != nil {
			failure(w, 503, "cannot apply client service profile")
			return
		}
		// Default deny also applies before a public CLIN catalogue exists.
		// No applicable client permissions must never fall back to all services.
		if len(target.Specialties) == 0 {
			limit, start, e := pagination(f)
			if e != nil {
				failure(w, 400, e.Error())
				return
			}
			listOutput(w, 200, []json.RawMessage{}, 0, map[string]any{"type": "CLIN", "atDate": target.Day, "limit": limit, "start": start})
			return
		}
	}
	snapshots, e := tx.Query(ctx, `SELECT i.id::text,i.catalogue_type,i.as_of::text FROM siui_data_active a JOIN siui_data_imports i ON i.id=a.import_id WHERE a.catalogue_type=ANY($1::text[]) ORDER BY a.catalogue_type`, types)
	if e != nil {
		failure(w, 503, "cannot select active catalogues")
		return
	}
	ids := []string{}
	versions := map[string]string{}
	dates := map[string]string{}
	for snapshots.Next() {
		var id, typ, day string
		if e = snapshots.Scan(&id, &typ, &day); e != nil {
			break
		}
		ids = append(ids, id)
		versions[typ] = id
		dates[typ] = day
	}
	snapshots.Close()
	if e != nil || snapshots.Err() != nil {
		failure(w, 503, "cannot select active catalogues")
		return
	}
	for _, typ := range types {
		if versions[typ] == "" {
			failure(w, 503, "catalogue type not yet imported: "+typ)
			return
		}
	}
	meta := map[string]any{"versions": versions, "asOfByType": dates}
	if target != nil {
		meta["atDate"] = target.Day
	}
	if len(versions) == 1 {
		for typ, id := range versions {
			meta["type"] = typ
			meta["importId"] = id
			meta["asOf"] = dates[typ]
			w.Header().Set("X-Catalogue-As-Of", dates[typ])
			w.Header().Set("X-Catalogue-Version", id)
		}
	}
	if route == "physicians" || route == "simple-physicians" || strings.HasPrefix(route, "physicians/") {
		m.physiciansHTTP(ctx, w, r, tx, route, f, ids, meta)
		return
	}
	if route == "physician-catalogues" {
		route = "catalogues"
	}
	if strings.HasPrefix(route, "physician-records/") {
		kind := strings.TrimPrefix(route, "physician-records/")
		if !containsFilter([]string{"Physician", "PhysicianSpeciality", "Speciality", "InsuranceHouse", "InsuranceHouseType"}, kind) {
			failure(w, 400, "unknown physician catalogue kind")
			return
		}
		route = "records/" + kind
	}
	if route == "drugs" || route == "simple-drugs" || route == "drug-compensations" || route == "drug-relations" || strings.HasPrefix(route, "drugs/") {
		m.drugsHTTP(ctx, w, r, tx, route, f, ids, meta)
		return
	}
	if route == "drug-catalogues" {
		route = "catalogues"
		if len(f["kind"]) == 0 {
			f["kind"] = drugRelatedKinds
		} else {
			for _, kind := range f["kind"] {
				if !containsFilter(drugRelatedKinds, kind) {
					failure(w, 400, "unknown medicine catalogue kind")
					return
				}
			}
		}
	}
	if strings.HasPrefix(route, "drug-records/") {
		kind := strings.TrimPrefix(route, "drug-records/")
		if !containsFilter(drugRelatedKinds, kind) {
			failure(w, 400, "unknown medicine catalogue kind")
			return
		}
		route = "records/" + kind
	}
	if strings.HasPrefix(route, "services/") {
		if len(versions) != 1 {
			failure(w, 400, "service detail requires one catalogue type")
			return
		}
		var data []byte
		e = tx.QueryRow(ctx, `SELECT document FROM siui_data_services WHERE import_id=$1 AND code=$2`, ids[0], strings.TrimPrefix(route, "services/")).Scan(&data)
		if errors.Is(e, pgx.ErrNoRows) {
			failure(w, 404, "service not found")
			return
		}
		if e != nil {
			failure(w, 503, "cannot read service")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method != "HEAD" {
			output(w, 200, json.RawMessage(data))
		}
		return
	}
	limit, start, e := pagination(f)
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	meta["limit"] = limit
	meta["start"] = start
	if route == "catalogues" {
		rows, e := tx.Query(ctx, `SELECT i.catalogue_type,r.kind,r.container,count(*),count(*) FILTER(WHERE r.active) FROM siui_data_records r JOIN siui_data_imports i ON i.id=r.import_id WHERE r.import_id=ANY($1::uuid[]) GROUP BY i.catalogue_type,r.kind,r.container ORDER BY i.catalogue_type,r.kind`, ids)
		if e != nil {
			failure(w, 503, "cannot list nomenclatures")
			return
		}
		defer rows.Close()
		items := []any{}
		for rows.Next() {
			var typ, kind, container string
			var total, active int64
			if e = rows.Scan(&typ, &kind, &container, &total, &active); e != nil {
				break
			}
			if len(f["kind"]) > 0 && !containsFilter(f["kind"], kind) {
				continue
			}
			items = append(items, map[string]any{"type": typ, "kind": kind, "container": container, "records": total, "active": active})
		}
		if e != nil || rows.Err() != nil {
			failure(w, 503, "cannot list nomenclatures")
			return
		}
		total := len(items)
		lo := start
		if lo > total {
			lo = total
		}
		hi := lo + limit
		if hi > total {
			hi = total
		}
		listOutput(w, 200, items[lo:hi], int64(total), meta)
		return
	}
	services := route == "services" || route == "simple-services"
	if !services && !strings.HasPrefix(route, "records/") {
		failure(w, 404, "route not found")
		return
	}
	if len(f["offset"]) > 0 {
		failure(w, 400, "use start for offset pagination")
		return
	}
	args := []any{ids}
	where := []string{"s.import_id=ANY($1::uuid[])"}
	add := func(expr string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(expr, len(args)))
	}
	table, payload, order := "siui_data_services", "s.document", "s.code,s.import_id"
	if !services {
		table, payload, order = "siui_data_records", "s.payload", "s.ordinal,s.import_id"
		add("s.kind=$%d", strings.TrimPrefix(route, "records/"))
		if physicianRoute {
			payload = "s.payload || jsonb_build_object('source',s.source)"
		}
		if len(f["source"]) > 0 {
			add("s.source=ANY($%d::text[])", f["source"])
		}
		inactive, e := f.one("includeInactive")
		if e != nil {
			failure(w, 400, e.Error())
			return
		}
		if inactive != "true" {
			where = append(where, "s.active")
		}
	}
	if len(f["code"]) > 0 {
		add("s.code=ANY($%d::text[])", f["code"])
	}
	nameColumn := "s.name_normalized"
	if services {
		nameColumn = "siui_data_drug_fold(s.name)"
	}
	f.textClauses(&args, &where, nameColumn, "name")
	if services {
		if err := f.searchClauses(&args, &where, "s.search_text"); err != nil {
			failure(w, 400, err.Error())
			return
		}
	} else {
		f.textClauses(&args, &where, "s.name_normalized", "q")
	}
	if services && len(f["kind"]) > 0 {
		add("s.document->>'kind'=ANY($%d::text[])", f["kind"])
	}
	if services {
		if target != nil {
			add("s.specialty_codes && $%d::text[]", target.Specialties)
			targetArg := len(args)
			add("COALESCE(s.attributes->>'validFrom','0001-01-01')::date<=$%d::date", target.Day)
			add("(NULLIF(s.attributes->>'validTo','') IS NULL OR (s.attributes->>'validTo')::date>=$%d::date)", target.Day)
			payload = fmt.Sprintf("jsonb_set(s.document,'{specialties}',COALESCE((SELECT jsonb_agg(sp) FROM jsonb_array_elements(s.document->'specialties') sp WHERE sp->>'code'=ANY($%d::text[])),'[]'::jsonb))", targetArg)
		}
		if len(f["specialty"]) > 0 {
			requested := f["specialty"]
			if target != nil {
				requested = []string{}
				for _, code := range f["specialty"] {
					if containsFilter(target.Specialties, code) {
						requested = append(requested, code)
					}
				}
			}
			add("s.specialty_codes && $%d::text[]", requested)
		}
		if len(f["package"]) > 0 {
			add("s.package_codes && $%d::text[]", f["package"])
		}
		if len(f["specialtyName"]) > 0 {
			patterns := []string{}
			for _, v := range f["specialtyName"] {
				patterns = append(patterns, likePattern(v))
			}
			if target == nil {
				add("s.specialty_names LIKE ANY($%d::text[])", patterns)
			} else {
				args = append(args, target.Specialties)
				codesArg := len(args)
				args = append(args, patterns)
				patternsArg := len(args)
				where = append(where, fmt.Sprintf("EXISTS(SELECT 1 FROM jsonb_array_elements(s.document->'specialties') sp WHERE sp->>'code'=ANY($%d::text[]) AND siui_data_drug_fold(sp->>'name') LIKE ANY($%d::text[]))", codesArg, patternsArg))
			}
		}
		if len(f["specialtyNameAll"]) > 0 {
			clauses := []string{}
			only := queryFilters{"specialtyNameAll": f["specialtyNameAll"]}
			only.textClauses(&args, &clauses, "siui_data_drug_fold(sp->>'name')", "specialtyName")
			if target != nil {
				args = append(args, target.Specialties)
				clauses = append(clauses, fmt.Sprintf("sp->>'code'=ANY($%d::text[])", len(args)))
			}
			where = append(where, "EXISTS(SELECT 1 FROM jsonb_array_elements(s.document->'specialties') sp WHERE "+strings.Join(clauses, " AND ")+")")
		}

	}
	attrs := map[string][]string{}
	for name, values := range f {
		if strings.HasPrefix(name, "attr.") {
			if len(attrs) >= 32 || len(name) > 128 {
				failure(w, 400, "invalid attribute filters")
				return
			}
			attrs[strings.TrimPrefix(name, "attr.")] = values
		}
	}
	for _, name := range []string{"isExam", "opRoom", "isConnectedService", "highLevel", "recByPC", "formula", "inClinic"} {
		if len(f[name]) == 0 {
			continue
		}
		options := []string{}
		for _, v := range f[name] {
			b, e := strconv.ParseBool(v)
			if e != nil {
				failure(w, 400, "invalid boolean filter "+name)
				return
			}
			if b {
				options = append(options, "true", "1")
			} else {
				options = append(options, "false", "0")
			}
		}
		add("s.attributes->>'"+name+"'=ANY($%d::text[])", options)
	}
	if len(f["group"]) > 0 {
		attrs["serviceGroup"] = f["group"]
	}
	if len(f["serviceType"]) > 0 {
		attrs["serviceType"] = f["serviceType"]
	}
	for name, values := range attrs {
		// Attribute names and values are both parameters. Preserve JSONB GIN usage.
		or := []string{}
		for _, v := range values {
			b, _ := json.Marshal(map[string]string{name: v})
			args = append(args, string(b))
			or = append(or, fmt.Sprintf("s.attributes @> $%d::jsonb", len(args)))
		}
		where = append(where, "("+strings.Join(or, " OR ")+")")
	}
	from := table + " s"
	if route == "simple-services" {
		if len(f["after"]) > 0 {
			failure(w, 400, "simple-services uses start/limit pagination")
			return
		}
		from += ` LEFT JOIN LATERAL jsonb_array_elements(s.document->'packages') WITH ORDINALITY AS pkg(value,position) ON true LEFT JOIN LATERAL jsonb_array_elements(s.document->'specialties') WITH ORDINALITY AS sp(value,position) ON true`
		payload = simpleServiceSQL
		if target != nil {
			add("sp.value->>'code'=ANY($%d::text[])", target.Specialties)
		}
		order += `,pkg.value->'package'->>'code' NULLS FIRST,sp.value->>'code' NULLS FIRST,pkg.position,sp.position`
		// Apply relation filters to each expanded row as well as the indexed
		// service prefilter, so unmatched packages/specialties are not returned.
		if len(f["package"]) > 0 {
			add("pkg.value->'package'->>'code'=ANY($%d::text[])", f["package"])
		}
		if len(f["specialty"]) > 0 {
			add("sp.value->>'code'=ANY($%d::text[])", f["specialty"])
		}
		f.textClauses(&args, &where, "siui_data_drug_fold(sp.value->>'name')", "specialtyName")
	}
	var total int64
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM "+from+" WHERE "+strings.Join(where, " AND "), args...).Scan(&total); e != nil {
		failure(w, 503, "cannot count catalogue results")
		return
	}
	after, e := f.one("after")
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	if after != "" {
		if start != 0 || len(versions) != 1 {
			failure(w, 400, "after cannot be combined with start or multiple catalogue types")
			return
		}
		if services {
			add("s.code>$%d", after)
		} else {
			n, e := strconv.ParseInt(after, 10, 64)
			if e != nil || n < 0 {
				failure(w, 400, "invalid record cursor")
				return
			}
			add("s.ordinal>$%d", n)
		}
	}
	args = append(args, limit+1, start)
	cursor := "s.code"
	if !services {
		cursor = "s.ordinal::text"
	}
	rows, e := tx.Query(ctx, fmt.Sprintf("SELECT %s,%s FROM %s WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d", payload, cursor, from, strings.Join(where, " AND "), order, len(args)-1, len(args)), args...)
	if e != nil {
		failure(w, 503, "cannot search catalogue")
		return
	}
	defer rows.Close()
	items := []json.RawMessage{}
	cursors := []string{}
	for rows.Next() {
		var data []byte
		var cursor string
		if e = rows.Scan(&data, &cursor); e != nil {
			break
		}
		items = append(items, json.RawMessage(data))
		cursors = append(cursors, cursor)
	}
	if e != nil || rows.Err() != nil {
		failure(w, 503, "cannot search catalogue")
		return
	}
	meta["nextAfter"] = ""
	if len(items) > limit {
		if len(versions) == 1 && route != "simple-services" {
			meta["nextAfter"] = cursors[limit-1]
		}
		meta["nextStart"] = start + limit
		items = items[:limit]
	}
	listOutput(w, 200, items, total, meta)
}
func containsFilter(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

// Return the original decimal strings without floating-point rounding.
const simpleServiceSQL = `jsonb_build_object(
 'code',s.code,'kind',s.document->'kind','name',s.name,'type',s.document->'type',
 'isExam',COALESCE((s.document->>'isExam')::boolean,false),'opRoom',COALESCE((s.document->>'opRoom')::boolean,false),'isConnectedService',COALESCE((s.document->>'isConnectedService')::boolean,false),
 'group_code',s.attributes->>'serviceGroup',
 'package_code',pkg.value->'package'->'code','package_name',pkg.value->'package'->'name','package_points',pkg.value->'attributes'->'points','package_value_lei',pkg.value->'attributes'->'valueLei','package_points_srg',pkg.value->'attributes'->'pointsSrg',
 'speciality_code',sp.value->'code','speciality_name',sp.value->'name')`
