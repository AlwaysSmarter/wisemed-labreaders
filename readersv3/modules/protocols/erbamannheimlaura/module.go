// Package erbamannheimlaura reads the STX/ETX print reports produced by LAURA.
package erbamannheimlaura

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
	"unicode/utf8"

	"go.bug.st/serial"
	"wisemed-labreaders/readersv3/core/module"
	model "wisemed-labreaders/readersv3/modules/core/model"
	"wisemed-labreaders/readersv3/shared/analyzeractivity"
	"wisemed-labreaders/readersv3/shared/commtrace"
)

var tags = []string{"BLD", "BIL", "UBG", "KET", "GLU", "PRO", "pH", "NIT", "LEU", "SG"}
var names = []string{"Sânge", "Bilirubină", "Urobilinogen", "Corpi cetonici", "Glucoză", "Proteine", "pH", "Nitriți", "Leucocite", "Densitate urinară"}

type report struct {
	sample, date, measured, sequence string
	records                          []model.ImportedRecord
}

// A partial report must never replace a complete urine panel.
func parseReport(raw []byte) (report, error) {
	r := report{}
	if !utf8.Valid(raw) {
		return r, errors.New("invalid text encoding")
	}
	seen := map[string]bool{}
	header, footer := false, false
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r", "\n"), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "DEKAPHAN LAURA":
			header = true
			continue
		case strings.HasPrefix(line, "Seq.No:"):
			r.sequence = strings.TrimSpace(strings.TrimPrefix(line, "Seq.No:"))
			continue
		case strings.HasPrefix(line, "ID:"):
			r.sample = strings.TrimSpace(strings.TrimPrefix(line, "ID:"))
			continue
		case strings.HasPrefix(line, "----------------"):
			footer = true
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 2 {
			if dt, err := time.Parse("2006.01.02 15:04", strings.Join(fields, " ")); err == nil {
				r.date = dt.Format("2006-01-02")
				r.measured = strings.Join(fields, " ")
				continue
			}
		}
		if len(fields) < 2 {
			continue
		}
		tag := strings.TrimPrefix(fields[0], "*")
		for i, known := range tags {
			if tag != known {
				continue
			}
			if seen[tag] {
				return r, fmt.Errorf("duplicate analyte %s", tag)
			}
			seen[tag] = true
			r.records = append(r.records, model.ImportedRecord{AnalyteTag: strings.ToUpper(tag), AnalyteName: names[i], ResultValue: fields[1], RawValue: fields[1], Unit: strings.Join(fields[2:], " "), Flags: map[string]interface{}{"abnormal": strings.HasPrefix(fields[0], "*")}})
		}
	}
	if !header || !footer || r.sample == "" || r.date == "" || len(seen) != len(tags) {
		return r, errors.New("incomplete LAURA report: header, ID, date, footer and all 10 analytes are required")
	}
	for i := range r.records {
		r.records[i].SampleID = r.sample
		r.records[i].FileID = r.sample
		r.records[i].Meta = map[string]interface{}{"protocol": "erba-mannheim-laura", "sequence": r.sequence, "measured_at": r.measured}
	}
	return r, nil
}

// Decoder is per connection: arbitrary chunks, multiple reports and noise are safe.
type decoder struct {
	active bool
	frame  []byte
}

func (d *decoder) feed(data []byte, emit func([]byte)) {
	for _, b := range data {
		switch b {
		case 2:
			d.active = true
			d.frame = d.frame[:0]
		case 3:
			if d.active {
				emit(d.frame)
			}
			d.active = false
			d.frame = d.frame[:0]
		default:
			if d.active {
				if len(d.frame) >= 65536 {
					d.active = false
					d.frame = nil
				} else {
					d.frame = append(d.frame, b)
				}
			}
		}
	}
}

type importStore interface {
	RecordImportedPanel(string, []model.ImportedRecord, string) (int64, error)
	ReapplyOrderTransformations([]int64) error
	ListAnalytes() ([]model.Analyte, error)
	SaveAnalyte(model.Analyte) (model.Analyte, error)
}
type Module struct{ rt module.Runtime }

