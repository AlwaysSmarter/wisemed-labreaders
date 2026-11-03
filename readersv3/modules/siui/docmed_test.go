package siui

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"wisemed-labreaders/readersv3/shared/apibridge"
)

type docMedTestBackend struct {
	t              *testing.T
	mu             sync.Mutex
	calls          []string
	status         string
	failWrite      bool
	responseStatus int
	response       func(Operation) string
}

func (b *docMedTestBackend) Validate(ctx context.Context, schema string, data []byte) error {
	return validateSchema(ctx, schema, data)
}
func (b *docMedTestBackend) Do(_ context.Context, method, address string, headers http.Header, data []byte) (int, http.Header, []byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if strings.Contains(address, "/OCSP/") {
		b.calls = append(b.calls, "OCSP")
		return 200, http.Header{"Oscp_response": []string{"docmed-session"}}, nil, nil
	}
	if method != "POST" || headers.Get("SessionID") != "docmed-session" || headers.Get("SOAPAction") != `""` {
		b.t.Error("incorrect DocMED authentication or method")
	}
	root, e := parseXMLLimit(data, docMedXMLLimit)
	if e != nil {
		b.t.Fatal(e)
	}
	body := root.child("Body")
	if len(body.Children) != 1 {
		b.t.Fatal("request is not document/literal")
	}
	request := body.Children[0]
	var op Operation
	for _, candidate := range docMedOperations() {
		if candidate.RequestElement == request.XMLName.Local {
			op = candidate
			break
		}
	}
	if op.Name == "" || request.XMLName.Space != docMedNS {
		b.t.Fatal("unexpected request root")
	}
	if address != DefaultDocMedBaseURL+op.EndpointPath {
		b.t.Fatalf("wrong DocMED destination: %s", address)
	}
	b.calls = append(b.calls, op.Name)
	if op.Mutating && b.failWrite {
		return 0, nil, nil, errors.New("connection lost after sending")
	}
	response := docMedTestResponse(b.t, op)
	if op.Name == "GetDocumentStatus" && b.status != "" {
		response = `<GetDocumentStatusResponse xmlns="` + docMedNS + `" status="` + b.status + `"/>`
	}
	if b.response != nil {
		response = b.response(op)
	}
	status := b.responseStatus
	if status == 0 {
		status = 200
	}
	return status, http.Header{}, []byte(`<s:Envelope xmlns:s="` + soapNS + `"><s:Body>` + response + `</s:Body></s:Envelope>`), nil
}
func docMedTestZIP(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	f, e := w.Create("document.pdf")
	if e != nil {
		t.Fatal(e)
	}
	_, _ = f.Write([]byte("%PDF-1.4\nsynthetic fixture"))
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	return out.Bytes()
}
func docMedTestResponse(t *testing.T, op Operation) string {
	attrs, children := "", ""
	switch op.Name {
	case "GetDocumentStatus":
		attrs = ` status="FINALIZED"`
	case "GetMedicalDocumentXml":
		attrs = ` documentType="LR"`
		children = `<document><original xmlns="http://www.cnas.ro/siui/2.0" serialCode="TEST" serialNo="1"/></document>`
	case "GetMedicalDocument":
		children = `<attachedFile>` + base64.StdEncoding.EncodeToString(docMedTestZIP(t)) + `</attachedFile>`
	case "GenDocumentSeries", "GetDocumentSeries":
		children = `<documentSeriesList><documentSeriesItem documentType="LR" series="TEST" firstNo="1" lastNo="1000" validFrom="2026-11-02"/></documentSeriesList>`
	default:
		if op.Service == "DocMedService" {
			attrs = ` DocID="AAAAAAAAAAAAAAAAAAAAAAAAA" AppID="test"`
		}
	}
	return `<` + op.ResponseElement + ` xmlns="` + docMedNS + `"` + attrs + `>` + children + `</` + op.ResponseElement + `>`
}
func docMedTestClient(b backend) client {
	return client{cfg: settings{BaseURL: DefaultBaseURL, PEBaseURL: DefaultPEBaseURL, DocMedBaseURL: DefaultDocMedBaseURL, Username: "123_CAS-DJ", Licence: "synthetic"}, backend: b, session: &ocspSession{}}
}

