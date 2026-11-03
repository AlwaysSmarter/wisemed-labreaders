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
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const docMedNS = "http://www.cnas.ro/pias/2.0"
const docMedXMLLimit = 32 * 1024 * 1024

type docMedMessage struct {
	Code       string `json:"code"`
	AlertLevel string `json:"alert_level,omitempty"`
	Message    string `json:"message"`
}
type docMedBusinessError struct{ Messages []docMedMessage }

func docMedResponseValidated(err error) bool {
	var business *docMedBusinessError
	return err == nil || errors.As(err, &business)
}

func (e *docMedBusinessError) Error() string {
	codes := []string{}
	for _, m := range e.Messages {
		if len(codes) < 8 {
			codes = append(codes, m.Code)
		}
	}
	return "DocMED rejected the request: " + strings.Join(codes, ", ")
}
func docMedOperations() []Operation {
	data, _ := assets.ReadFile("assets/docmed_operations.json")
	var out []Operation
	_ = json.Unmarshal(data, &out)
	examplesData, _ := assets.ReadFile("assets/docmed_examples.json")
	var examples map[string]string
	_ = json.Unmarshal(examplesData, &examples)
	for i := range out {
		out[i].RequestExample = examples[out[i].Name]
		out[i].Form = docMedForm(out[i].Name)
	}
	return out
}
func docMedOperation(name string) (Operation, bool) { return findOperation("docmed", name) }
func isDocMedCall(name string) bool                 { _, ok := docMedOperation(name); return ok }

func docMedEnvelope(raw string) []byte {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "<?xml") && len(raw) > 5 && strings.ContainsRune(" \t\r\n", rune(raw[5])) {
		if end := strings.Index(raw, "?>"); end >= 0 {
			raw = strings.TrimSpace(raw[end+2:])
		}
	}
	return []byte(`<soap:Envelope xmlns:soap="` + soapNS + `"><soap:Body>` + raw + `</soap:Body></soap:Envelope>`)
}

