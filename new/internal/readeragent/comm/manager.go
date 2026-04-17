package comm

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"wisemed-labreaders/new/internal/readeragent/storage"
	"wisemed-labreaders/new/protocols/astm"
)

type Manager struct {
	store            *storage.SQLiteStore
	analyzerCode     string
	worklistResolver func(sampleID string, tags []string) ([]string, error)

	mu        sync.RWMutex
	running   bool
	transport string
	mode      string
	details   map[string]interface{}

	cancel chan struct{}
	wg     sync.WaitGroup
	seen   map[string]time.Time
}

func NewManager(store *storage.SQLiteStore, analyzerCode string) *Manager {
	return &Manager{store: store, analyzerCode: analyzerCode, seen: map[string]time.Time{}}
}

func (m *Manager) SetWorklistResolver(resolve func(sampleID string, tags []string) ([]string, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.worklistResolver = resolve
}

func (m *Manager) StartFromConfig() error {
	cfg, err := m.store.GetCommunicationConfig(m.analyzerCode)
	if err != nil {
		return err
	}
	return m.Start(cfg)
}

func (m *Manager) Start(cfg *storage.CommunicationConfig) error {
	if cfg == nil {
		return errors.New("missing communication config")
	}
	m.Stop()
	m.mu.Lock()
	m.cancel = make(chan struct{})
	m.running = true
	m.transport = cfg.Transport
	m.mode = cfg.Mode
	m.details = cfg.Settings
	m.mu.Unlock()

	switch cfg.Transport {
	case "network":
		ip, _ := cfg.Settings["ip"].(string)
		port := intFromAny(cfg.Settings["port"])
		addr := net.JoinHostPort(ip, itoa(port))
		if cfg.Mode == "server" {
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				m.runTCPServer(addr)
			}()
			return nil
		}
		if cfg.Mode == "client" {
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				m.runTCPClient(addr)
			}()
			return nil
		}
		return errors.New("unsupported network mode")
	case "file":
		dir, _ := cfg.Settings["directory"].(string)
		mask, _ := cfg.Settings["mask"].(string)
		poll := intFromAny(cfg.Settings["poll_seconds"])
		if poll <= 0 {
			poll = 2
		}
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.runFilePoll(dir, mask, time.Duration(poll)*time.Second)
		}()
		return nil
	case "serial":
		// TODO: add physical RS232 adapter loop (can be implemented with go.bug.st/serial)
		_ = m.store.AppendEvent("comm_serial_not_implemented", cfg.Settings)
		return nil
	default:
		return errors.New("unsupported transport")
	}
}

func (m *Manager) Stop() {
	m.mu.Lock()
	if m.cancel != nil {
		close(m.cancel)
		m.cancel = nil
	}
	m.running = false
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *Manager) Restart() error {
	cfg, err := m.store.GetCommunicationConfig(m.analyzerCode)
	if err != nil {
		return err
	}
	return m.Start(cfg)
}

func (m *Manager) Status() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]interface{}{
		"running":   m.running,
		"transport": m.transport,
		"mode":      m.mode,
		"details":   m.details,
	}
}

func (m *Manager) runTCPServer(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		_ = m.store.AppendEvent("comm_error", map[string]interface{}{"transport": "network", "mode": "server", "error": err.Error()})
		return
	}
	defer ln.Close()
	_ = m.store.AppendEvent("comm_started", map[string]interface{}{"transport": "network", "mode": "server", "addr": addr})

	for {
		select {
		case <-m.cancel:
			return
		default:
		}
		_ = ln.(*net.TCPListener).SetDeadline(time.Now().Add(2 * time.Second))
		conn, err := ln.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			continue
		}
		m.handleStream(conn)
		_ = conn.Close()
	}
}

func (m *Manager) runTCPClient(addr string) {
	_ = m.store.AppendEvent("comm_started", map[string]interface{}{"transport": "network", "mode": "client", "addr": addr})
	for {
		select {
		case <-m.cancel:
			return
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		m.handleStream(conn)
		_ = conn.Close()
		time.Sleep(1 * time.Second)
	}
}

func (m *Manager) handleStream(conn net.Conn) {
	r := bufio.NewReader(conn)
	var b strings.Builder
	for {
		select {
		case <-m.cancel:
			return
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		chunk, err := r.ReadString('\r')
		if chunk != "" {
			b.WriteString(chunk)
			if strings.HasPrefix(strings.TrimSpace(chunk), "L|") || strings.Contains(chunk, "EOT") {
				outbound := m.consumePayload(b.String())
				for _, msg := range outbound {
					_, _ = conn.Write([]byte(msg))
				}
				b.Reset()
			}
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if errors.Is(err, io.EOF) {
				if b.Len() > 0 {
					outbound := m.consumePayload(b.String())
					for _, msg := range outbound {
						_, _ = conn.Write([]byte(msg))
					}
				}
			}
			return
		}
	}
}

func (m *Manager) runFilePoll(dir, mask string, every time.Duration) {
	_ = m.store.AppendEvent("comm_started", map[string]interface{}{"transport": "file", "mode": "polling", "directory": dir, "mask": mask})
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-m.cancel:
			return
		case <-tick.C:
			pattern := filepath.Join(dir, mask)
			files, _ := filepath.Glob(pattern)
			for _, p := range files {
				st, err := os.Stat(p)
				if err != nil {
					continue
				}
				if prev, ok := m.seen[p]; ok && !st.ModTime().After(prev) {
					continue
				}
				raw, err := os.ReadFile(p)
				if err != nil {
					continue
				}
				m.seen[p] = st.ModTime()
				_ = m.consumePayload(string(raw))
			}
		}
	}
}

func (m *Manager) consumePayload(raw string) []string {
	outbound := make([]string, 0)
	queries := astm.ParseQueryRequests(raw)
	for _, q := range queries {
		resp := m.handleQueryRequest(q)
		if resp != "" {
			outbound = append(outbound, resp)
		}
	}
	items := astm.ParseResultBatch(raw)
	if len(items) == 0 {
		return outbound
	}
	for _, it := range items {
		if err := m.store.EnqueueResult(it); err != nil {
			log.Printf("enqueue result failed: %v", err)
		}
	}
	_ = m.store.AppendEvent("astm_results_parsed", map[string]interface{}{"count": len(items)})
	return outbound
}

func (m *Manager) handleQueryRequest(q astm.QueryRequest) string {
	_ = m.store.AppendEvent("astm_query_received", map[string]interface{}{
		"sample_id": q.SampleID,
		"tags":      q.Tags,
	})
	m.mu.RLock()
	resolve := m.worklistResolver
	m.mu.RUnlock()
	if resolve == nil {
		_ = m.store.AppendEvent("astm_query_skipped", map[string]interface{}{
			"sample_id": q.SampleID,
			"reason":    "resolver_not_configured",
		})
		return ""
	}
	approved, err := resolve(q.SampleID, q.Tags)
	if err != nil {
		_ = m.store.AppendEvent("astm_query_failed", map[string]interface{}{
			"sample_id": q.SampleID,
			"error":     err.Error(),
		})
		return ""
	}
	_ = m.store.AppendEvent("astm_query_resolved", map[string]interface{}{
		"sample_id":     q.SampleID,
		"requested":     q.Tags,
		"approved_tags": approved,
	})
	return astm.BuildWorklistResponse(q.SampleID, approved)
}

func intFromAny(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	buf := [20]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
