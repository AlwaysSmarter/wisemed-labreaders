//go:build linux

package siui

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPKCS11SoftwareTokenPINLifecycle(t *testing.T) {
	module := os.Getenv("SIUI_TEST_PKCS11_MODULE")
	if module == "" {
		t.Skip("requires an isolated software token")
	}
	v, cfg := testPIN(t)
	cfg.PKCS11Module = module
	certs, e := configuredCertificates(cfg)
	if e != nil || len(certs) == 0 {
		t.Fatal("enumeration", e)
	}
	cfg.Thumbprint = certs[0].Thumbprint
	pin := os.Getenv("SIUI_TEST_PKCS11_PIN")
	if e = v.save(cfg, pin, false, false); e != nil {
		t.Fatal(e)
	}
	n := nativeBackend{cfg: cfg}
	cert, e := n.certificate()
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256([]byte("test-only message"))
	sig, e := cert.PrivateKey.(crypto.Signer).Sign(rand.Reader, digest[:], crypto.SHA256)
	if e != nil {
		t.Fatal(e)
	}
	if e = rsa.VerifyPKCS1v15(cert.Leaf.PublicKey.(*rsa.PublicKey), crypto.SHA256, digest[:], sig); e != nil {
		t.Fatal(e)
	}

	// Exercise TLS client authentication with the token signer against a local server.
	roots := x509.NewCertPool()
	roots.AddCert(cert.Leaf)
	hits := 0
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; w.Write([]byte("ok")) }))
	server.TLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots, MaxVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	trust := filepath.Join(t.TempDir(), "server.pem")
	if e = os.WriteFile(trust, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("SSL_CERT_FILE", trust)
	status, _, body, e := n.Do(context.Background(), "POST", server.URL, http.Header{}, []byte("synthetic request"))
	if e != nil || status != 200 || string(body) != "ok" || hits != 1 {
		t.Fatal("mTLS", status, string(body), hits, e)
	}
	if signed, e := n.SignInvoice(context.Background(), []byte("<invoice/>")); e != nil || len(signed) == 0 {
		t.Fatal("CMS", e)
	}
	_, blocked, e := v.status(cfg)
	if e != nil || blocked {
		t.Fatal("successful PIN suspended", e)
	}
	if e = v.save(cfg, "deliberately-wrong-software-token-pin", false, false); e != nil {
		t.Fatal(e)
	}
	if _, e = cert.PrivateKey.(crypto.Signer).Sign(rand.Reader, digest[:], crypto.SHA256); e == nil {
		t.Fatal("incorrect PIN accepted")
	}
	_, blocked, _ = v.status(cfg)
	if !blocked {
		t.Fatal("incorrect PIN not suspended")
	}
	if _, e = cert.PrivateKey.(crypto.Signer).Sign(rand.Reader, digest[:], crypto.SHA256); e == nil || !strings.Contains(e.Error(), "PIN") {
		t.Fatal("manual fallback missing", e)
	}
	if e = v.save(cfg, pin, false, true); e != nil {
		t.Fatal(e)
	}
	if _, e = cert.PrivateKey.(crypto.Signer).Sign(rand.Reader, digest[:], crypto.SHA256); e != nil {
		t.Fatal("manual recovery", e)
	}
	_, blocked, _ = v.status(cfg)
	if !blocked {
		t.Fatal("manual PIN reenabled wrong saved PIN")
	}
}
