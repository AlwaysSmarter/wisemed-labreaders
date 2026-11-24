package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"wisemed-labreaders/readersv3/core/config"
)

func TestVerbosityChangesPersistWithoutReplacingRuntime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("reader:\n  id: test\nlogging:\n  verbose_level: 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{cfg: cfg}
	for _, level := range []int{5, 1} {
		if err := app.ChangeDebugVerbosity(level); err != nil {
			t.Fatal(err)
		}
		if app.ModuleSettings("logging")["verbose_level"] != level {
			t.Fatal("live level unchanged")
		}
		disk, err := config.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if disk.Logging.VerboseLevel != level || disk.Reader.ID != "test" {
			t.Fatal("configuration not preserved")
		}
	}
	if err := app.ChangeDebugVerbosity(6); err == nil {
		t.Fatal("invalid level accepted")
	}
	if app.ModuleSettings("logging")["verbose_level"] != 1 {
		t.Fatal("invalid update changed runtime")
	}
}