func TestDocMedAllWSDLContractsAndSchemas(t *testing.T) {
	ops := docMedOperations()
	if len(ops) != 22 {
		t.Fatal("expected 22 DocMED operations")
	}
	seen := map[string]bool{}
	for _, op := range ops {
		t.Run(op.Name, func(t *testing.T) {
			data, e := assets.ReadFile("assets/wsdl/" + op.Service + ".wsdl")
			if e != nil {
				t.Fatal(e)
			}
			wsdl, e := parseXML(data)
			if e != nil {
				t.Fatal(e)
			}
			found := false
			for _, pt := range wsdl.Children {
				if pt.XMLName.Local == "portType" {
					for _, o := range pt.Children {
						if o.attr("name") == op.Name {
							found = true
						}
					}
				}
			}
			if !found {
				t.Fatal("catalog operation absent from WSDL")
			}
			seen[op.Name] = true
			if e = validateParameters(op, map[string]string{"requestXml": op.RequestExample}); e != nil {
				t.Fatal(e)
			}
			b := &docMedTestBackend{t: t}
			if op.Name == "UnconsumeDocument" {
				b.status = "CONSUMED"
			}
			c := docMedTestClient(b)
			values, submitted, e := c.invokeOperation(context.Background(), op, map[string]string{"requestXml": op.RequestExample})
			if e != nil || len(values) != 1 || submitted != op.Mutating {
				t.Fatalf("submitted=%t values=%d error=%v", submitted, len(values), e)
			}
		})
	}
	for _, service := range []string{"DocMedService", "DocumentSeries", "MedicalDocumentDownloader", "DocStatusChangerService"} {
		data, _ := assets.ReadFile("assets/wsdl/" + service + ".wsdl")
		wsdl, _ := parseXML(data)
		for _, pt := range wsdl.Children {
			if pt.XMLName.Local == "portType" {
				for _, o := range pt.Children {
					if !seen[o.attr("name")] {
						t.Fatal("missing WSDL operation", o.attr("name"))
					}
				}
			}
		}
	}
}
func TestDocMedBusinessMessagesAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name, level string
		reject      bool
	}{{"GenDocumentSeries", "W", false}, {"GetDocumentSeries", "I", false}, {"GenDocumentSeries", "", false}, {"GenDocumentSeries", "E", true}, {"MedicalLetter", "W", true}, {"GetMedicalDocumentXml", "I", true}, {"GetDocumentStatus", "E", true}} {
		t.Run(tc.name+tc.level, func(t *testing.T) {
			op, _ := docMedOperation(tc.name)
			b := &docMedTestBackend{t: t, response: func(op Operation) string {
				return `<` + op.ResponseElement + ` xmlns="` + docMedNS + `"><errors><error code="DOC_NOT_FOUND"` + func() string {
					if tc.level != "" {
						return ` alertLevel="` + tc.level + `"`
					}
					return ""
				}() + `>Synthetic response</error></errors></` + op.ResponseElement + `>`
			}}
			c := docMedTestClient(b)
			values, submitted, e := c.invokeOperation(context.Background(), op, map[string]string{"requestXml": op.RequestExample})
			var business *docMedBusinessError
			if errors.As(e, &business) != tc.reject || len(values) != 1 {
				t.Fatalf("rejection=%v values=%d err=%v", tc.reject, len(values), e)
			}
			if tc.reject && submitted {
				t.Fatal("definitive business rejection marked ambiguous")
			}
		})
	}
	op, _ := docMedOperation("GetMedicalDocumentXml")
	b := &docMedTestBackend{t: t}
	c := docMedTestClient(b)
	for _, raw := range []string{`<!DOCTYPE x [<!ENTITY bad SYSTEM "file:///etc/passwd">]><GetMedicalDocumentXmlRequest xmlns="` + docMedNS + `">&bad;</GetMedicalDocumentXmlRequest>`, `<GetMedicalDocumentRequest xmlns="` + docMedNS + `"/>`, `<GetMedicalDocumentXmlRequest xmlns="` + docMedNS + `"/>`} {
		if _, _, e := c.invokeOperation(context.Background(), op, map[string]string{"requestXml": raw}); e == nil {
			t.Fatal("invalid XML accepted")
		}
	}
	if len(b.calls) != 0 {
		t.Fatal("invalid input contacted CNAS")
	}
}
func TestDocMedStatusPreflightAndNoWriteRetry(t *testing.T) {
	for _, name := range []string{"ConsumeDocument", "UnconsumeDocument"} {
		op, _ := docMedOperation(name)
		b := &docMedTestBackend{t: t, status: "CANCELED"}
		c := docMedTestClient(b)
		if _, submitted, e := c.invokeOperation(context.Background(), op, map[string]string{"requestXml": op.RequestExample}); e == nil || submitted {
			t.Fatal("invalid preflight allowed a write")
		}
		for _, call := range b.calls {
			if call == name {
				t.Fatal("write was sent")
			}
		}
		b = &docMedTestBackend{t: t, status: "FINALIZED", failWrite: true}
		if name == "UnconsumeDocument" {
			b.status = "CONSUMED"
		}
		c = docMedTestClient(b)
		if _, submitted, e := c.invokeOperation(context.Background(), op, map[string]string{"requestXml": op.RequestExample}); e == nil || !submitted {
			t.Fatal("lost write response not ambiguous")
		}
		count := 0
		for _, call := range b.calls {
			if call == name {
				count++
			}
		}
		if count != 1 {
			t.Fatal("write replayed", b.calls)
		}
	}
}

