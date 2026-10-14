//go:build darwin

package siui

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// kSecKeyOperationTypeSign from Security/SecKey.h. Verify is 1 and is not
// supported by some hardware-backed private keys.
const secKeyOperationSign int64 = 0

// Security.framework accesses CryptoTokenKit identities without exporting private
// keys. Optional PIN credentials are scoped to one native signing attempt.
type securityAPI struct {
	cf, sec             uintptr
	release             func(uintptr)
	retain              func(uintptr) uintptr
	dictionaryCreate    func(uintptr, int64, uintptr, uintptr) uintptr
	dictionarySet       func(uintptr, uintptr, uintptr)
	arrayCreate         func(uintptr, int64, uintptr) uintptr
	arrayAppend         func(uintptr, uintptr)
	arrayCount          func(uintptr) int64
	arrayValue          func(uintptr, int64) uintptr
	dataCreate          func(uintptr, *byte, int64) uintptr
	dataLength          func(uintptr) int64
	dataPointer         func(uintptr) *byte
	readPointer         func(*uintptr, uintptr, uintptr) uintptr
	stringCreate        func(uintptr, string, uint32) uintptr
	itemCopy            func(uintptr, *uintptr) int32
	identityCertificate func(uintptr, *uintptr) int32
	identityKey         func(uintptr, *uintptr) int32
	certificateData     func(uintptr) uintptr
	certificateCreate   func(uintptr, uintptr) uintptr
	policySSL           func(bool, uintptr) uintptr
	policyBasic         func() uintptr
	trustCreate         func(uintptr, uintptr, *uintptr) int32
	copyAnchors         func(*uintptr) int32
	trustOptions        func(uintptr, uint32) int32
	trustEvaluate       func(uintptr, *uintptr) bool
	trustChain          func(uintptr) uintptr
	sign                func(uintptr, uintptr, uintptr, *uintptr) uintptr
	supports            func(uintptr, int64, uintptr) bool
	errorCode           func(uintptr) int64
}

var macSecurity struct {
	once sync.Once
	api  *securityAPI
	err  error
}

