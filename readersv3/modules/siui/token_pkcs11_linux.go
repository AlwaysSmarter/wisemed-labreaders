//go:build linux

package siui

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ebitengine/purego"
	"io"
	"math/big"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"
)

type p11Attribute struct {
	Type   uint64
	Value  uintptr
	Length uint64
}
type p11Mechanism struct {
	Type      uint64
	Parameter uintptr
	Length    uint64
}
type p11API struct {
	init      func(uintptr) uint64
	slots     func(uint8, *uint64, *uint64) uint64
	info      func(uint64, uintptr) uint64
	open      func(uint64, uint64, uintptr, uintptr, *uint64) uint64
	close     func(uint64) uint64
	login     func(uint64, uint64, uintptr, uint64) uint64
	findInit  func(uint64, *p11Attribute, uint64) uint64
	find      func(uint64, *uint64, uint64, *uint64) uint64
	findFinal func(uint64) uint64
	attr      func(uint64, uint64, *p11Attribute, uint64) uint64
	signInit  func(uint64, *p11Mechanism, uint64) uint64
	sign      func(uint64, uintptr, uint64, uintptr, *uint64) uint64
}

var p11Modules = struct {
	sync.Mutex
	libs map[string]*p11API
}{libs: map[string]*p11API{}}

func p11Load(path string) (*p11API, error) {
	if (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") || !filepath.IsAbs(path) {
		return nil, errors.New("Configurați local pkcs11_module cu biblioteca PKCS#11 (Linux amd64/arm64)")
	}
	if a := p11Modules.libs[path]; a != nil {
		return a, nil
	}
	lib, e := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if e != nil {
		return nil, errors.New("Biblioteca PKCS#11 nu poate fi încărcată")
	}
	a := &p11API{}
	for _, v := range []struct {
		dst  any
		name string
	}{{&a.init, "C_Initialize"}, {&a.slots, "C_GetSlotList"}, {&a.info, "C_GetTokenInfo"}, {&a.open, "C_OpenSession"}, {&a.close, "C_CloseSession"}, {&a.login, "C_Login"}, {&a.findInit, "C_FindObjectsInit"}, {&a.find, "C_FindObjects"}, {&a.findFinal, "C_FindObjectsFinal"}, {&a.attr, "C_GetAttributeValue"}, {&a.signInit, "C_SignInit"}, {&a.sign, "C_Sign"}} {
		ptr, e := purego.Dlsym(lib, v.name)
		if e != nil {
			purego.Dlclose(lib)
			return nil, fmt.Errorf("PKCS#11 missing %s", v.name)
		}
		purego.RegisterFunc(v.dst, ptr)
	}
	if rv := a.init(0); rv != 0 && rv != 0x191 {
		purego.Dlclose(lib)
		return nil, fmt.Errorf("PKCS#11 initialization failed (0x%x)", rv)
	}
	p11Modules.libs[path] = a
	return a, nil
}
func (a *p11API) objects(session uint64, attrs []p11Attribute) ([]uint64, error) {
	if rv := a.findInit(session, &attrs[0], uint64(len(attrs))); rv != 0 {
		return nil, fmt.Errorf("PKCS#11 object search failed (0x%x)", rv)
	}
	defer a.findFinal(session)
	var result []uint64
	for len(result) < 4096 {
		var objs [32]uint64
		var n uint64
		if rv := a.find(session, &objs[0], 32, &n); rv != 0 || n > 32 {
			return nil, errors.New("PKCS#11 object enumeration failed")
		}
		if n == 0 {
			return result, nil
		}
		result = append(result, objs[:n]...)
	}
	return nil, errors.New("PKCS#11 object limit exceeded")
}
func (a *p11API) attribute(session, obj, kind uint64) ([]byte, error) {
	attr := p11Attribute{Type: kind}
	if rv := a.attr(session, obj, &attr, 1); rv != 0 || attr.Length > 1024*1024 {
		return nil, errors.New("PKCS#11 attribute unavailable")
	}
	if attr.Length == 0 {
		return nil, nil
	}
	b := make([]byte, attr.Length)
	attr.Value = uintptr(unsafe.Pointer(&b[0]))
	if rv := a.attr(session, obj, &attr, 1); rv != 0 || attr.Length > uint64(len(b)) {
		return nil, errors.New("PKCS#11 attribute read failed")
	}
	runtime.KeepAlive(b)
	return b[:attr.Length], nil
}

type p11Cert struct {
	slot uint64
	id   []byte
	der  []byte
	cert *x509.Certificate
}

