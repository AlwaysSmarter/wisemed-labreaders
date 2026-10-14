package siui

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Parameter struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
type Operation struct {
	Label      string      `json:"label"`
	Name       string      `json:"name"`
	Group      string      `json:"group"`
	Service    string      `json:"service"`
	Namespace  string      `json:"namespace"`
	Style      string      `json:"style"`
	Parameters []Parameter `json:"parameters"`
	Mutating   bool        `json:"mutating"`
	Download   bool        `json:"download"`
}

func operations() []Operation {
	b, _ := assets.ReadFile("assets/operations.json")
	var out []Operation
	_ = json.Unmarshal(b, &out)
	return out
}
func findOperation(group, name string) (Operation, bool) {
	for _, op := range operations() {
		if op.Group == group && op.Name == name {
			return op, true
		}
	}
	return Operation{}, false
}
func validateParameters(op Operation, args map[string]string) error {
	if len(args) != len(op.Parameters) {
		return errors.New("supply exactly the parameters listed in the operation catalogue")
	}
	for _, p := range op.Parameters {
		v, ok := args[p.Name]
		if !ok {
			return fmt.Errorf("missing parameter %s", p.Name)
		}
		if len(v) > maxData {
			return fmt.Errorf("parameter %s exceeds 1 MiB", p.Name)
		}
		switch p.Type {
		case "dateTime":
			valid := false
			for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05"} {
				if _, err := time.Parse(layout, v); err == nil {
					valid = true
				}
			}
			if !valid {
				return fmt.Errorf("%s must be an XML dateTime, e.g. 2026-10-05T00:00:00+03:00", p.Name)
			}
		case "int":
			if _, err := strconv.ParseInt(v, 10, 32); err != nil {
				return fmt.Errorf("%s must be a signed 32-bit integer string", p.Name)
			}
		}
		if strings.HasSuffix(strings.ToLower(p.Name), "xml") {
			if _, err := parseXML([]byte(v)); err != nil {
				return fmt.Errorf("%s: %w", p.Name, err)
			}
		}
	}
	if op.Name == "validateReport" {
		if args["reportType"] != "CLIN" && args["reportType"] != "PARA" {
			return errors.New("validateReport supports CLIN and PARA")
		}
		if args["requestType"] != "RQ_"+args["reportType"]+"_SRV" {
			return errors.New("requestType must match RQ_CLIN_SRV or RQ_PARA_SRV")
		}
		root, _ := parseXML([]byte(args["reportXml"]))
		if root.attr("reportType") != args["reportType"] {
			return errors.New("XML reportType must match reportType")
		}
	}
	return nil
}
func operationEnvelope(op Operation, args map[string]string) []byte {
	var b bytes.Buffer
	b.WriteString(`<soap:Envelope xmlns:soap="` + soapNS + `" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema"><soap:Body>`)
	if op.Style == "rpc" {
		b.WriteString(`<m:` + op.Name + ` xmlns:m="` + op.Namespace + `" soap:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">`)
	} else {
		b.WriteString(`<` + op.Name + ` xmlns="` + op.Namespace + `">`)
	}
	for _, p := range op.Parameters {
		b.WriteString("<" + p.Name)
		if op.Style == "rpc" {
			b.WriteString(` xsi:type="xsd:` + p.Type + `"`)
		}
		b.WriteString(">")
		_ = xml.EscapeText(&b, []byte(args[p.Name]))
		b.WriteString("</" + p.Name + ">")
	}
	if op.Style == "rpc" {
		b.WriteString("</m:" + op.Name + ">")
	} else {
		b.WriteString("</" + op.Name + ">")
	}
	b.WriteString("</soap:Body></soap:Envelope>")
	return b.Bytes()
}

