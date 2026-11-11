package wisemedai

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	xhtml "golang.org/x/net/html"
)

type Field struct {
	Help        string   `json:"help,omitempty"`
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Unit        string   `json:"unit"`
	Required    bool     `json:"required"`
	Invisible   bool     `json:"invisible"`
	Aliases     []string `json:"aliases"`
	Min         *float64 `json:"min"`
	Max         *float64 `json:"max"`
	SemanticTag string   `json:"semantic_tag"`
	Section     string   `json:"section"`
	Repeatable  bool     `json:"repeatable"`
}
type Rule struct {
	Field         string `json:"field"`
	Equals        string `json:"equals"`
	RequiredField string `json:"required_field"`
}
type Section struct {
	SpeechTerms    []string         `json:"speech_terms,omitempty"`
	HideWhenEmpty  bool             `json:"hide_when_empty,omitempty"`
	Detail         string           `json:"detail,omitempty"`
	Variants       []SectionVariant `json:"variants,omitempty"`
	ID             string           `json:"id"`
	Label          string           `json:"label"`
	Parent         string           `json:"parent"`
	ConditionField string           `json:"condition_field"`
	ConditionValue string           `json:"condition_value"`
	Repeatable     bool             `json:"repeatable"`
}
type Template struct {
	AddonMonthlyMinor int64      `json:"addon_monthly_minor,omitempty"`
	AddonCurrency     string     `json:"addon_currency,omitempty"`
	AddonEndsAt       *time.Time `json:"addon_ends_at,omitempty"`

	ModelVersion    int                 `json:"model_version,omitempty"`
	DefaultDetail   string              `json:"default_detail,omitempty"`
	Presentation    PresentationOptions `json:"-"`
	AIContext       *TemplateContext    `json:"ai_context,omitempty"`
	GenerationID    string              `json:"generation_id,omitempty"`
	OwnerID         string              `json:"owner_id,omitempty"`
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Specialty       string              `json:"specialty"`
	Specialties     []string            `json:"specialties,omitempty"`
	VersionID       string              `json:"version_id"`
	Version         int                 `json:"version"`
	HTML            string              `json:"html"`
	Fields          []Field             `json:"fields"`
	Rules           []Rule              `json:"rules"`
	Sections        []Section           `json:"sections"`
	Fragments       map[string]string   `json:"fragments"`
	SemanticMapping map[string]string   `json:"semantic_mapping"`
}

func (t Template) allowedSpecialties() []string {
	if len(t.Specialties) > 0 {
		return t.Specialties
	}
	return []string{t.Specialty}
}

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var placeholder = regexp.MustCompile(`\{\{([a-z][a-z0-9_]*)\}\}`)

