package localtls

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestLocalCertificateReuse(t *testing.T) {
	dir := t.TempDir()
	cert, key, err := ensureMaterial(dir, "127.0.0.1:19111", nil, nil)
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
	if _, _, err := ensureMaterial(dir, "127.0.0.1:19111", nil, nil); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(cert)
	if string(before) != string(after) {
		t.Fatal("valid certificate was regenerated")
	}
}

func TestTrustFailureIsLoggedWithoutLosingHTTPSOrRotatingCA(t *testing.T) {
	dir := t.TempDir()
	var logs []string
	var firstCA []byte
	trust := func(_ string, ca *x509.Certificate) (string, error) {
		if !ca.IsCA {
			t.Fatal("not a CA")
		}
		if firstCA == nil {
			firstCA = append([]byte(nil), ca.Raw...)
		} else if !bytes.Equal(firstCA, ca.Raw) {
			t.Fatal("trust failure rotated CA")
		}
		return "", errors.New("access denied")
	}
	logf := func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	for i := 0; i < 2; i++ {
		cert, key, err := ensureMaterial(dir, "localhost:19112", trust, logf)
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
		root, err := x509.ParseCertificate(firstCA)
		if err != nil {
			t.Fatal(err)
		}
		pool := x509.NewCertPool()
		pool.AddCert(root)
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: "localhost"}); err != nil {
			t.Fatal(err)
		}
	}
	if len(logs) != 2 || !strings.Contains(logs[0], "access denied") || !strings.Contains(logs[0], "CERT_AUTHORITY_INVALID") {
		t.Fatalf("missing diagnostic: %v", logs)
	}
}

func TestTrustSuccessLogsStore(t *testing.T) {
	var message string
	_, _, err := ensureMaterial(t.TempDir(), "localhost:19112", func(string, *x509.Certificate) (string, error) { return `LocalMachine\Root`, nil }, func(format string, args ...any) { message = fmt.Sprintf(format, args...) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, `CA trusted in LocalMachine\Root`) {
		t.Fatal(message)
	}
}
