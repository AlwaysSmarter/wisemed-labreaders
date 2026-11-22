package main

import (
	"path/filepath"
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
