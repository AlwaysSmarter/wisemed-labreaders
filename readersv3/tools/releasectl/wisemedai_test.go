package main

import (
	"strings"
	"testing"
)

func TestWiseMEDAIServiceAndPayload(t *testing.T) {
	a := appInfo{ID: "wisemed-ai", BinaryName: "wisemed-ai", ServiceName: serviceNameForApp("wisemed-ai"), SupportsHeadless: true}
	m := runtimeManifest{App: a}
	if a.ServiceName != "wisemed-ai" {
		t.Fatal(a)
	}
	if s := renderSystemdService(m); strings.Contains(s, "User=root") || !strings.Contains(s, "User=wisemed-ai") || strings.Contains(s, "-headless") {
		t.Fatal(s)
	}
	for _, s := range []string{renderLinuxInstallHelper(a), renderLinuxPostInstall(m), renderRPMSpec(m)} {
		if !strings.Contains(s, "useradd --system") || !strings.Contains(s, "chown -R wisemed-ai:wisemed-ai") {
			t.Fatal(s)
		}
	}
	for _, s := range []string{renderLinuxInstallHelper(a), renderMacInstallHelper(a)} {
		if strings.Contains(s, "rsync -a --delete") {
			t.Fatal("installer removes existing local secrets", s)
		}
	}
	if strings.Contains(renderWixSource(m), "B220-wisemed-ai") {
		t.Fatal("invalid upgrade GUID")
	}
	if s := serviceArguments(a, "windows"); strings.Contains(s, "-headless") {
		t.Fatal(s)
	}
}
