package appupdateserver

import "testing"

func TestECardReleaseMapping(t *testing.T) {
	mapping, err := discoverManagedReader("../..", "wisemed-pias-ecard")
	if err != nil {
		t.Fatal(err)
	}
	if mapping.SourceAppID != "wisemed-pias-ecard" || mapping.UpdateAppID != "wisemed-pias-ecard" {
		t.Fatalf("unexpected eCard release mapping: %+v", mapping)
	}
}
