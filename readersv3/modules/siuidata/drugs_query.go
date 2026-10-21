package siuidata

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (m *Module) drugsHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request, tx pgx.Tx, route string, f queryFilters, ids []string, meta map[string]any) {
	var ready bool
	if e := tx.QueryRow(ctx, `SELECT bool_and(drug_projection_version>0) FROM siui_data_imports WHERE id=ANY($1::uuid[])`, ids).Scan(&ready); e != nil {
		failure(w, 503, "cannot read medicine projection status")
		return
	}
	if !ready {
		failure(w, 503, "reimport this catalogue to build the medicine graph; existing services remain available")
		return
	}
	if strings.HasPrefix(route, "drugs/") {
		if len(ids) != 1 {
			failure(w, 400, "drug detail requires one catalogue type")
			return
		}
		var data []byte
		e := tx.QueryRow(ctx, `SELECT document FROM siui_data_drugs WHERE import_id=$1 AND code=$2`, ids[0], strings.TrimPrefix(route, "drugs/")).Scan(&data)
		if e == pgx.ErrNoRows {
			failure(w, 404, "drug not found")
			return
		}
		if e != nil {
			failure(w, 503, "cannot read drug")
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
	if len(f["after"])+len(f["offset"]) > 0 {
		failure(w, 400, "medicine endpoints use start/limit pagination")
		return
	}
	for alias, name := range map[string]string{"drug": "name", "drugCode": "code", "medicine": "name", "substance": "activeSubstance", "form": "pharmaceuticalForm", "packaging": "presentationMode", "copaymentList": "copaymentListType", "nationalProgram": "nhpCode", "commission": "needApproval"} {
		if containsFilter([]string{"name", "activeSubstance", "pharmaceuticalForm", "presentationMode"}, name) && len(f[alias+"All"]) > 0 {
			f[name+"All"] = append(f[name+"All"], f[alias+"All"]...)
			delete(f, alias+"All")
		}
		if len(f[alias]) > 0 {
			f[name] = append(f[name], f[alias]...)
			delete(f, alias)
		}
	}
	args := []any{ids}
	where := []string{"d.import_id=ANY($1::uuid[])"}
	terms := []string{}
	add := func(dest *[]string, expr string, value any) {
		args = append(args, value)
		*dest = append(*dest, fmt.Sprintf(expr, len(args)))
	}
	known := map[string]bool{"type": true, "partnerCategory": true, "limit": true, "start": true}
	textFilter := func(dest *[]string, column, name string) {
		known[name], known[name+"All"] = true, true
		f.textClauses(&args, dest, column, name)
	}
	for name, column := range map[string]string{"name": "d.name_normalized", "activeSubstance": "d.active_substance", "concentration": "d.concentration", "pharmaceuticalForm": "d.pharmaceutical_form", "presentationMode": "d.presentation_mode", "company": "d.company"} {
		textFilter(&where, column, name)
	}
	for name, column := range map[string]string{"code": "d.code", "country": "d.country", "atc": "d.atc", "activeSubstanceCode": "d.attributes->>'activeSubstance'", "concentrationCode": "d.attributes->>'concentration'", "pharmaceuticalFormCode": "d.attributes->>'pharmaceuticalForm'", "prescriptionMode": "d.attributes->>'prescriptionMode'", "isNarcotic": "d.attributes->>'isNarcotic'"} {
		known[name] = true
		if len(f[name]) > 0 {
			add(&where, column+"=ANY($%d::text[])", f[name])
		}
	}
	known["atcPrefix"] = true
	if len(f["atcPrefix"]) > 0 {
		patterns := []string{}
		for _, v := range f["atcPrefix"] {
			patterns = append(patterns, strings.TrimSuffix(strings.TrimPrefix(likePattern(strings.ToUpper(v)), "%"), "%")+"%")
		}
		add(&where, "lower(d.atc) LIKE ANY($%d::text[])", patterns)
	}
	known["q"], known["qAll"] = true, true
	if err := f.searchClauses(&args, &where, "d.search_text"); err != nil {
		failure(w, 400, err.Error())
		return
	}
	for _, name := range []string{"isSpecial", "isFractional", "isBrand", "hasBioEchiv", "specialLaw", "needSpecialty", "overValue", "openCircuit"} {
		known[name] = true
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
		if name == "isSpecial" || name == "isFractional" || name == "isBrand" || name == "hasBioEchiv" {
			add(&where, "d.attributes->>'"+name+"'=ANY($%d::text[])", options)
		} else {
			add(&terms, "t.attributes->>'"+name+"'=ANY($%d::text[])", options)
		}
	}
	for name, column := range map[string]string{"copaymentListType": "t.copayment_list", "nhpCode": "t.nhp_code", "diseaseCode": "t.disease_code", "needApproval": "t.approval", "sourceKind": "t.source_kind", "contractCv": "t.attributes->>'contractCv'", "hgIcd10": "t.attributes->>'hgIcd10'", "classifInsulin": "t.attributes->>'classifInsulin'"} {
		known[name] = true
		if len(f[name]) > 0 {
			add(&terms, column+"=ANY($%d::text[])", f[name])
		}
	}
	for name, column := range map[string]string{"diseaseName": "t.disease_name", "nationalProgramName": "siui_data_drug_fold(t.document->'nationalProgram'->>'description')", "copaymentListName": "siui_data_drug_fold(t.document->'copaymentList'->>'description')", "commissionName": "siui_data_drug_fold(t.document->'approval'->>'name')"} {
		textFilter(&terms, column, name)
	}
	for _, price := range []struct{ name, column string }{{"maxPrice", "t.max_price"}, {"referencePrice", "t.reference_price"}, {"maxPriceUT", "t.max_price_ut"}, {"pricePerPackage", "NULLIF(d.attributes->>'pricePerPackage','')::numeric"}} {
		base := &terms
		if price.name == "pricePerPackage" {
			base = &where
		}
		known[price.name] = true
		if len(f[price.name]) > 0 {
			or := []string{}
			for _, v := range f[price.name] {
				if _, e := medicineDecimal(v); e != nil {
					failure(w, 400, price.name+": "+e.Error())
					return
				}
				add(&or, price.column+"=($%d::text)::numeric", v)
			}
			*base = append(*base, "("+strings.Join(or, " OR ")+")")
		}
		var lower, upper *big.Rat
		for _, bound := range []struct {
			suffix, op string
			target     **big.Rat
		}{{"Min", ">=", &lower}, {"Max", "<=", &upper}} {
			key := price.name + bound.suffix
			known[key] = true
			raw, e := f.one(key)
			if e != nil {
				failure(w, 400, e.Error())
				return
			}
			if raw == "" {
				continue
			}
			n, e := medicineDecimal(raw)
			if e != nil {
				failure(w, 400, key+": "+e.Error())
				return
			}
			*bound.target = n
			add(base, price.column+bound.op+"($%d::text)::numeric", raw)
		}
		if lower != nil && upper != nil && lower.Cmp(upper) > 0 {
			failure(w, 400, price.name+"Min must not exceed "+price.name+"Max")
			return
		}
	}
	known["diagnosisCode"] = true
	known["diagnosisName"] = true
	known["diagnosisNameAll"] = true
	if len(f["diagnosisCode"])+len(f["diagnosisName"])+len(f["diagnosisNameAll"]) > 0 {
		// ActSubstICD10 diagnoses are separate from compensation disease categories.
		diag := []string{"r.import_id=d.import_id", "r.active", "r.kind='ActSubstICD10'", "siui_data_drug_fold(r.attributes->>'activeSubstance')=d.active_substance"}
		if len(f["diagnosisCode"]) > 0 {
			add(&diag, "r.attributes->>'icd10'=ANY($%d::text[])", f["diagnosisCode"])
		}
		textFilter(&diag, "ic.name_normalized", "diagnosisName")
		where = append(where, "EXISTS(SELECT 1 FROM siui_data_records r LEFT JOIN siui_data_records ic ON ic.import_id=r.import_id AND ic.active AND ic.kind='ICD10' AND ic.code=r.attributes->>'icd10' WHERE "+strings.Join(diag, " AND ")+")")
	}
	attrCount := 0
	for name, values := range f {
		prefix, column := "attr.", "d.attributes"
		dest := &where
		if strings.HasPrefix(name, "compensation.attr.") {
			prefix, column = "compensation.attr.", "t.attributes"
			dest = &terms
		}
		if strings.HasPrefix(name, prefix) {
			attrCount++
			if attrCount > 32 || len(name) > 128 {
				failure(w, 400, "invalid attribute filters")
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
			failure(w, 400, "unknown medicine filter: "+name)
			return
		}
	}
	flat := route == "drug-compensations" || route == "drug-relations"
	from := "siui_data_drugs d"
	payload := "d.document"
	order := "d.code,d.import_id"
	if route == "simple-drugs" {
		from += ` LEFT JOIN siui_data_drug_terms t ON t.import_id=d.import_id AND t.drug_code=d.code LEFT JOIN LATERAL jsonb_array_elements(d.document->'diagnoses') WITH ORDINALITY AS dg(value,position) ON true`
		where = append(where, terms...)
		if len(f["diagnosisCode"]) > 0 {
			add(&where, "dg.value->'attributes'->>'icd10'=ANY($%d::text[])", f["diagnosisCode"])
		}
		textFilter(&where, "siui_data_drug_fold(dg.value->'diagnosis'->>'name')", "diagnosisName")
		payload = simpleDrugSQL
		order += ",t.ordinal NULLS FIRST,dg.position"
	} else if flat {
		from += " JOIN siui_data_drug_terms t ON t.import_id=d.import_id AND t.drug_code=d.code"
		if route == "drug-compensations" {
			terms = append(terms, "t.source_kind='CopaymentListDrug'")
		}
		where = append(where, terms...)
		payload = "t.document || jsonb_build_object('drugName',d.name,'type',d.document->'type')"
		order += ",t.ordinal"
	} else if len(terms) > 0 {
		where = append(where, "EXISTS(SELECT 1 FROM siui_data_drug_terms t WHERE t.import_id=d.import_id AND t.drug_code=d.code AND "+strings.Join(terms, " AND ")+")")
	}
	var total int64
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM "+from+" WHERE "+strings.Join(where, " AND "), args...).Scan(&total); e != nil {
		failure(w, 503, "cannot count medicines")
		return
	}
	args = append(args, limit, start)
	rows, e := tx.Query(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d", payload, from, strings.Join(where, " AND "), order, len(args)-1, len(args)), args...)
	if e != nil {
		failure(w, 503, "cannot search medicines")
		return
	}
	defer rows.Close()
	items := []json.RawMessage{}
	for rows.Next() {
		var data []byte
		if e = rows.Scan(&data); e != nil {
			break
		}
		items = append(items, json.RawMessage(data))
	}
	if e != nil || rows.Err() != nil {
		failure(w, 503, "cannot read medicines")
		return
	}
	meta["limit"] = limit
	meta["start"] = start
	if int64(start)+int64(len(items)) < total {
		meta["nextStart"] = start + len(items)
	}
	listOutput(w, 200, items, total, meta)
}

func medicineDecimal(v string) (*big.Rat, error) {
	if len(v) > 128 || !decimalPattern.MatchString(v) {
		return nil, fmt.Errorf("expected nonnegative decimal (dot separator, at most 128 characters)")
	}
	n, ok := new(big.Rat).SetString(v)
	if !ok || n.Sign() < 0 {
		return nil, fmt.Errorf("expected nonnegative decimal")
	}
	return n, nil
}

const simpleDrugSQL = `jsonb_build_object('code',d.code,'kind','Drug','type',d.document->'type','name',d.name,
 'activeSubstance',d.attributes->'activeSubstance','concentration',d.attributes->'concentration','pharmaceuticalForm',d.attributes->'pharmaceuticalForm','presentationMode',d.attributes->'presentationMode',
 'company',d.attributes->'company','country',d.attributes->'country','atc',d.attributes->'atc',
 'isSpecial',COALESCE(NULLIF(d.attributes->>'isSpecial','')::boolean,false),'isFractional',COALESCE(NULLIF(d.attributes->>'isFractional','')::boolean,false),'isBrand',COALESCE(NULLIF(d.attributes->>'isBrand','')::boolean,false),'hasBioEchiv',COALESCE(NULLIF(d.attributes->>'hasBioEchiv','')::boolean,false),'isNarcotic',d.attributes->'isNarcotic',
 'qtyPerPackage',d.attributes->'qtyPerPackage','pricePerPackage',d.attributes->'pricePerPackage','prescriptionMode',d.attributes->'prescriptionMode',
 'active_substance_name',COALESCE(d.document->'activeSubstance'->>'name',d.attributes->>'activeSubstance'),
 'concentration_name',COALESCE(d.document->'concentration'->>'name',d.attributes->>'concentration'),
 'pharmaceutical_form_name',COALESCE(d.document->'pharmaceuticalForm'->>'name',d.attributes->>'pharmaceuticalForm'),
 'country_name',d.document->'country'->>'name','atc_name',d.document->'atc'->>'description',
 'sourceKind',t.source_kind,
 'copayment_list_code',NULLIF(t.copayment_list,''),'copayment_list_name',t.document->'copaymentList'->>'description','copayment_list_percent',t.document->'copaymentList'->>'percent',
 'national_program_code',NULLIF(t.nhp_code,''),'national_program_name',t.document->'nationalProgram'->>'description',
 'disease_code',NULLIF(t.disease_code,''),'disease_name',t.document->'disease'->>'description',
 'approval_code',NULLIF(t.approval,''),'approval_name',t.document->'approval'->>'name',
 'maxPrice',t.attributes->>'maxPrice','referencePrice',t.attributes->>'referencePrice','maxPriceUT',t.attributes->>'maxPriceUT',
 'protocol_code',t.attributes->>'codeProtocolTherap','protocol_name',t.attributes->>'descProtocolTherap',
 'relation_diagnosis_code',COALESCE(t.attributes->>'hgIcd10',t.attributes->>'icd10'),'relation_diagnosis_name',t.document->'diagnosis'->>'name',
 'diagnosis_code',dg.value->'attributes'->>'icd10','diagnosis_name',dg.value->'diagnosis'->>'name',
 'diagnosis_disease_code',dg.value->'diagnosis'->>'diseaseCategory','diagnosis_disease_name',dg.value->'diseaseCategory'->>'description')
 || COALESCE((SELECT jsonb_object_agg('relation_'||a.key,a.value) FROM jsonb_each(t.attributes) a),'{}'::jsonb)`
