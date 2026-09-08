package main

import "testing"

func TestESignatureTarget(t *testing.T) {
	target, ok := targetMatrix["windows-386"]
	if !ok || target.GOOS != "windows" || target.GOARCH != "386" {
		t.Fatal(target)
	}
	if _, err := buildRuntime(t.TempDir(), appInfo{ID: "esignature-server"}, "windows-amd64", targetMatrix["windows-amd64"], "1.0.0"); err == nil {
		t.Fatal("incompatible DLL architecture accepted")
	}
	if windresTarget("386") != "pe-i386" || wixArch("386") != "x86" {
		t.Fatal("x86 packaging mappings missing")
	}
}
