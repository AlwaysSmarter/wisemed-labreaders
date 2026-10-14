//go:build windows

package siui

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"math/big"
	"runtime"
	"unsafe"
)

var ncrypt = windows.NewLazySystemDLL("ncrypt.dll")
var nSet = ncrypt.NewProc("NCryptSetProperty")
var nSign = ncrypt.NewProc("NCryptSignHash")
var nFree = ncrypt.NewProc("NCryptFreeObject")
var capi = windows.NewLazySystemDLL("advapi32.dll")
var cSet = capi.NewProc("CryptSetProvParam")
var cRelease = capi.NewProc("CryptReleaseContext")
var cHash = capi.NewProc("CryptCreateHash")
var cHashSet = capi.NewProc("CryptSetHashParam")
var cHashSign = capi.NewProc("CryptSignHashW")
var cHashFree = capi.NewProc("CryptDestroyHash")

type windowsPINSigner struct {
	cfg    settings
	cert   *windows.CertContext
	public crypto.PublicKey
}

func (s *windowsPINSigner) Public() crypto.PublicKey { return s.public }
func (s *windowsPINSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) (sig []byte, err error) {
	pin, finish, err := s.cfg.pin.begin(s.cfg)
	if err != nil {
		return nil, err
	}
	if len(pin) == 0 {
		return nil, errors.New("PIN automat suspendat; reluați operația cu introducere manuală")
	}
	defer func() {
		if err == nil {
			err = verifyTokenSignature(s.public, digest, sig, opts)
		}
		finish(err == nil)
	}()
	var key windows.Handle
	var spec uint32
	var free bool
	err = windows.CryptAcquireCertificatePrivateKey(s.cert, windows.CRYPT_ACQUIRE_ALLOW_NCRYPT_KEY_FLAG|windows.CRYPT_ACQUIRE_SILENT_FLAG, nil, &key, &spec, &free)
	if err != nil {
		return nil, errors.New("Token key unavailable for automatic PIN; use manual authentication")
	}
	if free {
		defer func() {
			if spec == windows.CERT_NCRYPT_KEY_SPEC {
				nFree.Call(uintptr(key))
			} else {
				cRelease.Call(uintptr(key), 0)
			}
		}()
	}
	if !free {
		return nil, errors.New("Driver returned a shared key; automatic PIN disabled to avoid retaining credentials")
	}
	if spec == windows.CERT_NCRYPT_KEY_SPEC {
		p, e := windows.UTF16FromString(string(pin))
		if e != nil {
			return nil, e
		}
		defer clear(p)
		result, _, _ := nSet.Call(uintptr(key), uintptr(unsafe.Pointer(wide("SmartCardPin"))), uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)*2), 0x40)
		runtime.KeepAlive(p)
		if result != 0 {
			return nil, fmt.Errorf("Token PIN configuration failed (0x%x); use manual authentication", result)
		}
		name := ""
		switch opts.HashFunc() {
		case crypto.SHA256:
			name = "SHA256"
		case crypto.SHA384:
			name = "SHA384"
		case crypto.SHA512:
			name = "SHA512"
		default:
			return nil, errors.New("unsupported digest")
		}
		padding := struct {
			Algorithm *uint16
			Salt      uint32
		}{Algorithm: wide(name)}
		var flags uintptr
		var info uintptr
		if _, ok := s.public.(*rsa.PublicKey); ok {
			flags = 2
			info = uintptr(unsafe.Pointer(&padding))
			if pss, ok := opts.(*rsa.PSSOptions); ok {
				flags = 8
				padding.Salt = uint32(opts.HashFunc().Size())
				if pss.SaltLength > 0 {
					padding.Salt = uint32(pss.SaltLength)
				}
			}
		}
		var size uint32
		result, _, _ = nSign.Call(uintptr(key), info, uintptr(unsafe.Pointer(&digest[0])), uintptr(len(digest)), 0, 0, uintptr(unsafe.Pointer(&size)), flags|0x40)
		if result != 0 || size == 0 || size > 16384 {
			return nil, errors.New("Token signing initialization failed; automatic PIN suspended")
		}
		out := make([]byte, size)
		result, _, _ = nSign.Call(uintptr(key), info, uintptr(unsafe.Pointer(&digest[0])), uintptr(len(digest)), uintptr(unsafe.Pointer(&out[0])), uintptr(size), uintptr(unsafe.Pointer(&size)), flags|0x40)
		runtime.KeepAlive(padding)
		runtime.KeepAlive(digest)
		if result != 0 {
			return nil, fmt.Errorf("Token signing failed (0x%x); automatic PIN suspended, use manual authentication", result)
		}
		out = out[:size]
		if _, ok := s.public.(*ecdsa.PublicKey); ok {
			if len(out)%2 != 0 {
				return nil, errors.New("invalid EC signature")
			}
			return asn1.Marshal(struct{ R, S *big.Int }{new(big.Int).SetBytes(out[:len(out)/2]), new(big.Int).SetBytes(out[len(out)/2:])})
		}
		return out, nil
	}
	if _, ok := s.public.(*rsa.PublicKey); !ok {
		return nil, errors.New("CSP automatic PIN currently requires RSA")
	}
	if _, ok := opts.(*rsa.PSSOptions); ok {
		return nil, errors.New("CSP does not support RSA-PSS")
	}
	p := append(append([]byte(nil), pin...), 0)
	defer clear(p)
	param := uintptr(33)
	if spec == windows.AT_KEYEXCHANGE {
		param = 32
	}
	r, _, _ := cSet.Call(uintptr(key), param, uintptr(unsafe.Pointer(&p[0])), 0)
	runtime.KeepAlive(p)
	if r == 0 {
		return nil, errors.New("CSP PIN configuration failed; use manual authentication")
	}
	var alg uintptr
	switch opts.HashFunc() {
	case crypto.SHA256:
		alg = 0x800c
	case crypto.SHA384:
		alg = 0x800d
	case crypto.SHA512:
		alg = 0x800e
	default:
		return nil, errors.New("unsupported CSP digest")
	}
	var hash uintptr
	r, _, _ = cHash.Call(uintptr(key), alg, 0, 0, uintptr(unsafe.Pointer(&hash)))
	if r == 0 {
		return nil, errors.New("CSP hash unavailable")
	}
	defer cHashFree.Call(hash)
	r, _, _ = cHashSet.Call(hash, 2, uintptr(unsafe.Pointer(&digest[0])), 0)
	runtime.KeepAlive(digest)
	if r == 0 {
		return nil, errors.New("CSP hash setup failed")
	}
	var size uint32
	r, _, _ = cHashSign.Call(hash, uintptr(spec), 0, 0, 0, uintptr(unsafe.Pointer(&size)))
	if r == 0 || size == 0 || size > 16384 {
		return nil, errors.New("CSP signature initialization failed")
	}
	out := make([]byte, size)
	r, _, _ = cHashSign.Call(hash, uintptr(spec), 0, 0, uintptr(unsafe.Pointer(&out[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return nil, errors.New("Token PIN/signature rejected; automatic PIN suspended, use manual authentication")
	}
	out = out[:size]
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}
func (n nativeBackend) pinCertificate() (tls.Certificate, func(), bool, error) {
	empty := tls.Certificate{}
	noop := func() {}
	if !n.cfg.pin.available(n.cfg) {
		return empty, noop, false, nil
	}
	c, e := n.certificate()
	if e != nil {
		return empty, noop, true, e
	}
	release := func() { windows.CertFreeCertificateContext(c) }
	der := append([]byte(nil), unsafe.Slice(c.EncodedCert, c.Length)...)
	leaf, e := x509.ParseCertificate(der)
	if e != nil {
		release()
		return empty, noop, true, e
	}
	schemes := []tls.SignatureScheme{tls.PKCS1WithSHA256, tls.PKCS1WithSHA384, tls.PKCS1WithSHA512, tls.ECDSAWithP256AndSHA256, tls.ECDSAWithP384AndSHA384, tls.ECDSAWithP521AndSHA512}
	return tls.Certificate{Certificate: [][]byte{der}, Leaf: leaf, PrivateKey: &windowsPINSigner{cfg: n.cfg, cert: c, public: leaf.PublicKey}, SupportedSignatureAlgorithms: schemes}, release, true, nil
}
func (n nativeBackend) signWithPIN(ctx context.Context, data []byte) ([]byte, bool, error) {
	cert, release, active, err := n.pinCertificate()
	if !active || err != nil {
		return nil, active, err
	}
	defer release()
	if err = ctx.Err(); err != nil {
		return nil, true, err
	}
	signed, err := signInvoiceCMS(data, cert.Leaf, cert.PrivateKey.(crypto.Signer), cert.Certificate)
	return signed, true, err
}
