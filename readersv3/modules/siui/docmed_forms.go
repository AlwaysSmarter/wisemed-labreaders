package siui

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type docMedFormOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type docMedFormField struct {
	Name         string             `json:"name"`
	Label        string             `json:"label"`
	Description  string             `json:"description,omitempty"`
	Type         string             `json:"type"`
	Required     bool               `json:"required"`
	Options      []docMedFormOption `json:"options,omitempty"`
	Length       string             `json:"length,omitempty"`
	MinLength    string             `json:"minLength,omitempty"`
	MaxLength    string             `json:"maxLength,omitempty"`
	MinInclusive string             `json:"minInclusive,omitempty"`
	MaxInclusive string             `json:"maxInclusive,omitempty"`
	Pattern      string             `json:"pattern,omitempty"`
	List         bool               `json:"list,omitempty"`
}
type docMedFormNode struct {
	Name        string            `json:"name"`
	Label       string            `json:"label"`
	Description string            `json:"description,omitempty"`
	Min         int               `json:"min"`
	Max         int               `json:"max"`
	Attributes  []docMedFormField `json:"attributes,omitempty"`
	Children    []docMedFormNode  `json:"children,omitempty"`
	Value       *docMedFormField  `json:"value,omitempty"`
}

var docMedFormsOnce sync.Once
var docMedForms map[string]*docMedFormNode

func docMedForm(name string) *docMedFormNode {
	docMedFormsOnce.Do(func() { data, _ := assets.ReadFile("assets/docmed_forms.json"); _ = json.Unmarshal(data, &docMedForms) })
	return docMedForms[name]
}

