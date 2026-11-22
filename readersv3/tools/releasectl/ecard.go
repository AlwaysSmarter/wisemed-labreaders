package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Validate vendor SDKs before building. The host is always compiled from source;
// a checked-in EXE or manifest is never used as a release input.
func validateECardPayload(root string) error {
	base := filepath.Join(root, "apps", "wisemed-pias-ecard", "deployments")
	if _, err := os.Stat(filepath.Join(base, "host", "ECardHost.cs")); err != nil {
		return err
	}
	raw, e := os.ReadFile(filepath.Join(base, "sdks", "manifest.json"))
	if e != nil {
		return e
	}
	var libs []struct {
		SDK  string `json:"sdk"`
		File string `json:"file"`
		SHA  string `json:"sha256"`
	}
	if e = json.Unmarshal(raw, &libs); e != nil {
		return e
	}
	found := map[string]bool{}
	for _, lib := range libs {
		if strings.Contains(lib.File, "..") || filepath.IsAbs(lib.File) {
			return fmt.Errorf("unsafe SDK manifest path")
		}
		if e = checkECardHash(filepath.Join(base, "sdks", filepath.FromSlash(lib.File)), lib.SHA); e != nil {
			return e
		}
		if strings.HasSuffix(lib.File, "eCard.SDK.dll") {
			found[lib.SDK] = true
		}
	}
	for _, id := range []string{"2.0.0.4", "2.0.0.3", "thinkit-1.3.1.4", "1.3.0.4", "1.3.0.2", "1.3.0.1", "1.2.0.1"} {
		if !found[id] {
			return fmt.Errorf("eCard SDK %s missing from release", id)
		}
	}
	return nil
}
func checkECardHash(path, want string) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(b)
	if want == "" || hex.EncodeToString(sum[:]) != want {
		return fmt.Errorf("eCard payload hash mismatch: %s; rebuild and update manifest", path)
	}
	return nil
}

// Build in a private directory so parallel targets cannot overwrite each other's
// host or modify the source checkout. Only runtime files enter the release.
func buildECardHost(root, destination string) error {
	temp, err := os.MkdirTemp("", "wisemed-ecard-host-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	app := filepath.Join(root, "apps", "wisemed-pias-ecard")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(app, "deployments", "host", "build-host.ps1"), "-OutputDirectory", temp)
	} else {
		cmd = exec.CommandContext(ctx, "bash", filepath.Join(app, "scripts", "build-host.sh"), temp)
	}
	cmd.Dir = root
	logf("eCard host compilation start (Windows x86 / .NET Framework 4.8)")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("eCard host compilation failed (no prebuilt fallback): %w\n%s", err, strings.TrimSpace(string(output)))
	}
	if err = packageECardHost(temp, filepath.Join(app, "deployments", "host"), destination); err != nil {
		return err
	}
	logf("eCard host compiled and packaged successfully")
	return nil
}

func packageECardHost(compiled, source, destination string) error {
	exe := filepath.Join(compiled, "ECardHost.exe")
	binary, err := os.ReadFile(exe)
	if err != nil {
		return fmt.Errorf("compiled eCard host missing: %w", err)
	}
	if len(binary) < 2 || string(binary[:2]) != "MZ" {
		return fmt.Errorf("compiled eCard host is not a Windows executable")
	}
	// Remove checked-in sources, scripts and binaries copied with deployments.
	if err = os.RemoveAll(destination); err != nil {
		return err
	}
	if err = os.MkdirAll(destination, 0755); err != nil {
		return err
	}
	if err = copyFile(exe, filepath.Join(destination, "ECardHost.exe"), 0644); err != nil {
		return err
	}
	if err = copyFile(filepath.Join(source, "ECardHost.exe.config"), filepath.Join(destination, "ECardHost.exe.config"), 0644); err != nil {
		return err
	}
	hashes := map[string]string{}
	for _, name := range []string{"ECardHost.exe", "ECardHost.exe.config"} {
		data, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		hashes[name] = hex.EncodeToString(sum[:])
	}
	data, err := json.MarshalIndent(hashes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destination, "manifest.json"), append(data, '\n'), 0644)
}