func (a *p11API) certificates() ([]p11Cert, error) {
	var count uint64
	if rv := a.slots(1, nil, &count); rv != 0 || count > 256 {
		return nil, errors.New("PKCS#11 slot enumeration failed")
	}
	if count == 0 {
		return nil, nil
	}
	slots := make([]uint64, count)
	if rv := a.slots(1, &slots[0], &count); rv != 0 || count > uint64(len(slots)) {
		return nil, errors.New("PKCS#11 token list changed; reload certificates")
	}
	var certs []p11Cert
	for _, slot := range slots[:count] {
		var session uint64
		if a.open(slot, 4, 0, 0, &session) != 0 {
			continue
		}
		class := uint64(1)
		attrs := []p11Attribute{{Type: 0, Value: uintptr(unsafe.Pointer(&class)), Length: 8}}
		objs, e := a.objects(session, attrs)
		runtime.KeepAlive(class)
		if e == nil {
			for _, obj := range objs {
				der, e := a.attribute(session, obj, 0x11)
				if e != nil {
					continue
				}
				cert, e := x509.ParseCertificate(der)
				if e != nil {
					continue
				}
				id, e := a.attribute(session, obj, 0x102)
				if e == nil && len(id) > 0 {
					certs = append(certs, p11Cert{slot, id, der, cert})
				}
			}
		}
		a.close(session)
	}
	return certs, nil
}
func configuredCertificates(cfg settings) ([]Certificate, error) {
	p11Modules.Lock()
	defer p11Modules.Unlock()
	a, e := p11Load(cfg.PKCS11Module)
	if e != nil {
		return nil, e
	}
	certs, e := a.certificates()
	if e != nil {
		return nil, e
	}
	result := []Certificate{}
	for _, c := range certs {
		now := time.Now()
		result = append(result, Certificate{Thumbprint: fmt.Sprintf("%X", sha1.Sum(c.der)), Subject: c.cert.Subject.String(), Issuer: c.cert.Issuer.String(), NotBefore: c.cert.NotBefore.Format(time.RFC3339), NotAfter: c.cert.NotAfter.Format(time.RFC3339), Valid: !now.Before(c.cert.NotBefore) && !now.After(c.cert.NotAfter), HasPrivateKey: true})
	}
	return result, nil
}
func listCertificates(string) ([]Certificate, error) {
	return nil, errors.New("Selectați biblioteca PKCS#11 în configurația locală")
}

type nativeBackend struct {
	cfg  settings
	logf func(string, ...interface{})
}

func (n nativeBackend) Validate(ctx context.Context, schema string, data []byte) error {
	return validateSchema(ctx, schema, data)
}
func (n nativeBackend) certificate() (tls.Certificate, error) {
	p11Modules.Lock()
	defer p11Modules.Unlock()
	a, e := p11Load(n.cfg.PKCS11Module)
	if e != nil {
		return tls.Certificate{}, e
	}
	certs, e := a.certificates()
	if e != nil {
		return tls.Certificate{}, e
	}
	for _, c := range certs {
		if strings.EqualFold(fmt.Sprintf("%X", sha1.Sum(c.der)), strings.ReplaceAll(n.cfg.Thumbprint, " ", "")) {
			if time.Now().Before(c.cert.NotBefore) || time.Now().After(c.cert.NotAfter) {
				return tls.Certificate{}, errors.New("Token certificate expired or not yet valid")
			}
			return tls.Certificate{Certificate: [][]byte{c.der}, Leaf: c.cert, PrivateKey: &p11Signer{cfg: n.cfg, cert: c}, SupportedSignatureAlgorithms: []tls.SignatureScheme{tls.PKCS1WithSHA256, tls.PKCS1WithSHA384, tls.PKCS1WithSHA512, tls.ECDSAWithP256AndSHA256, tls.ECDSAWithP384AndSHA384, tls.ECDSAWithP521AndSHA512}}, nil
		}
	}
	return tls.Certificate{}, errors.New("Configured certificate not found in PKCS#11 module")
}
func (n nativeBackend) Do(ctx context.Context, method, address string, h http.Header, body []byte) (int, http.Header, []byte, error) {
	c, e := n.certificate()
	if e != nil {
		return 0, nil, nil, e
	}
	return tokenHTTP(ctx, n.cfg, c, method, address, h, body, n.logf)
}
func (n nativeBackend) SignInvoice(ctx context.Context, data []byte) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	c, e := n.certificate()
	if e != nil {
		return nil, e
	}
	return signInvoiceCMS(data, c.Leaf, c.PrivateKey.(crypto.Signer), c.Certificate)
}

type p11Signer struct {
	cfg  settings
	cert p11Cert
}

