package siui

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func docMedExampleFields(t *testing.T, model docMedFormNode, n node) any {
	t.Helper()
	if model.Value != nil {
		return n.Text
	}
	fields := map[string]any{}
	for _, a := range n.Attrs {
		if a.Name.Local != "xmlns" && a.Name.Space != "xmlns" {
			fields[a.Name.Local] = a.Value
		}
	}
	for _, child := range model.Children {
		var values []any
		for _, c := range n.Children {
			if c.XMLName.Local == child.Name {
				values = append(values, docMedExampleFields(t, child, c))
			}
		}
		if len(values) > 0 {
			if child.Max == 1 {
				fields[child.Name] = values[0]
			} else {
				fields[child.Name] = values
			}
		}
	}
	return fields
}
func allDocMedFormFields(t *testing.T, model docMedFormNode) any {
	t.Helper()
	value := func(field docMedFormField) string {
		if len(field.Options) > 0 {
			return field.Options[0].Value
		}
		if field.Name == "attachedFile" {
			return base64.StdEncoding.EncodeToString(docMedTestZIP(t))
		}
		if field.MinInclusive != "" {
			return field.MinInclusive
		}
		switch field.Type {
		case "date":
			return "2026-11-03"
		case "boolean":
			return "false"
		case "base64Binary":
			return "AA=="
		case "integer":
			return "1"
		}
		if field.Pattern == "[0-9A-Z]{25}" {
			return strings.Repeat("A", 25)
		}
		if field.Length != "" {
			n, _ := strconv.Atoi(field.Length)
			return strings.Repeat("0", n)
		}
		result := "TEST"
		if field.Pattern == "[0-9]*" {
			result = "123"
		}
		if field.MaxLength != "" {
			n, _ := strconv.Atoi(field.MaxLength)
			if len(result) > n {
				result = result[:n]
			}
		}
		return result
	}
	if model.Value != nil {
		return value(*model.Value)
	}
	result := map[string]any{}
	for _, field := range model.Attributes {
		result[field.Name] = value(field)
	}
	for _, child := range model.Children {
		v := allDocMedFormFields(t, child)
		if child.Max == 1 {
			result[child.Name] = v
		} else {
			result[child.Name] = []any{v}
		}
	}
	return result
}
func TestDocMedFormsBuildEveryRequestIncludingOptionalFields(t *testing.T) {
	for _, op := range docMedOperations() {
		t.Run(op.Name, func(t *testing.T) {
			if op.Form == nil {
				t.Fatal("missing form")
			}
			root, err := parseXMLLimit([]byte(op.RequestExample), docMedXMLLimit)
			if err != nil {
				t.Fatal(err)
			}
			for _, fields := range []any{docMedExampleFields(t, *op.Form, root), allDocMedFormFields(t, *op.Form)} {
				data, _ := json.Marshal(fields)
				request, err := buildDocMedRequest(op, data)
				if err != nil {
					t.Fatal(err)
				}
				if err = validateSchema(context.Background(), "docmed:"+op.RequestSchema, []byte(request)); err != nil {
					t.Fatalf("built XML does not match schema: %v", err)
				}
				parsed, err := parseXMLLimit([]byte(request), docMedXMLLimit)
				if err != nil || parsed.XMLName.Space != docMedNS || parsed.XMLName.Local != op.RequestElement {
					t.Fatal("wrong generated root", err)
				}
			}
		})
	}
}
func TestDocMedGetSeriesFormUserRequestEscapingAndBounds(t *testing.T) {
	op, _ := docMedOperation("GetDocumentSeries")
	fields := map[string]any{"providerCode": "7991962", "providerName": `HITMED & <test> "A"`, "providerInsuranceHouse": "CAS-DJ", "getDocumentSeries": map[string]any{"insuranceHouseCode": "CAS-DJ", "documentType": "ML", "validFrom": "2026-11-02", "validTo": "2026-11-02"}}
	raw, _ := json.Marshal(fields)
	request, err := buildDocMedRequest(op, raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateSchema(context.Background(), "docmed:"+op.RequestSchema, []byte(request)); err != nil {
		t.Fatal(err)
	}
	root, _ := parseXMLLimit([]byte(request), docMedXMLLimit)
	if root.attr("providerName") != fields["providerName"] {
		t.Fatal("provider name changed")
	}
	if !strings.Contains(request, "&lt;test&gt;") {
		t.Fatal("XML injection was not escaped")
	}
	for _, invalid := range []string{`{}`, `{"requestXml":"<bad/>"}`, `{"providerCode":7991962}`, `null`} {
		if _, err = buildDocMedRequest(op, []byte(invalid)); err == nil {
			t.Fatal("invalid form accepted", invalid)
		}
	}
	fields["getDocumentSeries"].(map[string]any)["documentType"] = "INVALID"
	raw, _ = json.Marshal(fields)
	if _, err = buildDocMedRequest(op, raw); err == nil || !strings.Contains(err.Error(), "Tip document") {
		t.Fatal("missing enum feedback", err)
	}
	gen, _ := docMedOperation("GenDocumentSeries")
	root, _ = parseXMLLimit([]byte(gen.RequestExample), docMedXMLLimit)
	values := docMedExampleFields(t, *gen.Form, root).(map[string]any)
	values["genDocumentSeries"].(map[string]any)["documentCount"] = "12"
	raw, _ = json.Marshal(values)
	if _, err = buildDocMedRequest(gen, raw); err == nil || !strings.Contains(err.Error(), "1000") {
		t.Fatal("documentCount minimum not enforced", err)
	}
}
func TestDocMedPDFFormCreatesDeterministicZIP(t *testing.T) {
	op, _ := docMedOperation("PhysicianElectronicPrescription")
	root, _ := parseXMLLimit([]byte(op.RequestExample), docMedXMLLimit)
	fields := docMedExampleFields(t, *op.Form, root).(map[string]any)
	fields["physicianElectronicPrescription"].(map[string]any)["attachedFile"] = map[string]string{"filename": "prescription.pdf", "data_base64": base64.StdEncoding.EncodeToString([]byte("%PDF-1.4\nsynthetic test"))}
	data, _ := json.Marshal(fields)
	first, err := buildDocMedRequest(op, data)
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildDocMedRequest(op, data)
	if err != nil || first != second {
		t.Fatal("PDF packaging changes idempotency")
	}
	if err = validateSchema(context.Background(), "docmed:"+op.RequestSchema, []byte(first)); err != nil {
		t.Fatal(err)
	}
}
func TestDocMedFormHTTPAndResponseValidationDiagnostics(t *testing.T) {
	b := &docMedTestBackend{t: t, response: func(op Operation) string {
		return `<GetDocumentSeriesResponse xmlns="` + docMedNS + `"><unexpected/></GetDocumentSeriesResponse>`
	}}
	m, rt := serviceTestModule(t, b)
	m.mu.Lock()
	m.cfg.DocMedBaseURL = DefaultDocMedBaseURL
	m.mu.Unlock()
	op, _ := docMedOperation("GetDocumentSeries")
	root, _ := parseXMLLimit([]byte(op.RequestExample), docMedXMLLimit)
	fields := docMedExampleFields(t, *op.Form, root)
	payload, _ := json.Marshal(map[string]any{"operation_id": "form-series", "fields": fields})
	request := httptest.NewRequest("POST", "/api/siui/docmed/GetDocumentSeries", bytes.NewReader(payload))
	request.Header.Set("Authorization", "test-local-session")
	request.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	rt.mux.ServeHTTP(w, request)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var reply struct {
		Call serviceCall `json:"call"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &reply)
	var call serviceCall
	for until := time.Now().Add(5 * time.Second); time.Now().Before(until); {
		call, _ = readServiceCall(m.db, "local:operator", reply.Call.ID)
		if call.Status == "failed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if call.Status != "failed" || !strings.Contains(call.Error, "response validation") {
		t.Fatalf("request/response error not distinguished: %+v", call)
	}
	var result struct {
		DocMed struct {
			Validated bool `json:"validated"`
			Data      any  `json:"data"`
		} `json:"docmed"`
		File any `json:"file"`
	}
	_ = json.Unmarshal(call.Result, &result)
	if result.DocMed.Validated || result.DocMed.Data != nil || result.File == nil {
		t.Fatal("unvalidated response was trusted or response file lost", string(call.Result))
	}
	w = httptest.NewRecorder()
	m.serviceFileHTTP(w, httptest.NewRequest("GET", "/?offset=0", nil), call)
	if w.Code != 200 {
		t.Fatal("invalid response cannot be inspected", w.Code)
	}
}

func TestDocMedPESeriesMatchesPublishedNamespaceContract(t *testing.T) {
	op, _ := docMedOperation("GenDocumentSeries")
	fields := []byte(`{"providerCode":"123","providerName":"EXAMPLE","providerInsuranceHouse":"CAS-DJ","genDocumentSeries":{"insuranceHouseCode":"CAS-DJ","documentCount":"2500","documentType":"PE"}}`)
	request, err := buildDocMedRequest(op, fields)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateSchema(context.Background(), "docmed:"+op.RequestSchema, []byte(request)); err != nil {
		t.Fatal(err)
	}
	soap, err := parseXMLLimit(docMedEnvelope(request), docMedXMLLimit)
	if err != nil {
		t.Fatal(err)
	}
	payload := soap.child("Body").child("GenDocumentSeriesRequest")
	series := payload.child("genDocumentSeries")
	if payload.XMLName.Space != docMedNS || series.XMLName.Space != docMedNS || series.attr("documentType") != "PE" || series.attr("documentCount") != "2500" {
		t.Fatal("PE series payload missing or changed")
	}
	found := false
	for _, attr := range series.Attrs {
		if attr.Name.Local == "documentType" {
			found = true
			if attr.Name.Space != "" {
				t.Fatal("documentType must be an unqualified XML attribute")
			}
		}
	}
	if !found {
		t.Fatal("documentType attribute missing")
	}
	// Qualifying this attribute would violate the published schema, so a server
	// binding discrepancy must not be 'fixed' by silently changing the protocol.
	wrong := strings.Replace(request, `documentType="PE"`, `xmlns:d="`+docMedNS+`" d:documentType="PE"`, 1)
	if err = validateSchema(context.Background(), "docmed:"+op.RequestSchema, []byte(wrong)); err == nil {
		t.Fatal("qualified documentType unexpectedly accepted")
	}
}
