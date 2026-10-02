//go:build !windows && !darwin && !linux

package localtls

import (
	"crypto/x509"
	"errors"
)

func trustLocalCA(_ string, cert *x509.Certificate) (string, error) {
	return "", errors.New("install the local root CA in the operating system/browser trust store")
}
