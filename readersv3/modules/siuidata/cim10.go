package siuidata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const cim10Schema = `
CREATE TABLE IF NOT EXISTS siui_data_cim10 (
 import_id uuid NOT NULL REFERENCES siui_data_imports(id) ON DELETE CASCADE,
 code text NOT NULL, name text NOT NULL, name_normalized text NOT NULL,
 parent_code text, entity_level integer, path text[] NOT NULL,
 raw_name text NOT NULL, raw_flags jsonb NOT NULL, flag_status text NOT NULL,
 is_normal boolean, is_dagger boolean, is_asterisk boolean, is_injury boolean,
 is_external_cause boolean, is_sequela boolean,
 PRIMARY KEY(import_id,code)
);
CREATE INDEX IF NOT EXISTS siui_data_cim10_parent ON siui_data_cim10(import_id,parent_code);
CREATE INDEX IF NOT EXISTS siui_data_cim10_name ON siui_data_cim10 USING gin(name_normalized gin_trgm_ops);
CREATE INDEX IF NOT EXISTS siui_data_cim10_path ON siui_data_cim10 USING gin(path);
INSERT INTO siui_data_schema_version(version) VALUES(5) ON CONFLICT DO NOTHING;
`

// A suffix is recognized from the right only. Real CNAS names contain commas,
// residual CSV quotes, and missing/truncated final flags. Missing is not false.
var cim10Suffix = regexp.MustCompile(`,(-1|0),(-1|0),(-1|0),(-1|0),(-1|0),(-1|0|-)?$`)

type cim10Name struct {
	Name     string
	RawFlags []string
	Flags    [6]*bool
	Status   string
}

func parseCim10Name(raw string) cim10Name {
	result := cim10Name{Name: raw, RawFlags: []string{}, Status: "absent"}
	match := cim10Suffix.FindStringSubmatchIndex(raw)
	if match == nil {
		return result
	}
	result.Name = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(raw[:match[0]]), `"`))
	result.Status = "complete"
	for i := 0; i < 6; i++ {
		value := ""
		if match[2+i*2] >= 0 {
			value = raw[match[2+i*2]:match[3+i*2]]
		}
		result.RawFlags = append(result.RawFlags, value)
		if value == "0" || value == "-1" {
			b := value == "-1"
			result.Flags[i] = &b
		} else {
			result.Status = "incomplete"
		}
	}
	return result
}

type cim10Record struct {
	Code, Parent, RawName string
	Level                 *int
	Parsed                cim10Name
	Path                  []string
}