func TestDocMedHTTPWSMScopesDeduplicationAndAttachments(t *testing.T) {
	b := &docMedTestBackend{t: t}
	m, rt := serviceTestModule(t, b)
	m.mu.Lock()
	m.cfg.BaseURL = DefaultBaseURL
	m.cfg.DocMedBaseURL = DefaultDocMedBaseURL
	m.mu.Unlock()
	principal := apibridge.Principal{Subject: "operator", TenantID: "clinic", ConnectionID: "wsm"}
	invoke := func(p apibridge.Principal, method, path string, body []byte) apibridge.Response {
		res, e := apibridge.Invoke(context.Background(), rt.mux, p, apibridge.Request{Method: method, Path: path, Body: body})
		if e != nil {
			t.Fatal(e)
		}
		return res
	}
	op, _ := docMedOperation("GenDocumentSeries")
	payload, _ := json.Marshal(map[string]any{"operation_id": "mutating", "parameters": map[string]string{"requestXml": op.RequestExample}})
	if res := invoke(principal, "POST", "/api/siui/docmed/GenDocumentSeries", payload); res.Status != 403 {
		t.Fatal("non-admin mutated DocMED", res)
	}
	for _, name := range []string{"GetMedicalDocumentXml", "GetMedicalDocument"} {
		op, _ := docMedOperation(name)
		payload, _ = json.Marshal(map[string]any{"operation_id": name, "parameters": map[string]string{"requestXml": op.RequestExample}})
		res := invoke(principal, "POST", "/api/siui/docmed/"+name, payload)
		if res.Status != 202 {
			t.Fatal(res)
		}
		raw, _ := json.Marshal(res.Body)
		var reply struct {
			Call serviceCall `json:"call"`
		}
		_ = json.Unmarshal(raw, &reply)
		id := reply.Call.ID
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
			res = invoke(principal, "GET", "/api/siui/calls/"+id, nil)
			raw, _ = json.Marshal(res.Body)
			_ = json.Unmarshal(raw, &reply)
			if reply.Call.Status != "queued" && reply.Call.Status != "running" {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if reply.Call.Status != "completed" {
			t.Fatalf("call failed: %+v", reply.Call)
		}
		if res := invoke(principal, "POST", "/api/siui/docmed/"+name, payload); res.Status != 200 {
			t.Fatal("dedupe failed", res)
		}
		other := principal
		other.TenantID = "other"
		if res := invoke(other, "GET", "/api/siui/calls/"+id+"/file", nil); res.Status != 404 {
			t.Fatal("cross-tenant disclosure")
		}
		res = invoke(principal, "GET", "/api/siui/calls/"+id+"/file?offset=0", nil)
		if res.Status != 200 {
			t.Fatal(res)
		}
		var chunk struct {
			Data string `json:"data_base64"`
		}
		raw, _ = json.Marshal(res.Body)
		_ = json.Unmarshal(raw, &chunk)
		data, _ := base64.StdEncoding.DecodeString(chunk.Data)
		if !bytes.Contains(data, []byte(op.ResponseElement)) {
			t.Fatal("response XML file missing")
		}
		if name == "GetMedicalDocument" {
			res = invoke(principal, "GET", "/api/siui/calls/"+id+"/file?docmed_attachment=true&offset=0", nil)
			if res.Status != 200 {
				t.Fatal(res)
			}
			raw, _ = json.Marshal(res.Body)
			_ = json.Unmarshal(raw, &chunk)
			data, _ = base64.StdEncoding.DecodeString(chunk.Data)
			if _, e := zip.NewReader(bytes.NewReader(data), int64(len(data))); e != nil {
				t.Fatal("invalid attached ZIP", e)
			}
			if strings.Contains(string(reply.Call.Result), "JVBERi0") {
				t.Fatal("binary attachment leaked into polling")
			}
		}
	}
	// Local session authentication reaches the same read route.
	op, _ = docMedOperation("GetDocumentStatus")
	payload, _ = json.Marshal(map[string]any{"operation_id": "local-status", "parameters": map[string]string{"requestXml": op.RequestExample}})
	req := httptest.NewRequest("POST", "/api/siui/docmed/GetDocumentStatus", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "test-local-session")
	w := httptest.NewRecorder()
	rt.mux.ServeHTTP(w, req)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestDocMedFailedBusinessCallRetainsStructuredMessages(t *testing.T) {
	b := &docMedTestBackend{t: t, response: func(op Operation) string {
		return `<` + op.ResponseElement + ` xmlns="` + docMedNS + `"><errors><error code="DOC_NOT_FOUND" alertLevel="E">Synthetic missing document</error></errors></` + op.ResponseElement + `>`
	}}
	m, rt := serviceTestModule(t, b)
	m.mu.Lock()
	m.cfg.DocMedBaseURL = DefaultDocMedBaseURL
	m.mu.Unlock()
	op, _ := docMedOperation("GetMedicalDocumentXml")
	payload, _ := json.Marshal(map[string]any{"operation_id": "missing-doc", "parameters": map[string]string{"requestXml": op.RequestExample}})
	req := httptest.NewRequest("POST", "/api/siui/docmed/GetMedicalDocumentXml", bytes.NewReader(payload))
	req.Header.Set("Authorization", "test-local-session")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	rt.mux.ServeHTTP(w, req)
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	var response struct {
		Call serviceCall `json:"call"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	var call serviceCall
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		call, _ = readServiceCall(m.db, "local:operator", response.Call.ID)
		if call.Status == "failed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if call.Status != "failed" || !strings.Contains(string(call.Result), "DOC_NOT_FOUND") || !strings.Contains(call.Error, "DOC_NOT_FOUND") {
		t.Fatalf("business failure lost: %+v", call)
	}
	w = httptest.NewRecorder()
	m.serviceFileHTTP(w, httptest.NewRequest("GET", "/?offset=0", nil), call)
	if w.Code != 200 {
		t.Fatal("failed response XML unavailable", w.Body.String())
	}
}

func TestDocMedLargeResponseAndChunkedAttachment(t *testing.T) {
	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	entry, e := zipWriter.CreateHeader(&zip.FileHeader{Name: "document.pdf", Method: zip.Store})
	if e != nil {
		t.Fatal(e)
	}
	payload := bytes.Repeat([]byte("%PDF-1.4 synthetic fixture\n"), 50000)
	_, _ = entry.Write(payload)
	if e = zipWriter.Close(); e != nil {
		t.Fatal(e)
	}
	encoded := base64.StdEncoding.EncodeToString(archive.Bytes())
	op, _ := docMedOperation("GetMedicalDocument")
	b := &docMedTestBackend{t: t, response: func(op Operation) string {
		return `<GetMedicalDocumentResponse xmlns="` + docMedNS + `"><attachedFile>` + encoded + `</attachedFile></GetMedicalDocumentResponse>`
	}}
	c := docMedTestClient(b)
	values, _, e := c.invokeOperation(context.Background(), op, map[string]string{"requestXml": op.RequestExample})
	if e != nil {
		t.Fatal(e)
	}
	if len(values[0]) <= maxData {
		t.Fatal("fixture must exceed legacy SOAP limit")
	}
	m, rt := serviceTestModule(t, b)
	job := serviceWork{ID: "large-docmed", Operation: op, Config: m.cfg}
	summary, file, e := m.storeDocMedResult(context.Background(), job, []byte(values[0]))
	if e != nil {
		t.Fatal(e)
	}
	result, _ := json.Marshal(map[string]any{"docmed": summary, "file": file})
	if len(result) > 64*1024 {
		t.Fatal("polling includes large binary")
	}
	call := serviceCall{ID: job.ID, Operation: op.Name, Status: "completed", Result: result}
	var joined []byte
	for offset := 0; offset < archive.Len(); {
		req := httptest.NewRequest("GET", "/?docmed_attachment=true&offset="+strconv.Itoa(offset), nil)
		w := httptest.NewRecorder()
		m.serviceFileHTTP(w, req, call)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var part struct {
			Data string `json:"data_base64"`
			Next int    `json:"next_offset"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &part)
		decoded, _ := base64.StdEncoding.DecodeString(part.Data)
		if len(decoded) == 0 || part.Next != offset+len(decoded) {
			t.Fatal("stalled attachment")
		}
		joined = append(joined, decoded...)
		offset = part.Next
	}
	if !bytes.Equal(joined, archive.Bytes()) {
		t.Fatal("attachment chunks corrupted")
	}
	_ = rt
}

func TestDocMedLiteralNamespacesAndResponseCaps(t *testing.T) {
	for _, space := range []string{" ", "\t", "\r", "\n"} {
		envelope := docMedEnvelope("<?xml" + space + `version="1.0"?>` + `<GetDocumentStatusRequest xmlns="` + docMedNS + `" serialCode="TEST" serialNo="1" issueDate="2026-11-02"/>`)
		if _, err := parseXMLLimit(envelope, docMedXMLLimit); err != nil {
			t.Fatal("XML declaration embedded inside SOAP", err)
		}
	}
	op, _ := docMedOperation("GetDocumentStatus")
	raw, e := docMedSOAPResult(op, []byte(`<s:Envelope xmlns:s="`+soapNS+`" xmlns:d="`+docMedNS+`"><s:Body><d:GetDocumentStatusResponse status="FINALIZED"/></s:Body></s:Envelope>`))
	if e != nil {
		t.Fatal(e)
	}
	if e = validateSchema(context.Background(), "docmed:"+op.ResponseSchema, raw); e != nil {
		t.Fatal("inherited namespace lost", e)
	}
	for _, tc := range []struct {
		method, url string
		limit       int
	}{{"POST", DefaultDocMedBaseURL + "/download/ws", docMedXMLLimit}, {"POST", DefaultDocMedBaseURL + "/loader/ws", docMedXMLLimit}, {"POST", DefaultBaseURL + "/svapntws/services/SiuiWS", maxData}, {"GET", DefaultBaseURL + "/OCSP/validator", maxData}, {"GET", DefaultBaseURL + "/file.zip", maxDownload}} {
		if got := cnasResponseLimit(tc.method, tc.url); got != tc.limit {
			t.Fatal(tc, got)
		}
	}
}
