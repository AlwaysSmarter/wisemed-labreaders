package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Fail the release before building if the bundled host is stale or SDK payloads
// have been omitted/changed. Runtime keys, TDX and logs are never release inputs.
func validateECardPayload(root string) error {
	base := filepath.Join(root, "apps", "wisemed-pias-ecard", "deployments")
	raw, e := os.ReadFile(filepath.Join(base, "host", "manifest.json"))
	if e != nil {
		return e
	}
	var host map[string]string
	if e = json.Unmarshal(raw, &host); e != nil {
		return e
	}
	for _, name := range []string{"ECardHost.cs", "ECardHost.exe", "ECardHost.exe.config"} {
		if e = checkECardHash(filepath.Join(base, "host", name), host[name]); e != nil {
			return e
		}
	}
	raw, e = os.ReadFile(filepath.Join(base, "sdks", "manifest.json"))
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
