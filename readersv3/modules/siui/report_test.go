package siui

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type reportSigningBackend struct {
	*operationBackend
	cert     *x509.Certificate
	key      *rsa.PrivateKey
	failSign bool
}

func (b *reportSigningBackend) SignInvoice(_ context.Context, data []byte) ([]byte, error) {
	if b.failSign {
		return nil, errors.New("token signing failed")
	}
	return signInvoiceCMS(data, b.cert, b.key, nil)
}

func TestReportPackageRequiresAdmin(t *testing.T) {
	m := &Module{}
	w := httptest.NewRecorder()
	m.reportPackageHTTP(w, httptest.NewRequest("POST", "/api/siui/report/package", strings.NewReader(`{}`)), false)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}

func TestReportArchiveRejectsInvalidEntries(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"../PARA_123_20261105_1200.xml", []byte("signed")},
		{"PARA_123_20261105_1200.xml", nil},
		{"PARA_123_20261105_1200.xml", bytes.Repeat([]byte("a"), maxReportArchive+1)},
	} {
		packed, err := invoiceZIP(tc.name, tc.data)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateReportArchive(base64.StdEncoding.EncodeToString(packed)); err == nil {
			t.Fatal("invalid archive accepted", tc.name, len(tc.data))
		}
	}
}

func TestReportPackageAndSend(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic report signer"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	b := &reportSigningBackend{operationBackend: &operationBackend{t: t, fail: true}, cert: cert, key: key}
	_, rt := serviceTestModule(t, b)
	call := func(path string, data any) *httptest.ResponseRecorder {
		payload, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", path, bytes.NewReader(payload))
		r.Header.Set("Authorization", "test-local-session")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		rt.mux.ServeHTTP(w, r)
		return w
	}
	filename := "PARA_123_20261105_1200.xml"
	xml := "<report>" + strings.Repeat("a", maxReportXML-len("<report></report>")) + "</report>"
	w := call("/api/siui/report/package", map[string]string{"filename": filename, "report_xml": xml})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct {
		ReportXML string `json:"reportXML"`
		Signed    bool   `json:"signed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Signed {
		t.Fatal("unsigned package")
	}
	packed, err := base64.StdEncoding.DecodeString(result.ReportXML)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(packed), int64(len(packed)))
	if err != nil || len(z.File) != 1 || z.File[0].Name != filename {
		t.Fatal("wrong report archive", err)
	}
	r, err := z.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatal(err)
	}
	var outer struct {
		OID     asn1.ObjectIdentifier
		Content asn1.RawValue
	}
	if _, err := asn1.Unmarshal(signed, &outer); err != nil || !outer.OID.Equal(cmsSignedOID) {
		t.Fatal("missing CMS SignedData", err)
	}
	var cms cmsSignedData
	if _, err := asn1.Unmarshal(outer.Content.Bytes, &cms); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cms.Content.Data, []byte(xml)) || len(cms.Signers) != 1 {
		t.Fatal("attached CMS changed the XML bytes")
	}
	digest := sha256.Sum256(cms.Content.Data)
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], cms.Signers[0].Signature); err != nil {
		t.Fatal("CMS signature invalid", err)
	}
	w = call("/api/siui/general/sendReport", map[string]any{"operation_id": "large-report", "parameters": map[string]string{"reportType": "PARA", "reportXML": result.ReportXML}})
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, tc := range []struct{ name, xml string }{{"oversize", xml + " "}, {"DTD", "<!DOCTYPE report><report/>"}, {"empty", ""}} {
		t.Run(tc.name, func(t *testing.T) {
			w := call("/api/siui/report/package", map[string]string{"filename": filename, "report_xml": tc.xml})
			if w.Code != 400 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	w = call("/api/siui/report/package", map[string]string{"filename": "../report.xml", "report_xml": "<report/>"})
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	b.failSign = true
	w = call("/api/siui/report/package", map[string]string{"filename": filename, "report_xml": "<report/>"})
	if w.Code != 502 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, bad := range []string{"<report/>", base64.StdEncoding.EncodeToString([]byte("not zip"))} {
		w = call("/api/siui/general/sendReport", map[string]any{"operation_id": "invalid-report", "parameters": map[string]string{"reportType": "PARA", "reportXML": bad}})
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