func securityFramework() (*securityAPI, error) {
	macSecurity.once.Do(func() {
		a := &securityAPI{}
		a.cf, macSecurity.err = purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if macSecurity.err != nil {
			return
		}
		a.sec, macSecurity.err = purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if macSecurity.err != nil {
			return
		}
		for _, p := range []struct {
			dst  any
			lib  uintptr
			name string
		}{
			{&a.readPointer, purego.RTLD_DEFAULT, "memcpy"}, {&a.release, a.cf, "CFRelease"}, {&a.retain, a.cf, "CFRetain"}, {&a.dictionaryCreate, a.cf, "CFDictionaryCreateMutable"}, {&a.dictionarySet, a.cf, "CFDictionarySetValue"},
			{&a.arrayCreate, a.cf, "CFArrayCreateMutable"}, {&a.arrayAppend, a.cf, "CFArrayAppendValue"}, {&a.arrayCount, a.cf, "CFArrayGetCount"}, {&a.arrayValue, a.cf, "CFArrayGetValueAtIndex"},
			{&a.dataCreate, a.cf, "CFDataCreate"}, {&a.dataLength, a.cf, "CFDataGetLength"}, {&a.dataPointer, a.cf, "CFDataGetBytePtr"}, {&a.stringCreate, a.cf, "CFStringCreateWithCString"}, {&a.errorCode, a.cf, "CFErrorGetCode"},
			{&a.itemCopy, a.sec, "SecItemCopyMatching"}, {&a.identityCertificate, a.sec, "SecIdentityCopyCertificate"}, {&a.identityKey, a.sec, "SecIdentityCopyPrivateKey"},
			{&a.certificateData, a.sec, "SecCertificateCopyData"}, {&a.certificateCreate, a.sec, "SecCertificateCreateWithData"}, {&a.policySSL, a.sec, "SecPolicyCreateSSL"}, {&a.policyBasic, a.sec, "SecPolicyCreateBasicX509"},
			{&a.copyAnchors, a.sec, "SecTrustCopyAnchorCertificates"}, {&a.trustCreate, a.sec, "SecTrustCreateWithCertificates"}, {&a.trustOptions, a.sec, "SecTrustSetOptions"}, {&a.trustEvaluate, a.sec, "SecTrustEvaluateWithError"}, {&a.trustChain, a.sec, "SecTrustCopyCertificateChain"},
			{&a.sign, a.sec, "SecKeyCreateSignature"}, {&a.supports, a.sec, "SecKeyIsAlgorithmSupported"},
		} {
			ptr, err := purego.Dlsym(p.lib, p.name)
			if err != nil {
				macSecurity.err = err
				return
			}
			purego.RegisterFunc(p.dst, ptr)
		}
		macSecurity.api = a
	})
	return macSecurity.api, macSecurity.err
}
func (a *securityAPI) symbol(lib uintptr, name string) uintptr {
	v, e := purego.Dlsym(lib, name)
	if e != nil {
		panic("missing macOS Security symbol: " + name)
	}
	return v
}
func (a *securityAPI) constant(name string) uintptr {
	lib := a.sec
	if len(name) > 3 && name[:3] == "kCF" {
		lib = a.cf
	}
	p := a.symbol(lib, name)
	var value uintptr
	a.readPointer(&value, p, unsafe.Sizeof(value))
	return value
}
func (a *securityAPI) data(b []byte) uintptr {
	if len(b) == 0 {
		return a.dataCreate(0, nil, 0)
	}
	v := a.dataCreate(0, &b[0], int64(len(b)))
	runtime.KeepAlive(b)
	return v
}
func (a *securityAPI) bytes(data uintptr) []byte {
	if data == 0 {
		return nil
	}
	n := a.dataLength(data)
	if n < 0 || n > maxData {
		return nil
	}
	return append([]byte(nil), unsafe.Slice(a.dataPointer(data), int(n))...)
}
func (a *securityAPI) array() uintptr {
	return a.arrayCreate(0, 0, a.symbol(a.cf, "kCFTypeArrayCallBacks"))
}
func (a *securityAPI) identities(store string) (uintptr, error) {
	return a.identitiesWithContext(store, 0)
}
func (a *securityAPI) identitiesWithContext(store string, auth uintptr) (uintptr, error) {
	if store != "CurrentUser" {
		return 0, errors.New("macOS: select CurrentUser (Keychain / CryptoTokenKit), not the Windows LocalMachine store")
	}
	query := a.dictionaryCreate(0, 0, a.symbol(a.cf, "kCFTypeDictionaryKeyCallBacks"), a.symbol(a.cf, "kCFTypeDictionaryValueCallBacks"))
	defer a.release(query)
	if auth != 0 {
		a.dictionarySet(query, a.constant("kSecUseAuthenticationContext"), auth)
	}
	a.dictionarySet(query, a.constant("kSecClass"), a.constant("kSecClassIdentity"))
	a.dictionarySet(query, a.constant("kSecReturnRef"), a.constant("kCFBooleanTrue"))
	a.dictionarySet(query, a.constant("kSecMatchLimit"), a.constant("kSecMatchLimitAll"))
	var result uintptr
	status := a.itemCopy(query, &result)
	if status == -25300 {
		return 0, nil
	}
	if status != 0 {
		return 0, fmt.Errorf("macOS certificate enumeration failed (OSStatus %d)", status)
	}
	return result, nil
}
func (a *securityAPI) identityDER(identity uintptr) ([]byte, error) {
	var cert uintptr
	status := a.identityCertificate(identity, &cert)
	if status != 0 {
		return nil, fmt.Errorf("macOS certificate unavailable (OSStatus %d)", status)
	}
	defer a.release(cert)
	data := a.certificateData(cert)
	if data == 0 {
		return nil, errors.New("macOS certificate data unavailable")
	}
	defer a.release(data)
	return a.bytes(data), nil
}

type macSigner struct {
	pinFinish func(bool)
	api       *securityAPI
	key       uintptr
	public    crypto.PublicKey
}

