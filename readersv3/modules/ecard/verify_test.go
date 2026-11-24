package ecard

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"
)

func TestVerifyCMSBindsExactTextAndExpectedSigner(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test only"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, e := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	cert, e := x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	for _, detached := range []bool{false, true} {
		sd, e := pkcs7.NewSignedData([]byte("text exact\n"))
		if e != nil {
			t.Fatal(e)
		}
		sd.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
		if e = sd.AddSigner(cert, key, pkcs7.SignerInfoConfig{}); e != nil {
			t.Fatal(e)
		}
		if detached {
			sd.Detach()
		}
		sig, e := sd.Finish()
		if e != nil {
			t.Fatal(e)
		}
		in := verifyRequest{Text: "text exact\n", Signature: base64.StdEncoding.EncodeToString(sig), Certificate: base64.StdEncoding.EncodeToString(der), Detached: detached}
		result, e := verifySignature(in)
		if e != nil || result["valid"] != true {
			t.Fatalf("valid detached=%v: %v %v", detached, result, e)
		}
		in.Text = "text exact"
		result, e = verifySignature(in)
		if e != nil || result["valid"] != false {
			t.Fatalf("changed content accepted: %v %v", result, e)
		}
		in.Text = "text exact\n"
		in.Certificate = ""
		if _, e = verifySignature(in); e == nil {
			t.Fatal("accepted no expected signer")
		}
		pub, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
		in.PublicKey = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub}))
		result, e = verifySignature(in)
		if e != nil || result["valid"] != true {
			t.Fatalf("public key: %v %v", result, e)
		}
		other, e := rsa.GenerateKey(rand.Reader, 2048)
		if e != nil {
			t.Fatal(e)
		}
		pub, _ = x509.MarshalPKIXPublicKey(&other.PublicKey)
		in.PublicKey = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub}))
		if _, e = verifySignature(in); e == nil {
			t.Fatal("accepted wrong public key")
		}
		in.PublicKey = ""
		in.Certificate = base64.StdEncoding.EncodeToString(der)
		sig[len(sig)-1] ^= 1
		in.Signature = base64.StdEncoding.EncodeToString(sig)
		result, e = verifySignature(in)
		if e == nil && result["valid"] == true {
			t.Fatal("accepted modified signature")
		}
	}
}

func TestVerificationEndpointAuthenticationAndMalformedInput(t *testing.T) {
	m := testModule(t)
	for _, user := range []string{"", "operator"} {
		m.guard = guard{user: user}
		for _, path := range []string{"sign/verify", "ui"} {
			method := "GET"
			if path == "sign/verify" {
				method = "POST"
			}
			r := httptest.NewRequest(method, "/api/ecard/"+path, strings.NewReader(`{"signature_base64":"invalid"}`))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			m.handle(w, r)
			expected := 200
			if user == "" {
				expected = 401
			} else if method == "POST" {
				expected = 400
			}
			if w.Code != expected {
				t.Fatalf("%s %s: %d, want %d", user, path, w.Code, expected)
			}
		}
	}
}

func TestExpiredCertificateSeparatesMathFromDates(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(42), Subject: pkix.Name{CommonName: "expired test"}, NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(-24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, e := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	cert, e := x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	sd, e := pkcs7.NewSignedData([]byte("exact"))
	if e != nil {
		t.Fatal(e)
	}
	sd.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
	if e = sd.AddSigner(cert, key, pkcs7.SignerInfoConfig{}); e != nil {
		t.Fatal(e)
	}
	sig, e := sd.Finish()
	if e != nil {
		t.Fatal(e)
	}
	in := verifyRequest{Text: "exact", Signature: base64.StdEncoding.EncodeToString(sig), Certificate: base64.StdEncoding.EncodeToString(der)}
	out, e := verifySignature(in)
	if e != nil {
		t.Fatal(e)
	}
	if out["valid"] != false || out["cryptographic_valid"] != true || out["certificate_valid_at_signing_time"] != false || out["reason"] != "certificate_outside_validity_period" {
		t.Fatal(out)
	}
	sig[len(sig)-1] ^= 1
	in.Signature = base64.StdEncoding.EncodeToString(sig)
	out, e = verifySignature(in)
	if e == nil && out["cryptographic_valid"] == true {
		t.Fatal("invalid signature accepted for expired certificate")
	}
}
