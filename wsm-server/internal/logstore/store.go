// Package logstore mirrors operational logs to stderr and daily UTC files.
package logstore

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const MaxReadBytes int64 = 256 * 1024

type Store struct {
	mu     sync.Mutex
	dir    string
	stderr io.Writer
	file   *os.File
	day    string
	now    func() time.Time
	closed bool
}
type Page struct {
	Date     string   `json:"date"`
	Today    string   `json:"today"`
	Timezone string   `json:"timezone"`
	Lines    []string `json:"lines"`
	Cursor   int64    `json:"cursor"`
	More     bool     `json:"more"`
}

func New(dir string, stderr io.Writer) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, stderr: stderr, now: time.Now}
	if err := s.rotate(); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Store) Today() string { return s.now().UTC().Format("2006-01-02") }
func validDate(day string) bool {
	t, err := time.Parse("2006-01-02", day)
	return err == nil && t.Format("2006-01-02") == day
}
func (s *Store) path(day string) string { return filepath.Join(s.dir, "wsm-"+day+".log") }
func (s *Store) rotate() error {
	day := s.Today()
	if s.file != nil && s.day == day {
		return nil
	}
	path := s.path(day)
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("log path is not a regular file")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if s.file != nil {
		_ = s.file.Close()
	}
	s.file = file
	s.day = day
	return nil
}
func (s *Store) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A file failure must never stop stderr output or create a logging recursion.
	if s.stderr != nil {
		_, _ = s.stderr.Write(data)
	}
	if s.closed {
		return len(data), nil
	}
	err := s.rotate()
	if err == nil {
		_, err = s.file.Write(data)
	}
	if err != nil && s.stderr != nil {
		_, _ = fmt.Fprintf(s.stderr, "WSM daily log unavailable: %v\n", err)
	}
	return len(data), nil
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}
func (s *Store) Dates() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	days := []string{}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !strings.HasPrefix(name, "wsm-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		day := strings.TrimSuffix(strings.TrimPrefix(name, "wsm-"), ".log")
		if validDate(day) {
			days = append(days, day)
		}
	}
	if !contains(days, s.Today()) {
		days = append(days, s.Today())
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	return days, nil
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

// Logs with an authenticated tenant marker are visible only inside that tenant.
// Generic operational/authentication diagnostics contain no unverified tenant ID.
func visible(line, tenant string) bool {
	for _, field := range strings.Fields(line) {
		if strings.HasPrefix(field, "tenant=") && strings.TrimPrefix(field, "tenant=") != tenant {
			return false
		}
	}
	return true
}

// Read uses byte cursors, bounds work per request and does not tail an open writer.
// Negative cursor selects the most recent page; oversized lines are omitted.
func (s *Store) Read(day string, cursor int64, tenant string) (Page, error) {
	out := Page{Date: day, Today: s.Today(), Timezone: "UTC", Lines: []string{}}
	if !validDate(day) || cursor < -1 {
		return out, errors.New("invalid date or cursor")
	}
	path := s.path(day)
	s.mu.Lock()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		s.mu.Unlock()
		return out, nil
	}
	if err != nil {
		s.mu.Unlock()
		return out, err
	}
	if !info.Mode().IsRegular() {
		s.mu.Unlock()
		return out, errors.New("log path is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		s.mu.Unlock()
		return out, err
	}
	actual, err := file.Stat()
	s.mu.Unlock()
	defer file.Close()
	if err != nil || !os.SameFile(info, actual) {
		return out, errors.New("log file changed during open")
	}
	size := actual.Size()
	if cursor > size {
		cursor = 0
	}
	partial := false
	if cursor < 0 {
		cursor = 0
		if size > MaxReadBytes {
			cursor = size - MaxReadBytes
			partial = true
		}
	}
	start := cursor
	if cursor > 0 {
		previous := []byte{0}
		if _, err := file.ReadAt(previous, cursor-1); err == nil {
			partial = previous[0] != '\n'
		}
	}
	incomplete := false
	reader := bufio.NewReaderSize(io.NewSectionReader(file, start, min(MaxReadBytes, size-start)), 64*1024)
	for cursor-start < MaxReadBytes {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			if cursor+int64(len(line)) < size {
				cursor += int64(len(line))
			} else {
				incomplete = true
			}
			break
		}
		cursor += int64(len(line))
		if partial {
			partial = false
			continue
		}
		if len(line) <= 64*1024 && visible(line, tenant) {
			out.Lines = append(out.Lines, strings.TrimSuffix(line, "\n"))
		}
	}
	out.Cursor = cursor
	out.More = cursor < size && !incomplete
	return out, nil
}
