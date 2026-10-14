//go:build !windows

package siui

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"os"
	"path/filepath"
)

// The installation key is kept outside YAML and the encrypted vault. Access is
// limited to the service account. Backups containing both files remain sensitive.
func pinKey(path string, create bool) ([]byte, error) {
	path += ".key"
	b, e := os.ReadFile(path)
	if e == nil {
		st, se := os.Stat(path)
		if se != nil || st.Mode().Perm()&0077 != 0 || len(b) != 32 {
			return nil, errors.New("invalid PIN key permissions or size")
		}
		return b, nil
	}
	if !os.IsNotExist(e) || !create {
		return nil, e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	b, e = randomBytes(32)
	if e != nil {
		return nil, e
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		clear(b)
		if os.IsExist(e) {
			return pinKey(path, false)
		}
		return nil, e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		clear(b)
		return nil, e
	}
	return b, nil
}
func pinAEAD(path string, create bool) (cipher.AEAD, error) {
	k, e := pinKey(path, create)
	if e != nil {
		return nil, e
	}
	defer clear(k)
	b, e := aes.NewCipher(k)
	if e != nil {
		return nil, e
	}
	return cipher.NewGCM(b)
}
func sealPIN(path string, p []byte, identity string) ([]byte, error) {
	a, e := pinAEAD(path, true)
	if e != nil {
		return nil, e
	}
	n, e := randomBytes(a.NonceSize())
	if e != nil {
		return nil, e
	}
	return a.Seal(n, n, p, []byte(identity)), nil
}
func openPIN(path string, p []byte, identity string) ([]byte, error) {
	a, e := pinAEAD(path, false)
	if e != nil {
		return nil, e
	}
	if len(p) < a.NonceSize() {
		return nil, errors.New("invalid encrypted PIN")
	}
	return a.Open(nil, p[:a.NonceSize()], p[a.NonceSize():], []byte(identity))
}
