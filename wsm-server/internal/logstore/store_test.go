package logstore

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDailyRotationStderrAndTenantRead(t *testing.T) {
	dir := t.TempDir()
	var stderr bytes.Buffer
	store, err := New(dir, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	day := time.Date(2026, 10, 28, 23, 59, 0, 0, time.UTC)
	store.now = func() time.Time { return day }
	for _, line := range []string{"startup\n", "connected tenant=a connection=one\n", "connected tenant=b connection=PRIVATE\n", "rejected tenant=ab reason=PRIVATE\n"} {
		store.Write([]byte(line))
	}
	page, err := store.Read("2026-10-28", 0, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Lines) != 2 || strings.Contains(strings.Join(page.Lines, " "), "PRIVATE") {
		t.Fatal(page)
	}
	day = day.Add(2 * time.Minute)
	store.Write([]byte("next day tenant=a\n"))
	next, err := store.Read("2026-10-29", 0, "a")
	if err != nil || len(next.Lines) != 1 {
		t.Fatal(next, err)
	}
	if !strings.Contains(stderr.String(), "PRIVATE") || !strings.Contains(stderr.String(), "next day") {
		t.Fatal("stderr not preserved")
	}
	dates, err := store.Dates()
	if err != nil || !slices.Contains(dates, "2026-10-28") || !slices.Contains(dates, "2026-10-29") {
		t.Fatal(dates, err)
	}
	info, err := os.Stat(filepath.Join(dir, "wsm-2026-10-28.log"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("permissions", err)
	}
}
func TestCursorBoundsIncompleteAndUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	day := store.Today()
	path := store.path(day)
	raw := "tenant=b " + strings.Repeat("PRIVATE", 45000) + "\ntenant=a visible\n"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := store.Read(day, 0, "a")
	if err != nil || first.Cursor > MaxReadBytes || !first.More {
		t.Fatal(first, err)
	}
	second, err := store.Read(day, first.Cursor, "a")
	if err != nil || len(second.Lines) != 1 || second.Lines[0] != "tenant=a visible" {
		t.Fatal(second, err)
	}
	for _, value := range []string{"../../secret", "2026-02-31", "2026-1-1", "2026-01-01.log"} {
		if _, err := store.Read(value, 0, "a"); err == nil {
			t.Fatal("unsafe date", value)
		}
	}
	if err := os.WriteFile(path, []byte("tenant=a partial"), 0600); err != nil {
		t.Fatal(err)
	}
	partial, err := store.Read(day, 0, "a")
	if err != nil || partial.Cursor != 0 || partial.More || len(partial.Lines) != 0 {
		t.Fatal(partial, err)
	}
	if err := os.WriteFile(path, []byte("tenant=a complete\n"), 0600); err != nil {
		t.Fatal(err)
	}
	completed, err := store.Read(day, partial.Cursor, "a")
	if err != nil || len(completed.Lines) != 1 {
		t.Fatal(completed, err)
	}
	target := filepath.Join(dir, "secret")
	os.WriteFile(target, []byte("SECRET"), 0600)
	link := store.path("2020-01-01")
	if err := os.Symlink(target, link); err == nil {
		if _, err := store.Read("2020-01-01", 0, "a"); err == nil {
			t.Fatal("symlink accepted")
		}
	}
}
func TestConcurrentWritesAndReads(t *testing.T) {
	store, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var group sync.WaitGroup
	for i := 0; i < 5; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < 30; j++ {
				store.Write([]byte("event tenant=a\n"))
				if _, err := store.Read(store.Today(), -1, "a"); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	group.Wait()
}
