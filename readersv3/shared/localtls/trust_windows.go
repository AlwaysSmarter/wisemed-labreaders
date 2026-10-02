//go:build windows

package localtls

import (
	"crypto/sha1"
	"crypto/x509"
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// Use the certificate's DER directly: no shell, PEM import or serial-number
// matching. A service's CurrentUser root store does not trust it for browsers.
func trustLocalCA(_ string, cert *x509.Certificate) (string, error) {
	if cert == nil || !cert.IsCA || len(cert.Raw) == 0 {
		return "", errors.New("invalid local CA")
	}
	machineErr := trustInWindowsStore(cert, windows.CERT_SYSTEM_STORE_LOCAL_MACHINE)
	if machineErr == nil {
		return `LocalMachine\Root`, nil
	}
	interactive, err := svc.IsAnInteractiveSession()
	if err != nil {
		return "", fmt.Errorf("LocalMachine: %v; session detection: %w", machineErr, err)
	}
	if !interactive {
		return "", fmt.Errorf("LocalMachine: %w; a service requires machine-wide trust", machineErr)
	}
	if err := trustInWindowsStore(cert, windows.CERT_SYSTEM_STORE_CURRENT_USER); err != nil {
		return "", fmt.Errorf("LocalMachine: %v; CurrentUser: %w", machineErr, err)
	}
	return `CurrentUser\Root (only the account running this process)`, nil
}

func trustInWindowsStore(cert *x509.Certificate, location uint32) error {
	name, _ := windows.UTF16PtrFromString("ROOT")
	// Read first so an already trusted machine CA needs no write permission.
	open := func(readonly bool) (windows.Handle, error) {
		flags := location | windows.CERT_STORE_OPEN_EXISTING_FLAG
		if readonly {
			flags |= windows.CERT_STORE_READONLY_FLAG
		}
		store, err := windows.CertOpenStore(windows.CERT_STORE_PROV_SYSTEM_W, 0, 0, flags, uintptr(unsafe.Pointer(name)))
		runtime.KeepAlive(name)
		return store, err
	}
	hash := sha1.Sum(cert.Raw) // Windows certificate identifier, not a signature.
	contains := func(store windows.Handle) bool {
		blob := windows.CryptDataBlob{Size: uint32(len(hash)), Data: &hash[0]}
		found, err := windows.CertFindCertificateInStore(store, windows.X509_ASN_ENCODING, 0, windows.CERT_FIND_HASH, unsafe.Pointer(&blob), nil)
		if err != nil {
			return false
		}
		windows.CertFreeCertificateContext(found)
		return true
	}
	if store, err := open(true); err == nil {
		exists := contains(store)
		windows.CertCloseStore(store, 0)
		if exists {
			return nil
		}
	}
	store, err := open(false)
	if err != nil {
		return err
	}
	defer windows.CertCloseStore(store, 0)
	context, err := windows.CertCreateCertificateContext(windows.X509_ASN_ENCODING, &cert.Raw[0], uint32(len(cert.Raw)))
	if err != nil {
		return err
	}
	defer windows.CertFreeCertificateContext(context)
	if err = windows.CertAddCertificateContextToStore(store, context, windows.CERT_STORE_ADD_USE_EXISTING, nil); err != nil {
		return err
	}
	if !contains(store) {
		return errors.New("CA missing after import")
	}
	return nil
}
