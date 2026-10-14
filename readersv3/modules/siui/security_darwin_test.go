//go:build darwin

package siui

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"github.com/ebitengine/purego"
	"math/big"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMacFrameworkAndUntrustedTLS(t *testing.T) {
	a, err := securityFramework()
	if err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, expired := range []bool{false, true} {
		end := time.Now().Add(time.Hour)
		if expired {
			end = time.Now().Add(-time.Hour)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: end, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		for _, ignoreDate := range []bool{false, true} {
			if err := a.verifyServer([][]byte{der}, "localhost", ignoreDate, false); err == nil {
				t.Fatalf("untrusted certificate accepted expired=%v exception=%v", expired, ignoreDate)
			}
			if err := a.verifyServer([][]byte{der}, "wrong.example", ignoreDate, false); err == nil {
				t.Fatal("wrong hostname accepted")
			}
		}
	}
}
func TestMacSignerAlgorithms(t *testing.T) {
	s := &macSigner{public: &rsa.PublicKey{}}
	if _, err := s.algorithm(crypto.SHA1); err == nil {
		t.Fatal("SHA1 accepted")
	}
	if _, err := s.algorithm(&rsa.PSSOptions{Hash: crypto.SHA256, SaltLength: 1}); err == nil {
		t.Fatal("invalid PSS salt accepted")
	}
	if _, err := s.Sign(nil, []byte{1}, crypto.SHA256); err == nil {
		t.Fatal("invalid digest accepted")
	}
}
func TestMacConnectedToken(t *testing.T) {
	if os.Getenv("SIUI_TEST_CONNECTED_TOKEN") != "1" {
		t.Skip("opt-in hardware enumeration")
	}
	certs, err := listCertificates("CurrentUser")
	if err != nil {
		t.Fatal(err)
	}
	expected := os.Getenv("SIUI_TEST_EXPECT_THUMBPRINT")
	matched := expected == ""
	valid := 0
	for _, c := range certs {
		if c.Valid && c.HasPrivateKey {
			valid++
			if strings.EqualFold(c.Thumbprint, expected) {
				matched = true
				a, err := securityFramework()
				if err != nil {
					t.Fatal(err)
				}
				cert, release, err := a.clientCertificate(settings{Store: "CurrentUser", Thumbprint: c.Thumbprint})
				if err != nil {
					t.Fatal(err)
				}
				if len(cert.SupportedSignatureAlgorithms) == 0 {
					release()
					t.Fatal("no signing algorithms for connected token")
				}
				release()
				t.Logf("Connected client identity supports %d TLS signature algorithms", len(cert.SupportedSignatureAlgorithms))
			}
		}
	}
	if !matched {
		t.Fatal("OS client identity missing from bridge enumeration")
	}
	if valid == 0 {
		t.Fatal("no valid client identity found")
	}
	t.Logf("Detected %d valid client identities with private key access", valid)
}

func TestMacNativeSigning(t *testing.T) {
	a, err := securityFramework()
	if err != nil {
		t.Fatal(err)
	}
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var create func(uintptr, uintptr, *uintptr) uintptr
	purego.RegisterFunc(&create, a.symbol(a.sec, "SecKeyCreateWithData"))
	attrs := a.dictionaryCreate(0, 0, a.symbol(a.cf, "kCFTypeDictionaryKeyCallBacks"), a.symbol(a.cf, "kCFTypeDictionaryValueCallBacks"))
	defer a.release(attrs)
	a.dictionarySet(attrs, a.constant("kSecAttrKeyType"), a.constant("kSecAttrKeyTypeRSA"))
	a.dictionarySet(attrs, a.constant("kSecAttrKeyClass"), a.constant("kSecAttrKeyClassPrivate"))
	data := a.data(x509.MarshalPKCS1PrivateKey(private))
	defer a.release(data)
	var failure uintptr
	key := create(data, attrs, &failure)
	if failure != 0 {
		defer a.release(failure)
	}
	if key == 0 {
		t.Fatal("native ephemeral key import failed")
	}
	defer a.release(key)
	signer := &macSigner{api: a, key: key, public: &private.PublicKey}
	digest := sha256.Sum256([]byte("local ephemeral TLS signer test"))
	sig, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if err := rsa.VerifyPKCS1v15(&private.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatal(err)
	}
	opts := &rsa.PSSOptions{Hash: crypto.SHA256, SaltLength: rsa.PSSSaltLengthEqualsHash}
	sig, err = signer.Sign(rand.Reader, digest[:], opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := rsa.VerifyPSS(&private.PublicKey, crypto.SHA256, digest[:], sig, opts); err != nil {
		t.Fatal(err)
	}
}

func TestMacTrustedTLSDateException(t *testing.T) {
	api, err := securityFramework()
	if err != nil {
		t.Fatal(err)
	}
	a := *api
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Ephemeral test CA"}, NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	data := a.data(rootDER)
	defer a.release(data)
	cert := a.certificateCreate(0, data)
	defer a.release(cert)
	anchors := a.array()
	defer a.release(anchors)
	a.arrayAppend(anchors, cert)
	var setAnchors func(uintptr, uintptr) int32
	purego.RegisterFunc(&setAnchors, a.symbol(a.sec, "SecTrustSetAnchorCertificates"))
	original := a.trustCreate
	a.trustCreate = func(c, p uintptr, out *uintptr) int32 {
		status := original(c, p, out)
		if status == 0 {
			return setAnchors(*out, anchors)
		}
		return status
	}
	for _, expired := range []bool{false, true} {
		end := time.Now().Add(time.Hour)
		if expired {
			end = time.Now().Add(-time.Hour)
		}
		leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-24 * time.Hour), NotAfter: end, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		der, err := x509.CreateCertificate(rand.Reader, leaf, root, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		for _, exception := range []bool{false, true} {
			err := a.verifyServer([][]byte{der, rootDER}, "localhost", exception, false)
			if (err == nil) != (!expired || exception) {
				t.Fatalf("expired=%v exception=%v: %v", expired, exception, err)
			}
			if err := a.verifyServer([][]byte{der, rootDER}, "wrong.example", exception, false); err == nil {
				t.Fatal("trusted certificate accepted for wrong hostname")
			}
		}
	}
}

// Software keys can support both sign and verify on a private key; a token may
// only expose sign. This regression check models that hardware restriction.
func TestMacSignerQueriesSignOperation(t *testing.T) {
	a, err := securityFramework()
	if err != nil {
		t.Fatal(err)
	}
	api := *a
	queried := false
	api.supports = func(_ uintptr, operation int64, _ uintptr) bool {
		queried = true
		if operation != 0 {
			t.Errorf("queried operation %d; expected native sign (0)", operation)
		}
		return false
	}
	signer := &macSigner{api: &api, public: &rsa.PublicKey{}}
	_, err = signer.Sign(nil, make([]byte, 32), crypto.SHA256)
	if !queried || err == nil {
		t.Fatal("expected native capability check")
	}
}

func TestMacRevocationDiagnostic(t *testing.T) {
	for _, exception := range []bool{false, true} {
		message := macTrustError(-67820, exception).Error()
		if !strings.Contains(message, "revocat") || !strings.Contains(message, "nu permite certificate revocate") {
			t.Fatalf("missing revocation explanation: %s", message)
		}
	}
}

func TestMacRevocationOverridePreservesChainChecks(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test CA"}, NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(-time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, leaf, root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	roots := [][]byte{rootDER}
	if err := verifyChainWithoutRevocation(der, nil, roots, "localhost", true); err != nil {
		t.Fatal(err)
	}
	if err := verifyChainWithoutRevocation(der, nil, roots, "localhost", false); err == nil {
		t.Fatal("expiry bypass without date exception")
	}
	if err := verifyChainWithoutRevocation(der, nil, roots, "wrong.example", true); err == nil {
		t.Fatal("hostname bypass")
	}
	if err := verifyChainWithoutRevocation(der, nil, nil, "localhost", true); err == nil {
		t.Fatal("untrusted root bypass")
	}
	tampered := append([]byte(nil), der...)
	tampered[len(tampered)-1] ^= 1
	if err := verifyChainWithoutRevocation(tampered, nil, roots, "localhost", true); err == nil {
		t.Fatal("signature bypass")
	}
}

func TestMacCNASServerTrust(t *testing.T) {
	if os.Getenv("SIUI_TEST_CNAS_TLS") != "1" {
		t.Skip("opt-in public endpoint TLS check; no HTTP request/client token")
	}
	a, err := securityFramework()
	if err != nil {
		t.Fatal(err)
	}
	verified := false
	config := &tls.Config{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12, InsecureSkipVerify: true,
		CipherSuites: []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_RSA_WITH_AES_128_CBC_SHA, tls.TLS_RSA_WITH_AES_256_CBC_SHA},
		VerifyConnection: func(state tls.ConnectionState) error {
			chain := [][]byte{}
			for _, cert := range state.PeerCertificates {
				chain = append(chain, cert.Raw)
			}
			if err := a.verifyServer(chain, "www.siui.ro", true, true); err != nil {
				return err
			}
			verified = true
			return nil
		}}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", "www.siui.ro:443", config)
	if conn != nil {
		conn.Close()
	}
	if !verified {
		t.Fatalf("server trust was not verified: %v", err)
	}
	t.Log("CNAS server chain/hostname verified with explicit date and revocation exceptions; no HTTP request sent")
}