func (s *p11Signer) Public() crypto.PublicKey { return s.cert.cert.PublicKey }
func (s *p11Signer) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	p11Modules.Lock()
	defer p11Modules.Unlock()
	a, e := p11Load(s.cfg.PKCS11Module)
	if e != nil {
		return nil, e
	}
	// Check low-attempt and protected-path flags before offering a saved PIN.
	var info [256]byte
	if a.info(s.cert.slot, uintptr(unsafe.Pointer(&info[0]))) != 0 {
		return nil, errors.New("Token state unavailable")
	}
	flags := binary.LittleEndian.Uint64(info[96:104])
	if flags&0x40000 != 0 {
		return nil, errors.New("Token PIN locked; contact the token provider")
	}
	if flags&(0x10000|0x20000) != 0 && !s.cfg.pin.manualAvailable(s.cfg) {
		return nil, errors.New("Token reports few PIN attempts remaining; automatic PIN refused. Use manual PIN in CNAS settings")
	}
	var pin []byte
	finish := func(bool) {}
	if flags&0x100 == 0 {
		pin, finish, err = s.cfg.pin.begin(s.cfg)
		if err != nil {
			return nil, err
		}
		if len(pin) == 0 {
			return nil, errors.New("Introduceți manual PIN-ul în Setări CNAS → PIN pentru următoarea operație")
		}
	}
	defer func() {
		if err == nil {
			err = verifyTokenSignature(s.Public(), digest, signature, opts)
		}
		finish(err == nil)
	}()
	var session uint64
	if rv := a.open(s.cert.slot, 4, 0, 0, &session); rv != 0 {
		return nil, fmt.Errorf("PKCS#11 session failed (0x%x)", rv)
	}
	defer a.close(session)
	var ptr uintptr
	if len(pin) > 0 {
		ptr = uintptr(unsafe.Pointer(&pin[0]))
	}
	if rv := a.login(session, 1, ptr, uint64(len(pin))); rv != 0 && rv != 0x100 {
		runtime.KeepAlive(pin)
		return nil, fmt.Errorf("Token authentication failed (PKCS#11 0x%x); automatic PIN suspended; use manual PIN", rv)
	}
	runtime.KeepAlive(pin)
	class := uint64(3)
	attrs := []p11Attribute{{0, uintptr(unsafe.Pointer(&class)), 8}, {0x102, uintptr(unsafe.Pointer(&s.cert.id[0])), uint64(len(s.cert.id))}}
	keys, e := a.objects(session, attrs)
	runtime.KeepAlive(class)
	runtime.KeepAlive(s.cert.id)
	if e != nil || len(keys) != 1 {
		return nil, errors.New("PKCS#11 private key not uniquely identified")
	}
	if always, e := a.attribute(session, keys[0], 0x202); e == nil && len(always) > 0 && always[0] != 0 {
		return nil, errors.New("Token requires authentication for every signature; this PKCS#11 mode is not supported")
	}
	payload := digest
	mech := p11Mechanism{Type: 0x1041}
	if _, ok := s.Public().(*rsa.PublicKey); ok {
		if _, pss := opts.(*rsa.PSSOptions); pss {
			return nil, errors.New("PKCS#11 RSA-PSS not enabled")
		}
		mech.Type = 1
		prefixes := map[crypto.Hash]string{crypto.SHA256: "3031300d060960864801650304020105000420", crypto.SHA384: "3041300d060960864801650304020205000430", crypto.SHA512: "3051300d060960864801650304020305000440"}
		prefix, ok := prefixes[opts.HashFunc()]
		if !ok {
			return nil, errors.New("unsupported PKCS#11 digest")
		}
		b, _ := hex.DecodeString(prefix)
		payload = append(b, digest...)
	} else if _, ok := s.Public().(*ecdsa.PublicKey); !ok {
		return nil, errors.New("unsupported token key type")
	}
	if len(digest) != opts.HashFunc().Size() {
		return nil, errors.New("invalid digest length")
	}
	if rv := a.signInit(session, &mech, keys[0]); rv != 0 {
		return nil, fmt.Errorf("PKCS#11 signing initialization failed (0x%x)", rv)
	}
	// Allocate once to avoid a second C_Sign call in middleware with unusual retry behavior.
	out := make([]byte, 16384)
	size := uint64(len(out))
	rv := a.sign(session, uintptr(unsafe.Pointer(&payload[0])), uint64(len(payload)), uintptr(unsafe.Pointer(&out[0])), &size)
	runtime.KeepAlive(payload)
	runtime.KeepAlive(out)
	if rv != 0 || size > uint64(len(out)) {
		return nil, fmt.Errorf("PKCS#11 signing failed (0x%x); automatic PIN suspended", rv)
	}
	out = out[:size]
	if _, ok := s.Public().(*ecdsa.PublicKey); ok {
		if len(out) == 0 || len(out)%2 != 0 {
			return nil, errors.New("invalid EC signature")
		}
		return asn1.Marshal(struct{ R, S *big.Int }{new(big.Int).SetBytes(out[:len(out)/2]), new(big.Int).SetBytes(out[len(out)/2:])})
	}
	return out, nil
}