func docMedFieldValue(field docMedFormField, raw json.RawMessage, path string) (string, error) {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		if field.Type != "base64Binary" || field.Name != "attachedFile" {
			return "", fmt.Errorf("%s: completează o valoare text", path)
		}
		var upload struct {
			Filename string `json:"filename"`
			Data     string `json:"data_base64"`
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&upload) != nil {
			return "", fmt.Errorf("%s: fișier invalid", path)
		}
		data, err := base64.StdEncoding.DecodeString(upload.Data)
		if err != nil || len(data) == 0 || len(data) > docMedXMLLimit {
			return "", fmt.Errorf("%s: fișier gol, prea mare sau Base64 invalid", path)
		}
		switch {
		case strings.HasSuffix(strings.ToLower(upload.Filename), ".pdf"):
			if !bytes.HasPrefix(data, []byte("%PDF-")) {
				return "", fmt.Errorf("%s: fișierul nu este PDF", path)
			}
			var archive bytes.Buffer
			w := zip.NewWriter(&archive)
			entry, err := w.Create("document.pdf")
			if err != nil {
				return "", err
			}
			if _, err = entry.Write(data); err != nil {
				return "", err
			}
			if err = w.Close(); err != nil {
				return "", err
			}
			data = archive.Bytes()
		case strings.HasSuffix(strings.ToLower(upload.Filename), ".zip"):
			if _, err = zip.NewReader(bytes.NewReader(data), int64(len(data))); err != nil {
				return "", fmt.Errorf("%s: arhivă ZIP invalidă", path)
			}
		default:
			return "", fmt.Errorf("%s: selectează un PDF sau ZIP", path)
		}
		value = base64.StdEncoding.EncodeToString(data)
	}
	if len(value) > docMedXMLLimit {
		return "", fmt.Errorf("%s: valoare prea mare", path)
	}
	if field.Required && strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s: câmp obligatoriu", path)
	}
	if value == "" && !field.Required {
		return "", nil
	}
	if len(field.Options) > 0 {
		found := false
		for _, option := range field.Options {
			if option.Value == value {
				found = true
			}
		}
		if !found {
			return "", fmt.Errorf("%s: selectează o valoare din listă", path)
		}
	}
	length := utf8.RuneCountInString(value)
	for _, bound := range []struct {
		raw   string
		valid func(int) bool
	}{{field.Length, func(n int) bool { return length == n }}, {field.MinLength, func(n int) bool { return length >= n }}, {field.MaxLength, func(n int) bool { return length <= n }}} {
		if bound.raw != "" {
			n, _ := strconv.Atoi(bound.raw)
			if !bound.valid(n) {
				return "", fmt.Errorf("%s: lungime neconformă (limită %s)", path, bound.raw)
			}
		}
	}
	switch field.Type {
	case "boolean":
		if value != "true" && value != "false" && value != "0" && value != "1" {
			return "", fmt.Errorf("%s: alege Da sau Nu", path)
		}
	case "date":
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return "", fmt.Errorf("%s: dată invalidă", path)
		}
	case "dateTime":
		if len(value) == 16 {
			value += ":00"
		}
		valid := false
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05"} {
			if _, err := time.Parse(layout, value); err == nil {
				valid = true
			}
		}
		if !valid {
			return "", fmt.Errorf("%s: dată și oră invalide", path)
		}
	case "integer", "int", "long", "short", "nonNegativeInteger", "positiveInteger", "unsignedInt":
		if !regexp.MustCompile(`^[+-]?[0-9]+$`).MatchString(value) {
			return "", fmt.Errorf("%s: introdu un număr întreg", path)
		}
	case "decimal":
		if !regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)$`).MatchString(value) {
			return "", fmt.Errorf("%s: introdu un număr zecimal cu punct", path)
		}
	case "base64Binary":
		if _, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(value), "")); err != nil {
			return "", fmt.Errorf("%s: Base64 invalid", path)
		}
	}
	if field.MinInclusive != "" || field.MaxInclusive != "" {
		number, ok := new(big.Rat).SetString(value)
		if !ok {
			return "", fmt.Errorf("%s: număr invalid", path)
		}
		if field.MinInclusive != "" {
			bound, _ := new(big.Rat).SetString(field.MinInclusive)
			if bound != nil && number.Cmp(bound) < 0 {
				return "", fmt.Errorf("%s: valoarea minimă este %s", path, field.MinInclusive)
			}
		}
		if field.MaxInclusive != "" {
			bound, _ := new(big.Rat).SetString(field.MaxInclusive)
			if bound != nil && number.Cmp(bound) > 0 {
				return "", fmt.Errorf("%s: valoarea maximă este %s", path, field.MaxInclusive)
			}
		}
	}
	if field.Pattern != "" {
		pattern, err := regexp.Compile("^(?:" + field.Pattern + ")$")
		if err != nil || !pattern.MatchString(value) {
			return "", fmt.Errorf("%s: format invalid", path)
		}
	}
	return value, nil
}
func buildDocMedRequest(op Operation, raw json.RawMessage) (string, error) {
	model := docMedForm(op.Name)
	if model == nil || op.Group != "docmed" {
		return "", errors.New("formular DocMED indisponibil")
	}
	var out bytes.Buffer
	encoder := xml.NewEncoder(&out)
	nodes := 0
	var emit func(docMedFormNode, json.RawMessage, string, int) error
	emit = func(model docMedFormNode, raw json.RawMessage, path string, depth int) error {
		nodes++
		if depth > 35 || nodes > 50000 {
			return errors.New("formular DocMED prea mare")
		}
		start := xml.StartElement{Name: xml.Name{Local: model.Name}}
		if depth == 0 {
			start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "xmlns"}, Value: docMedNS})
		}
		if model.Value != nil {
			value, err := docMedFieldValue(*model.Value, raw, path)
			if err != nil {
				return err
			}
			if err = encoder.EncodeToken(start); err != nil {
				return err
			}
			if err = encoder.EncodeToken(xml.CharData(value)); err != nil {
				return err
			}
			return encoder.EncodeToken(start.End())
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			return fmt.Errorf("%s: secțiune invalidă", path)
		}
		allowed := map[string]bool{}
		for _, a := range model.Attributes {
			allowed[a.Name] = true
		}
		for _, child := range model.Children {
			allowed[child.Name] = true
		}
		for key := range fields {
			if !allowed[key] {
				return fmt.Errorf("%s.%s: câmp necunoscut", path, key)
			}
		}
		for _, attr := range model.Attributes {
			value, exists := fields[attr.Name]
			if !exists {
				if attr.Required {
					return fmt.Errorf("%s / %s: câmp obligatoriu", path, attr.Label)
				}
				continue
			}
			text, err := docMedFieldValue(attr, value, path+" / "+attr.Label)
			if err != nil {
				return err
			}
			if text != "" {
				start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: attr.Name}, Value: text})
			}
		}
		if err := encoder.EncodeToken(start); err != nil {
			return err
		}
		for _, child := range model.Children {
			value, exists := fields[child.Name]
			if !exists {
				if child.Min > 0 {
					return fmt.Errorf("%s / %s: secțiune obligatorie", path, child.Label)
				}
				continue
			}
			repeated := child.Max != 1
			if repeated {
				var list []json.RawMessage
				if json.Unmarshal(value, &list) != nil {
					return fmt.Errorf("%s / %s: lista este invalidă", path, child.Label)
				}
				if len(list) < child.Min || len(list) > 5000 || (child.Max >= 0 && len(list) > child.Max) {
					return fmt.Errorf("%s / %s: număr de înregistrări neconform", path, child.Label)
				}
				for i, item := range list {
					if err := emit(child, item, fmt.Sprintf("%s / %s [%d]", path, child.Label, i+1), depth+1); err != nil {
						return err
					}
				}
			} else if err := emit(child, value, path+" / "+child.Label, depth+1); err != nil {
				return err
			}
		}
		return encoder.EncodeToken(start.End())
	}
	if err := emit(*model, raw, op.Name, 0); err != nil {
		return "", err
	}
	if err := encoder.Flush(); err != nil {
		return "", err
	}
	if out.Len() > docMedXMLLimit {
		return "", errors.New("cererea DocMED depășește 32 MiB")
	}
	return out.String(), nil
}

func docMedFormJSONSchema(model docMedFormNode) map[string]any {
	var scalar func(docMedFormField) map[string]any
	scalar = func(field docMedFormField) map[string]any {
		schema := map[string]any{"type": "string", "title": field.Label, "description": field.Description}
		if len(field.Options) > 0 {
			values := []string{}
			for _, option := range field.Options {
				values = append(values, option.Value)
			}
			schema["enum"] = values
		}
		if field.Pattern != "" {
			schema["pattern"] = "^(?:" + field.Pattern + ")$"
		}
		for _, bound := range []struct{ key, value string }{{"minLength", field.MinLength}, {"maxLength", field.MaxLength}} {
			if bound.value != "" {
				n, _ := strconv.Atoi(bound.value)
				schema[bound.key] = n
			}
		}
		if field.Length != "" {
			n, _ := strconv.Atoi(field.Length)
			schema["minLength"] = n
			schema["maxLength"] = n
		}
		if field.Type == "date" {
			schema["format"] = "date"
		}
		if field.Type == "boolean" {
			schema["enum"] = []string{"true", "false", "0", "1"}
		}
		if field.Name == "attachedFile" {
			return map[string]any{"oneOf": []any{schema, map[string]any{"type": "object", "additionalProperties": false, "required": []string{"filename", "data_base64"}, "properties": map[string]any{"filename": map[string]any{"type": "string"}, "data_base64": map[string]any{"type": "string", "format": "byte"}}}}}
		}
		return schema
	}
	if model.Value != nil {
		return scalar(*model.Value)
	}
	props := map[string]any{}
	required := []string{}
	for _, field := range model.Attributes {
		props[field.Name] = scalar(field)
		if field.Required {
			required = append(required, field.Name)
		}
	}
	for _, child := range model.Children {
		sub := docMedFormJSONSchema(child)
		if child.Max != 1 {
			max := child.Max
			if max < 0 || max > 5000 {
				max = 5000
			}
			sub = map[string]any{"type": "array", "minItems": child.Min, "maxItems": max, "items": sub}
		}
		props[child.Name] = sub
		if child.Min > 0 {
			required = append(required, child.Name)
		}
	}
	out := map[string]any{"type": "object", "additionalProperties": false, "properties": props, "title": model.Label}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}