func sanitizeHTML(src string) string {
	root, e := xhtml.Parse(strings.NewReader(src))
	if e != nil {
		return ""
	}
	var out strings.Builder
	allowed := map[string]bool{"p": true, "h1": true, "h2": true, "h3": true, "strong": true, "em": true, "ul": true, "ol": true, "li": true, "table": true, "thead": true, "tbody": true, "tr": true, "th": true, "td": true, "br": true, "span": true, "blockquote": true, "section": true, "u": true, "s": true, "hr": true}
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.TextNode {
			out.WriteString(html.EscapeString(n.Data))
			return
		}
		if n.Type == xhtml.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "iframe" || n.Data == "svg" || n.Data == "object") {
			return
		}
		tag := n.Type == xhtml.ElementNode && allowed[n.Data]
		if tag {
			out.WriteString("<" + n.Data)
			for _, a := range n.Attr {
				// Retain the editor's constrained formatting, never arbitrary CSS.
				if a.Key == "style" && (n.Data == "p" || n.Data == "h1" || n.Data == "h2" || n.Data == "h3") {
					for _, declaration := range strings.Split(a.Val, ";") {
						parts := strings.SplitN(declaration, ":", 2)
						if len(parts) != 2 || strings.ToLower(strings.TrimSpace(parts[0])) != "text-align" {
							continue
						}
						value := strings.ToLower(strings.TrimSpace(parts[1]))
						if value == "left" || value == "center" || value == "right" || value == "justify" {
							out.WriteString(` style="text-align: ` + value + `"`)
							break
						}
					}
				}
				if (a.Key == "colspan" || a.Key == "rowspan") && (n.Data == "td" || n.Data == "th") {
					if size, err := strconv.Atoi(a.Val); err == nil && size >= 1 && size <= 100 {
						out.WriteString(" " + a.Key + `="` + strconv.Itoa(size) + `"`)
					}
				}
				if a.Key == "start" && n.Data == "ol" {
					if start, err := strconv.Atoi(a.Val); err == nil && start >= 1 && start <= 10000 {
						out.WriteString(` start="` + strconv.Itoa(start) + `"`)
					}
				}
				if (a.Key == "data-field" || a.Key == "data-section" || a.Key == "data-review-field") && keyPattern.MatchString(a.Val) {
					out.WriteString(" " + a.Key + `="` + html.EscapeString(a.Val) + `"`)
				}
			}
			out.WriteString(">")
			if n.Data == "br" || n.Data == "hr" {
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if tag {
			out.WriteString("</" + n.Data + ">")
		}
	}
	walk(root)
	return out.String()
}
func (t *Template) validate() error {
	if len(t.Name) < 2 || len(t.Name) > 180 || len(t.Fields) == 0 || len(t.Fields) > 150 || len(t.HTML) > 200000 {
		return errors.New("invalid_template")
	}
	if e := t.AIContext.validate(); e != nil {
		return e
	}
	keys := map[string]bool{}
	for _, f := range t.Fields {
		if !keyPattern.MatchString(f.Key) || keys[f.Key] || f.Label == "" || f.Key == "patient_ref" || f.Key == "doctor_name" {
			return errors.New("invalid_field_key")
		}
		keys[f.Key] = true
		if f.Type != "string" && f.Type != "number" && f.Type != "boolean" {
			return errors.New("invalid_field_type")
		}
		if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
			return errors.New("invalid_field_range")
		}
	}
	for _, r := range t.Rules {
		if !keys[r.Field] || !keys[r.RequiredField] {
			return errors.New("invalid_rule")
		}
	}
	sections := map[string]bool{}
	for _, s := range t.Sections {
		budget := 0
		if len(s.SpeechTerms) > 64 {
			return errors.New("too_many_section_speech_terms")
		}
		seenTerms := map[string]bool{}
		for _, term := range s.SpeechTerms {
			key := speechKey(term)
			if term != normalizeSpeech(term) || !safeSpeechTerm(term) || seenTerms[key] {
				return errors.New("invalid_section_speech_term")
			}
			seenTerms[key] = true
			budget += len(term) + 1
		}
		if budget > activeSpeechBudget {
			return errors.New("section_speech_terms_too_large")
		}
		if !keyPattern.MatchString(s.ID) || sections[s.ID] || s.Parent == s.ID {
			return errors.New("invalid_section")
		}
		if s.ConditionField != "" && !keys[s.ConditionField] {
			return errors.New("invalid_section_condition")
		}
		sections[s.ID] = true
	}
	for _, s := range t.Sections {
		seen := map[string]bool{s.ID: true}
		parent := s.Parent
		for parent != "" {
			if seen[parent] || !sections[parent] {
				return errors.New("cyclic_or_missing_parent")
			}
			seen[parent] = true
			next := ""
			for _, p := range t.Sections {
				if p.ID == parent {
					next = p.Parent
				}
			}
			parent = next
		}
	}
	for _, f := range t.Fields {
		if f.Section != "" && !sections[f.Section] {
			return errors.New("missing_section")
		}
	}
	if e := validateVariants(*t); e != nil {
		return e
	}
	t.HTML = sanitizeHTML(t.HTML)
	for k, v := range t.Fragments {
		t.Fragments[k] = sanitizeHTML(v)
	}
	return nil
}
func loadTemplate(ctx context.Context, tx pgx.Tx, tenant, id string) (t Template, e error) {
	var b []byte
	e = tx.QueryRow(ctx, `SELECT document FROM wai_template_versions WHERE tenant_id=$1 AND id::text=$2`, tenant, id).Scan(&b)
	if e == nil {
		e = json.Unmarshal(b, &t)
	}
	return
}
func saveTemplate(ctx context.Context, tx pgx.Tx, tenant string, t *Template) error {
	if e := t.validate(); e != nil {
		return e
	}
	if t.ID == "" {
		t.ID = uuid.NewString()
		if _, e := tx.Exec(ctx, `INSERT INTO wai_templates(id,tenant_id,name,specialty)VALUES($1,$2,$3,$4)`, t.ID, tenant, t.Name, t.Specialty); e != nil {
			return e
		}
	}
	var exists string
	if e := tx.QueryRow(ctx, `SELECT id FROM wai_templates WHERE id::text=$1 AND tenant_id=$2 FOR UPDATE`, t.ID, tenant).Scan(&exists); e != nil {
		return e
	}
	if e := tx.QueryRow(ctx, `SELECT coalesce(max(version),0)+1 FROM wai_template_versions WHERE template_id=$1 AND tenant_id=$2`, t.ID, tenant).Scan(&t.Version); e != nil {
		return e
	}
	t.VersionID = uuid.NewString()
	stored := *t
	stored.AIContext = nil
	b, e := json.Marshal(stored)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO wai_template_versions(id,tenant_id,template_id,version,document)VALUES($1,$2,$3,$4,$5)`, t.VersionID, tenant, t.ID, t.Version, b); e != nil {
		return e
	}
	if e = publishSpeechGrammar(ctx, tx, tenant, *t); e != nil {
		return e
	}
	for _, f := range t.Fields {
		b, _ := json.Marshal(f)
		if _, e = tx.Exec(ctx, `INSERT INTO wai_template_fields(id,tenant_id,version_id,field_key,definition)VALUES($1,$2,$3,$4,$5)`, uuid.NewString(), tenant, t.VersionID, f.Key, b); e != nil {
			return e
		}
	}
	for _, r := range t.Rules {
		b, _ := json.Marshal(r)
		if _, e = tx.Exec(ctx, `INSERT INTO wai_template_rules(id,tenant_id,version_id,definition)VALUES($1,$2,$3,$4)`, uuid.NewString(), tenant, t.VersionID, b); e != nil {
			return e
		}
	}
	_, e = tx.Exec(ctx, `UPDATE wai_templates SET current_version=$1,name=$2,specialty=$3 WHERE id=$4 AND tenant_id=$5`, t.VersionID, t.Name, t.Specialty, t.ID, tenant)
	return e
}

const visibleTemplateSQL = `t.tenant_id=$1 AND (t.owner_id IS NULL OR t.owner_id=$2 OR $3)
	AND ($3 OR EXISTS(SELECT 1 FROM wai_user_specialties s WHERE s.user_id=$2
		AND (s.specialty=t.specialty OR jsonb_exists(v.document->'specialties',s.specialty))))`

func (m *Module) templates(w http.ResponseWriter, r *http.Request, p principal, suffix string) {
	ctx := r.Context()
	if r.Method == "GET" {
		rows, e := m.db.Query(ctx, `SELECT v.document,t.owner_id::text,c.payload,coalesce(a.monthly_minor,0),coalesce(a.currency,''),a.ends_at FROM wai_templates t JOIN wai_template_versions v ON v.id=t.current_version AND v.tenant_id=t.tenant_id LEFT JOIN wai_template_contexts c ON c.template_version=v.id AND c.tenant_id=t.tenant_id LEFT JOIN wai_template_addons a ON a.template_id=t.id WHERE `+visibleTemplateSQL+` ORDER BY t.name`, p.Tenant, p.User, p.InstanceAdmin)
		if e != nil {
			fail(w, 503, "database_error")
			return
		}
		defer rows.Close()
		out := []Template{}
		for rows.Next() {
			var b, contextRaw []byte
			var t Template
			var owner *string
			var monthly int64
			var currency string
			var ends *time.Time
			if rows.Scan(&b, &owner, &contextRaw, &monthly, &currency, &ends) != nil || json.Unmarshal(b, &t) != nil {
				fail(w, 500, "invalid_template_data")
				return
			}
			if len(contextRaw) > 0 {
				var c TemplateContext
				if m.box.open(contextRaw, p.Tenant+":"+t.VersionID+":context", &c) != nil {
					fail(w, 503, "template_context_unavailable")
					return
				}
				t.AIContext = &c
			}
			t.AddonMonthlyMinor, t.AddonCurrency, t.AddonEndsAt = monthly, currency, ends
			if owner != nil {
				t.OwnerID = *owner
			}
			if suffix == "" || suffix == "/" || t.ID == strings.Trim(suffix, "/") {
				out = append(out, t)
			}
		}
		respond(w, 200, out)
		return
	}
	if r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		Template
		AddonConfirmation *templateAddonConfirmation `json:"addon_confirmation"`
	}
	if !decode(w, r, &in) {
		return
	}
	t := in.Template
	if len(t.Specialties) > 0 {
		seen := make(map[string]bool, len(t.Specialties))
		var specialties []string
		for _, specialty := range t.Specialties {
			specialty = strings.TrimSpace(specialty)
			if specialty != "" && !seen[specialty] {
				specialties = append(specialties, specialty)
				seen[specialty] = true
			}
		}
		t.Specialties = specialties
		if len(specialties) > 0 {
			t.Specialty = specialties[0]
		}
	}
	if len(t.Specialties) > 100 || strings.TrimSpace(t.Specialty) == "" {
		fail(w, 400, "invalid_specialty")
		return
	}
	t.OwnerID = ""
	t.AddonMonthlyMinor, t.AddonCurrency, t.AddonEndsAt = 0, "", nil
	if in.AddonConfirmation != nil {
		if _, err := uuid.Parse(in.AddonConfirmation.RequestID); err != nil {
			fail(w, 400, "invalid_template_addon_confirmation")
			return
		}
	}
	tx, e := m.db.Begin(ctx)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(ctx)
	var addon ServicePlan
	if e = templatePublication(ctx, tx, p, &t, in.AddonConfirmation, &addon); e != nil {
		billingFailure(w, e)
		return
	}
	newTemplate := t.ID == ""
	if e = saveTemplate(ctx, tx, p.Tenant, &t); e != nil {
		fail(w, 400, "invalid_template")
		return
	}
	if newTemplate {
		t.OwnerID = p.User
		if _, e = tx.Exec(ctx, `UPDATE wai_templates SET owner_id=$1 WHERE id=$2 AND tenant_id=$3`, p.User, t.ID, p.Tenant); e != nil {
			fail(w, 503, "database_error")
			return
		}
	}
	if addon.ExtraTemplateMinor > 0 {
		now := time.Now().UTC()
		tag, err := tx.Exec(ctx, `INSERT INTO wai_template_addons(template_id,user_id,request_id,plan_version,currency,monthly_minor,started_at)
			VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(user_id,request_id) DO NOTHING`,
			t.ID, p.User, in.AddonConfirmation.RequestID, addon.VersionID, addon.Currency, addon.ExtraTemplateMinor, now)
		if err != nil {
			fail(w, 503, "database_error")
			return
		}
		if tag.RowsAffected() == 0 {
			fail(w, 409, "template_addon_already_added")
			return
		}
		if err = syncTemplateAddons(ctx, tx, p.User, now); err != nil {
			fail(w, 503, "database_error")
			return
		}
		t.AddonMonthlyMinor, t.AddonCurrency = addon.ExtraTemplateMinor, addon.Currency
	}
	if t.AIContext != nil {
		raw, e := m.box.seal(t.AIContext, p.Tenant+":"+t.VersionID+":context")
		if e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO wai_template_contexts(template_version,tenant_id,created_by,payload)VALUES($1,$2,$3,$4)`, t.VersionID, p.Tenant, p.User, raw)
		}
		if e != nil {
			fail(w, 503, "template_context_save_failed")
			return
		}
	}
	if t.GenerationID != "" {
		tag, e := tx.Exec(ctx, `UPDATE wai_usage_records SET template_version=coalesce(template_version,$1::uuid) WHERE id::text=$2 AND tenant_id=$3 AND user_id=$4 AND operation='template_field_proposals'`, t.VersionID, t.GenerationID, p.Tenant, p.User)
		if e != nil || tag.RowsAffected() != 1 {
			fail(w, 400, "invalid_owned_generation")
			return
		}
	}
	if e = audit(ctx, tx, p, "template.publish", t.VersionID); e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 201, t)
}
func num(v float64) *float64 { return &v }
func rawBaseTemplates() []Template {
	return []Template{
		{Name: "Ecografie abdominală", Specialty: "Imagistică", HTML: `<h2>Ecografie abdominală</h2><p>Ficat: {{ficat}}; dimensiune {{ficat_mm}} mm.</p><p>Splină: {{splina}}.</p><p>Rinichi drept: {{rinichi_drept}}. Rinichi stâng: {{rinichi_stang}}.</p><p>Formațiuni: {{formatiuni}}.</p>`, Fields: []Field{{Key: "ficat", Label: "Aspect ficat", Type: "string", Required: true, Aliases: []string{"ficat"}}, {Key: "ficat_mm", Label: "Dimensiune ficat", Type: "number", Unit: "mm", Min: num(20), Max: num(400), Aliases: []string{"ficat_mm"}}, {Key: "splina", Label: "Splină", Type: "string", Required: true, Aliases: []string{"splina", "splină"}}, {Key: "rinichi_drept", Label: "Rinichi drept", Type: "string", Aliases: []string{"rinichi drept"}}, {Key: "rinichi_stang", Label: "Rinichi stâng", Type: "string", Aliases: []string{"rinichi stâng"}}, {Key: "formatiuni", Label: "Formațiuni", Type: "string", Repeatable: true, Aliases: []string{"formațiune", "formatiuni"}}}, Fragments: map[string]string{"normal": "<p>Aspect normal — folosiți numai după examinare și confirmare.</p>", "patologic": "<p>Descrieți localizarea, aspectul și măsurătorile observate.</p>"}},
		{Name: "Consultație cardiologie", Specialty: "Cardiologie", HTML: `<h2>Consultație cardiologie</h2><p>Anamneză: {{anamneza}}</p><p>TA {{ta}} mmHg; puls {{puls}} bpm</p><p>EKG: {{ekg}}</p><p>Ecocardiografie: {{ecocardiografie}}</p>`, Fields: []Field{{Key: "anamneza", Label: "Anamneză", Type: "string", Required: true}, {Key: "ta", Label: "Tensiune arterială", Type: "string", Unit: "mmHg"}, {Key: "puls", Label: "Puls", Type: "number", Unit: "bpm", Min: num(20), Max: num(250)}, {Key: "ekg", Label: "EKG efectuat", Type: "string"}, {Key: "ecocardiografie", Label: "Ecocardiografie efectuată", Type: "string"}}},
		{Name: "Vizită zilnică", Specialty: "Medicină internă", HTML: `<h2>Vizită zilnică</h2><p>Evoluție: {{evolutie}}</p><p>Examen: {{examen}}</p><p>Plan propus pentru revizuire: {{plan}}</p>`, Fields: []Field{{Key: "evolutie", Label: "Evoluție", Type: "string", Required: true}, {Key: "examen", Label: "Examen obiectiv", Type: "string"}, {Key: "plan", Label: "Plan menționat", Type: "string"}}},
	}
}
func baseTemplates() []Template {
	out := rawBaseTemplates()
	for i := range out {
		out[i] = adaptCatalogTemplate(out[i])
	}
	return out
}
func (m *Module) seed(ctx context.Context, tx pgx.Tx, tenant string) error {
	examples := baseTemplates()
	for i := range examples {
		t := &examples[i]
		if e := saveTemplate(ctx, tx, tenant, t); e != nil {
			return e
		}
		sid := uuid.NewString()
		if _, e := tx.Exec(ctx, `INSERT INTO wai_specialties(id,tenant_id,name)VALUES($1,$2,$3)`, sid, tenant, t.Specialty); e != nil {
			return e
		}
		b, _ := json.Marshal(map[string]any{"template_ids": []string{t.ID}, "synthetic": true, "context": []string{"ambulatoriu", "internare"}})
		if _, e := tx.Exec(ctx, `INSERT INTO wai_specialty_packs(id,tenant_id,specialty_id,name,payload)VALUES($1,$2,$3,$4,$5)`, uuid.NewString(), tenant, sid, t.Name, b); e != nil {
			return e
		}
	}
	return nil
}

