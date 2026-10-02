// Package siui implements PIAS paraclinical validation through api.request/WSM.
package siui

import (
	"bytes"
	"embed"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

//go:embed assets/wsdl/*.wsdl assets/schemas/validation/*.xsd
var assets embed.FS

const soapNS = "http://schemas.xmlsoap.org/soap/envelope/"
const validateNS = "http://siuiValidate.webservices.utils.svapnt.siveco.ro"
const piasNS = "http://www.cnas.ro/siui/2.0"
const maxData = 1024 * 1024

type node struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Text     string     `xml:",chardata"`
	Children []node     `xml:",any"`
}

func (n node) attr(k string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == k {
			return a.Value
		}
	}
	return ""
}
func (n node) child(k string) node {
	for _, c := range n.Children {
		if c.XMLName.Local == k {
			return c
		}
	}
	return node{}
}
func parseXML(b []byte) (node, error) {
	if len(b) > maxData {
		return node{}, errors.New("XML too large")
	}
	d := xml.NewDecoder(bytes.NewReader(b))
	depth, roots := 0, 0
	for {
		t, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return node{}, errors.New("invalid XML")
		}
		switch v := t.(type) {
		case xml.Directive:
			return node{}, errors.New("XML directives are forbidden")
		case xml.StartElement:
			if depth == 0 {
				roots++
			}
			depth++
			if depth > 40 {
				return node{}, errors.New("XML nesting too deep")
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(v)) != "" {
				return node{}, errors.New("unexpected XML text")
			}
		}
	}
	if roots != 1 {
		return node{}, errors.New("one XML root required")
	}
	var n node
	e := xml.Unmarshal(b, &n)
	return n, e
}
func requestIDs(data string) ([]string, error) {
	n, e := parseXML([]byte(data))
	if e != nil {
		return nil, e
	}
	if n.XMLName.Local != "request" || n.XMLName.Space != piasNS || n.attr("reportType") != "PARA" {
		return nil, errors.New("expected a PARA request in the PIAS namespace")
	}
	seen := map[string]bool{}
	var ids []string
	for _, s := range n.Children {
		if s.XMLName.Local != "laboratoryService" {
			continue
		}
		id := s.attr("AppID")
		if id == "" || len(id) > 40 || seen[id] {
			return nil, errors.New("laboratoryService AppID must be present and unique")
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 || len(ids) > 1000 {
		return nil, errors.New("request must contain 1 to 1000 laboratoryService records")
	}
	return ids, nil
}
func envelope(report string) []byte {
	var b bytes.Buffer
	b.WriteString(`<soap:Envelope xmlns:soap="` + soapNS + `" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema"><soap:Body><m:validateReport xmlns:m="` + validateNS + `" soap:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><reportXml xsi:type="xsd:string">`)
	xml.EscapeText(&b, []byte(report))
	b.WriteString(`</reportXml><reportType xsi:type="xsd:string">PARA</reportType><requestType xsi:type="xsd:string">RQ_PARA_SRV</requestType></m:validateReport></soap:Body></soap:Envelope>`)
	return b.Bytes()
}
func decodeSOAP(b []byte) (string, error) {
	r, e := parseXML(b)
	if e != nil {
		return "", e
	}
	if r.XMLName.Local != "Envelope" || r.XMLName.Space != soapNS {
		return "", errors.New("invalid SOAP envelope")
	}
	body := r.child("Body")
	if body.XMLName.Space != soapNS {
		return "", errors.New("invalid SOAP body")
	}
	for _, n := range body.Children {
		if n.XMLName.Local == "Fault" && n.XMLName.Space == soapNS {
			return "", errors.New("CNAS SOAP fault")
		}
	}
	response := body.child("validateReportResponse")
	if response.XMLName.Space != validateNS {
		return "", errors.New("unexpected SOAP operation")
	}
	var values []node
	for _, n := range response.Children {
		if n.XMLName.Local == "validateReportReturn" {
			values = append(values, n)
		}
	}
	if len(values) != 1 {
		return "", errors.New("missing or duplicate SOAP result")
	}
	n := values[0]
	if href := n.attr("href"); href != "" {
		if !strings.HasPrefix(href, "#") {
			return "", errors.New("external SOAP reference rejected")
		}
		var refs []node
		for _, ref := range body.Children {
			if ref.attr("id") == strings.TrimPrefix(href, "#") {
				refs = append(refs, ref)
			}
		}
		if len(refs) != 1 || refs[0].attr("href") != "" {
			return "", errors.New("unresolved SOAP reference")
		}
		n = refs[0]
	}
	if n.Text == "" || len(n.Children) > 0 {
		return "", errors.New("empty or invalid SOAP result")
	}
	return n.Text, nil
}

type ValidationError struct {
	Code     string `json:"code"`
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message"`
}
type ServiceResult struct {
	AppID  string            `json:"app_id"`
	State  string            `json:"state"`
	Errors []ValidationError `json:"errors"`
}
type ValidationResult struct {
	State          string            `json:"state"`
	Validated      bool              `json:"validated"`
	ValidationDate string            `json:"validation_date"`
	Services       []ServiceResult   `json:"services"`
	Errors         []ValidationError `json:"errors"`
	XML            string            `json:"response_xml"`
}

func validationErrors(n node) []ValidationError {
	out := []ValidationError{}
	for _, e := range n.child("errors").Children {
		out = append(out, ValidationError{e.attr("code"), e.attr("errorType"), e.Text})
	}
	return out
}
func parseResult(raw string, ids []string) (ValidationResult, error) {
	n, e := parseXML([]byte(raw))
	if e != nil {
		return ValidationResult{}, e
	}
	if n.XMLName.Space != piasNS || n.XMLName.Local != "response" {
		return ValidationResult{}, errors.New("invalid PIAS response root")
	}
	out := ValidationResult{State: n.attr("state"), ValidationDate: n.attr("validationDate"), XML: raw, Services: []ServiceResult{}, Errors: validationErrors(n)}
	expected := map[string]bool{}
	for _, id := range ids {
		expected[id] = false
	}
	all := true
	for _, s := range n.Children {
		if s.XMLName.Local != "laboratoryService" {
			continue
		}
		id := s.attr("AppID")
		seen, ok := expected[id]
		if !ok || seen {
			return out, errors.New("unexpected or duplicate AppID in CNAS response")
		}
		expected[id] = true
		state := s.attr("state")
		if state != "1" {
			all = false
		}
		out.Services = append(out.Services, ServiceResult{id, state, validationErrors(s)})
	}
	for _, seen := range expected {
		if !seen {
			all = false
		}
	}
	out.Validated = out.State == "1" && all
	return out, nil
}
