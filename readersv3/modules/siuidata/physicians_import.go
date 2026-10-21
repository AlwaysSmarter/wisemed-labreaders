package siuidata

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

var physicianFilename = regexp.MustCompile(`(?i)^Medici(CNAS|OPSNAJ)_([0-9]{8})\.xml$`)

func physicianSource(name string) string {
	p := physicianFilename.FindStringSubmatch(filepath.Base(name))
	if p == nil {
		return ""
	}
	return strings.ToUpper(p[1])
}

func buildPhysicians(ctx context.Context, tx pgx.Tx, id string) (int64, error) {
	result, e := tx.Exec(ctx, `INSERT INTO siui_data_physicians(import_id,source,stencil,name,name_normalized,attributes,document)
 SELECT import_id,source,code,attributes->>'name',name_normalized,attributes,
 jsonb_build_object('kind','Physician','type','PHYS','source',source,'stencil',code,'code',code,'name',attributes->>'name','attributes',attributes,'specialities','[]'::jsonb)
 FROM siui_data_records WHERE import_id=$1 AND active AND kind='Physician'`, id)
	if e != nil {
		return 0, e
	}
	count := result.RowsAffected()
	_, e = tx.Exec(ctx, `INSERT INTO siui_data_physician_relations(import_id,source,stencil,ordinal,attributes,document,specialty_code,specialty_name,contract_type,contract_no,physician_type,insurance_house,insurance_house_name)
 SELECT a.import_id,a.source,p.stencil,a.ordinal,a.attributes,
 jsonb_build_object('attributes',a.attributes,'speciality',sp.attributes,'insuranceHouse',ih.attributes,'insuranceHouseType',ht.attributes),
 COALESCE(a.attributes->>'specialityCode',''),COALESCE(sp.name_normalized,''),COALESCE(a.attributes->>'contractType',''),COALESCE(a.attributes->>'contractNo',''),COALESCE(a.attributes->>'physicianType',''),COALESCE(a.attributes->>'insuranceHouse',''),COALESCE(ih.name_normalized,'')
 FROM siui_data_records a JOIN siui_data_physicians p ON p.import_id=a.import_id AND p.source=a.source AND p.stencil=a.attributes->>'stencil'
 LEFT JOIN siui_data_records sp ON sp.import_id=a.import_id AND sp.source=a.source AND sp.active AND sp.kind='Speciality' AND sp.code=a.attributes->>'specialityCode'
 LEFT JOIN siui_data_records ih ON ih.import_id=a.import_id AND ih.source=a.source AND ih.active AND ih.kind='InsuranceHouse' AND ih.code=a.attributes->>'insuranceHouse'
 LEFT JOIN siui_data_records ht ON ht.import_id=a.import_id AND ht.source=a.source AND ht.active AND ht.kind='InsuranceHouseType' AND ht.code=ih.attributes->>'type'
 WHERE a.import_id=$1 AND a.active AND a.kind='PhysicianSpeciality'`, id)
	if e != nil {
		return 0, fmt.Errorf("physician relationships: %w", e)
	}
	_, e = tx.Exec(ctx, `WITH relations AS(SELECT source,stencil,jsonb_agg(document ORDER BY ordinal) AS details FROM siui_data_physician_relations WHERE import_id=$1 GROUP BY source,stencil)
 UPDATE siui_data_physicians p SET document=p.document||jsonb_build_object('specialities',r.details) FROM relations r WHERE p.import_id=$1 AND p.source=r.source AND p.stencil=r.stencil`, id)
	return count, e
}

func physicianArchiveDates(f *os.File, maxXML int64) (map[string]string, error) {
	info, e := f.Stat()
	if e != nil {
		return nil, e
	}
	members, e := archiveMembers(f, info.Size(), maxXML)
	if e != nil {
		return nil, e
	}
	dates := map[string]string{}
	for _, member := range members {
		source := physicianSource(member.Name)
		if source == "" || dates[source] != "" {
			return nil, errors.New("invalid or duplicate physician source member")
		}
		r, e := member.Open()
		if e != nil {
			return nil, e
		}
		day, e := xmlIssueDate(r)
		r.Close()
		if e != nil {
			return nil, e
		}
		if day == "" {
			return nil, errors.New("physician source has no issueDate")
		}
		dates[source] = day
	}
	if dates["CNAS"] == "" || dates["OPSNAJ"] == "" {
		return nil, errors.New("physician archive must include both CNAS and OPSNAJ")
	}
	return dates, nil
}
