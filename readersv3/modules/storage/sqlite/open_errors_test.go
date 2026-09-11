package sqlite

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadonlyErrorPreservesCauseAndIdentifiesPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reader.db")
	cause := errors.New("attempt to write a readonly database (8)")
	err := explainSQLiteOpenError(path, cause)
	if !errors.Is(err, cause) {
		t.Fatal("lost original error")
	}
	for _, text := range []string{path, filepath.Dir(path), "Read-only", "-wal/-shm", "do not delete"} {
		if !strings.Contains(err.Error(), text) {
			t.Fatalf("missing %q: %v", text, err)
		}
	}
}
func TestIOErrorDoesNotRecommendDeletingWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reader.db")
	wal := []byte("uncheckpointed results")
	if err := os.WriteFile(path+"-wal", wal, 0600); err != nil {
		t.Fatal(err)
	}
	cause := errors.New("disk I/O error (522)")
	err := explainSQLiteOpenError(path, cause)
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "Do not delete the WAL") {
		t.Fatal(err)
	}
	got, readErr := os.ReadFile(path + "-wal")
	if readErr != nil || string(got) != string(wal) {
		t.Fatal("WAL modified")
	}
}
