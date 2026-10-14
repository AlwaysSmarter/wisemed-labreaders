package siui

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"github.com/makiuchi-d/gozxing"
	zx "github.com/makiuchi-d/gozxing/datamatrix"
	"image/png"
	"io"
	"math/big"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func exampleInvoice() invoiceInput {
	return invoiceInput{
		Header:   map[string]string{"serialCode": "TEST", "serialNumber": "1", "issueDate": "2026-10-12", "startDate": "2026-10-01", "endDate": "2026-10-12", "orgUnitCode": "CAS-DJ", "providerCode": "TEST-PARTNER", "providerCategory": "PARA", "contractNumber": "TEST-CONTRACT", "contractType": "PARA", "contractDate": "2026-01-01"},
		Provider: map[string]string{"fiscalCode": "00000000", "companyName": "FURNIZOR FICTIV — TEST & <xml>", "capital": "0", "signature": "REPREZENTANT TEST"},
		Insurer:  map[string]string{"fiscalCode": "00000000", "companyName": "CAS — EXEMPLU FICTIV"},
		Items:    []map[string]string{{"code": "TEST-COM-ITEM", "description": "Servicii — exemplu fictiv", "unitName": "serviciu", "unitPrice": "10.25", "quantity": "2", "vat": "0.00"}},
	}
}
func TestInvoicePreviewSchemaAndBarcode(t *testing.T) {
	m, rt := serviceTestModule(t, &operationBackend{t: t})
	_ = m
	data, _ := json.Marshal(exampleInvoice())
	r := httptest.NewRequest("POST", "/api/siui/invoice/preview", bytes.NewReader(data))
	r.Header.Set("Authorization", "test-local-session")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	rt.mux.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct {
		XML     string       `json:"invoice_xml"`
		Barcode string       `json:"barcode_xml"`
		PNG     string       `json:"barcode_png_base64"`
		ZIP     string       `json:"barcode_zip_base64"`
		Data    invoiceInput `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Header["totalAmmount"] != "20.50" {
		t.Fatal("wrong totals")
	}
	imageData, _ := base64.StdEncoding.DecodeString(result.PNG)
	img, err := png.Decode(bytes.NewReader(imageData))
	if err != nil {
		t.Fatal(err)
	}
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := zx.NewDataMatrixReader().Decode(bitmap, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The independent decoder preserves binary octets in its Go string.
	raw := []byte(decoded.GetText())
	packed, _ := base64.StdEncoding.DecodeString(result.ZIP)
	if !bytes.Equal(raw, packed) {
		t.Fatal("DataMatrix changed the ZIP bytes")
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 1 {
		t.Fatal("unexpected ZIP")
	}
	f, _ := zr.File[0].Open()
	xmlData, _ := io.ReadAll(f)
	f.Close()
	if string(xmlData) != result.Barcode {
		t.Fatal("barcode XML mismatch")
	}
	if dir := os.Getenv("SIUI_INVOICE_TEST_ARTIFACTS"); dir != "" {
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, "preview.json"), w.Body.Bytes(), 0600)
		os.WriteFile(filepath.Join(dir, "barcode.png"), imageData, 0600)
	}
}
func TestInvoiceTotalsAndCategories(t *testing.T) {
	for _, category := range partnerCategories {
		in := exampleInvoice()
		in.Header["providerCategory"] = category
		if err := checkInvoice(&in); err != nil {
			t.Fatal(category, err)
		}
		data, _ := renderInvoiceXML(in, false)
		if err := validateSchema(context.Background(), "ElectronicInvoice.xsd", data); err != nil {
			t.Fatal(err)
		}
	}
	in := exampleInvoice()
	in.Items[0]["unitPrice"] = "0.10"
	in.Items[0]["quantity"] = "3"
	if err := checkInvoice(&in); err != nil {
		t.Fatal(err)
	}
	if in.Header["totalAmmount"] != "0.30" {
		t.Fatal("decimal precision lost")
	}
	in = exampleInvoice()
	in.Header["totalAmmount"] = "99"
	if checkInvoice(&in) == nil {
		t.Fatal("inconsistent total accepted")
	}
	in = exampleInvoice()
	in.Header["isSigned"] = "true"
	if checkInvoice(&in) == nil {
		t.Fatal("response flag accepted")
	}
	in = exampleInvoice()
	in.Items[0]["vat"] = "NaN"
	if checkInvoice(&in) == nil {
		t.Fatal("non-decimal accepted")
	}
}
func TestInvoiceCMSWithOpenSSL(t *testing.T) {
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("OpenSSL verification tool unavailable")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "Synthetic invoice signer"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	original := []byte("<invoice>synthetic exact bytes</invoice>")
	signed, err := signInvoiceCMS(original, cert, key, [][]byte{der})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cms := filepath.Join(dir, "invoice.der")
	out := filepath.Join(dir, "verified.xml")
	os.WriteFile(cms, signed, 0600)
	os.WriteFile(filepath.Join(dir, "cert.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600)
	command := exec.Command(openssl, "cms", "-verify", "-binary", "-inform", "DER", "-in", cms, "-noverify", "-out", out)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	verified, _ := os.ReadFile(out)
	if !bytes.Equal(original, verified) {
		t.Fatal("CMS content mismatch")
	}
	signed[len(signed)-1] ^= 1
	os.WriteFile(cms, signed, 0600)
	if err := exec.Command(openssl, "cms", "-verify", "-binary", "-inform", "DER", "-in", cms, "-noverify", "-out", out).Run(); err == nil {
		t.Fatal("tampered CMS signature accepted")
	}
}
func TestInvoicePackageRequiresAdmin(t *testing.T) {
	m, rt := serviceTestModule(t, &operationBackend{t: t})
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/siui/invoice/package", strings.NewReader(`{}`))
	m.invoiceHTTP(w, r, "invoice/package", false)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	_ = rt
}

func TestInvoiceLargeValuesRejected(t *testing.T) {
	in := exampleInvoice()
	in.Items[0]["unitPrice"] = "999999999999999"
	in.Items[0]["quantity"] = "999999999999999"
	in.Items[0]["ammount"] = "1"
	if err := checkInvoice(&in); err == nil {
		t.Fatal("overflow accepted")
	}
	in = exampleInvoice()
	in.Items[0]["vat"] = "999999999999999.999999"
	if err := checkInvoice(&in); err == nil {
		t.Fatal("sub-cent overflow accepted")
	}
}
