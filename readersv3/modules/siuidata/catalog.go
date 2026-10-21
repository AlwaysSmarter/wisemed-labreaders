package siuidata

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/text/unicode/norm"
)

type record map[string]string

// A record is bounded, but the catalogue is not materialized in Go memory.
// Unknown catalogues and attributes are preserved alongside the known models.
type xmlRecord struct {
	Kind       string      `json:"kind"`
	Attributes record      `json:"attributes"`
	Children   []xmlRecord `json:"children,omitempty"`
	Text       string      `json:"text,omitempty"`
}

type catalogueModel struct {
	Service, Container, Group, GroupAssociation, PackageAssociation string
	Flags                                                           []string
}

var models = map[string]catalogueModel{
	"PHYS": {"", "Physicians", "", "", "", nil},
	"PHM":  {"", "Drugs", "", "", "", nil},
	"CLIN": {"ClinicService", "ClinicServices", "ClinicServiceGroup", "ClinSrvGrpSpecialty", "ClinSrvMedPack", []string{"isExam", "opRoom", "isConnectedService"}},
	"PARA": {"LaboratoriesService", "LaboratoriesServices", "", "", "LabSrvMedPack", []string{"highLevel", "recByPC", "formula", "inClinic"}},
}

var decimalPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)$`)

func catalogueType(s string) (string, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if _, ok := models[s]; !ok {
		return "", errors.New("unknown catalogue type; expected CLIN, PARA, PHM or PHYS")
	}
	return s, nil
}

func normalizeText(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return norm.NFC.String(b.String())
}

func date(s string) (string, error) {
	if len(s) < 10 {
		return "", errors.New("expected XML date")
	}
	if _, e := time.Parse("2006-01-02", s[:10]); e != nil {
		return "", e
	}
	if tail := s[10:]; tail != "" && tail != "Z" {
		if _, e := time.Parse("2006-01-02Z07:00", s); e != nil {
			return "", e
		}
	}
	return s[:10], nil
}

func validity(a record, today string) (from, to *string, active bool, err error) {
	if a["validFrom"] != "" {
		v, e := date(a["validFrom"])
		if e != nil {
			return nil, nil, false, e
		}
		from = &v
	}
	if a["validTo"] != "" {
		v, e := date(a["validTo"])
		if e != nil {
			return nil, nil, false, e
		}
		to = &v
	}
	if from != nil && to != nil && *to < *from {
		// Real CNAS sources contain reversed historical periods. Preserve the
		// source record, keep it inactive and report it in version statistics.
		return from, to, false, nil
	}
	return from, to, (from == nil || *from <= today) && (to == nil || today <= *to), nil
}

// catalogueRows implements pgx.CopyFromSource: each Next decodes just one XML
// record. Multiple XML members may contribute to a single import transaction.
type catalogueRows struct {
	ctx       context.Context
	d         *xml.Decoder
	model     catalogueModel
	asOf      string
	stack     []xml.Name
	ns        string
	issueDate string
	roots     int
	seen      map[string]bool
	row       xmlRecord
	container string
	ordinal   *int64
	err       error
	bytes     *countReader
}

func newCatalogueRows(ctx context.Context, r io.Reader, model catalogueModel, asOf string, ordinal *int64) *catalogueRows {
	c := &countReader{r: r}
	return &catalogueRows{ctx: ctx, d: xml.NewDecoder(c), model: model, asOf: asOf, ordinal: ordinal, seen: map[string]bool{}, bytes: c}
}

func (c *catalogueRows) Next() bool {
	if c.err != nil {
		return false
	}
	c.bytes.until = c.bytes.n + (8 << 20)
	for {
		if c.err = c.ctx.Err(); c.err != nil {
			return false
		}
		tok, e := c.d.Token()
		if e == io.EOF {
			if c.roots != 1 || len(c.stack) != 0 {
				c.err = errors.New("one complete Catalogues document required")
			}
			return false
		}
		if e != nil {
			c.err = e
			return false
		}
		switch v := tok.(type) {
		case xml.Directive:
			c.err = errors.New("XML directives/DTDs are forbidden")
			return false
		case xml.CharData:
			if strings.TrimSpace(string(v)) != "" {
				c.err = errors.New("unexpected text outside catalogue record")
				return false
			}
		case xml.StartElement:
			if len(c.stack) == 0 {
				c.roots++
				if c.roots != 1 || v.Name.Local != "Catalogues" || (v.Name.Space != "" && v.Name.Space != "http://www.cnas.ro/siui/2.0") {
					c.err = errors.New("expected CNAS Catalogues root")
					return false
				}
				c.ns = v.Name.Space
				for _, a := range v.Attr {
					if a.Name.Local == "issueDate" && len(a.Value) >= 10 {
						if value, e := date(a.Value[:10]); e == nil {
							c.issueDate = value
						} else {
							c.err = errors.New("invalid catalogue issueDate")
							return false
						}
					}
				}
			}
			if v.Name.Space != c.ns {
				c.err = errors.New("unexpected XML namespace")
				return false
			}
			if len(c.stack) == 1 {
				if c.seen[v.Name.Local] {
					c.err = errors.New("duplicate catalogue container")
					return false
				}
				c.seen[v.Name.Local] = true
			}
			if len(c.stack) == 2 {
				start := c.d.InputOffset()
				c.row, c.err = c.decodeRecord(v, 0, start)
				if c.err != nil {
					return false
				}
				c.container = c.stack[1].Local
				if c.row.Kind == c.model.Service && c.container != c.model.Container {
					c.err = errors.New("service in incorrect catalogue container")
					return false
				}
				if c.err = c.validate(); c.err != nil {
					return false
				}
				*c.ordinal++
				return true
			}
			c.stack = append(c.stack, v.Name)
		case xml.EndElement:
			if len(c.stack) == 0 {
				c.err = errors.New("unexpected XML closing tag")
				return false
			}
			c.stack = c.stack[:len(c.stack)-1]
		}
	}
}

func (c *catalogueRows) decodeRecord(start xml.StartElement, depth int, offset int64) (xmlRecord, error) {
	r := xmlRecord{Kind: start.Name.Local, Attributes: record{}}
	if start.Name.Space != c.ns || depth > 32 {
		return r, errors.New("invalid namespace or XML nesting too deep")
	}
	for _, a := range start.Attr {
		if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
			continue
		}
		key := a.Name.Local
		if a.Name.Space != "" {
			key = "{" + a.Name.Space + "}" + key
		}
		if _, exists := r.Attributes[key]; exists {
			return r, errors.New("duplicate XML attribute")
		}
		r.Attributes[key] = a.Value
	}
	for {
		if e := c.ctx.Err(); e != nil {
			return r, e
		}
		if c.d.InputOffset()-offset > 8<<20 {
			return r, errors.New("single XML record exceeds 8 MiB")
		}
		tok, e := c.d.Token()
		if e != nil {
			return r, e
		}
		switch v := tok.(type) {
		case xml.Directive:
			return r, errors.New("XML directives/DTDs are forbidden")
		case xml.StartElement:
			child, e := c.decodeRecord(v, depth+1, offset)
			if e != nil {
				return r, e
			}
			r.Children = append(r.Children, child)
		case xml.CharData:
			r.Text += string(v)
		case xml.EndElement:
			r.Text = strings.TrimSpace(r.Text)
			return r, nil
		}
	}
}

func (c *catalogueRows) validate() error {
	a, kind := c.row.Attributes, c.row.Kind
	required := map[string][]string{
		c.model.Service: {"code", "name", "validFrom"}, "ClinicServiceGroup": {"code", "description", "validFrom"},
		"Speciality": {"code", "name", "validFrom"}, "MedSrvPack": {"code", "name", "validFrom"},
		"ClinSrvGrpSpecialty": {"serviceGroup", "specialty", "validFrom"},
		"ClinSrvMedPack":      {"service", "medSrvPack", "validFrom"}, "LabSrvMedPack": {"service", "medSrvPack", "validFrom"},
	}
	// Strict physician graph checks belong to getPhysicianCatalogues. Ordinary
	// catalogues can contain incomplete auxiliary physician records to preserve.
	if c.model.Container == "Physicians" {
		required["Physician"] = []string{"stencil", "name", "validFrom"}
		required["PhysicianSpeciality"] = []string{"stencil", "specialityCode", "insuranceHouse", "contractType", "contractNo", "physicianType", "validFrom"}
	}
	for k, v := range map[string][]string{
		"Drug":                     {"code", "name", "activeSubstance", "concentration", "pharmaceuticalForm", "country", "atc", "validFrom"},
		"CopaymentListDrug":        {"drug", "copaymentListType", "validFrom"},
		"CopaymentListActiveSubst": {"activeSubstance", "copaymentListType", "validFrom"},
		"ActSubstICD10":            {"activeSubstance", "icd10", "validFrom"},
	} {
		required[k] = v
	}
	if kind == "Drug" || kind == "CopaymentListDrug" || kind == "NHP_DRUG" || kind == "CopaymentListType" {
		for _, key := range []string{"pricePerPackage", "wholeSalePricePerPackage", "qtyPerPackage", "maxPrice", "maxPriceUT", "referencePrice", "wholeSalePrice", "copaymentValue", "copaymentValue90", "price", "percent"} {
			if value := a[key]; value != "" && (!decimalPattern.MatchString(value) || strings.HasPrefix(value, "-")) {
				return fmt.Errorf("invalid nonnegative decimal %s.%s", kind, key)
			}
		}
	}
	if kind == "Drug" {
		for _, key := range []string{"isSpecial", "isFractional", "hasBioEchiv", "isBrand"} {
			if value := a[key]; value != "" && value != "true" && value != "false" && value != "0" && value != "1" {
				return fmt.Errorf("invalid boolean Drug.%s", key)
			}
		}
	}
	for _, key := range required[kind] {
		if strings.TrimSpace(a[key]) == "" {
			return fmt.Errorf("%s missing %s", kind, key)
		}
	}
	if kind == c.model.Service {
		for _, key := range c.model.Flags {
			if a[key] != "true" && a[key] != "false" && a[key] != "0" && a[key] != "1" {
				return fmt.Errorf("invalid or missing %s.%s", kind, key)
			}
		}
	}
	if kind == "ClinSrvMedPack" || kind == "LabSrvMedPack" {
		for _, key := range []string{"points", "pointsCtrl", "pointsSrg", "valueLei"} {
			if a[key] != "" && !decimalPattern.MatchString(a[key]) {
				return fmt.Errorf("invalid decimal %s.%s", kind, key)
			}
		}
	}
	_, _, _, e := validity(a, c.asOf)
	return e
}

func (c *catalogueRows) Values() ([]any, error) {
	a := c.row.Attributes
	from, to, active, e := validity(a, c.asOf)
	if e != nil {
		return nil, e
	}
	attrs, e := json.Marshal(a)
	if e != nil {
		return nil, e
	}
	payload, e := json.Marshal(c.row)
	if e != nil {
		return nil, e
	}
	toDate := func(s *string) pgtype.Date {
		if s == nil {
			return pgtype.Date{}
		}
		v, _ := time.Parse("2006-01-02", *s)
		return pgtype.Date{Time: v, Valid: true}
	}
	code := a["code"]
	if c.row.Kind == "Physician" {
		code = a["stencil"]
	}
	if c.row.Kind == "Concentration" {
		code = a["concentration"]
	}
	if c.row.Kind == "CopaymentListProtocolTherapeutic" {
		code = a["codeProtocolTherap"]
	}
	label := a["name"] + " " + a["description"]
	if strings.TrimSpace(label) == "" {
		label = code + " " + a["descProtocolTherap"]
	}
	return []any{*c.ordinal, c.row.Kind, c.container, code, normalizeText(label), attrs, payload, toDate(from), toDate(to), active}, nil
}
func (c *catalogueRows) Err() error { return c.err }

type countReader struct {
	r     io.Reader
	n     int64
	until int64
}

func (r *countReader) Read(p []byte) (int, error) {
	if r.until > 0 {
		if r.n >= r.until {
			return 0, errors.New("single XML record exceeds 8 MiB")
		}
		if int64(len(p)) > r.until-r.n {
			p = p[:r.until-r.n]
		}
	}
	n, e := r.r.Read(p)
	r.n += int64(n)
	return n, e
}

func catalogueLockKey(typ string) int32 {
	switch typ {
	case "PARA":
		return 2
	case "PHM":
		return 3
	case "PHYS":
		return 4
	default:
		return 1
	}
}
