package runner

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateHandoffErrorIncludesLauncherOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apply-update.log")
	if err := os.WriteFile(path+".launcher", []byte("PowerShell startup diagnostic\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	message := updateHandoffError(nil, path).Error()
	if !strings.Contains(message, "exit code 0") || !strings.Contains(message, "PowerShell startup diagnostic") || strings.Contains(message, "<nil>") {
		t.Fatal(message)
	}
	if err := os.WriteFile(path+".launcher", nil, 0600); err != nil {
		t.Fatal(err)
	}
	if message = updateHandoffError(nil, path).Error(); !strings.Contains(message, "no diagnostic output") {
		t.Fatal(message)
	}
}

func TestExtractUpdateArchiveReturnsAbsoluteStage(t *testing.T) {
	dir, err := os.MkdirTemp(".", "update-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	archive := filepath.Join(dir, "release.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	entry, err := z.Create("deployments/host/ECardHost.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("host")); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	stage, err := extractUpdateArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(stage) {
		t.Fatalf("relative updater stage: %s", stage)
	}
	data, err := os.ReadFile(filepath.Join(stage, "deployments/host/ECardHost.exe"))
	if err != nil || string(data) != "host" {
		t.Fatalf("extraction failed: %q %v", data, err)
	}
}
