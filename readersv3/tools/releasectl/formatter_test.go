package main

import (
	"strings"
	"testing"
)

func TestFormatterSandboxService(t *testing.T) {
	a := appInfo{ID: "html-print-formatter", BinaryName: "html-print-formatter", SupportsHeadless: true}
	m := runtimeManifest{App: a}
	if s := renderSystemdService(m); strings.Contains(s, "User=root") || !strings.Contains(s, "User=html-print-formatter") || strings.Contains(s, "-headless") {
		t.Fatal(s)
	}
	for _, s := range []string{renderLinuxInstallHelper(a), renderLinuxPostInstall(m), renderRPMSpec(m)} {
		if !strings.Contains(s, "useradd --system") || !strings.Contains(s, "chown -R html-print-formatter:html-print-formatter") {
			t.Fatal(s)
		}
	}
	if strings.Contains(serviceArguments(a, "windows"), "-headless") {
		t.Fatal("service wrapper must supervise foreground process")
	}
	if !strings.Contains(serviceArguments(appInfo{ID: "other", SupportsHeadless: true}, "linux"), "-headless") {
		t.Fatal("changed existing app behavior")
	}
}
