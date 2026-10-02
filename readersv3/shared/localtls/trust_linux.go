package localtls

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func trustLocalCA(path string, cert *x509.Certificate) (string, error) {
	pool, err := x509.SystemCertPool()
	if err == nil {
		if _, err = cert.Verify(x509.VerifyOptions{Roots: pool}); err == nil {
			return "Linux system trust store", nil
		}
	}
	command, err := exec.LookPath("update-ca-certificates")
	dir := "/usr/local/share/ca-certificates"
	args := []string{}
	if err != nil {
		command, err = exec.LookPath("update-ca-trust")
		dir = "/etc/pki/ca-trust/source/anchors"
		args = []string{"extract"}
	}
	if err != nil {
		return "", fmt.Errorf("no system trust update tool; import %s in the system/browser trusted authorities", path)
	}
	if os.Geteuid() != 0 {
		return "", fmt.Errorf("system trust installation requires root; install %s under %s and run %s", path, dir, command)
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	sum := sha256.Sum256(cert.Raw)
	dest := filepath.Join(dir, fmt.Sprintf("wisemed-local-%x.crt", sum[:12]))
	if err = os.WriteFile(dest, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0644); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, command, args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("Linux trust update: %w (%s)", err, output)
	}
	return "Linux system trust store (browsers with separate stores may need an import)", nil
}
