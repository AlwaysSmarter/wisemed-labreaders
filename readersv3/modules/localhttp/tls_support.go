package localhttp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type localCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

func ensureLocalHTTPSMaterial(configDir, addr string) (string, string, error) {
	baseDir := filepath.Join(strings.TrimSpace(configDir), "tls")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", "", err
	}
	caCertPath := filepath.Join(baseDir, "wisemed-local-root-ca.pem")
	caKeyPath := filepath.Join(baseDir, "wisemed-local-root-ca.key")
	serverCertPath := filepath.Join(baseDir, "wisemed-local-server.pem")
	serverKeyPath := filepath.Join(baseDir, "wisemed-local-server.key")

	hosts := tlsServerHosts(addr)
	ca, err := ensureLocalCA(caCertPath, caKeyPath)
	if err != nil {
		return "", "", err
	}
	if err := ensureServerCertificate(serverCertPath, serverKeyPath, ca, hosts); err != nil {
		return "", "", err
	}
	if runtime.GOOS == "windows" {
		_ = trustLocalCAWindows(caCertPath, ca.cert)
	}
	return serverCertPath, serverKeyPath, nil
}

func tlsServerHosts(addr string) []string {
	out := []string{"localhost", "127.0.0.1", "::1"}
	host := strings.TrimSpace(addr)
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = strings.TrimSpace(h)
	}
	host = strings.Trim(host, "[]")
	if host != "" && host != "0.0.0.0" && host != "::" {
		out = append(out, host)
	}
	if machine, err := os.Hostname(); err == nil && strings.TrimSpace(machine) != "" {
		out = append(out, strings.TrimSpace(machine))
	}
	seen := map[string]bool{}
	unique := make([]string, 0, len(out))
	for _, item := range out {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, item)
	}
	sort.Strings(unique)
	return unique
}

func ensureLocalCA(certPath, keyPath string) (*localCA, error) {
	if item, err := loadLocalCA(certPath, keyPath); err == nil && item != nil && item.cert != nil && time.Now().Before(item.cert.NotAfter.Add(-24*time.Hour)) {
		return item, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := randomSerialNumber()
	if err != nil {
		return nil, err
	}
	now := time.Now().Add(-1 * time.Hour)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "WiseMED Local Root CA",
			Organization: []string{"WiseMED"},
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	if err := writePEMCertificate(certPath, der); err != nil {
		return nil, err
	}
	if err := writePEMECKey(keyPath, key); err != nil {
		return nil, err
	}
	return &localCA{cert: templateFromDER(der), key: key}, nil
}

func loadLocalCA(certPath, keyPath string) (*localCA, error) {
	certDER, err := readPEMBlock(certPath, "CERTIFICATE")
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}
	keyDER, err := readPEMAny(keyPath)
	if err != nil {
		return nil, err
	}
	keyAny, err := x509.ParseECPrivateKey(keyDER)
	if err != nil {
		return nil, err
	}
	return &localCA{cert: cert, key: keyAny}, nil
}

func ensureServerCertificate(certPath, keyPath string, ca *localCA, hosts []string) error {
	if valid, err := existingServerCertValid(certPath, keyPath, ca, hosts); err == nil && valid {
		return nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := randomSerialNumber()
	if err != nil {
		return err
	}
	now := time.Now().Add(-1 * time.Hour)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "localhost",
			Organization: []string{"WiseMED Local HTTPS"},
		},
		NotBefore:   now,
		NotAfter:    now.AddDate(2, 0, 0),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return err
	}
	if err := writePEMCertificate(certPath, der); err != nil {
		return err
	}
	return writePEMECKey(keyPath, key)
}

func existingServerCertValid(certPath, keyPath string, ca *localCA, hosts []string) (bool, error) {
	if _, err := os.Stat(certPath); err != nil {
		return false, err
	}
	if _, err := os.Stat(keyPath); err != nil {
		return false, err
	}
	certDER, err := readPEMBlock(certPath, "CERTIFICATE")
	if err != nil {
		return false, err
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return false, err
	}
	keyDER, err := readPEMAny(keyPath)
	if err != nil {
		return false, err
	}
	key, err := x509.ParseECPrivateKey(keyDER)
	if err != nil {
		return false, err
	}
	certPublic, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return false, err
	}
	keyPublic, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return false, err
	}
	if string(certPublic) != string(keyPublic) {
		return false, nil
	}
	if time.Now().After(cert.NotAfter.Add(-24 * time.Hour)) {
		return false, nil
	}
	if ca != nil && ca.cert != nil {
		roots := x509.NewCertPool()
		roots.AddCert(ca.cert)
		if _, err := cert.Verify(x509.VerifyOptions{
			Roots:       roots,
			CurrentTime: time.Now(),
		}); err != nil {
			return false, nil
		}
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			found := false
			for _, existing := range cert.IPAddresses {
				if existing.Equal(ip) {
					found = true
					break
				}
			}
			if !found {
				return false, nil
			}
			continue
		}
		found := false
		for _, existing := range cert.DNSNames {
			if strings.EqualFold(existing, host) {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}

func trustLocalCAWindows(certPath string, cert *x509.Certificate) error {
	if cert == nil {
		return errors.New("ca certificate is nil")
	}
	thumbprint := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%X", cert.SerialNumber.Bytes())))
	script := `
$path = $env:WMR_CA_CERT_PATH
$serial = $env:WMR_CA_SERIAL
if (-not (Test-Path $path)) { exit 0 }
$stores = @("Cert:\LocalMachine\Root", "Cert:\CurrentUser\Root")
foreach ($store in $stores) {
  try {
    $exists = Get-ChildItem -Path $store -ErrorAction Stop | Where-Object { $_.SerialNumber -eq $serial } | Select-Object -First 1
    if (-not $exists) {
      Import-Certificate -FilePath $path -CertStoreLocation $store -ErrorAction Stop | Out-Null
    }
  } catch {}
}
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), "WMR_CA_CERT_PATH="+certPath, "WMR_CA_SERIAL="+thumbprint)
	return cmd.Run()
}

func randomSerialNumber() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

func writePEMCertificate(path string, der []byte) error {
	block := &pem.Block{Type: "CERTIFICATE", Bytes: der}
	return os.WriteFile(path, pem.EncodeToMemory(block), 0o644)
}

func writePEMECKey(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	block := &pem.Block{Type: "EC PRIVATE KEY", Bytes: der}
	return os.WriteFile(path, pem.EncodeToMemory(block), 0o600)
}

func readPEMBlock(path, expectedType string) ([]byte, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(blob)
	if block == nil {
		return nil, fmt.Errorf("invalid pem file: %s", path)
	}
	if expectedType != "" && block.Type != expectedType {
		return nil, fmt.Errorf("unexpected pem block %q in %s", block.Type, path)
	}
	return block.Bytes, nil
}

func readPEMAny(path string) ([]byte, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(blob)
	if block == nil {
		return nil, fmt.Errorf("invalid pem file: %s", path)
	}
	return block.Bytes, nil
}

func templateFromDER(der []byte) *x509.Certificate {
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil
	}
	return cert
}