func (s *macSigner) Public() crypto.PublicKey { return s.public }
func (s *macSigner) algorithm(opts crypto.SignerOpts) (string, error) {
	hash := ""
	switch opts.HashFunc() {
	case crypto.SHA256:
		hash = "SHA256"
	case crypto.SHA384:
		hash = "SHA384"
	case crypto.SHA512:
		hash = "SHA512"
	default:
		return "", errors.New("unsupported token signature hash")
	}
	switch s.public.(type) {
	case *rsa.PublicKey:
		if pss, ok := opts.(*rsa.PSSOptions); ok {
			if pss.SaltLength != rsa.PSSSaltLengthEqualsHash && pss.SaltLength != opts.HashFunc().Size() {
				return "", errors.New("unsupported RSA-PSS salt length")
			}
			return "kSecKeyAlgorithmRSASignatureDigestPSS" + hash, nil
		}
		return "kSecKeyAlgorithmRSASignatureDigestPKCS1v15" + hash, nil
	case *ecdsa.PublicKey:
		return "kSecKeyAlgorithmECDSASignatureDigestX962" + hash, nil
	}
	return "", errors.New("unsupported token key type")
}
func (s *macSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	if s.pinFinish != nil {
		defer func() {
			if err == nil {
				err = verifyTokenSignature(s.public, digest, signature, opts)
			}
			s.pinFinish(err == nil)
		}()
	}
	algorithm, err := s.algorithm(opts)
	if err != nil {
		return nil, err
	}
	if len(digest) != opts.HashFunc().Size() {
		return nil, errors.New("invalid signing digest length")
	}
	alg := s.api.constant(algorithm)
	if !s.api.supports(s.key, secKeyOperationSign, alg) {
		return nil, errors.New("token driver does not support the requested TLS signature")
	}
	data := s.api.data(digest)
	defer s.api.release(data)
	var failure uintptr
	result := s.api.sign(s.key, alg, data, &failure)
	if failure != 0 {
		defer s.api.release(failure)
	}
	if result == 0 {
		code := int64(0)
		if failure != 0 {
			code = s.api.errorCode(failure)
		}
		if s.pinFinish != nil {
			return nil, fmt.Errorf("macOS token signing failed (code %d); PIN automat suspendat. Reluați operația pentru introducere manuală sau actualizați PIN-ul în Setări CNAS", code)
		}
		return nil, fmt.Errorf("macOS token signing failed (code %d); check the PIN/authorization dialog in your desktop session", code)
	}
	defer s.api.release(result)
	return s.api.bytes(result), nil
}
func (a *securityAPI) verifyServer(rawCerts [][]byte, hostname string, allowInvalidDate, allowRevoked bool) error {
	if len(rawCerts) == 0 {
		return errors.New("missing CNAS server certificate")
	}
	certs := a.array()
	defer a.release(certs)
	for _, der := range rawCerts {
		data := a.data(der)
		cert := a.certificateCreate(0, data)
		a.release(data)
		if cert == 0 {
			return errors.New("invalid server certificate")
		}
		a.arrayAppend(certs, cert)
		a.release(cert)
	}
	host := a.stringCreate(0, hostname, 0x08000100)
	defer a.release(host)
	policy := a.policySSL(true, host)
	defer a.release(policy)
	var trust uintptr
	if status := a.trustCreate(certs, policy, &trust); status != 0 {
		return fmt.Errorf("macOS TLS trust creation failed (%d)", status)
	}
	defer a.release(trust)
	if allowInvalidDate {
		if status := a.trustOptions(trust, 0x1|0x8); status != 0 {
			return fmt.Errorf("macOS certificate date exception unavailable (%d)", status)
		}
	}
	var failure uintptr
	valid := a.trustEvaluate(trust, &failure)
	if failure != 0 {
		defer a.release(failure)
	}
	if !valid {
		code := int64(0)
		if failure != 0 {
			code = a.errorCode(failure)
		}
		if code == -67820 && allowRevoked {
			return a.verifyWithoutRevocation(trust, rawCerts, hostname, allowInvalidDate)
		}
		return macTrustError(code, allowInvalidDate)
	}
	return nil
}
func (a *securityAPI) clientChain(leaf []byte) [][]byte {
	chain := [][]byte{leaf}
	data := a.data(leaf)
	cert := a.certificateCreate(0, data)
	a.release(data)
	if cert == 0 {
		return chain
	}
	defer a.release(cert)
	policy := a.policyBasic()
	defer a.release(policy)
	var trust uintptr
	if a.trustCreate(cert, policy, &trust) != 0 {
		return chain
	}
	defer a.release(trust)
	var failure uintptr
	a.trustEvaluate(trust, &failure)
	if failure != 0 {
		a.release(failure)
	}
	certs := a.trustChain(trust)
	if certs == 0 {
		return chain
	}
	defer a.release(certs)
	for i := int64(1); i < a.arrayCount(certs); i++ {
		cert := a.arrayValue(certs, i)
		data := a.certificateData(cert)
		der := a.bytes(data)
		if data != 0 {
			a.release(data)
		}
		parsed, err := x509.ParseCertificate(der)
		if err == nil && parsed.CheckSignatureFrom(parsed) != nil {
			chain = append(chain, der)
		}
	}
	return chain
}

