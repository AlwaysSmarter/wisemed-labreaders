package siuidata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (m *Module) physiciansHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request, tx pgx.Tx, route string, f queryFilters, ids []string, meta map[string]any) {
	args := []any{ids}
	where := []string{"p.import_id=ANY($1::uuid[])"}
	relations := []string{}
	add := func(dest *[]string, expr string, value any) {
		args = append(args, value)
		*dest = append(*dest, fmt.Sprintf(expr, len(args)))
	}
	detail := strings.HasPrefix(route, "physicians/")
	if detail {
		add(&where, "p.stencil=$%d", strings.TrimPrefix(route, "physicians/"))
	}
	for alias, name := range map[string]string{"code": "stencil", "parafa": "stencil", "specialityCode": "specialty", "specialityName": "specialtyName"} {
		if name == "specialtyName" && len(f[alias+"All"]) > 0 {
			f[name+"All"] = append(f[name+"All"], f[alias+"All"]...)
			delete(f, alias+"All")
		}
		if len(f[alias]) > 0 {
			f[name] = append(f[name], f[alias]...)
			delete(f, alias)
		}
	}
	known := map[string]bool{"type": true, "partnerCategory": true, "limit": true, "start": true}
	for name, column := range map[string]string{"stencil": "p.stencil", "source": "p.source"} {
		known[name] = true
		if len(f[name]) > 0 {
			add(&where, column+"=ANY($%d::text[])", f[name])
		}
	}
	for name, column := range map[string]string{"name": "p.name_normalized", "specialtyName": "a.specialty_name", "insuranceHouseName": "a.insurance_house_name"} {
		known[name], known[name+"All"] = true, true
		dest := &relations
		if name == "name" {
			dest = &where
		}
		f.textClauses(&args, dest, column, name)
	}
	known["q"], known["qAll"] = true, true
	if err := f.searchClauses(&args, &where, "siui_data_drug_fold(p.name||' '||p.stencil)"); err != nil {
		failure(w, 400, err.Error())
		return
	}
	for name, column := range map[string]string{"specialty": "a.specialty_code", "contractType": "a.contract_type", "contractNo": "a.contract_no", "physicianType": "a.physician_type", "insuranceHouse": "a.insurance_house", "insuranceHouseType": "a.document->'insuranceHouse'->>'type'"} {
		known[name] = true
		if len(f[name]) > 0 {
			add(&relations, column+"=ANY($%d::text[])", f[name])
		}
	}
	for name, values := range f {
		prefix, column, dest := "attr.", "p.attributes", &where
		if strings.HasPrefix(name, "association.attr.") {
			prefix, column, dest = "association.attr.", "a.attributes", &relations
		}
		if strings.HasPrefix(name, prefix) {
			if len(name) > 128 {
				failure(w, 400, "attribute name too long")
				return
			}
			or := []string{}
			for _, v := range values {
				b, _ := json.Marshal(map[string]string{strings.TrimPrefix(name, prefix): v})
				add(&or, column+" @> $%d::jsonb", string(b))
			}
			*dest = append(*dest, "("+strings.Join(or, " OR ")+")")
			continue
		}
		if !known[name] {
			failure(w, 400, "unknown physician filter: "+name)
			return
		}
	}
	from := "siui_data_physicians p"
	payload := "p.document"
	order := "p.stencil,p.source,p.import_id"
	if route == "simple-physicians" {
		from += " LEFT JOIN siui_data_physician_relations a ON a.import_id=p.import_id AND a.source=p.source AND a.stencil=p.stencil"
		where = append(where, relations...)
		payload = simplePhysicianSQL
		order += ",a.ordinal NULLS FIRST"
	} else if len(relations) > 0 {
		where = append(where, "EXISTS(SELECT 1 FROM siui_data_physician_relations a WHERE a.import_id=p.import_id AND a.source=p.source AND a.stencil=p.stencil AND "+strings.Join(relations, " AND ")+")")
	}
	limit, start, e := pagination(f)
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	var total int64
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM "+from+" WHERE "+strings.Join(where, " AND "), args...).Scan(&total); e != nil {
		failure(w, 503, "cannot count physicians")
		return
	}
	if detail {
		if total == 0 {
			failure(w, 404, "physician not found")
			return
		}
		if total > 1 {
			failure(w, 409, "stencil exists in multiple sources; select source=CNAS or source=OPSNAJ")
			return
		}
		limit, start = 1, 0
	}
	args = append(args, limit, start)
	rows, e := tx.Query(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d", payload, from, strings.Join(where, " AND "), order, len(args)-1, len(args)), args...)
	if e != nil {
		failure(w, 503, "cannot search physicians")
		return
	}
	defer rows.Close()
	items := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			break
		}
		items = append(items, json.RawMessage(b))
	}
	if e != nil || rows.Err() != nil {
		failure(w, 503, "cannot read physicians")
		return
	}
	if detail {
		if r.Method != "HEAD" {
			output(w, 200, items[0])
		}
		return
	}
	meta["limit"], meta["start"] = limit, start
	if int64(start+len(items)) < total {
		meta["nextStart"] = start + len(items)
	}
	listOutput(w, 200, items, total, meta)
}

const simplePhysicianSQL = `jsonb_build_object(
 'code',p.stencil,'stencil',p.stencil,'name',p.name,'kind','Physician','type','PHYS','source',p.source,
 'validFrom',p.attributes->>'validFrom','validTo',p.attributes->>'validTo',
 'speciality_code',a.attributes->>'specialityCode','speciality_name',a.document->'speciality'->>'name',
 'contractNo',a.attributes->>'contractNo','contractType',a.attributes->>'contractType','physicianType',a.attributes->>'physicianType',
 'insurance_house_code',a.attributes->>'insuranceHouse','insurance_house_name',a.document->'insuranceHouse'->>'name',
 'insurance_house_type_code',a.document->'insuranceHouse'->>'type','insurance_house_type_name',a.document->'insuranceHouseType'->>'description',
 'association_valid_from',a.attributes->>'validFrom','association_valid_to',a.attributes->>'validTo')
 || COALESCE((SELECT jsonb_object_agg('association_'||key,value) FROM jsonb_each(a.attributes)),'{}'::jsonb)`
