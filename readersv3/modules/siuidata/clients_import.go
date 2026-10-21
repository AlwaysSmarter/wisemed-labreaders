package siuidata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type clientDocument struct {
	Provider                      record
	CUI, Category, Filename, Hash string
	Effective                     time.Time
	Contracts, Employees          []xmlRecord
}

type providerDecoder struct {
	d   *xml.Decoder
	ctx context.Context
	ns  string
}

func (p *providerDecoder) token() (xml.Token, error) {
	if e := p.ctx.Err(); e != nil {
		return nil, e
	}
	t, e := p.d.Token()
	if e != nil {
		return nil, e
	}
	switch v := t.(type) {
	case xml.Directive:
		return nil, errors.New("XML directives are forbidden")
	case xml.StartElement:
		if v.Name.Space != p.ns {
			return nil, errors.New("unexpected XML namespace")
		}
	}
	return t, nil
}
func privateAttrs(start xml.StartElement) (record, error) {
	a := record{}
	for _, v := range start.Attr {
		if v.Name.Space == "xmlns" || v.Name.Local == "xmlns" {
			continue
		}
		if v.Name.Space != "" {
			continue
		}
		if _, ok := a[v.Name.Local]; ok {
			return nil, errors.New("duplicate XML attribute")
		}
		if len(v.Value) > 4096 {
			return nil, errors.New("XML attribute too large")
		}
		a[v.Name.Local] = v.Value
	}
	return a, nil
}
func (p *providerDecoder) skip() error {
	depth := 1
	for depth > 0 {
		t, e := p.token()
		if e != nil {
			return e
		}
		switch t.(type) {
		case xml.StartElement:
			depth++
			if depth > 64 {
				return errors.New("XML nesting too deep")
			}
		case xml.EndElement:
			depth--
		}
	}
	return nil
}
func (p *providerDecoder) node(start xml.StartElement, depth int, offset int64, employee bool) (xmlRecord, error) {
	if depth > 32 {
		return xmlRecord{}, errors.New("XML nesting too deep")
	}
	a, e := privateAttrs(start)
	r := xmlRecord{Kind: start.Name.Local, Attributes: a}
	if e != nil {
		return r, e
	}
	for {
		if p.d.InputOffset()-offset > 8<<20 {
			return r, errors.New("personalization record exceeds 8 MiB")
		}
		t, e := p.token()
		if e != nil {
			return r, e
		}
		switch v := t.(type) {
		case xml.StartElement:
			// A provider export can include patient enrolments (MF). Do not retain them
			// in this client-contract knowledge base; only employee specialities matter.
			if employee && v.Name.Local != "specialities" {
				if e = p.skip(); e != nil {
					return r, e
				}
				continue
			}
			child, e := p.node(v, depth+1, offset, false)
			if e != nil {
				return r, e
			}
			r.Children = append(r.Children, child)
		case xml.CharData:
			if strings.TrimSpace(string(v)) != "" {
				return r, errors.New("unexpected personalization text")
			}
		case xml.EndElement:
			return r, nil
		}
	}
}

