package siui

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestFileProcessing(t *testing.T) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.Create("report.xml")
	w.Write([]byte("<root cid=\"40172426789613933080\">before<item>1</item>after<item>2</item></root>"))
	w, _ = z.Create("another.txt")
	w.Write([]byte("other"))
	z.Close()
	ctx := context.Background()
	if _, err := processCallFile(ctx, b.Bytes(), url.Values{"unzip": {"true"}}); err == nil {
		t.Fatal("ambiguous ZIP accepted")
	}
	data, err := processCallFile(ctx, b.Bytes(), url.Values{"unzip": {"true"}, "entry": {"report.xml"}, "format": {"json"}, "validateXSD": {"false"}})
	if err != nil {
		t.Fatal(err)
	}
	var result fileXMLNode
	if err = json.Unmarshal(data, &result); err != nil || result.Name != "root" || result.Attributes[0].Value != "40172426789613933080" || len(result.Content) != 4 {
		t.Fatal(string(data), err)
	}
	for _, q := range []url.Values{{"unzip": {"true"}, "entry": {"missing"}}, {"format": {"invalid"}}, {"schema": {"../bad.xsd"}}, {"entry": {"report.xml"}}} {
		if _, err = processCallFile(ctx, b.Bytes(), q); err == nil {
			t.Fatal(q)
		}
	}
	for _, bad := range []string{"<!DOCTYPE x><x/>", "<a/><b/>", "<a>", "text<a/>"} {
		if _, err = processCallFile(ctx, []byte(bad), url.Values{"format": {"json"}, "validateXSD": {"false"}}); err == nil {
			t.Fatal("accepted bad XML")
		}
	}
	raw := []byte("pdf-data")
	got, err := processCallFile(ctx, raw, url.Values{"validateXSD": {"false"}})
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal(err)
	}
}

func TestProcessedFileOffsets(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "siui-downloads"), 0700); err != nil {
		t.Fatal(err)
	}
	raw := []byte("<root code=\"001\"/>")
	if err := os.WriteFile(filepath.Join(dir, "siui-downloads", "call"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	m := &Module{cfg: settings{Database: filepath.Join(dir, "jobs.db")}}
	result, _ := json.Marshal(map[string]any{"file": map[string]any{"size": len(raw)}})
	call := serviceCall{ID: "call", Status: "completed", Result: result}
	expected, err := processCallFile(context.Background(), raw, url.Values{"format": {"json"}, "validateXSD": {"false"}})
	if err != nil {
		t.Fatal(err)
	}
	offset := len(raw) + 1 // Valid in processed JSON even though it exceeds original XML size.
	rec := httptest.NewRecorder()
	m.serviceFileHTTP(rec, httptest.NewRequest("GET", "/file?format=json&validateXSD=false&offset="+strconv.Itoa(offset), nil), call)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var chunk struct {
		Data string `json:"data_base64"`
		EOF  bool   `json:"eof"`
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &chunk); err != nil {
		t.Fatal(err)
	}
	got, err := base64.StdEncoding.DecodeString(chunk.Data)
	if err != nil || !bytes.Equal(got, expected[offset:]) || !chunk.EOF {
		t.Fatal("wrong transformed chunk", err)
	}
}

func TestServerSelectedXSDDefaults(t *testing.T) {
	ctx := context.Background()
	raw := []byte(localInsuredXML)
	q := url.Values{"format": {"json"}}
	schema := resultFileSchema("getInsured", nil)
	if _, err := processCallFile(ctx, raw, q, schema); err != nil {
		t.Fatal(err)
	}
	invalid := []byte("<invalid/>")
	if _, err := processCallFile(ctx, invalid, q, schema); err == nil {
		t.Fatal("default validation skipped")
	}
	q.Set("validateXSD", "false")
	if _, err := processCallFile(ctx, invalid, q, schema); err != nil {
		t.Fatal("explicit escape hatch failed", err)
	}
	if _, err := processCallFile(ctx, []byte("<broken>"), q, schema); err == nil {
		t.Fatal("malformed XML accepted")
	}
	q.Set("schema", "GetInsuredResponse.xsd")
	if _, err := processCallFile(ctx, raw, q, schema); err == nil {
		t.Fatal("client selected schema")
	}
	if resultFileSchema("getProviderInfo", map[string]string{"partnerCategory": "CLIN"}) != "PersonalizedFileClinic.xsd" {
		t.Fatal("wrong personalization schema")
	}
	if resultFileSchema("getProviderInfo", map[string]string{"partnerCategory": "PARA"}) != "PersonalizedFilePara.xsd" {
		t.Fatal("wrong personalization schema")
	}
	if _, err := processCallFile(ctx, raw, url.Values{"format": {"json"}}); err == nil {
		t.Fatal("missing server schema silently accepted")
	}
}