// Re-emit a standalone XML element with its namespaces, including namespaces
// originally inherited from the SOAP envelope. Binary signatures remain base64.
func standaloneDocMedXML(n node) ([]byte, error) {
	var clean func(*node)
	clean = func(n *node) {
		attrs := n.Attrs[:0]
		for _, a := range n.Attrs {
			if a.Name.Local != "xmlns" && a.Name.Space != "xmlns" {
				attrs = append(attrs, a)
			}
		}
		n.Attrs = attrs
		for i := range n.Children {
			clean(&n.Children[i])
		}
	}
	clean(&n)
	return xml.Marshal(n)
}
func docMedSOAPResult(op Operation, data []byte) ([]byte, error) {
	root, err := parseXMLLimit(data, docMedXMLLimit)
	if err != nil {
		return nil, err
	}
	if root.XMLName.Local != "Envelope" || root.XMLName.Space != soapNS {
		return nil, errors.New("invalid DocMED SOAP envelope")
	}
	var body *node
	for i := range root.Children {
		child := &root.Children[i]
		if child.XMLName.Local == "Body" && child.XMLName.Space == soapNS {
			if body != nil {
				return nil, errors.New("duplicate SOAP body")
			}
			body = child
		}
	}
	if body == nil || len(body.Children) != 1 {
		return nil, errors.New("one DocMED SOAP response required")
	}
	response := body.Children[0]
	if response.XMLName.Local == "Fault" && response.XMLName.Space == soapNS {
		fault, err := parseDocMedFault(response)
		if err != nil {
			return nil, err
		}
		return fault.RawXML, fault
	}
	if response.XMLName.Local != op.ResponseElement || response.XMLName.Space != docMedNS {
		return nil, errors.New("unexpected DocMED response element or namespace")
	}
	return standaloneDocMedXML(response)
}
func docMedMessages(root node) []docMedMessage {
	var out []docMedMessage
	for _, container := range root.Children {
		if container.XMLName.Local != "errors" || container.XMLName.Space != docMedNS {
			continue
		}
		for _, e := range container.Children {
			if e.XMLName.Local == "error" && e.XMLName.Space == docMedNS {
				out = append(out, docMedMessage{e.attr("code"), e.attr("alertLevel"), strings.TrimSpace(e.Text)})
			}
		}
	}
	return out
}
func docMedRejection(op Operation, root node) error {
	messages := docMedMessages(root)
	var blocking []docMedMessage
	for _, m := range messages {
		if op.Service != "DocumentSeries" || m.AlertLevel == "E" {
			blocking = append(blocking, m)
		}
	}
	if len(blocking) > 0 {
		return &docMedBusinessError{blocking}
	}
	return nil
}
func docMedAttachment(raw []byte) ([]byte, error) {
	root, err := parseXMLLimit(raw, docMedXMLLimit)
	if err != nil {
		return nil, err
	}
	if root.XMLName.Local != "GetMedicalDocumentResponse" || root.XMLName.Space != docMedNS {
		return nil, errors.New("attachment is only available for GetMedicalDocument")
	}
	attached := root.child("attachedFile")
	if attached.XMLName.Space != docMedNS || strings.TrimSpace(attached.Text) == "" {
		return nil, errors.New("DocMED response is missing attachedFile")
	}
	data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(attached.Text), ""))
	if err != nil || len(data) == 0 {
		return nil, errors.New("invalid DocMED attachedFile base64")
	}
	if _, err = zip.NewReader(bytes.NewReader(data), int64(len(data))); err != nil {
		return nil, errors.New("DocMED attachedFile is not a ZIP archive")
	}
	return data, nil
}
func (c *client) invokeDocMed(ctx context.Context, op Operation, args map[string]string, nulls ...string) (values []string, submitted bool, err error) {
	address := strings.TrimRight(c.cfg.serviceBaseURL(op.Service), "/") + op.EndpointPath
	defer func() { err = withConnectionError(address, err) }()
	if err = validateParameters(op, args, nulls...); err != nil {
		return
	}
	if err = c.backend.Validate(ctx, "docmed:"+op.RequestSchema, []byte(args["requestXml"])); err != nil {
		err = fmt.Errorf("DocMED request validation (%s): %w", op.RequestSchema, err)
		return
	}
	if op.Name == "ConsumeDocument" || op.Name == "UnconsumeDocument" {
		request, _ := parseXMLLimit([]byte(args["requestXml"]), docMedXMLLimit)
		statusRequest := node{XMLName: xml.Name{Space: docMedNS, Local: "GetDocumentStatusRequest"}}
		for _, key := range []string{"personCID", "serialCode", "serialNo", "issueDate"} {
			if value := request.attr(key); value != "" {
				statusRequest.Attrs = append(statusRequest.Attrs, xml.Attr{Name: xml.Name{Local: key}, Value: value})
			}
		}
		raw, _ := standaloneDocMedXML(statusRequest)
		statusOp, _ := docMedOperation("GetDocumentStatus")
		statusValues, _, statusErr := c.invokeDocMed(ctx, statusOp, map[string]string{"requestXml": string(raw)})
		if statusErr != nil {
			return nil, false, fmt.Errorf("DocMED status preflight failed: %w", statusErr)
		}
		statusRoot, _ := parseXMLLimit([]byte(statusValues[0]), docMedXMLLimit)
		expected := "FINALIZED"
		if op.Name == "UnconsumeDocument" {
			expected = "CONSUMED"
		}
		if statusRoot.attr("status") != expected {
			return nil, false, fmt.Errorf("DocMED %s requires status %s; received %s", op.Name, expected, statusRoot.attr("status"))
		}
	}
	headers, e := c.sessionHeaders(ctx)
	if e != nil {
		return nil, false, e
	}
	if e = ctx.Err(); e != nil {
		return nil, false, e
	}
	code, _, data, e := c.backend.Do(ctx, http.MethodPost, address, headers, docMedEnvelope(args["requestXml"]))
	submitted = op.Mutating
	if e != nil {
		if connectionFailedBeforeSend(e) {
			submitted = false
		}
		return nil, submitted, e
	}
	if code != 200 && code != 500 {
		return nil, submitted, fmt.Errorf("DocMED HTTP %d", code)
	}
	response, e := docMedSOAPResult(op, data)
	if e != nil {
		var fault *docMedSOAPFault
		if errors.As(e, &fault) {
			fault.HTTPStatus = code
			fault.WriteMayHaveBeenApplied = op.Mutating
			if redactErr := fault.redact(c.cfg.Licence, headers.Get("OSCP_RESPONSE"), headers.Get("SessionID"), headers.Get("Authorization"), strings.TrimPrefix(headers.Get("Authorization"), "Basic ")); redactErr != nil {
				return nil, submitted, redactErr
			}
			return []string{string(fault.RawXML)}, submitted, fault
		}
		return nil, submitted, e
	}
	if code != 200 {
		return nil, submitted, fmt.Errorf("DocMED HTTP %d", code)
	}
	if e = c.backend.Validate(ctx, "docmed:"+op.ResponseSchema, response); e != nil {
		return []string{string(response)}, submitted, fmt.Errorf("DocMED response validation (%s): %w", op.ResponseSchema, e)
	}
	root, e := parseXMLLimit(response, docMedXMLLimit)
	if e != nil {
		return nil, submitted, e
	}
	values = []string{string(response)}
	if e = docMedRejection(op, root); e != nil {
		return values, false, e
	}
	if op.Name == "GetMedicalDocument" {
		if _, e = docMedAttachment(response); e != nil {
			return nil, false, e
		}
	}
	if op.Name == "GetMedicalDocumentXml" {
		doc := root.child("document")
		if doc.XMLName.Space != docMedNS || len(doc.Children) != 1 {
			return nil, false, errors.New("DocMED response is missing document XML")
		}
	}
	if op.Name == "GetDocumentStatus" && root.attr("status") == "" {
		return nil, false, errors.New("DocMED response is missing document status")
	}
	return values, submitted, nil
}