func parseClientDocument(ctx context.Context, r io.Reader, category, filename string, loc *time.Location) (clientDocument, error) {
	out := clientDocument{Category: category, Filename: filename}
	h := sha256.New()
	d := xml.NewDecoder(io.TeeReader(contextReader{ctx, r}, h))
	p := providerDecoder{d: d, ctx: ctx}
	rootSeen, rootClosed := false, false
	for {
		t, e := d.Token()
		if e == io.EOF {
			if !rootSeen || !rootClosed {
				return out, errors.New("complete provider document required")
			}
			break
		}
		if e != nil {
			return out, e
		}
		switch v := t.(type) {
		case xml.Directive:
			return out, errors.New("XML directives are forbidden")
		case xml.CharData:
			if strings.Trim(strings.TrimSpace(string(v)), "\ufeff") != "" {
				return out, errors.New("unexpected text outside provider")
			}
		case xml.StartElement:
			if rootSeen || v.Name.Local != "provider" || (v.Name.Space != "" && v.Name.Space != "http://www.cnas.ro/siui/2.0") {
				return out, errors.New("expected one CNAS provider document")
			}
			rootSeen = true
			p.ns = v.Name.Space
			out.Provider, e = privateAttrs(v)
			if e != nil {
				return out, e
			}
			out.CUI, e = normalizeCUI(out.Provider["uic"])
			if e != nil {
				return out, e
			}
			for _, key := range []string{"code", "name", "issueDate"} {
				if strings.TrimSpace(out.Provider[key]) == "" {
					return out, fmt.Errorf("provider missing %s", key)
				}
			}
			out.Effective, e = time.Parse(time.RFC3339, out.Provider["issueDate"])
			if e != nil {
				out.Effective, e = time.ParseInLocation("2006-01-02T15:04:05", out.Provider["issueDate"], loc)
			}
			if e != nil {
				return out, errors.New("invalid provider issueDate")
			}
			containers := map[string]bool{}
			for !rootClosed {
				t, e := p.token()
				if e != nil {
					return out, e
				}
				switch c := t.(type) {
				case xml.EndElement:
					rootClosed = true
				case xml.CharData:
					if strings.TrimSpace(string(c)) != "" {
						return out, errors.New("unexpected provider text")
					}
				case xml.StartElement:
					container := c.Name.Local
					if containers[container] {
						return out, errors.New("duplicate provider container")
					}
					containers[container] = true
					if container != "contracts" && container != "employees" && container != "physicians" && container != "conventions" {
						if e = p.skip(); e != nil {
							return out, e
						}
						continue
					}
					for {
						t, e := p.token()
						if e != nil {
							return out, e
						}
						if _, ok := t.(xml.EndElement); ok {
							break
						}
						start, ok := t.(xml.StartElement)
						if !ok {
							if txt, ok := t.(xml.CharData); ok && strings.TrimSpace(string(txt)) != "" {
								return out, errors.New("unexpected container text")
							}
							continue
						}
						expected := map[string]string{"contracts": "contract", "employees": "employee", "physicians": "physician", "conventions": "convention"}[container]
						if start.Name.Local != expected {
							return out, errors.New("unexpected provider record")
						}
						node, e := p.node(start, 0, p.d.InputOffset(), container == "employees" || container == "physicians")
						if e != nil {
							return out, e
						}
						if container == "contracts" || container == "conventions" {
							out.Contracts = append(out.Contracts, node)
						} else {
							out.Employees = append(out.Employees, node)
						}
					}
				}
			}
		}
	}
	out.Hash = hex.EncodeToString(h.Sum(nil))
	return out, nil
}

func contractCategory(raw, fallback string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	aliases := map[string]string{"CLN": "CLIN", "PAR": "PARA", "CMG": "MF", "STM": "STOM", "PNS": "NHP", "PRM": "MF", "RCA": "REC", "RCS": "REC", "CM": "SICK"}
	if v := aliases[s]; v != "" {
		s = v
	}
	if s == "" {
		return fallback
	}
	return s
}
func clientPeriod(a record, required bool) (string, *string, error) {
	from := a["validFrom"]
	if from == "" && required {
		return "", nil, errors.New("missing validFrom")
	}
	if from == "" {
		from = "0001-01-01"
	}
	v, e := date(from)
	if e != nil {
		return "", nil, e
	}
	var to *string
	if a["validTo"] != "" {
		value, e := date(a["validTo"])
		if e != nil {
			return "", nil, e
		}
		if value < v {
			return "", nil, errors.New("invalid reversed validity period")
		}
		to = &value
	}
	return v, to, nil
}

