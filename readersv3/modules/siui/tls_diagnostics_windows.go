//go:build windows

package siui

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var secureRequests sync.Map
var secureSequence atomic.Uint32
var whSetStatusCallback = winhttp.NewProc("WinHttpSetStatusCallback")

// Allocate one callback for the process, not one per request (Windows callbacks
// allocated by Go are never released). Unique IDs prevent handle-reuse races.
var secureCallback = windows.NewCallback(func(handle, id, status, info, length uintptr) uintptr {
	if status == 0x10000 && info != 0 && length >= 4 {
		if entry, ok := secureRequests.Load(id); ok {
			entry.(*atomic.Uint32).Or(*(*uint32)(unsafe.Pointer(info)))
		}
	}
	return 0
})

func attachSecureDiagnostics(request uintptr) (uintptr, *atomic.Uint32, func(), error) {
	id := uintptr(secureSequence.Add(1))
	flags := new(atomic.Uint32)
	secureRequests.Store(id, flags)
	cleanup := func() { secureRequests.Delete(id) }
	previous, _, err := whSetStatusCallback.Call(request, secureCallback, 0x10000, 0)
	if previous == ^uintptr(0) {
		cleanup()
		return 0, nil, nil, nativeError("TLS diagnostic callback", err)
	}
	return id, flags, cleanup, nil
}
func nativeRequestError(stage, host string, err error, flags *atomic.Uint32) error {
	if errors.Is(err, syscall.Errno(12175)) {
		return fmt.Errorf("Windows CNAS %s [%s]: %s", stage, host, secureFailureMessage(flags.Load()))
	}
	return nativeError(stage, err)
}
