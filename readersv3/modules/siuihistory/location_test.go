package siuihistory

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchivedLocalLocationIsAlreadyResolved(t *testing.T) {
	t.Chdir(t.TempDir())
	m := setup(t)
	m.rt.(*testRuntime).dir = "deployments"
	m.cfg.BasePath = "./changed-storage"
	root := filepath.Join("deployments", "history")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "original"), []byte("archived file"), 0600); err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{root, absolute} {
		store, err := m.storeForLocation(location{Storage: "local", BasePath: base})
		if err != nil {
			t.Fatal(err)
		}
		f, err := store.Open(context.Background(), "original", 0)
		if err != nil {
			t.Fatalf("cannot read saved location %q: %v", base, err)
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil || string(data) != "archived file" {
			t.Fatal(string(data), err)
		}
	}
	if _, err = m.storeForLocation(location{Storage: "local"}); err == nil || !strings.Contains(err.Error(), "base path") {
		t.Fatal("empty path accepted", err)
	}
}