func saveClientDocument(ctx context.Context, tx pgx.Tx, doc clientDocument) (string, bool, error) {
	id := uuid.NewString()
	stats := map[string]int{"contracts": len(doc.Contracts), "employees": len(doc.Employees), "specialities": 0}
	var existing, hash string
	e := tx.QueryRow(ctx, `SELECT id::text,sha256 FROM siui_client_snapshots WHERE cui=$1 AND category=$2 AND provider_code=$3 AND effective_at=$4`, doc.CUI, doc.Category, doc.Provider["code"], doc.Effective).Scan(&existing, &hash)
	if e == nil {
		if hash != doc.Hash {
			return "", false, errors.New("different personalization already exists at this exact issueDate; import a new issueDate")
		}
		return existing, false, nil
	}
	if e != pgx.ErrNoRows {
		return "", false, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO siui_client_snapshots(id,cui,category,provider_code,provider_name,provider,effective_at,source_filename,sha256,statistics) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, doc.CUI, doc.Category, doc.Provider["code"], doc.Provider["name"], doc.Provider, doc.Effective, doc.Filename, doc.Hash, stats)
	if e != nil {
		return "", false, e
	}
	for i, node := range doc.Contracts {
		a := node.Attributes
		if node.Kind == "contract" {
			for _, key := range []string{"categoryCode", "orgUnitCode", "companyCode", "contractDate"} {
				if strings.TrimSpace(a[key]) == "" {
					return "", false, fmt.Errorf("contract missing %s", key)
				}
			}
			if _, e := date(a["contractDate"]); e != nil {
				return "", false, errors.New("invalid contractDate")
			}
		}
		from, to, e := clientPeriod(a, true)
		if e != nil {
			return "", false, fmt.Errorf("contract validity: %w", e)
		}
		number := a["contractNo"]
		if node.Kind == "convention" {
			number = a["conventionNo"]
		}
		if number == "" {
			return "", false, errors.New("missing contract/convention number")
		}
		category := contractCategory(a["categoryCode"], doc.Category)
		_, e = tx.Exec(ctx, `INSERT INTO siui_client_contracts(snapshot_id,ordinal,category,contract_no,valid_from,valid_to,document) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, i, category, number, from, to, node)
		if e != nil {
			return "", false, e
		}
	}
	for i, node := range doc.Employees {
		a := node.Attributes
		from, to, e := clientPeriod(a, true)
		if e != nil {
			return "", false, fmt.Errorf("employee validity: %w", e)
		}
		stencil := a["stencilNo"]
		if stencil == "" {
			stencil = a["stencil"]
		}
		name := strings.TrimSpace(a["firstName"] + " " + a["lastName"])
		if name == "" {
			name = a["name"]
		}
		_, e = tx.Exec(ctx, `INSERT INTO siui_client_employees(snapshot_id,ordinal,stencil,name,valid_from,valid_to,attributes) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, i, stencil, name, from, to, a)
		if e != nil {
			return "", false, e
		}
		n := 0
		for _, container := range node.Children {
			if container.Kind != "specialities" {
				continue
			}
			for _, sp := range container.Children {
				if sp.Kind != "speciality" || sp.Attributes["code"] == "" {
					return "", false, errors.New("invalid employee speciality")
				}
				sf, st, e := clientPeriod(sp.Attributes, false)
				if e != nil {
					return "", false, e
				}
				_, e = tx.Exec(ctx, `INSERT INTO siui_client_specialties(snapshot_id,employee_ordinal,ordinal,code,valid_from,valid_to,attributes) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, i, n, sp.Attributes["code"], sf, st, sp.Attributes)
				if e != nil {
					return "", false, e
				}
				n++
				stats["specialities"]++
			}
		}
	}
	_, e = tx.Exec(ctx, `UPDATE siui_client_snapshots SET statistics=$2 WHERE id=$1`, id, stats)
	return id, true, e
}

// Same-day query semantics: select the last exported snapshot by the end of
// the requested Bucharest day, independently for each provider/category.
const clientLatestSQL = `SELECT DISTINCT ON(category,provider_code) id,category,provider_code FROM siui_client_snapshots WHERE cui=$1 AND effective_at<(($2::date+1)::timestamp AT TIME ZONE 'Europe/Bucharest') ORDER BY category,provider_code,effective_at DESC`

func clientSpecialties(ctx context.Context, tx pgx.Tx, cui, day string) ([]string, error) {
	rows, e := tx.Query(ctx, `WITH latest AS (`+clientLatestSQL+`) SELECT DISTINCT sp.code FROM latest l
 JOIN siui_client_employees e ON e.snapshot_id=l.id JOIN siui_client_specialties sp ON sp.snapshot_id=e.snapshot_id AND sp.employee_ordinal=e.ordinal
 WHERE e.stencil<>'' AND e.valid_from<=$2::date AND (e.valid_to IS NULL OR e.valid_to>=$2::date)
 AND sp.valid_from<=$2::date AND (sp.valid_to IS NULL OR sp.valid_to>=$2::date)
 AND EXISTS(SELECT 1 FROM siui_client_contracts c WHERE c.snapshot_id=l.id AND c.category='CLIN' AND c.document->>'kind'='contract' AND c.valid_from<=$2::date AND (c.valid_to IS NULL OR c.valid_to>=$2::date)) ORDER BY sp.code`, cui, day)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	codes := []string{}
	for rows.Next() {
		var code string
		if e = rows.Scan(&code); e != nil {
			return nil, e
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}
