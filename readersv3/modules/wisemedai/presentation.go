package wisemedai

import (
	"errors"
	"html"
	"strings"

	xhtml "golang.org/x/net/html"
)

type SectionVariant struct {
	ConditionOperator string   `json:"condition_operator,omitempty"`
	ID                string   `json:"id"`
	Label             string   `json:"label"`
	Detail            string   `json:"detail"`
	ConditionField    string   `json:"condition_field"`
	ConditionValue    string   `json:"condition_value"`
	Text              string   `json:"text"`
	RequiredFields    []string `json:"required_fields,omitempty"`
}
type PresentationOptions struct {
	Detail   string            `json:"detail"`
	Sections map[string]string `json:"sections"`
}
type SectionPresentation struct {
	SectionID    string `json:"section_id"`
	Label        string `json:"label"`
	Detail       string `json:"detail"`
	VariantID    string `json:"variant_id,omitempty"`
	VariantLabel string `json:"variant_label,omitempty"`
	Reason       string `json:"reason"`
}

func validDetail(s string) bool { return s == "brief" || s == "standard" || s == "detailed" }
func validatePresentation(t Template, p PresentationOptions) error {
	if p.Detail != "" && !validDetail(p.Detail) {
		return errors.New("invalid_detail")
	}
	if len(p.Sections) > len(t.Sections) {
		return errors.New("invalid_section_presentation")
	}
	for id, detail := range p.Sections {
		known := false
		for _, s := range t.Sections {
			known = known || s.ID == id
		}
		if !known || !validDetail(detail) {
			return errors.New("invalid_section_presentation")
		}
	}
	return nil
}
func validateVariants(t Template) error {
	if t.DefaultDetail != "" && !validDetail(t.DefaultDetail) {
		return errors.New("invalid_default_detail")
	}
	placed := map[string]bool{}
	root, _ := xhtml.Parse(strings.NewReader(t.HTML))
	var visit func(*xhtml.Node)
	visit = func(n *xhtml.Node) {
		for _, a := range n.Attr {
			if n.Type == xhtml.ElementNode && n.Data == "section" && a.Key == "data-section" {
				placed[a.Val] = true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	if root != nil {
		visit(root)
	}
	fields := map[string]bool{}
	hidden := map[string]bool{}
	for _, f := range t.Fields {
		fields[f.Key] = true
		hidden[f.Key] = f.Invisible
	}
	if len(t.Sections) > 100 {
		return errors.New("too_many_sections")
	}
	for _, s := range t.Sections {
		if len(s.Variants) > 0 && !placed[s.ID] {
			return errors.New("variant_section_missing_in_document")
		}
		if s.Detail != "" && !validDetail(s.Detail) {
			return errors.New("invalid_section_detail")
		}
		if len(s.Variants) > 30 {
			return errors.New("too_many_section_variants")
		}
		ids := map[string]bool{}
		for _, v := range s.Variants {
			if !keyPattern.MatchString(v.ID) || ids[v.ID] || len(v.Label) < 1 || len(v.Label) > 120 || !validDetail(v.Detail) || (v.ConditionOperator != "" && v.ConditionOperator != "equals" && v.ConditionOperator != "documented") || !fields[v.ConditionField] || (v.ConditionOperator != "documented" && strings.TrimSpace(v.ConditionValue) == "") || len(v.ConditionValue) > 500 || len(v.Text) == 0 || len(v.Text) > 8000 || len(v.RequiredFields) > 150 {
				return errors.New("invalid_section_variant")
			}
			ids[v.ID] = true
			for _, key := range v.RequiredFields {
				if !fields[key] {
					return errors.New("unknown_variant_field")
				}
			}
			for _, match := range placeholder.FindAllStringSubmatch(v.Text, -1) {
				if !fields[match[1]] || hidden[match[1]] {
					return errors.New("unknown_variant_placeholder")
				}
			}
		}
	}
	return nil
}
func presentField(data map[string]any, key string) bool {
	if values, ok := data[key].([]any); ok {
		if len(values) == 0 {
			return false
		}
		for _, value := range values {
			if !presentField(map[string]any{key: value}, key) {
				return false
			}
		}
		return true
	}
	x, ok := data[key].(map[string]any)
	return ok && x["state"] == "present" && strings.TrimSpace(displayValue(x)) != ""
}
func presentationText(text string, data map[string]any) string {
	text = html.EscapeString(text)
	for _, match := range placeholder.FindAllStringSubmatch(text, -1) {
		text = strings.ReplaceAll(text, match[0], fieldMarkup(match[1], data))
	}
	return "<p>" + strings.ReplaceAll(text, "\n", "</p><p>") + "</p>"
}

// Select authored wording from supported field values; never infer pathology or normality.
func composeSections(t Template, data map[string]any) (string, []SectionPresentation, []string) {
	choices := []SectionPresentation{}
	warnings := []string{}
	selected := map[string]string{}
	for _, s := range t.Sections {
		if !sectionActive(t, s.ID, data) {
			continue
		}
		detail := sectionDetail(t, s.ID)
		choice := SectionPresentation{SectionID: s.ID, Label: s.Label, Detail: detail, Reason: "base_layout"}
		candidates := []SectionVariant{}
		for _, v := range s.Variants {
			if v.Detail != detail || !presentField(data, v.ConditionField) || (v.ConditionOperator != "documented" && !strings.EqualFold(strings.TrimSpace(displayValue(data[v.ConditionField])), strings.TrimSpace(v.ConditionValue))) {
				continue
			}
			supported := true
			for _, key := range v.RequiredFields {
				supported = supported && presentField(data, key)
			}
			for _, match := range placeholder.FindAllStringSubmatch(v.Text, -1) {
				supported = supported && presentField(data, match[1])
			}
			if supported && detail != "brief" {
				visible := map[string]bool{v.ConditionField: true}
				for _, match := range placeholder.FindAllStringSubmatch(v.Text, -1) {
					visible[match[1]] = true
				}
				for _, f := range t.Fields {
					if f.Section == s.ID && !f.Invisible && data[f.Key] != nil && !visible[f.Key] {
						supported = false
					}
				}
			}
			if supported {
				candidates = append(candidates, v)
			}
		}
		specific := []SectionVariant{}
		for _, v := range candidates {
			if v.ConditionOperator != "documented" {
				specific = append(specific, v)
			}
		}
		if len(specific) > 0 {
			candidates = specific
		}
		if len(candidates) == 1 {
			v := candidates[0]
			choice.VariantID = v.ID
			choice.VariantLabel = v.Label
			choice.Reason = "supported_variant"
			selected[s.ID] = presentationText(v.Text, data)
		}
		if len(candidates) > 1 {
			choice.Reason = "ambiguous_variants"
			warnings = append(warnings, "Mai multe variante de redactare se potrivesc pentru "+s.Label+"; este păstrat documentul de bază.")
		}
		choices = append(choices, choice)
	}
	if len(selected) == 0 {
		return t.HTML, choices, warnings
	}
	root, e := xhtml.Parse(strings.NewReader(t.HTML))
	if e != nil {
		return t.HTML, choices, warnings
	}
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		id := ""
		for _, a := range n.Attr {
			if a.Key == "data-section" {
				id = a.Val
			}
		}
		if text, ok := selected[id]; ok {
			// Preserve nested section subtrees and headings when changing this section's own body.
			var containsSection func(*xhtml.Node) bool
			containsSection = func(node *xhtml.Node) bool {
				for _, a := range node.Attr {
					if a.Key == "data-section" {
						return true
					}
				}
				for c := node.FirstChild; c != nil; c = c.NextSibling {
					if containsSection(c) {
						return true
					}
				}
				return false
			}
			for c := n.FirstChild; c != nil; {
				next := c.NextSibling
				if !containsSection(c) && !(c.Type == xhtml.ElementNode && (c.Data == "h1" || c.Data == "h2" || c.Data == "h3")) {
					n.RemoveChild(c)
				}
				c = next
			}
			fragment, _ := xhtml.Parse(strings.NewReader(text))
			var body *xhtml.Node
			var find func(*xhtml.Node)
			find = func(node *xhtml.Node) {
				if node.Type == xhtml.ElementNode && node.Data == "body" {
					body = node
					return
				}
				for c := node.FirstChild; c != nil; c = c.NextSibling {
					find(c)
				}
			}
			find(fragment)
			if body != nil {
				var before *xhtml.Node
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if containsSection(c) {
						before = c
						break
					}
				}
				for c := body.FirstChild; c != nil; {
					next := c.NextSibling
					body.RemoveChild(c)
					n.InsertBefore(c, before)
					c = next
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	var out strings.Builder
	xhtml.Render(&out, root)
	return sanitizeHTML(out.String()), choices, warnings
}

func sectionDetail(t Template, id string) string {
	detail := "standard"
	if t.DefaultDetail != "" {
		detail = t.DefaultDetail
	}
	if t.Presentation.Detail != "" {
		detail = t.Presentation.Detail
	}
	chain := []Section{}
	seen := map[string]bool{}
	for id != "" && !seen[id] {
		seen[id] = true
		found := false
		for _, s := range t.Sections {
			if s.ID == id {
				chain = append(chain, s)
				id = s.Parent
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	for i := len(chain) - 1; i >= 0; i-- {
		s := chain[i]
		if t.Presentation.Detail == "" && s.Detail != "" {
			detail = s.Detail
		}
		if value := t.Presentation.Sections[s.ID]; value != "" {
			detail = value
		}
	}
	return detail
}

func sectionHasData(t Template, id string, data map[string]any) bool {
	for _, f := range t.Fields {
		if _, ok := data[f.Key]; !ok {
			continue
		}
		at := f.Section
		seen := map[string]bool{}
		for at != "" && !seen[at] {
			if at == id {
				return true
			}
			seen[at] = true
			next := ""
			for _, s := range t.Sections {
				if s.ID == at {
					next = s.Parent
					break
				}
			}
			at = next
		}
	}
	return false
}
