//go:build windows

package siui

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

func pinBlob(b []byte) *windows.DataBlob {
	r := &windows.DataBlob{Size: uint32(len(b))}
	if len(b) > 0 {
		r.Data = &b[0]
	}
	return r
}
func sealPIN(_ string, p []byte, identity string) ([]byte, error) {
	var out windows.DataBlob
	e := windows.CryptProtectData(pinBlob(p), nil, pinBlob([]byte(identity)), 0, nil, 1, &out)
	if e != nil {
		return nil, e
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
func openPIN(_ string, p []byte, identity string) ([]byte, error) {
	var out windows.DataBlob
	e := windows.CryptUnprotectData(pinBlob(p), nil, pinBlob([]byte(identity)), 0, nil, 1, &out)
	if e != nil {
		return nil, e
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	defer clear(unsafe.Slice(out.Data, out.Size))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
