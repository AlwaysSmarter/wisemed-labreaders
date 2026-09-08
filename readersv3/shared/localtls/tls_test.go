package localtls

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"testing"
)

func TestLocalCertificateReuse(t *testing.T) {
	dir := t.TempDir()
	cert, key, err := EnsureMaterial(dir, "127.0.0.1:19111")
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		if err := leaf.VerifyHostname(host); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadFile(cert)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := EnsureMaterial(dir, "127.0.0.1:19111"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(cert)
	if string(before) != string(after) {
		t.Fatal("valid certificate was regenerated")
	}
}
