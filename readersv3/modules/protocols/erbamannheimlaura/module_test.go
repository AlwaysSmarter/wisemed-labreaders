package erbamannheimlaura

import (
	"bytes"
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"
	"wisemed-labreaders/readersv3/core/module"
	model "wisemed-labreaders/readersv3/modules/core/model"
)

func capture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/report.txt")
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestCaptureAndChunking(t *testing.T) {
	raw := capture(t)
	for _, size := range []int{1, 7, 1024} {
		d := decoder{}
		count := 0
		input := append([]byte("noise"), raw...)
		input = append(input, raw...)
		for len(input) > 0 {
			n := size
			if n > len(input) {
				n = len(input)
			}
			d.feed(input[:n], func(frame []byte) {
				count++
				r, err := parseReport(frame)
				if err != nil {
					t.Fatal(err)
				}
				if r.sample != "21009" || r.date != "2026-05-05" || len(r.records) != 10 {
					t.Fatalf("unexpected report: %+v", r)
				}
				for _, rec := range r.records {
					if rec.AnalyteTag == "PRO" && (rec.ResultValue != "100" || rec.Unit != "mg/dl" || rec.Flags["abnormal"] != true) {
						t.Fatalf("protein: %+v", rec)
					}
					if rec.AnalyteTag == "SG" && rec.ResultValue != "1.000" {
						t.Fatalf("SG: %+v", rec)
					}
				}
			})
			input = input[n:]
		}
		if count != 2 {
			t.Fatalf("chunk %d: got %d reports", size, count)
		}
	}
}
func TestRejectIncompleteAndCorrupt(t *testing.T) {
	raw := bytes.Trim(capture(t), "\x02\x03\r\n")
	for _, bad := range [][]byte{bytes.Replace(raw, []byte("ID: 21009"), []byte("ID:"), 1), bytes.Replace(raw, []byte(" LEU      NEG"), nil, 1), append(raw, []byte("\nPRO 30")...), []byte{0xff, 0xfe}} {
		if _, err := parseReport(bad); err == nil {
			t.Fatalf("accepted invalid report %q", bad)
		}
	}
	corrupt, err := os.ReadFile("testdata/corrupt.txt")
	if err != nil {
		t.Fatal(err)
	}
	d := decoder{}
	d.feed(corrupt, func(b []byte) {
		if _, err := parseReport(b); err == nil {
			t.Fatal("accepted corrupt capture")
		}
	})
	d.feed([]byte("\x02"+strings.Repeat("x", 65537)+"\x03"), func([]byte) { t.Fatal("accepted oversized frame") })
	d.feed(capture(t), func(b []byte) {
		if _, err := parseReport(b); err != nil {
			t.Fatal(err)
		}
	})
}

type testStore struct{ imported chan []model.ImportedRecord }

func (s *testStore) RecordImportedPanel(_ string, r []model.ImportedRecord, _ string) (int64, error) {
	s.imported <- r
	return 1, nil
}
func (*testStore) ReapplyOrderTransformations([]int64) error          { return nil }
func (*testStore) ListAnalytes() ([]model.Analyte, error)             { return nil, nil }
func (*testStore) SaveAnalyte(a model.Analyte) (model.Analyte, error) { return a, nil }

type testRuntime struct {
	module.Runtime
	store *testStore
}

func (r testRuntime) ModuleSettings(string) map[string]interface{} { return nil }
func (r testRuntime) Logf(string, ...interface{})                  {}
func (r testRuntime) Service(name string) (interface{}, bool) {
	if name == "storage" {
		return r.store, true
	}
	return nil, false
}
func TestTCPStreamImportAndShutdown(t *testing.T) {
	store := &testStore{imported: make(chan []model.ImportedRecord, 2)}
	m := &Module{rt: testRuntime{store: store}}
	server, client := net.Pipe()
	defer client.Close()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.readTCP(ctx, server) }()
	raw := capture(t)
	if err := client.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	for _, b := range raw {
		if _, err := client.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case records := <-store.imported:
		if len(records) != 10 || records[0].SampleID != "21009" {
			t.Fatal(records)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no imported panel")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("TCP read did not stop on cancellation")
	}
}
