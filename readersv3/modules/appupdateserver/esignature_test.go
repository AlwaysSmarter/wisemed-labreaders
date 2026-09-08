package appupdateserver

import "testing"

func TestESignatureReleaseMapping(t *testing.T) {
	mapping, err := discoverManagedReader("../..", "esignature-server")
	if err != nil {
		t.Fatal(err)
	}
	if mapping.SourceAppID != "esignature-server" || mapping.UpdateAppID != "esignature-server" {
		t.Fatal(mapping)
	}
	if !isSupportedReleaseTarget("windows", "386") {
		t.Fatal("Windows x86 rejected")
	}
	if isSupportedReleaseTarget("linux", "386") || isSupportedReleaseTarget("darwin", "386") {
		t.Fatal("unsupported x86 target accepted")
	}
}
