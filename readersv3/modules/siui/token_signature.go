package siui

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"errors"
)

func verifyTokenSignature(key crypto.PublicKey, digest, signature []byte, opts crypto.SignerOpts) error {
	switch k := key.(type) {
	case *rsa.PublicKey:
		if p, ok := opts.(*rsa.PSSOptions); ok {
			return rsa.VerifyPSS(k, opts.HashFunc(), digest, signature, p)
		}
		return rsa.VerifyPKCS1v15(k, opts.HashFunc(), digest, signature)
	case *ecdsa.PublicKey:
		if ecdsa.VerifyASN1(k, digest, signature) {
			return nil
		}
	}
	return errors.New("Token returned a signature that does not match the selected certificate")
}
