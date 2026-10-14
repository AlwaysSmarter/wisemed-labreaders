//go:build darwin

package siui

import (
	"errors"
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"sync"
)

var localAuth struct {
	once sync.Once
	err  error
}

func macPINContext(a *securityAPI, pin []byte) (uintptr, func(), error) {
	noop := func() {}
	if len(pin) == 0 {
		return 0, noop, nil
	}
	localAuth.once.Do(func() {
		_, localAuth.err = purego.Dlopen("/System/Library/Frameworks/LocalAuthentication.framework/LocalAuthentication", purego.RTLD_NOW|purego.RTLD_LOCAL)
	})
	if localAuth.err != nil {
		return 0, noop, errors.New("macOS LocalAuthentication unavailable")
	}
	ctx := objc.ID(objc.GetClass("LAContext")).Send(objc.RegisterName("new"))
	if ctx == 0 {
		return 0, noop, errors.New("macOS PIN context unavailable")
	}
	cleanup := func() {
		ctx.Send(objc.RegisterName("setCredential:type:"), uintptr(0), int64(-3))
		ctx.Send(objc.RegisterName("invalidate"))
		ctx.Send(objc.RegisterName("release"))
	}
	data := a.data(pin)
	defer a.release(data)
	if ctx.Send(objc.RegisterName("setCredential:type:"), data, int64(-3)) == 0 {
		cleanup()
		return 0, noop, errors.New("macOS driver does not accept a supplied PIN")
	}
	// No OS dialog/retry may reuse this supplied PIN during the automatic attempt.
	ctx.Send(objc.RegisterName("setInteractionNotAllowed:"), true)
	return uintptr(ctx), cleanup, nil
}