func New() module.Module                       { return &Module{} }
func (m *Module) ID() string                   { return "protocol-erba-mannheim-laura" }
func (m *Module) Init(rt module.Runtime) error { m.rt = rt; return nil }
func value(settings map[string]interface{}, key, fallback string) string {
	if v := settings[key]; v != nil && strings.TrimSpace(fmt.Sprint(v)) != "" {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return fallback
}
func number(settings map[string]interface{}, key string, fallback int) int {
	var n int
	if _, err := fmt.Sscanf(value(settings, key, ""), "%d", &n); err == nil && n > 0 {
		return n
	}
	return fallback
}
func (m *Module) Start(ctx context.Context) error {
	for ctx.Err() == nil {
		settings := m.rt.ModuleSettings("analyzer")
		if svc, ok := m.rt.Service("analyzer-config"); ok {
			if cfg, ok := svc.(map[string]interface{}); ok {
				settings = cfg
			}
		}
		var err error
		switch value(settings, "comm_type", "serial") {
		case "serial":
			err = m.runSerial(ctx)
		case "tcpip":
			err = m.runTCP(ctx)
		default:
			err = errors.New("unsupported communication type")
		}
		if ctx.Err() != nil {
			break
		}
		m.rt.Logf("Laura communication: %v", err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(3 * time.Second):
		}
	}
	return nil
}
func (m *Module) runSerial(ctx context.Context) error {
	cfg := m.rt.ModuleSettings("transport-serial")
	parity := map[string]serial.Parity{"none": serial.NoParity, "odd": serial.OddParity, "even": serial.EvenParity, "mark": serial.MarkParity, "space": serial.SpaceParity}
	p, ok := parity[strings.ToLower(value(cfg, "parity", "none"))]
	if !ok {
		return errors.New("invalid serial parity")
	}
	stops := map[string]serial.StopBits{"1": serial.OneStopBit, "1.5": serial.OnePointFiveStopBits, "2": serial.TwoStopBits}
	stop, ok := stops[value(cfg, "stop_bits", "1")]
	if !ok {
		return errors.New("invalid serial stop bits")
	}
	path := value(cfg, "port", "")
	if path == "" {
		return errors.New("configure transport-serial.port")
	}
	port, err := serial.Open(path, &serial.Mode{BaudRate: number(cfg, "baud", 9600), DataBits: number(cfg, "data_bits", 8), Parity: p, StopBits: stop})
	if err != nil {
		return err
	}
	defer port.Close()
	if err = port.SetReadTimeout(500 * time.Millisecond); err != nil {
		return err
	}
	return m.read(ctx, port, "serial:"+path, "serial")
}
func (m *Module) runTCP(ctx context.Context) error {
	cfg := m.rt.ModuleSettings("transport-tcpip")
	if value(cfg, "mode", "server") == "client" {
		conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(value(cfg, "remote_host", "127.0.0.1"), value(cfg, "remote_port", "5150")))
		if err != nil {
			return err
		}
		defer conn.Close()
		return m.readTCP(ctx, conn)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(value(cfg, "host", "127.0.0.1"), value(cfg, "port", "5150")))
	if err != nil {
		return err
	}
	defer listener.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			listener.Close()
		case <-done:
		}
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer conn.Close()
			if err := m.readTCP(ctx, conn); err != nil && ctx.Err() == nil {
				m.rt.Logf("Laura TCP: %v", err)
			}
		}()
	}
}
func (m *Module) readTCP(ctx context.Context, conn net.Conn) error {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	return m.read(ctx, conn, "tcp:"+conn.RemoteAddr().String(), "tcpip")
}
func (m *Module) read(ctx context.Context, reader io.Reader, source, transport string) error {
	var tracker *analyzeractivity.Tracker
	if svc, ok := m.rt.Service("analyzer-activity"); ok {
		tracker, _ = svc.(*analyzeractivity.Tracker)
	}
	if tracker != nil {
		tracker.Connected(1)
		defer tracker.Connected(-1)
	}
	d := decoder{}
	buf := make([]byte, 1024)
	for ctx.Err() == nil {
		n, err := reader.Read(buf)
		if n > 0 {
			if level := number(m.rt.ModuleSettings("logging"), "verbose_level", 1); level >= 4 {
				m.rt.Logf("%s", commtrace.Format("in", transport, source, buf[:n], level))
			}
			if tracker != nil {
				tracker.Packet("rx", transport)
			}
			d.feed(buf[:n], func(raw []byte) {
				if err := m.process(raw, source); err != nil {
					m.rt.Logf("Laura report rejected: %v", err)
				}
			})
		}
		if err != nil {
			return err
		}
	}
	return nil
}
func (m *Module) process(raw []byte, source string) error {
	r, err := parseReport(raw)
	if err != nil {
		return err
	}
	svc, _ := m.rt.Service("storage")
	store, ok := svc.(importStore)
	if !ok {
		return errors.New("storage unavailable")
	}
	known, err := store.ListAnalytes()
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for _, a := range known {
		existing[a.Tag] = true
	}
	for _, rec := range r.records {
		if !existing[rec.AnalyteTag] {
			if _, err := store.SaveAnalyte(model.Analyte{Tag: rec.AnalyteTag, Name: rec.AnalyteName, Active: true}); err != nil {
				return err
			}
		}
	}
	id, err := store.RecordImportedPanel(r.date, r.records, source)
	if err != nil {
		return err
	}
	if err = store.ReapplyOrderTransformations([]int64{id}); err != nil {
		return err
	}
	m.rt.Logf("Laura import ok: sample=%s results=%d", r.sample, len(r.records))
	return nil
}