// The complete XML is a chunked, owner-scoped file. Polling returns a bounded
// structured summary, so attached PDFs never inflate a WSM polling response.
func (m *Module) storeDocMedResult(ctx context.Context, job serviceWork, raw []byte, validation ...bool) (map[string]any, map[string]any, error) {
	root, err := parseXMLLimit(raw, docMedXMLLimit)
	if err != nil {
		return nil, nil, err
	}
	attrs := map[string]string{}
	for _, a := range root.Attrs {
		if a.Name.Space != "xmlns" && a.Name.Local != "xmlns" {
			attrs[a.Name.Local] = a.Value
		}
	}
	trusted := len(validation) == 0 || validation[0]
	messages := docMedMessages(root)
	if !trusted {
		messages = nil
	}
	count := len(messages)
	if len(messages) > 100 {
		messages = messages[:100]
	}
	summary := map[string]any{"response_element": root.XMLName.Local, "attributes": attrs, "messages": messages, "message_count": count, "validated": trusted}
	if rejected := docMedRejection(job.Operation, root); trusted && rejected == nil {
		// Include small structured results (series, status, document) without binaries.
		small := root
		small.Children = append([]node(nil), root.Children...)
		for i := range small.Children {
			if small.Children[i].XMLName.Local == "attachedFile" {
				small.Children[i].Text = ""
			}
		}
		if encoded, e := standaloneDocMedXML(small); e == nil && len(encoded) <= 64*1024 {
			if data, e := fileXMLJSON(ctx, encoded); e == nil {
				summary["data"] = json.RawMessage(data)
			}
		}
		if job.Operation.Name == "GetMedicalDocument" {
			data, e := docMedAttachment(raw)
			if e != nil {
				return nil, nil, e
			}
			summary["attachment"] = map[string]any{"size": len(data), "filename": "document.zip", "content_type": "application/zip", "chunk_size": downloadChunk, "path": "/api/siui/calls/" + job.ID + "/file?docmed_attachment=true"}
		}
	}
	downloadStorageMu.RLock()
	defer downloadStorageMu.RUnlock()
	path := datedDownloadPath(job.Config.Database, job.ID, time.Now().UTC())
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, nil, err
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		return nil, nil, errors.New("cannot save DocMED response")
	}
	file := map[string]any{"size": len(raw), "filename": job.Operation.ResponseElement + ".xml", "content_type": "application/xml", "chunk_size": downloadChunk, "path": "/api/siui/calls/" + job.ID + "/file", "validation_schema": "docmed:" + job.Operation.ResponseSchema}
	if isDocMedFaultRoot(root) {
		file["filename"] = "SOAPFault.xml"
		delete(file, "validation_schema")
	}
	return summary, file, nil
}

// Transport caps are identical on Windows, macOS and PKCS#11 transports.
func cnasResponseLimit(method, address string) int {
	u, err := url.Parse(address)
	if err != nil {
		return maxData
	}
	if method == http.MethodPost {
		switch u.Path {
		case "/loader/ws", "/seq/ws", "/download/ws", "/status/ws":
			return docMedXMLLimit
		}
	}
	if method == http.MethodGet && !strings.HasPrefix(u.Path, "/OCSP/") {
		return maxDownload
	}
	return maxData
}
