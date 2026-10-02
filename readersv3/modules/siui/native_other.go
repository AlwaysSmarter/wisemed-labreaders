//go:build !windows

package siui

import (
	"context"
	"errors"
	"net/http"
)

type nativeBackend struct{ cfg settings }

func (n nativeBackend) Do(context.Context, string, string, http.Header, []byte) (int, http.Header, []byte, error) {
	return 0, nil, nil, errors.New("CNAS USB certificate transport requires Windows and the token driver")
}
func (n nativeBackend) Validate(ctx context.Context, schema string, data []byte) error {
	return validateSchema(ctx, schema, data)
}
func listCertificates(string) ([]Certificate, error) {
	return nil, errors.New("token certificate selection requires Windows")
}
