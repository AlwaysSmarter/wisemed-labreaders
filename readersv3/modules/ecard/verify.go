package ecard

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/digitorus/pkcs7"
)

type verifyRequest struct {
	Text        string `json:"text"`
	Encoding    string `json:"encoding"`
	Signature   string `json:"signature_base64"`
	Certificate string `json:"certificate_base64"`
	PublicKey   string `json:"public_key_pem"`
	Detached    bool   `json:"detached"`
}

func verifySignature(in verifyRequest) (map[string]interface{}, error) {
	if len(in.Text) > 65536 || len(in.Signature) > 1<<20 || len(in.Certificate) > 65536 || len(in.PublicKey) > 65536 {
		return nil, errors.New("verification input too large")
	}
	if in.Encoding == "" {
		in.Encoding = "ascii"
	}
	if in.Encoding != "ascii" && in.Encoding != "utf-8" {
		return nil, errors.New("encoding must be ascii or utf-8")
	}
	if in.Encoding == "ascii" {
		for _, c := range in.Text {
			if c > 127 {
				return nil, errors.New("non-ASCII text; specify utf-8")
			}
		}
	}
	raw, e := base64.StdEncoding.DecodeString(strings.TrimSpace(in.Signature))
	if e != nil {
		return nil, errors.New("invalid signature Base64")
	}
	cms, e := pkcs7.Parse(raw)
	if e != nil {
		return nil, errors.New("invalid CMS/PKCS7 signature")
	}
	if len(cms.Signers) != 1 {
		return nil, errors.New("exactly one CMS signer required")
	}
	var cert *x509.Certificate
	if in.Certificate != "" {
		der, e := base64.StdEncoding.DecodeString(strings.TrimSpace(in.Certificate))
		if e != nil {
			return nil, errors.New("invalid certificate Base64")
		}
		cert, e = x509.ParseCertificate(der)
		if e != nil {
			return nil, errors.New("invalid X.509 certificate")
		}
		cms.Certificates = []*x509.Certificate{cert}
	}
	signer := cms.GetOnlySigner()
	if signer == nil {
		return nil, errors.New("signer certificate unavailable or does not match expected certificate")
	}
	if cert == nil && in.PublicKey == "" {
		return nil, errors.New("expected certificate_base64 or public_key_pem required")
	}
	if in.PublicKey != "" {
		block, _ := pem.Decode([]byte(in.PublicKey))
		if block == nil {
			return nil, errors.New("invalid public key PEM")
		}
		key, e := x509.ParsePKIXPublicKey(block.Bytes)
		if e != nil {
			key, e = x509.ParsePKCS1PublicKey(block.Bytes)
		}
		if e != nil {
			return nil, errors.New("invalid public key")
		}
		expected, e := x509.MarshalPKIXPublicKey(key)
		if e != nil {
			return nil, e
		}
		actual, _ := x509.MarshalPKIXPublicKey(signer.PublicKey)
		if !bytes.Equal(expected, actual) {
			return nil, errors.New("signer public key mismatch")
		}
	}
	out := map[string]interface{}{"valid": false, "certificate_chain_checked": false, "revocation_checked": false, "cnas_acceptance_checked": false}
	if in.Detached {
		if len(cms.Content) != 0 {
			return nil, errors.New("detached requested but CMS contains content")
		}
		cms.Content = []byte(in.Text)
	} else if !bytes.Equal(cms.Content, []byte(in.Text)) {
		out["reason"] = "content_mismatch"
		return out, nil
	}
	// Check signature mathematics separately from certificate dates. A copy of
	// the parsed certificate widens only the library's implicit date gate; raw
	// certificate bytes, issuer/serial, public key and signed attributes stay intact.
	out["cryptographic_valid"] = false
	out["certificate_not_before"] = signer.NotBefore.UTC().Format(time.RFC3339)
	out["certificate_not_after"] = signer.NotAfter.UTC().Format(time.RFC3339)
	now := time.Now()
	out["certificate_valid_now"] = !now.Before(signer.NotBefore) && !now.After(signer.NotAfter)
	var signingTime time.Time
	hasTime := cms.UnmarshalSignedAttribute(pkcs7.OIDAttributeSigningTime, &signingTime) == nil
	dateValid := out["certificate_valid_now"].(bool)
	out["signing_time_trusted"] = false
	if hasTime {
		out["signing_time"] = signingTime.UTC().Format(time.RFC3339)
		dateValid = !signingTime.Before(signer.NotBefore) && !signingTime.After(signer.NotAfter)
		out["certificate_valid_at_signing_time"] = dateValid
	}
	cryptoCert := *signer
	cryptoCert.NotBefore = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	cryptoCert.NotAfter = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	cms.Certificates = []*x509.Certificate{&cryptoCert}
	if e = cms.Verify(); e != nil {
		out["reason"] = "signature_verification_failed"
		return out, nil
	}
	digest := sha256.Sum256(signer.Raw)
	out["cryptographic_valid"] = true
	out["valid"] = dateValid
	if !dateValid {
		out["reason"] = "certificate_outside_validity_period"
	}
	if !out["certificate_valid_now"].(bool) {
		out["certificate_time_warning"] = "certificate_outside_validity_period_now"
	}
	out["signer_certificate_sha256"] = hex.EncodeToString(digest[:])
	return out, nil
}
func (m *Module) verify(w http.ResponseWriter, r *http.Request) {
	var in verifyRequest
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, e)
		return
	}
	result, e := verifySignature(in)
	if e != nil {
		fail(w, 400, e)
		return
	}
	respond(w, 200, result)
}