func macTrustError(code int64, dateException bool) error {
	reason := "verificarea certificatului serverului a eșuat; verifică numele serverului și lanțul CA"
	switch code {
	case -67820:
		reason = "macOS raportează un certificat revocat în lanțul serverului CNAS; excepția pentru expirare nu permite certificate revocate. Verifică endpointul și lanțul certificatului cu administratorul CNAS; această eroare nu se referă la tokenul USB"
	case -67818:
		reason = "certificatul serverului este expirat"
	case -67819:
		reason = "certificatul serverului nu este încă valabil; verifică și data calculatorului"
	case -67602:
		reason = "numele serverului nu corespunde certificatului HTTPS"
	case -67843:
		reason = "lanțul certificatului serverului nu este de încredere în macOS"
	}
	return fmt.Errorf("macOS CNAS TLS verification failed (code %d): %s (excepție valabilitate=%t)", code, reason, dateException)
}

// Revocation override still requires a cryptographically valid chain to native
// system anchors and the correct DNS name/serverAuth usage. Explicit pools keep
// Go from delegating verification back to macOS revocation policy.
func (a *securityAPI) verifyWithoutRevocation(trust uintptr, peer [][]byte, hostname string, ignoreDate bool) error {
	var anchors uintptr
	if status := a.copyAnchors(&anchors); status != 0 || anchors == 0 {
		return fmt.Errorf("macOS system trust anchors unavailable (%d)", status)
	}
	defer a.release(anchors)
	roots := [][]byte{}
	for i := int64(0); i < a.arrayCount(anchors); i++ {
		data := a.certificateData(a.arrayValue(anchors, i))
		if data == 0 {
			continue
		}
		roots = append(roots, a.bytes(data))
		a.release(data)
	}
	intermediates := append([][]byte(nil), peer[1:]...)
	chain := a.trustChain(trust)
	if chain != 0 {
		defer a.release(chain)
		for i := int64(1); i < a.arrayCount(chain); i++ {
			data := a.certificateData(a.arrayValue(chain, i))
			if data == 0 {
				continue
			}
			intermediates = append(intermediates, a.bytes(data))
			a.release(data)
		}
	}
	return verifyChainWithoutRevocation(peer[0], intermediates, roots, hostname, ignoreDate)
}

func verifyChainWithoutRevocation(leafDER []byte, intermediateDER, rootDER [][]byte, hostname string, ignoreDate bool) error {
	now := time.Now()
	parse := func(der []byte) (*x509.Certificate, error) {
		cert, err := x509.ParseCertificate(der)
		if err == nil && ignoreDate {
			// Change only the verifier's validity bounds, never signed DER or constraints.
			cert.NotBefore = now.Add(-time.Hour)
			cert.NotAfter = now.Add(time.Hour)
		}
		return cert, err
	}
	leaf, err := parse(leafDER)
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	intermediates := x509.NewCertPool()
	for _, der := range rootDER {
		cert, err := parse(der)
		if err != nil {
			return err
		}
		roots.AddCert(cert)
	}
	for _, der := range intermediateDER {
		cert, err := parse(der)
		if err != nil {
			return err
		}
		intermediates.AddCert(cert)
	}
	_, err = leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, DNSName: hostname, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	if err != nil {
		return fmt.Errorf("CNAS TLS verification with revocation exception: %w", err)
	}
	return nil
}
