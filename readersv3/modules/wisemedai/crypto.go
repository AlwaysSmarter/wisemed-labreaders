package wisemedai

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type cipherBox struct{ a cipher.AEAD }

func secretFile(path string, n int) ([]byte, error) {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	b, e := os.ReadFile(path)
	if e == nil {
		return hex.DecodeString(string(b))
	}
	if !os.IsNotExist(e) {
		return nil, e
	}
	b = make([]byte, n)
	if _, e = rand.Read(b); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(e) {
		return secretFile(path, n)
	}
	if e != nil {
		return nil, e
	}
	_, e = f.WriteString(hex.EncodeToString(b))
	ce := f.Close()
	if e != nil {
		return nil, e
	}
	return b, ce
}
func loadBox(path string) (*cipherBox, error) {
	b, e := secretFile(path, 32)
	if e != nil {
		return nil, e
	}
	c, e := aes.NewCipher(b)
	if e != nil {
		return nil, e
	}
	a, e := cipher.NewGCM(c)
	return &cipherBox{a}, e
}
func (b *cipherBox) seal(v any, scope string) ([]byte, error) {
	raw, e := json.Marshal(v)
	if e != nil {
		return nil, e
	}
	n := make([]byte, b.a.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return nil, e
	}
	return b.a.Seal(n, n, raw, []byte(scope)), nil
}
func (b *cipherBox) open(raw []byte, scope string, v any) error {
	n := b.a.NonceSize()
	if len(raw) < n {
		return errors.New("invalid encrypted record")
	}
	p, e := b.a.Open(nil, raw[:n], raw[n:], []byte(scope))
	if e != nil {
		return e
	}
	return json.Unmarshal(p, v)
}
func digest(raw string) string { b := sha256.Sum256([]byte(raw)); return hex.EncodeToString(b[:]) }
func randomToken() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
