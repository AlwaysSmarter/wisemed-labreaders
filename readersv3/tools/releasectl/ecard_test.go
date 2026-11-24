package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestECardBundledReleasePayload(t *testing.T) {
	root, e := filepath.Abs("../..")
	if e != nil {
		t.Fatal(e)
	}
	if e = validateECardPayload(root); e != nil {
		t.Fatal(e)
	}
}

func TestECardHostPackageContainsOnlyFreshRuntimeFiles(t *testing.T) {
	root := t.TempDir()
	compiled, source, dest := filepath.Join(root, "compiled"), filepath.Join(root, "source"), filepath.Join(root, "destination")
	for _, dir := range []string{compiled, source, dest} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range map[string]string{
		filepath.Join(compiled, "ECardHost.exe"):      "MZfresh-host",
		filepath.Join(source, "ECardHost.exe.config"): "<configuration />",
		filepath.Join(dest, "ECardHost.exe"):          "MZstale-host",
		filepath.Join(dest, "ECardHost.cs"):           "source",
		filepath.Join(dest, "build-host.ps1"):         "build",
		filepath.Join(dest, "run.ps1"):                "run",
	} {
		if err := os.WriteFile(name, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := packageECardHost(compiled, source, dest); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("unexpected payload: %v", entries)
	}
	binary, _ := os.ReadFile(filepath.Join(dest, "ECardHost.exe"))
	if string(binary) != "MZfresh-host" {
		t.Fatal("stale host packaged")
	}
	raw, _ := os.ReadFile(filepath.Join(dest, "manifest.json"))
	var hashes map[string]string
	if err := json.Unmarshal(raw, &hashes); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ECardHost.exe", "ECardHost.exe.config"} {
		if err := checkECardHash(filepath.Join(dest, name), hashes[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(compiled, "ECardHost.exe")); err != nil {
		t.Fatal(err)
	}
	if err := packageECardHost(compiled, source, dest); err == nil {
		t.Fatal("missing compiler output accepted")
	}
}

func TestECardHostCompilerFailureNeverUsesPrebuilt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix build script test")
	}
	root := t.TempDir()
	app := filepath.Join(root, "apps", "wisemed-pias-ecard")
	for _, dir := range []string{"scripts", "deployments/host"} {
		if err := os.MkdirAll(filepath.Join(app, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(app, "scripts", "build-host.sh"), []byte("#!/bin/sh\necho compiler-test-error >&2\nexit 42\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "deployments", "host", "ECardHost.exe"), []byte("MZold"), 0644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "runtime", "host")
	err := buildECardHost(root, dest, "test-version")
	if err == nil || !strings.Contains(err.Error(), "compiler-test-error") {
		t.Fatalf("compiler failure lost: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("failed build published host")
	}
}

func TestHostBuildReceivesExactReleaseVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	root := t.TempDir()
	app := filepath.Join(root, "apps", "wisemed-pias-ecard")
	if e := os.MkdirAll(filepath.Join(app, "scripts"), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(filepath.Join(app, "deployments", "host"), 0755); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(app, "deployments", "host", "ECardHost.exe.config"), []byte("config"), 0600)
	script := "#!/bin/sh\n[ \"$2\" = \"2026.11.23.cnas.2\" ] || exit 45\nprintf MZtest > \"$1/ECardHost.exe\"\n"
	if e := os.WriteFile(filepath.Join(app, "scripts", "build-host.sh"), []byte(script), 0755); e != nil {
		t.Fatal(e)
	}
	if e := buildECardHost(root, filepath.Join(root, "output"), "2026.11.23.cnas.2"); e != nil {
		t.Fatal(e)
	}
}
