//go:build darwin

package siui

import (
	"github.com/ebitengine/purego/objc"
	"testing"
)

func TestMacPINContextNoUI(t *testing.T) {
	a, e := securityFramework()
	if e != nil {
		t.Fatal(e)
	}
	ctx, release, e := macPINContext(a, []byte("test-only-not-a-real-token-pin"))
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	c := objc.ID(ctx)
	if c.Send(objc.RegisterName("interactionNotAllowed")) == 0 {
		t.Fatal("automatic attempt could display a PIN dialog")
	}
	if c.Send(objc.RegisterName("isCredentialSet:"), int64(-3)) == 0 {
		t.Fatal("smart card credential missing")
	}
}
