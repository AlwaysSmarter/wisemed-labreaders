package runner

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"wisemed-labreaders/readersv3/core/config"
)

func setupLogging(cfg *config.Config, showLog bool) (string, func(), error) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	logDir := filepath.Dir(cfg.Path())
	if logDir == "." || logDir == "" {
		logDir = "."
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", nil, err
	}
	if err := archiveCompletedLogWeeks(logDir, logBaseName(cfg), time.Now()); err != nil {
		return "", nil, err
	}
	fileName := fmt.Sprintf("%s-%s.log", logBaseName(cfg), time.Now().Format("20060102"))
	logPath := filepath.Join(logDir, fileName)
	fh, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", nil, err
	}

	var writer io.Writer = fh
	if showLog {
		writer = io.MultiWriter(os.Stdout, fh)
	}
	log.SetOutput(writer)
	return logPath, func() { _ = fh.Close() }, nil
}

func logBaseName(cfg *config.Config) string {
	for _, value := range []string{cfg.Reader.ID, cfg.Reader.AnalyzerCode, cfg.Reader.Label, "reader"} {
		if token := sanitizeLogToken(value); token != "" {
			return token
		}
	}
	return "reader"
}

func sanitizeLogToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", ";", "-", ",", "-", ".", "-", "(", "", ")", "", "[", "", "]", "", "{", "", "}", "", "\"", "", "'", "")
	value = replacer.Replace(value)
	return strings.Trim(value, "-_")
}

type datedLogFile struct {
	path string
	name string
	day  time.Time
}

func archiveCompletedLogWeeks(logDir, base string, now time.Time) error {
	files, err := os.ReadDir(logDir)
	if err != nil {
		return err
	}
	currentWeekStart := weekStart(now)
	weekly := map[string][]datedLogFile{}
	for _, entry := range files {
		if entry.IsDir() {
			continue
		}
		item, ok := parseDatedLogFile(logDir, base, entry.Name())
		if !ok {
			continue
		}
		if !item.day.Before(currentWeekStart) {
			continue
		}
		key := weekStart(item.day).Format("20060102")
		weekly[key] = append(weekly[key], item)
	}

	weekKeys := make([]string, 0, len(weekly))
	for key := range weekly {
		weekKeys = append(weekKeys, key)
	}
	sort.Strings(weekKeys)

	for _, key := range weekKeys {
		items := weekly[key]
		sort.Slice(items, func(i, j int) bool { return items[i].day.Before(items[j].day) })
		from := items[0].day.Format("20060102")
		to := items[len(items)-1].day.Format("20060102")
		archiveName := fmt.Sprintf("%s-%s-%s.zip", base, from, to)
		archivePath := filepath.Join(logDir, archiveName)
		if _, err := os.Stat(archivePath); err == nil {
			if err := removeFiles(items); err != nil {
				return err
			}
			continue
		}
		if err := writeZipArchive(archivePath, items); err != nil {
			return err
		}
		if err := removeFiles(items); err != nil {
			return err
		}
	}
	return nil
}

func parseDatedLogFile(logDir, base, name string) (datedLogFile, bool) {
	prefix := base + "-"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".log") {
		return datedLogFile{}, false
	}
	dateToken := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".log")
	day, err := time.Parse("20060102", dateToken)
	if err != nil {
		return datedLogFile{}, false
	}
	return datedLogFile{
		path: filepath.Join(logDir, name),
		name: name,
		day:  day,
	}, true
}

func weekStart(day time.Time) time.Time {
	normalized := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	weekday := int(normalized.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return normalized.AddDate(0, 0, -(weekday - 1))
}

func writeZipArchive(path string, items []datedLogFile) error {
	fh, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()

	zw := zip.NewWriter(fh)
	for _, item := range items {
		src, err := os.Open(item.path)
		if err != nil {
			_ = zw.Close()
			return err
		}
		writer, err := zw.Create(item.name)
		if err != nil {
			_ = src.Close()
			_ = zw.Close()
			return err
		}
		if _, err := io.Copy(writer, src); err != nil {
			_ = src.Close()
			_ = zw.Close()
			return err
		}
		_ = src.Close()
	}
	return zw.Close()
}

func removeFiles(items []datedLogFile) error {
	for _, item := range items {
		if err := os.Remove(item.path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