// Resolve only local Axis references. Return exact string values, including XML,
// file locations, empty strings and numeric cancellation/report receipts.
func operationValues(op Operation, data []byte) ([]string, error) {
	root, e := parseXML(data)
	if e != nil {
		return nil, e
	}
	if root.XMLName.Local != "Envelope" || root.XMLName.Space != soapNS {
		return nil, errors.New("invalid SOAP envelope")
	}
	body := root.child("Body")
	if body.XMLName.Space != soapNS {
		return nil, errors.New("invalid SOAP body")
	}
	if f := body.child("Fault"); f.XMLName.Local != "" {
		code := f.child("faultcode").Text
		message := f.child("faultstring").Text
		return nil, fmt.Errorf("CNAS SOAP fault %s: %s", code, message)
	}
	resp := body.child(op.Name + "Response")
	if resp.XMLName.Space != op.Namespace {
		return nil, errors.New("unexpected SOAP response operation")
	}
	var walk func(node, int) ([]string, error)
	walk = func(n node, depth int) ([]string, error) {
		if depth > 8 {
			return nil, errors.New("cyclic or deep SOAP reference")
		}
		if ref := n.attr("href"); ref != "" {
			if !strings.HasPrefix(ref, "#") {
				return nil, errors.New("external SOAP reference rejected")
			}
			var found []node
			for _, v := range body.Children {
				if v.attr("id") == ref[1:] {
					found = append(found, v)
				}
			}
			if len(found) != 1 {
				return nil, errors.New("ambiguous SOAP reference")
			}
			return walk(found[0], depth+1)
		}
		if len(n.Children) == 0 {
			return []string{n.Text}, nil
		}
		var out []string
		for _, v := range n.Children {
			values, err := walk(v, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, values...)
		}
		return out, nil
	}
	var values []string
	for _, n := range resp.Children {
		if n.XMLName.Local != op.Name+"Return" {
			return nil, errors.New("unexpected SOAP result element")
		}
		v, e := walk(n, 0)
		if e != nil {
			return nil, e
		}
		values = append(values, v...)
	}
	if len(values) == 0 {
		return nil, errors.New("missing SOAP result")
	}
	return values, nil
}
func operationSchema(op Operation, args map[string]string) string {
	switch op.Name {
	case "validateReport":
		if args["reportType"] == "CLIN" {
			return "ClinicSpecialityServicesValidateRequest.xsd"
		}
		return "ParaclinicServicesValidateRequest.xsd"
	case "validatePrescription":
		return "PhysicianPrescriptionsValidateRequest.xsd"
	case "validateClinicReferral", "validateLabReferral":
		return "PhysicianReferralsValidateRequest.xsd"
	case "processPrescribedPrescription":
		return "PhysicianDrugPERequest.xsd"
	case "processHospitalPrescription":
		return "HospitalRegisterPERequest.xsd"
	case "getPrescribedPrescriptionsForCid":
		return "getPrescribedPrescriptionsForCidRequest.xsd"
	case "validateSickLeave":
		return "PhysicianCertificatesValidateRequest.xsd"
	}
	return ""
}
func (c *client) invokeOperation(ctx context.Context, op Operation, args map[string]string) (values []string, submitted bool, err error) {
	address := ""
	defer func() { err = withConnectionError(address, err) }()
	if err = validateParameters(op, args); err != nil {
		return
	}
	if schema := operationSchema(op, args); schema != "" {
		if err = c.backend.Validate(ctx, schema, []byte(xmlParameter(args))); err != nil {
			return
		}
	}
	h, e := c.sessionHeaders(ctx)
	if e != nil {
		return nil, false, e
	}
	defer func() {
		if err != nil {
			message := err.Error()
			for _, secret := range []string{c.cfg.Licence, h.Get("OSCP_RESPONSE")} {
				if secret != "" {
					message = strings.ReplaceAll(message, secret, "[redacted]")
				}
			}
			err = errors.New(message)
		}
	}()
	base := c.cfg.serviceBaseURL(op.Service)
	if e = ctx.Err(); e != nil {
		return nil, false, e
	}
	address = strings.TrimRight(base, "/") + "/svapntws/services/" + op.Service
	status, _, data, e := c.backend.Do(ctx, "POST", address, h, operationEnvelope(op, args))
	submitted = op.Mutating
	if e != nil {
		if connectionFailedBeforeSend(e) {
			submitted = false
		}
		return nil, submitted, e
	}
	if status != 200 && status != 500 {
		return nil, submitted, fmt.Errorf("CNAS HTTP %d", status)
	}
	values, e = operationValues(op, data)
	if e != nil {
		return nil, submitted, e
	}
	if status != 200 {
		return nil, submitted, fmt.Errorf("CNAS HTTP %d", status)
	}
	if op.Name == "getEInvoiceId" && len(values) > 1 {
		// The legacy response is [ID, error]; the three-value response has data
		// in the first two positions and the validation message in the third.
		message := values[len(values)-1]
		if strings.TrimSpace(message) != "" {
			return nil, false, fmt.Errorf("CNAS getEInvoiceId: %s", message)
		}
	}
	return values, submitted, nil
}
func downloadAddress(raw, base string) (string, error) {
	origin, e := url.Parse(base)
	if e != nil {
		return "", e
	}
	ref, e := url.Parse(raw)
	if e != nil {
		return "", e
	}
	u := origin.ResolveReference(ref)
	if u.Scheme != "https" || !strings.EqualFold(u.Host, origin.Host) || u.User != nil || u.Fragment != "" || strings.Contains(u.Path, "\\") {
		return "", errors.New("download must remain on configured CNAS HTTPS origin")
	}
	return u.String(), nil
}

func xmlParameter(args map[string]string) string {
	for _, k := range []string{"reportXml", "requestXml", "reportXML"} {
		if v, ok := args[k]; ok {
			return v
		}
	}
	return ""
}

func (s settings) serviceBaseURL(service string) string {
	if service == "EPrescriptionWS" {
		return s.PEBaseURL
	}
	if service == "SiuiEInvoiceWS" && s.EInvoiceBaseURL != "" {
		return s.EInvoiceBaseURL
	}
	return s.BaseURL
}
