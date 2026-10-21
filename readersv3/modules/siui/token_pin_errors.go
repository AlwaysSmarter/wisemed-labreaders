package siui

import (
	"errors"
	"syscall"
)

// Only explicit credential errors suspend the saved PIN, never generic device,
// session, provider, cancellation, network or TLS failures.
func windowsPINRejected(code uint32) bool { return code == 0x8010006b || code == 0x8010006c }
func windowsPINErrorRejected(err error) bool {
	var code syscall.Errno
	return errors.As(err, &code) && windowsPINRejected(uint32(code))
}
func pkcs11PINRejected(code uint64) bool {
	return code == 0xa0 || code == 0xa1 || code == 0xa2 || code == 0xa4
}
