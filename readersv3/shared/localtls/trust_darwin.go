package localtls

import (
	"context"
	"crypto/x509"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func trustLocalCA(path string, _ *x509.Certificate) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	verify := func() error {
		return exec.CommandContext(ctx, "/usr/bin/security", "verify-cert", "-c", path, "-p", "basic").Run()
	}
	if verify() == nil {
		return "macOS Keychain", nil
	}
	output, err := exec.CommandContext(ctx, "/usr/bin/security", "default-keychain", "-d", "user").Output()
	if err != nil {
		return "", fmt.Errorf("macOS user keychain unavailable: %w; import %s in Keychain Access and set Always Trust", err, path)
	}
	keychain := strings.Trim(strings.TrimSpace(string(output)), "\"")
	if keychain == "" {
		return "", fmt.Errorf("macOS user keychain is empty; import %s in Keychain Access", path)
	}
	// macOS may ask the signed-in user to authorize this trust change.
	output, err = exec.CommandContext(ctx, "/usr/bin/security", "add-trusted-cert", "-r", "trustRoot", "-k", keychain, path).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("macOS trust: %w (%s); import %s in Keychain Access and set Always Trust; daemons require System keychain trust", err, strings.TrimSpace(string(output)), path)
	}
	if err = verify(); err != nil {
		return "", fmt.Errorf("macOS CA verification after import: %w", err)
	}
	return "macOS user Keychain", nil
}