func renderDocument(src string, t Template, data map[string]any) string {
	root, e := xhtml.Parse(strings.NewReader(src))
	if e != nil {
		return ""
	}
	sectionVisible := func(id string) bool { return sectionActive(t, id, data) }
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			remove := false
			for _, a := range c.Attr {
				if a.Key == "data-section" && !sectionVisible(a.Val) {
					remove = true
				}
				if a.Key == "data-field" {
					c.Attr = append(c.Attr, xhtml.Attribute{Key: "data-review-field", Val: a.Val})
					value := "—"
					for _, f := range t.Fields {
						if f.Key == a.Val && f.Invisible {
							value = ""
						}
					}
					if x, ok := data[a.Val]; ok && value != "" {
						value = displayValue(x)
					}
					for c.FirstChild != nil {
						c.RemoveChild(c.FirstChild)
					}
					c.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: value})
				}
			}
			if remove {
				n.RemoveChild(c)
			} else {
				walk(c)
			}
			c = next
		}
	}
	walk(root)
	var b strings.Builder
	_ = xhtml.Render(&b, root)
	return sanitizeHTML(b.String())
}

// Catalog returns the synthetic starter library without patient data or database identifiers.
func Catalog() []Template {
	out := baseTemplates()
	for _, s := range specialtyCatalog {
		out = append(out, catalogTemplate(s))
	}
	return out
}

func sectionActive(t Template, id string, data map[string]any) bool {
	if id == "" {
		return true
	}
	for _, s := range t.Sections {
		if s.ID == id {
			if s.HideWhenEmpty && !sectionHasData(t, s.ID, data) {
				return false
			}
			if s.ConditionField != "" {
				x, ok := data[s.ConditionField].(map[string]any)
				if !ok || displayValue(x) != s.ConditionValue {
					return false
				}
			}
			return sectionActive(t, s.Parent, data)
		}
	}
	return false
}
