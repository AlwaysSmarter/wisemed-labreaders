package localhttp

import (
	"bytes"
	"testing"
	"time"
)

func TestSIUISessionSigningUsesPrivateRuntimeKey(t *testing.T) {
	rt := &wssSecretRuntime{services: map[string]interface{}{"analyzer-config": map[string]interface{}{"protocol": "siui"}}}
	first := &Module{rt: rt, utilitySessionKey: bytes.Repeat([]byte{1}, 32)}
	second := &Module{rt: rt, utilitySessionKey: bytes.Repeat([]byte{2}, 32)}
	claims := session{ID: "random-id", Username: "admin", UserType: 0, ExpiresAt: time.Now().Add(time.Minute)}
	token, err := first.encodeSessionToken(claims)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := first.decodeSessionToken(token); !ok {
		t.Fatal("own session rejected")
	}
	if _, ok := second.decodeSessionToken(token); ok {
		t.Fatal("session survived key rotation")
	}
	legacyRT := &wssSecretRuntime{services: map[string]interface{}{}}
	legacy := &Module{rt: legacyRT}
	forged, err := legacy.encodeSessionToken(claims)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := first.decodeSessionToken(forged); ok {
		t.Fatal("predictable legacy signing key accepted for SIUI")
	}
}
