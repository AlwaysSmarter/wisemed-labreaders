//go:build darwin

package siui

import (
	"context"
	"crypto"
	"errors"
)

func (n nativeBackend) SignInvoice(ctx context.Context, data []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a, err := securityFramework()
	if err != nil {
		return nil, err
	}
	cert, release, err := a.clientCertificate(n.cfg)
	if err != nil {
		return nil, err
	}
	defer release()
	signer, ok := cert.PrivateKey.(crypto.Signer)
	if !ok {
		return nil, errors.New("token has no CMS signer")
	}
	signed, err := signInvoiceCMS(data, cert.Leaf, signer, cert.Certificate)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return signed, nil
}