// This small dictionary is resolved independently of the large drug catalogue.
// Paths preserve the explicit CNAS parent links, never underscore guesses.
func buildCim10(ctx context.Context, tx pgx.Tx, id string) error {
	rows, err := tx.Query(ctx, `SELECT code,COALESCE(attributes->>'name',''),COALESCE(attributes->>'parentCode',''),NULLIF(attributes->>'entityLevel','')::integer FROM siui_data_records WHERE import_id=$1 AND active AND kind='Cim10' ORDER BY ordinal`, id)
	if err != nil {
		return err
	}
	nodes := map[string]*cim10Record{}
	ordered := []*cim10Record{}
	for rows.Next() {
		n := &cim10Record{}
		if err = rows.Scan(&n.Code, &n.RawName, &n.Parent, &n.Level); err != nil {
			break
		}
		if _, exists := nodes[n.Code]; exists {
			err = fmt.Errorf("duplicate Cim10 code %s", n.Code)
			break
		}
		n.Parsed = parseCim10Name(n.RawName)
		nodes[n.Code] = n
		ordered = append(ordered, n)
	}
	rows.Close()
	if err != nil {
		return err
	}
	if err = rows.Err(); err != nil {
		return err
	}
	state := map[string]int{}
	var visit func(*cim10Record, int) error
	visit = func(n *cim10Record, depth int) error {
		if depth > 32 || state[n.Code] == 1 {
			return errors.New("Cim10 hierarchy contains a cycle or exceeds 32 levels")
		}
		if state[n.Code] == 2 {
			return nil
		}
		state[n.Code] = 1
		if p := nodes[n.Parent]; p != nil {
			if err := visit(p, depth+1); err != nil {
				return err
			}
			n.Path = append(append([]string{}, p.Path...), n.Code)
		} else {
			n.Path = []string{n.Code}
		}
		if len(n.Path) > 33 {
			return errors.New("Cim10 hierarchy exceeds 32 levels")
		}
		state[n.Code] = 2
		return nil
	}
	for _, n := range ordered {
		if err = visit(n, 0); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM siui_data_cim10 WHERE import_id=$1`, id); err != nil {
		return err
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"siui_data_cim10"}, []string{"import_id", "code", "name", "name_normalized", "parent_code", "entity_level", "path", "raw_name", "raw_flags", "flag_status", "is_normal", "is_dagger", "is_asterisk", "is_injury", "is_external_cause", "is_sequela"}, pgx.CopyFromSlice(len(ordered), func(i int) ([]any, error) {
		n := ordered[i]
		p := n.Parsed
		flags, _ := json.Marshal(p.RawFlags)
		var parent *string
		if n.Parent != "" {
			parent = &n.Parent
		}
		return []any{id, n.Code, p.Name, normalizeText(p.Name), parent, n.Level, n.Path, n.RawName, flags, p.Status, p.Flags[0], p.Flags[1], p.Flags[2], p.Flags[3], p.Flags[4], p.Flags[5]}, nil
	}))
	return err
}

func backfillCim10(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `SELECT import_id::text FROM siui_data_active ORDER BY catalogue_type`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err != nil {
		return err
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err = buildCim10(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

type cim10Item struct {
	Code string `json:"code"`
	Name string `json:"name"`
}
type cim10TreeItem struct {
	cim10Item
	Children []*cim10TreeItem `json:"children"`
}

const cim10DisplayName = `c.name || CASE WHEN c.is_dagger OR right(c.code,1)='+' THEN ' +' WHEN c.is_asterisk OR right(c.code,1)='*' THEN ' *' ELSE '' END`

func (m *Module) cim10HTTP(ctx context.Context, w http.ResponseWriter, r *http.Request, tx pgx.Tx, route string, f queryFilters, ids []string, meta map[string]any) {
	if len(ids) != 1 {
		failure(w, 400, "CIM10 requires one catalogue type")
		return
	}
	limit, start, err := pagination(f)
	if err != nil {
		failure(w, 400, err.Error())
		return
	}
	args := []any{ids[0]}
	where := []string{"c.import_id=$1"}
	add := func(expr string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(expr, len(args)))
	}
	known := map[string]bool{"type": true, "partnerCategory": true, "limit": true, "start": true}
	for _, name := range []string{"name", "q"} {
		known[name], known[name+"All"] = true, true
		f.textClauses(&args, &where, "c.name_normalized", name)
	}
	for name, column := range map[string]string{"code": "c.code", "parentCode": "c.parent_code", "flagStatus": "c.flag_status"} {
		known[name] = true
		if len(f[name]) > 0 {
			add(column+"=ANY($%d::text[])", f[name])
		}
	}
	known["entityLevel"] = true
	levels := []int32{}
	for _, v := range f["entityLevel"] {
		n, e := strconv.ParseInt(v, 10, 32)
		if e != nil {
			failure(w, 400, "entityLevel must contain integers")
			return
		}
		levels = append(levels, int32(n))
	}
	if len(levels) > 0 {
		add("c.entity_level=ANY($%d::integer[])", levels)
	}

	for name, column := range map[string]string{"isNormal": "is_normal", "isDagger": "is_dagger", "isAsterisk": "is_asterisk", "isInjury": "is_injury", "isExternalCause": "is_external_cause", "isSequela": "is_sequela"} {
		known[name] = true
		values := []bool{}
		for _, v := range f[name] {
			b, e := strconv.ParseBool(v)
			if e != nil {
				failure(w, 400, "invalid boolean filter "+name)
				return
			}
			values = append(values, b)
		}
		if len(values) > 0 {
			add("c."+column+"=ANY($%d::boolean[])", values)
		}
	}
	known["roots"] = true
	if raw, e := f.one("roots"); e != nil {
		failure(w, 400, e.Error())
		return
	} else if raw != "" {
		roots, e := strconv.ParseBool(raw)
		if e != nil {
			failure(w, 400, "invalid roots filter")
			return
		}
		if roots {
			where = append(where, "cardinality(c.path)=1")
		} else {
			where = append(where, "cardinality(c.path)>1")
		}
	}
	for name := range f {
		if !known[name] {
			failure(w, 400, "unknown CIM10 filter: "+name)
			return
		}
	}
	predicate := strings.Join(where, " AND ")
	var total int64
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM siui_data_cim10 c WHERE "+predicate, args...).Scan(&total); err != nil {
		failure(w, 503, "cannot count CIM10 matches")
		return
	}
	args = append(args, limit, start)
	selected := fmt.Sprintf("SELECT c.* FROM siui_data_cim10 c WHERE %s ORDER BY c.code LIMIT $%d OFFSET $%d", predicate, len(args)-1, len(args))
	query := "SELECT c.code," + cim10DisplayName + " FROM (" + selected + ") c ORDER BY c.code"
	if route == "cim10/tree" {
		query = "WITH matches AS (" + selected + "), ancestors AS (SELECT DISTINCT unnest(path) AS code FROM matches) SELECT c.code," + cim10DisplayName + ",COALESCE(c.parent_code,'') FROM siui_data_cim10 c JOIN ancestors a ON a.code=c.code WHERE c.import_id=$1 ORDER BY c.code"
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		failure(w, 503, "cannot search CIM10")
		return
	}
	items := []cim10Item{}
	nodes := map[string]*cim10TreeItem{}
	parents := map[string]string{}
	ordered := []string{}
	for rows.Next() {
		var item cim10Item
		var parent string
		if route == "cim10/tree" {
			err = rows.Scan(&item.Code, &item.Name, &parent)
		} else {
			err = rows.Scan(&item.Code, &item.Name)
		}
		if err != nil {
			break
		}
		items = append(items, item)
		if route == "cim10/tree" {
			nodes[item.Code] = &cim10TreeItem{item, []*cim10TreeItem{}}
			parents[item.Code] = parent
			ordered = append(ordered, item.Code)
		}
	}
	rows.Close()
	if err != nil || rows.Err() != nil {
		failure(w, 503, "cannot read CIM10")
		return
	}
	meta["limit"], meta["start"] = limit, start
	if int64(start+limit) < total {
		meta["nextStart"] = start + limit
	}
	if route == "cim10/tree" {
		roots := []*cim10TreeItem{}
		for _, code := range ordered {
			n := nodes[code]
			if p := nodes[parents[code]]; p != nil {
				p.Children = append(p.Children, n)
			} else {
				roots = append(roots, n)
			}
		}
		meta["nodeCount"] = len(nodes)
		meta["pagination"] = "matches"
		listOutput(w, 200, roots, total, meta)
	} else {
		listOutput(w, 200, items, total, meta)
	}
}
