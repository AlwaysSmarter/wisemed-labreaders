package siui

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/datamatrix"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/big"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

var partnerCategories = []string{"MF", "PHM", "CLIN", "PARA", "STOM", "AMB", "MD", "HC", "REC", "RECA", "SPT", "NHP", "DIA", "SICK", "CBRET"}

type invoiceInput struct {
	Header          map[string]string   `json:"header"`
	Provider        map[string]string   `json:"provider"`
	Insurer         map[string]string   `json:"insurer"`
	Items           []map[string]string `json:"items"`
	ProviderAddress string              `json:"provider_address"`
	InsurerAddress  string              `json:"insurer_address"`
}
type invoiceField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

func invoiceFields() map[string][]invoiceField {
	b, _ := assets.ReadFile("assets/invoice_fields.json")
	var v map[string][]invoiceField
	_ = json.Unmarshal(b, &v)
	return v
}

var invoiceDecimal = regexp.MustCompile(`^[+-]?[0-9]{1,15}(\.[0-9]{1,6})?$`)

func decimal(v string) (*big.Rat, error) {
	if !invoiceDecimal.MatchString(v) {
		return nil, errors.New("număr zecimal invalid (maxim 6 zecimale, separator punct)")
	}
	n, ok := new(big.Rat).SetString(v)
	if !ok {
		return nil, errors.New("număr invalid")
	}
	return n, nil
}
func invoiceTotals(in *invoiceInput) error {
	if len(in.Items) < 1 || len(in.Items) > 200 {
		return errors.New("factura trebuie să aibă între 1 și 200 articole")
	}
	net, vat := new(big.Rat), new(big.Rat)
	for i, item := range in.Items {
		price, err := decimal(item["unitPrice"])
		if err != nil {
			return fmt.Errorf("articol %d: unitPrice: %w", i+1, err)
		}
		qty, err := decimal(item["quantity"])
		if err != nil {
			return fmt.Errorf("articol %d: quantity: %w", i+1, err)
		}
		amount := new(big.Rat).Mul(price, qty).FloatString(2)
		expected, amountErr := decimal(amount)
		if amountErr != nil {
			return fmt.Errorf("articol %d: valoare prea mare", i+1)
		}
		if v := item["ammount"]; v != "" {
			n, e := decimal(v)
			if e != nil || n.Cmp(expected) != 0 {
				return fmt.Errorf("articol %d: valoarea nu corespunde preț × cantitate, rotunjit la 2 zecimale", i+1)
			}
		}
		item["ammount"] = amount
		net.Add(net, expected)
		t, err := decimal(item["vat"])
		if err != nil {
			return fmt.Errorf("articol %d: vat este valoare TVA, nu procent: %w", i+1, err)
		}
		rounded, _ := new(big.Rat).SetString(t.FloatString(2))
		if rounded.Cmp(t) != 0 {
			return fmt.Errorf("articol %d: valoarea TVA trebuie să aibă cel mult 2 zecimale", i+1)
		}
		vat.Add(vat, t)
	}
	if in.Header == nil {
		in.Header = map[string]string{}
	}
	for k, v := range map[string]*big.Rat{"totalAmmount": net, "vatAmmount": vat} {
		if supplied := in.Header[k]; supplied != "" {
			n, e := decimal(supplied)
			if e != nil || n.Cmp(v) != 0 {
				return fmt.Errorf("%s nu corespunde sumei articolelor", k)
			}
		}
		in.Header[k] = v.FloatString(2)
	}
	return nil
}
func checkInvoice(in *invoiceInput) error {
	if err := invoiceTotals(in); err != nil {
		return err
	}
	sections := map[string][]map[string]string{"header": {in.Header}, "provider": {in.Provider}, "insurer": {in.Insurer}, "items": in.Items}
	for section, rows := range sections {
		allowed := map[string]invoiceField{}
		for _, f := range invoiceFields()[section] {
			allowed[f.Name] = f
		}
		for _, row := range rows {
			for k := range row {
				if _, ok := allowed[k]; !ok {
					return fmt.Errorf("%s: atribut necunoscut %s", section, k)
				}
			}
			for name, f := range allowed {
				v := row[name]
				if f.Required && strings.TrimSpace(v) == "" {
					return fmt.Errorf("%s.%s este obligatoriu", section, name)
				}
				if v == "" {
					continue
				}
				if f.Type == "decimal" {
					if _, err := decimal(v); err != nil {
						return fmt.Errorf("%s.%s: %w", section, name, err)
					}
				}
				if f.Type == "date" {
					if _, err := time.Parse("2006-01-02", v); err != nil {
						return fmt.Errorf("%s.%s: data trebuie YYYY-MM-DD", section, name)
					}
				}
			}
		}
	}
	if in.Header["isSigned"] != "" {
		return errors.New("isSigned este numai în răspunsul CNAS, nu se completează la emitere")
	}
	valid := false
	for _, v := range partnerCategories {
		if in.Header["providerCategory"] == v {
			valid = true
		}
	}
	if !valid {
		return errors.New("providerCategory necunoscut")
	}
	if in.Header["startDate"] > in.Header["endDate"] {
		return errors.New("perioada de facturare este inversată")
	}
	return nil
}
func invoiceElement(enc *xml.Encoder, name string, attrs map[string]string, namespace bool, children func() error) error {
	start := xml.StartElement{Name: xml.Name{Local: name}}
	if namespace {
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "xmlns"}, Value: piasNS})
	}
	keys := make([]string, 0, len(attrs))
	for k, v := range attrs {
		if v != "" || k == "SC" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: k}, Value: attrs[k]})
	}
	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	if children != nil {
		if err := children(); err != nil {
			return err
		}
	}
	return enc.EncodeToken(start.End())
}
func renderInvoiceXML(in invoiceInput, compact bool) ([]byte, error) {
	var out bytes.Buffer
	enc := xml.NewEncoder(&out)
	header, provider, insurer := in.Header, in.Provider, in.Insurer
	if compact {
		fields := map[string]string{"SC": header["serialCode"], "SN": header["serialNumber"], "ID": header["issueDate"], "TA": header["totalAmmount"], "TT": header["vatAmmount"], "FA": in.ProviderAddress, "CA": in.InsurerAddress}
		for _, p := range []struct {
			prefix string
			values map[string]string
		}{{"F", provider}, {"C", insurer}} {
			for k, v := range map[string]string{"N": "companyName", "F": "fiscalCode", "R": "registerNo", "B": "bankName", "O": "bankAccount"} {
				fields[p.prefix+k] = p.values[v]
			}
		}
		fields["FC"] = provider["capital"]
		fields["FS"] = provider["signature"]
		err := invoiceElement(enc, "F", fields, true, func() error {
			for _, item := range in.Items {
				attrs := map[string]string{}
				for k, v := range map[string]string{"AD": "description", "AU": "unitName", "AP": "unitPrice", "AC": "quantity", "AV": "ammount", "AT": "vat"} {
					attrs[k] = item[v]
				}
				if err := invoiceElement(enc, "A", attrs, false, nil); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		err := invoiceElement(enc, "invoice", header, true, func() error {
			if err := invoiceElement(enc, "provider", provider, false, nil); err != nil {
				return err
			}
			if err := invoiceElement(enc, "insurer", insurer, false, nil); err != nil {
				return err
			}
			for _, item := range in.Items {
				if err := invoiceElement(enc, "item", item, false, nil); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
func invoiceZIP(name string, data []byte) ([]byte, error) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return nil, err
	}
	if _, err = w.Write(data); err != nil {
		return nil, err
	}
	if err = z.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
func invoiceBarcode(data []byte) (string, error) {
	code, err := datamatrix.Encode(string(data))
	if err != nil {
		return "", fmt.Errorf("factura depășește capacitatea unui DataMatrix; codul nu a fost trunchiat: %w", err)
	}
	size := code.Bounds().Dx() * 5
	scaled, err := barcode.Scale(code, size, size)
	if err != nil {
		return "", err
	}
	pic := image.NewRGBA(image.Rect(0, 0, size+20, size+20))
	draw.Draw(pic, pic.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(pic, image.Rect(10, 10, size+10, size+10), scaled, image.Point{}, draw.Src)
	var out bytes.Buffer
	if err := png.Encode(&out, pic); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(out.Bytes()), nil
}
func (m *Module) invoiceHTTP(w http.ResponseWriter, r *http.Request, route string, admin bool) {
	w.Header().Set("Cache-Control", "no-store")
	if route == "invoice/model" && r.Method == "GET" {
		b, _ := assets.ReadFile("assets/invoice_fields.json")
		items, _ := assets.ReadFile("assets/invoice_items.json")
		respond(w, 200, map[string]any{"ok": true, "invoice_items": json.RawMessage(items), "fields": json.RawMessage(b), "partner_categories": partnerCategories, "insurance_houses": insuranceHouses()})
		return
	}
	if r.Method != "POST" {
		fail(w, 405, "POST required")
		return
	}
	if route == "invoice/preview" {
		var in invoiceInput
		if err := decode(w, r, &in); err != nil {
			fail(w, 400, err.Error())
			return
		}
		if err := checkInvoice(&in); err != nil {
			fail(w, 400, err.Error())
			return
		}
		invoice, err := renderInvoiceXML(in, false)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		compact, err := renderInvoiceXML(in, true)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		for schema, data := range map[string][]byte{"ElectronicInvoice.xsd": invoice, "InvoiceBarcode.xsd": compact} {
			if err := validateSchema(ctx, schema, data); err != nil {
				fail(w, 400, schema+": "+err.Error())
				return
			}
		}
		packed, err := invoiceZIP("invoice.xml", compact)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		png, barcodeErr := invoiceBarcode(packed)
		warning := ""
		if barcodeErr != nil {
			warning = barcodeErr.Error()
		}
		respond(w, 200, map[string]any{"ok": true, "data": in, "invoice_xml": string(invoice), "barcode_xml": string(compact), "barcode_zip_base64": base64.StdEncoding.EncodeToString(packed), "barcode_png_base64": png, "barcode_warning": warning, "barcode_format": "DataMatrix", "total_with_vat": invoicePayable(in), "cnas_contacted": false, "signed": false})
		return
	}
	if route == "invoice/package" {
		if !admin {
			fail(w, 403, "administrator required for token signing")
			return
		}
		var in struct {
			XML string `json:"invoice_xml"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, 400, err.Error())
			return
		}
		if err := validateSchema(r.Context(), "ElectronicInvoice.xsd", []byte(in.XML)); err != nil {
			fail(w, 400, err.Error())
			return
		}
		c, release, ok := m.passiveClient(w, "invoice/package")
		if !ok {
			return
		}
		defer release()
		signer, ok := c.backend.(interface {
			SignInvoice(context.Context, []byte) ([]byte, error)
		})
		if !ok {
			fail(w, 503, "CMS signing unavailable on this platform")
			return
		}
		signed, err := signer.SignInvoice(r.Context(), []byte(in.XML))
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		packed, err := invoiceZIP("invoice.p7m", signed)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		encoded := base64.StdEncoding.EncodeToString(packed)
		if len(encoded) > maxData {
			fail(w, 413, "signed invoice exceeds API parameter size")
			return
		}
		respond(w, 200, map[string]any{"ok": true, "invoiceFile": encoded, "signed": true, "cnas_contacted": false})
		return
	}
	fail(w, 404, "unknown invoice route")
}

func addInvoiceOpenAPI(paths map[string]any) {
	sections := map[string]any{}
	for section, fields := range invoiceFields() {
		props := map[string]any{}
		required := []string{}
		for _, f := range fields {
			props[f.Name] = map[string]any{"type": "string"}
			if f.Required && f.Name != "totalAmmount" && f.Name != "vatAmmount" && f.Name != "ammount" {
				required = append(required, f.Name)
			}
		}
		obj := map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
		if section == "items" {
			sections[section] = map[string]any{"type": "array", "items": obj, "minItems": 1, "maxItems": 200}
		} else {
			sections[section] = obj
		}
	}
	sections["provider_address"] = map[string]any{"type": "string"}
	sections["insurer_address"] = map[string]any{"type": "string"}
	for _, spec := range []struct {
		path, description string
		schema            map[string]any
	}{
		{"preview", "Local XSD validation, invoice XML and ZIP-based DataMatrix PNG. No token or CNAS call. Monetary inputs are decimal strings; vat is an amount, not a rate.", map[string]any{"type": "object", "properties": sections, "required": []string{"header", "provider", "insurer", "items"}, "additionalProperties": false}},
		{"package", "Administrator only. Sign the exact XML with configured token (CMS attached), ZIP and Base64. PIN handled by driver. Does not send to CNAS.", map[string]any{"type": "object", "properties": map[string]any{"invoice_xml": map[string]any{"type": "string"}}, "required": []string{"invoice_xml"}, "additionalProperties": false}},
	} {
		paths["/api/siui/invoice/"+spec.path] = map[string]any{"post": map[string]any{"tags": []string{"facturare"}, "summary": spec.description, "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": spec.schema}}}, "responses": map[string]any{"200": map[string]any{"description": "JSON result"}, "400": map[string]any{"description": "Invalid invoice"}, "403": map[string]any{"description": "Administrator required for signing"}, "409": map[string]any{"description": "Token busy or not configured"}}}}
	}
}

func invoicePayable(in invoiceInput) string {
	net, _ := decimal(in.Header["totalAmmount"])
	vat, _ := decimal(in.Header["vatAmmount"])
	return new(big.Rat).Add(net, vat).FloatString(2)
}
