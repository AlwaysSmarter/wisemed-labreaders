package siui

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDocMedSOAPFaultDetailsArePreservedAndRedacted(t *testing.T) {
	for _, name := range []string{"GetDocumentSeries", "GenDocumentSeries"} {
		for _, status := range []int{200, 500} {
			t.Run(fmt.Sprintf("%s-HTTP-%d", name, status), func(t *testing.T) {
				op, _ := docMedOperation(name)
				b := &docMedTestBackend{t: t, responseStatus: status, response: func(Operation) string {
					return `<s:Fault><faultcode>s:Server</faultcode><faultstring>Request rejected: synthetic docmed-session</faultstring><faultactor>DocMED</faultactor><detail><error xmlns="urn:docmed:fault" code="INVALID_SERIES">Check series permissions synthetic docmed-session</error></detail></s:Fault>`
				}}
				c := docMedTestClient(b)
				values, submitted, err := c.invokeOperation(context.Background(), op, map[string]string{"requestXml": op.RequestExample})
				var fault *docMedSOAPFault
				if !errors.As(err, &fault) || submitted != op.Mutating || len(values) != 1 {
					t.Fatalf("fault lost or mutation replay semantics changed: submitted=%t values=%d err=%v", submitted, len(values), err)
				}
				if fault.Code != "s:Server" || fault.HTTPStatus != status || !strings.Contains(fault.DetailXML, "INVALID_SERIES") || !strings.Contains(err.Error(), "Request rejected") {
					t.Fatalf("fault detail missing: %+v", fault)
				}
				encoded, _ := json.Marshal(fault)
				for _, secret := range []string{"synthetic", "docmed-session"} {
					if strings.Contains(string(encoded), secret) || strings.Contains(values[0], secret) || strings.Contains(err.Error(), secret) {
						t.Fatal("secret leaked into SOAP fault diagnostics")
					}
				}
				count := 0
				for _, call := range b.calls {
					if call == name {
						count++
					}
				}
				if count != 1 {
					t.Fatal("SOAP fault triggered replay", b.calls)
				}
			})
		}
	}
}
func TestDocMedFaultXMLRedactsEscapedCredentialsAndAuthorization(t *testing.T) {
	root, err := parseXMLLimit([]byte(`<s:Fault xmlns:s="`+soapNS+`"><faultcode>s:Client</faultcode><faultstring>secret&lt;&amp; refused</faultstring><detail auth="Basic ABC123"><e token="TOKEN">Basic ABC123 secret&lt;&amp; TOKEN</e></detail></s:Fault>`), docMedXMLLimit)
	if err != nil {
		t.Fatal(err)
	}
	fault, err := parseDocMedFault(root)
	if err != nil {
		t.Fatal(err)
	}
	if err = fault.redact("secret<&", "TOKEN", "Basic ABC123", "ABC123"); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret", "TOKEN", "ABC123"} {
		if strings.Contains(string(fault.RawXML), secret) || strings.Contains(fault.DetailXML, secret) || strings.Contains(fault.Error(), secret) {
			t.Fatal("escaped or attribute secret not redacted")
		}
	}
	if _, err = parseXMLLimit(fault.RawXML, docMedXMLLimit); err != nil {
		t.Fatal("redacted XML invalid", err)
	}
}
func TestDocMedSOAPFaultWorkerKeepsUnknownAndDownloadableDetails(t *testing.T) {
	b := &docMedTestBackend{t: t, responseStatus: 500, response: func(Operation) string {
		return `<s:Fault><faultcode>s:Server</faultcode><faultstring>Generation could not be confirmed</faultstring><detail><code>REMOTE_FAILURE</code></detail></s:Fault>`
	}}
	m, rt := serviceTestModule(t, b)
	m.mu.Lock()
	m.cfg.DocMedBaseURL = DefaultDocMedBaseURL
	m.mu.Unlock()
	op, _ := docMedOperation("GenDocumentSeries")
	body, _ := json.Marshal(map[string]any{"operation_id": "fault-series", "parameters": map[string]string{"requestXml": op.RequestExample}})
	req := httptest.NewRequest("POST", "/api/siui/docmed/GenDocumentSeries", bytes.NewReader(body))
	req.Header.Set("Authorization", "test-local-session")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	rt.mux.ServeHTTP(w, req)
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	var reply struct {
		Call serviceCall `json:"call"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &reply)
	var call serviceCall
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		call, _ = readServiceCall(m.db, "local:operator", reply.Call.ID)
		if call.Status == "unknown" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	var result struct {
		DocMed struct {
			Fault     docMedSOAPFault `json:"fault"`
			Validated bool            `json:"validated"`
		} `json:"docmed"`
		File struct {
			Filename string `json:"filename"`
			Schema   string `json:"validation_schema"`
		} `json:"file"`
	}
	_ = json.Unmarshal(call.Result, &result)
	if call.Status != "unknown" || result.DocMed.Validated || result.DocMed.Fault.Code != "s:Server" || result.DocMed.Fault.HTTPStatus != 500 || result.File.Filename != "SOAPFault.xml" || result.File.Schema != "" {
		t.Fatalf("fault was lost or reclassified: %+v %s", call, call.Result)
	}
	req = httptest.NewRequest("GET", "/api/siui/calls/"+call.ID+"/file?offset=0", nil)
	req.Header.Set("Authorization", "test-local-session")
	w = httptest.NewRecorder()
	rt.mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var chunk struct {
		Data string `json:"data_base64"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &chunk)
	data, _ := base64.StdEncoding.DecodeString(chunk.Data)
	if !bytes.Contains(data, []byte("REMOTE_FAILURE")) {
		t.Fatal("SOAP fault file missing detail")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	count := 0
	for _, name := range b.calls {
		if name == "GenDocumentSeries" {
			count++
		}
	}
	if count != 1 {
		t.Fatal("generation was replayed")
	}
}
