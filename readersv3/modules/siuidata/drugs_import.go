package siuidata

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
)

var drugEntityKinds = []string{"Country", "PharmaceuticalForm", "Concentration", "ActiveSubstance", "ATC", "ICD10", "DiseaseCategory", "NHP", "CopaymentListType", "MedicalFormType", "NHP_DRUG_TYPE", "PersonState"}
var drugRelatedKinds = []string{"Drug", "Country", "PharmaceuticalForm", "Concentration", "ActiveSubstance", "ATC", "ICD10", "DiseaseCategory", "NHP", "CopaymentListType", "ActSubstICD10", "CopaymentListDrug", "CopaymentListActiveSubst", "CopaymentListProtocolTherapeutic", "NHP_DRUG", "NHP_DRUG_TYPE", "TherapeuticalSchema", "MedicalFormType", "PrescriptionType", "CpmetLstTypePersState", "PersonState", "NHPCategory"}

func buildDrugs(ctx context.Context, tx pgx.Tx, id, typ string) (int64, error) {
	var count int64
	if e := tx.QueryRow(ctx, `SELECT count(*) FROM siui_data_records WHERE import_id=$1 AND active AND kind='Drug'`, id).Scan(&count); e != nil {
		return 0, e
	}
	if count == 0 {
		return 0, nil
	}
	// Resolve small dictionaries once; the full catalogue stays in PostgreSQL.
	if _, e := tx.Exec(ctx, `CREATE TEMP TABLE siui_drug_entities ON COMMIT DROP AS SELECT kind,CASE WHEN kind='Concentration' THEN attributes->>'concentration' ELSE code END AS code,attributes FROM siui_data_records WHERE import_id=$1 AND active AND kind=ANY($2::text[])`, id, drugEntityKinds); e != nil {
		return 0, e
	}
	var duplicate bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM siui_drug_entities GROUP BY kind,code HAVING count(*)>1) OR EXISTS(SELECT 1 FROM siui_data_records WHERE import_id=$1 AND kind='Drug' AND active GROUP BY code HAVING count(*)>1)`, id).Scan(&duplicate); e != nil {
		return 0, e
	}
	if duplicate {
		return 0, errors.New("overlapping active drug or medicine dictionary definitions")
	}
	if _, e := tx.Exec(ctx, `CREATE UNIQUE INDEX ON siui_drug_entities(kind,code); ANALYZE siui_drug_entities`); e != nil {
		return 0, e
	}
	for _, ref := range []struct{ key, kind string }{{"activeSubstance", "ActiveSubstance"}, {"concentration", "Concentration"}, {"pharmaceuticalForm", "PharmaceuticalForm"}, {"atc", "ATC"}, {"country", "Country"}} {
		var missing bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM siui_data_records d WHERE d.import_id=$1 AND d.active AND d.kind='Drug' AND NOT EXISTS(SELECT 1 FROM siui_drug_entities e WHERE e.kind=$2 AND e.code=d.attributes->>$3))`, id, ref.kind, ref.key).Scan(&missing); e != nil {
			return 0, e
		}
		if missing {
			return 0, fmt.Errorf("active Drug references missing %s", ref.kind)
		}
	}
	// List percentages may depend on the insured person's status. Keep those
	// rules separate from the base percent instead of flattening them away.
	if _, e := tx.Exec(ctx, `UPDATE siui_drug_entities cl SET attributes=cl.attributes||jsonb_build_object('personStateRules',COALESCE((SELECT jsonb_agg(jsonb_build_object('attributes',r.attributes,'personState',p.attributes) ORDER BY r.ordinal) FROM siui_data_records r LEFT JOIN siui_drug_entities p ON p.kind='PersonState' AND p.code=r.attributes->>'personState' WHERE r.import_id=$1 AND r.active AND r.kind='CpmetLstTypePersState' AND r.attributes->>'copaymentListType'=cl.code),'[]'::jsonb)) WHERE cl.kind='CopaymentListType'`, id); e != nil {
		return 0, e
	}
	// ATC paths are computed once, with cycle detection and no invented ancestors.
	if _, e := tx.Exec(ctx, `CREATE TEMP TABLE siui_drug_atc_paths ON COMMIT DROP AS WITH RECURSIVE paths AS (
 SELECT code AS origin,code,attributes,ARRAY[code] AS path,0 AS depth,false AS cycle FROM siui_drug_entities WHERE kind='ATC'
 UNION ALL SELECT p.origin,e.code,e.attributes,p.path||e.code,p.depth+1,e.code=ANY(p.path) FROM paths p JOIN siui_drug_entities e ON e.kind='ATC' AND e.code=p.attributes->>'parentATC' WHERE NOT p.cycle AND p.depth<32
 ) SELECT origin,jsonb_agg(attributes ORDER BY depth) AS document,bool_or(cycle OR depth=32) AS invalid FROM paths GROUP BY origin; CREATE UNIQUE INDEX ON siui_drug_atc_paths(origin)`); e != nil {
		return 0, e
	}
	var bad bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM siui_drug_atc_paths WHERE invalid)`).Scan(&bad); e != nil {
		return 0, e
	}
	if bad {
		return 0, errors.New("ATC hierarchy contains a cycle or exceeds 32 levels")
	}
	_, e := tx.Exec(ctx, `INSERT INTO siui_data_drugs(import_id,code,name,name_normalized,search_text,attributes,document,active_substance,concentration,pharmaceutical_form,presentation_mode,company,country,atc)
 SELECT d.import_id,d.code,d.attributes->>'name',d.name_normalized,
 siui_data_drug_fold(concat_ws(' ',d.code,d.attributes->>'name',d.attributes->>'activeSubstance',d.attributes->>'concentration',d.attributes->>'pharmaceuticalForm',d.attributes->>'presentationMode',d.attributes->>'company')),
 d.attributes,jsonb_build_object('code',d.code,'kind','Drug','name',d.attributes->>'name','type',$2::text,'attributes',d.attributes,
 'activeSubstance',su.attributes,'concentration',co.attributes,'pharmaceuticalForm',fo.attributes,'country',ct.attributes,'atc',at.attributes,'atcHierarchy',ap.document,
 'compensations','[]'::jsonb,'substanceCompensations','[]'::jsonb,'therapeuticProtocols','[]'::jsonb,'nationalProgramDrugs','[]'::jsonb,'diagnoses','[]'::jsonb),
 siui_data_drug_fold(d.attributes->>'activeSubstance'),siui_data_drug_fold(d.attributes->>'concentration'),siui_data_drug_fold(d.attributes->>'pharmaceuticalForm'),siui_data_drug_fold(COALESCE(d.attributes->>'presentationMode','')),siui_data_drug_fold(COALESCE(d.attributes->>'company','')),d.attributes->>'country',d.attributes->>'atc'
 FROM siui_data_records d
 JOIN siui_drug_entities su ON su.kind='ActiveSubstance' AND su.code=d.attributes->>'activeSubstance'
 JOIN siui_drug_entities co ON co.kind='Concentration' AND co.code=d.attributes->>'concentration'
 JOIN siui_drug_entities fo ON fo.kind='PharmaceuticalForm' AND fo.code=d.attributes->>'pharmaceuticalForm'
 JOIN siui_drug_entities ct ON ct.kind='Country' AND ct.code=d.attributes->>'country'
 JOIN siui_drug_entities at ON at.kind='ATC' AND at.code=d.attributes->>'atc'
 JOIN siui_drug_atc_paths ap ON ap.origin=at.code
 WHERE d.import_id=$1 AND d.kind='Drug' AND d.active`, id, typ)
	if e != nil {
		return 0, e
	}
	// Fresh statistics and pre-normalized link keys avoid repeated folding of
	// the large raw XML table inside substance/programme joins.
	if _, e = tx.Exec(ctx, `ANALYZE siui_data_drugs`); e != nil {
		return 0, e
	}
	if _, e = tx.Exec(ctx, `CREATE TEMP TABLE siui_drug_links ON COMMIT DROP AS SELECT ordinal,kind,code,attributes,siui_data_drug_fold(COALESCE(attributes->>'activeSubstance',CASE WHEN kind='NHP_DRUG' AND attributes->>'type'='ACS' THEN code END)) AS active_substance,COALESCE(attributes->>'atc',attributes->>'aTC') AS atc FROM siui_data_records WHERE import_id=$1 AND active AND kind IN ('CopaymentListDrug','CopaymentListActiveSubst','CopaymentListProtocolTherapeutic','NHP_DRUG')`, id); e != nil {
		return 0, e
	}
	if _, e = tx.Exec(ctx, `CREATE INDEX ON siui_drug_links(kind,code); CREATE INDEX ON siui_drug_links(kind,active_substance,atc); ANALYZE siui_drug_links`); e != nil {
		return 0, e
	}
	// Joining through a source ordinal preserves distinct programmes, lists,
	// prices and approvals. Substance-level rules remain explicitly identified.
	_, e = tx.Exec(ctx, `CREATE TEMP TABLE siui_drug_matches ON COMMIT DROP AS
 SELECT d.code AS drug_code,r.ordinal,r.kind,r.attributes FROM siui_drug_links r JOIN siui_data_drugs d ON d.import_id=$1 AND d.code=r.attributes->>'drug' WHERE r.kind='CopaymentListDrug'
 UNION ALL
 SELECT d.code,r.ordinal,r.kind,r.attributes FROM siui_drug_links r JOIN siui_data_drugs d ON d.import_id=$1 AND d.active_substance=r.active_substance AND d.atc=r.atc WHERE r.kind IN ('CopaymentListActiveSubst','CopaymentListProtocolTherapeutic')
 UNION ALL
 SELECT d.code,r.ordinal,r.kind,r.attributes FROM siui_drug_links r JOIN siui_data_drugs d ON d.import_id=$1 AND d.code=r.code WHERE r.kind='NHP_DRUG' AND r.attributes->>'type'='DRG'
 UNION ALL
 SELECT d.code,r.ordinal,r.kind,r.attributes FROM siui_drug_links r JOIN siui_data_drugs d ON d.import_id=$1 AND d.active_substance=r.active_substance WHERE r.kind='NHP_DRUG' AND r.attributes->>'type'='ACS'`, id)
	if e != nil {
		return 0, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO siui_data_drug_terms(import_id,drug_code,ordinal,source_kind,attributes,document,copayment_list,nhp_code,disease_code,disease_name,approval,max_price,reference_price,max_price_ut)
 SELECT $1,m.drug_code,m.ordinal,m.kind,m.attributes,
 jsonb_build_object('drugCode',m.drug_code,'sourceKind',m.kind,'attributes',m.attributes,'copaymentList',cl.attributes,'nationalProgram',nh.attributes,'disease',dc.attributes,'diagnosis',ic.attributes,'nhpDrugType',nt.attributes,
 'unresolvedReferences',jsonb_strip_nulls(jsonb_build_object('copaymentListType',CASE WHEN cl.code IS NULL THEN NULLIF(m.attributes->>'copaymentListType','') END,'nhpCode',CASE WHEN nh.code IS NULL THEN NULLIF(COALESCE(m.attributes->>'nhpCode',m.attributes->>'nhp'),'') END,'diseaseCode',CASE WHEN dc.code IS NULL THEN NULLIF(COALESCE(m.attributes->>'diseaseCode',m.attributes->>'diseasecategory',m.attributes->>'diseaseCategory'),'') END,'icd10',CASE WHEN ic.code IS NULL THEN NULLIF(COALESCE(m.attributes->>'hgIcd10',m.attributes->>'icd10'),'') END)),
 'approval',jsonb_build_object('code',m.attributes->>'needApproval','name',CASE m.attributes->>'needApproval' WHEN '0' THEN 'Fără aprobare' WHEN 'M' THEN 'Ministerul Sănătății' WHEN 'P' THEN 'Ministerul Sănătății Publice' WHEN 'S' THEN 'Medic de specialitate' WHEN 'N' THEN 'CNAS' WHEN 'J' THEN 'CJAS' WHEN 'A' THEN 'Agenția Națională a Transplantului' ELSE NULL END)),
 COALESCE(m.attributes->>'copaymentListType',''),COALESCE(m.attributes->>'nhpCode',m.attributes->>'nhp',''),COALESCE(m.attributes->>'diseaseCode',m.attributes->>'diseasecategory',m.attributes->>'diseaseCategory',''),
 siui_data_drug_fold(concat_ws(' ',dc.attributes->>'description',ic.attributes->>'name')),COALESCE(m.attributes->>'needApproval',''),NULLIF(m.attributes->>'maxPrice','')::numeric,NULLIF(m.attributes->>'referencePrice','')::numeric,NULLIF(m.attributes->>'maxPriceUT','')::numeric
 FROM siui_drug_matches m
 LEFT JOIN siui_drug_entities cl ON cl.kind='CopaymentListType' AND cl.code=m.attributes->>'copaymentListType'
 LEFT JOIN siui_drug_entities nh ON nh.kind='NHP' AND nh.code=COALESCE(m.attributes->>'nhpCode',m.attributes->>'nhp')
 LEFT JOIN siui_drug_entities dc ON dc.kind='DiseaseCategory' AND dc.code=COALESCE(m.attributes->>'diseaseCode',m.attributes->>'diseasecategory',m.attributes->>'diseaseCategory')
 LEFT JOIN siui_drug_entities ic ON ic.kind='ICD10' AND ic.code=COALESCE(m.attributes->>'hgIcd10',m.attributes->>'icd10')
 LEFT JOIN siui_drug_entities nt ON nt.kind='NHP_DRUG_TYPE' AND nt.code=m.attributes->>'type'`, id)
	if e != nil {
		return 0, e
	}
	// Diagnose by active substance independently of compensation restrictions.
	_, e = tx.Exec(ctx, `WITH associations AS (
 SELECT d.code,jsonb_agg(jsonb_build_object('attributes',r.attributes,'diagnosis',ic.attributes,'diseaseCategory',dc.attributes) ORDER BY r.ordinal) AS diagnoses
 FROM siui_data_drugs d JOIN siui_data_records r ON r.import_id=d.import_id AND r.active AND r.kind='ActSubstICD10' AND siui_data_drug_fold(r.attributes->>'activeSubstance')=d.active_substance
 LEFT JOIN siui_drug_entities ic ON ic.kind='ICD10' AND ic.code=r.attributes->>'icd10'
 LEFT JOIN siui_drug_entities dc ON dc.kind='DiseaseCategory' AND dc.code=ic.attributes->>'diseaseCategory'
 WHERE d.import_id=$1 GROUP BY d.code
 ) UPDATE siui_data_drugs d SET document=d.document||jsonb_build_object('diagnoses',a.diagnoses) FROM associations a WHERE d.import_id=$1 AND d.code=a.code`, id)
	if e != nil {
		return 0, e
	}
	_, e = tx.Exec(ctx, `WITH terms AS (
 SELECT drug_code,
 COALESCE(jsonb_agg(document ORDER BY ordinal) FILTER(WHERE source_kind='CopaymentListDrug'),'[]'::jsonb) AS compensations,
 COALESCE(jsonb_agg(document ORDER BY ordinal) FILTER(WHERE source_kind='CopaymentListActiveSubst'),'[]'::jsonb) AS substances,
 COALESCE(jsonb_agg(document ORDER BY ordinal) FILTER(WHERE source_kind='CopaymentListProtocolTherapeutic'),'[]'::jsonb) AS protocols,
 COALESCE(jsonb_agg(document ORDER BY ordinal) FILTER(WHERE source_kind='NHP_DRUG'),'[]'::jsonb) AS programmes
 FROM siui_data_drug_terms WHERE import_id=$1 GROUP BY drug_code
 ) UPDATE siui_data_drugs d SET document=d.document||jsonb_build_object('compensations',t.compensations,'substanceCompensations',t.substances,'therapeuticProtocols',t.protocols,'nationalProgramDrugs',t.programmes) FROM terms t WHERE d.import_id=$1 AND d.code=t.drug_code`, id)
	return count, e
}
