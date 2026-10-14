package siui

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"math/big"
)

var cmsDataOID = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
var cmsSignedOID = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
var cmsSHA256OID = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}

type cmsSignerInfo struct {
	Version         int
	IssuerAndSerial struct {
		Issuer asn1.RawValue
		Serial *big.Int
	}
	DigestAlgorithm    pkix.AlgorithmIdentifier
	SignatureAlgorithm pkix.AlgorithmIdentifier
	Signature          []byte
}
type cmsSignedData struct {
	Version          int
	DigestAlgorithms []pkix.AlgorithmIdentifier `asn1:"set"`
	Content          struct {
		Type asn1.ObjectIdentifier
		Data []byte `asn1:"explicit,tag:0"`
	}
	Certificates asn1.RawValue
	Signers      []cmsSignerInfo `asn1:"set"`
}

// Attached CMS SignedData, RFC3852: without signedAttributes the signature covers
// the content octets directly. Nothing sets the response-only isSigned XML flag.
func signInvoiceCMS(data []byte, cert *x509.Certificate, key crypto.Signer, chain [][]byte) ([]byte, error) {
	hash := sha256.Sum256(data)
	algorithm := pkix.AlgorithmIdentifier{}
	switch key.Public().(type) {
	case *rsa.PublicKey:
		algorithm.Algorithm = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
		algorithm.Parameters = asn1.NullRawValue
	case *ecdsa.PublicKey:
		algorithm.Algorithm = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
	default:
		return nil, errors.New("unsupported CMS token key")
	}
	signature, err := key.Sign(rand.Reader, hash[:], crypto.SHA256)
	if err != nil {
		return nil, err
	}
	digest := pkix.AlgorithmIdentifier{Algorithm: cmsSHA256OID, Parameters: asn1.NullRawValue}
	info := cmsSignerInfo{Version: 1, DigestAlgorithm: digest, SignatureAlgorithm: algorithm, Signature: signature}
	info.IssuerAndSerial.Issuer = asn1.RawValue{FullBytes: cert.RawIssuer}
	info.IssuerAndSerial.Serial = cert.SerialNumber
	signed := cmsSignedData{Version: 1, DigestAlgorithms: []pkix.AlgorithmIdentifier{digest}, Signers: []cmsSignerInfo{info}}
	signed.Content.Type = cmsDataOID
	signed.Content.Data = data
	certificates := []byte{}
	for _, der := range chain {
		certificates = append(certificates, der...)
	}
	signed.Certificates = asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: certificates}
	inner, err := asn1.Marshal(signed)
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(struct {
		Type    asn1.ObjectIdentifier
		Content asn1.RawValue
	}{cmsSignedOID, asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: inner}})
}
