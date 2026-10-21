package siuihistory

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type debugRequest struct {
	At          string `json:"at"`
	Tenant      string `json:"-"`
	Bridge      string `json:"bridge"`
	IP          string `json:"ip"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Offset      string `json:"offset,omitempty"`
	Status      int    `json:"status"`
	Duration    int64  `json:"duration_ms"`
	Bytes       int64  `json:"bytes"`
	Error       string `json:"error,omitempty"`
	Operation   string `json:"operation,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
}
type debugContextKey struct{}
type debugBody struct {
	io.ReadCloser
	bytes int64
}

func (b *debugBody) Read(p []byte) (int, error) {
	n, e := b.ReadCloser.Read(p)
	b.bytes += int64(n)
	return n, e
}

type debugWriter struct {
	http.ResponseWriter
	status    int
	errorBody []byte
}

func (w *debugWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
		w.ResponseWriter.WriteHeader(code)
	}
}
func (w *debugWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	if w.status >= 400 && len(w.errorBody) < 8192 {
		w.errorBody = append(w.errorBody, data[:min(len(data), 8192-len(w.errorBody))]...)
	}
	return w.ResponseWriter.Write(data)
}
func debugIdentity(r *http.Request, tenant, bridge string) {
	if entry, ok := r.Context().Value(debugContextKey{}).(*debugRequest); ok {
		entry.Tenant = tenant
		entry.Bridge = bridge
	}
}
func (m *Module) debugAuthenticate(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || (r.URL.Path != "/api/history/identity" && !strings.HasPrefix(r.URL.Path, "/api/history/calls")) {
		m.authenticate(w, r)
		return
	}
	start := time.Now()
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	m.mu.Lock()
	tenant := m.cfg.Tenant
	m.mu.Unlock()
	entry := debugRequest{At: start.UTC().Format(time.RFC3339Nano), Tenant: tenant, IP: ip, Method: r.Method, Path: r.URL.Path, Offset: r.URL.Query().Get("offset")}
	body := &debugBody{ReadCloser: r.Body}
	r.Body = body
	recorder := &debugWriter{ResponseWriter: w}
	m.authenticate(recorder, r.WithContext(context.WithValue(r.Context(), debugContextKey{}, &entry)))
	entry.Status = recorder.status
	if entry.Status == 0 {
		entry.Status = 200
	}
	entry.Duration = time.Since(start).Milliseconds()
	entry.Bytes = body.bytes
	if entry.Status >= 400 {
		var response struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(recorder.errorBody, &response)
		entry.Error = response.Error
	}
	m.debugMu.Lock()
	if len(m.requests) >= 1000 {
		copy(m.requests, m.requests[1:])
		m.requests = m.requests[:999]
	}
	m.requests = append(m.requests, entry)
	m.debugMu.Unlock()
	m.rt.Logf("History bridge=%s method=%s path=%s status=%d bytes=%d duration_ms=%d error=%s", entry.Bridge, entry.Method, entry.Path, entry.Status, entry.Bytes, entry.Duration, entry.Error)
}
func (m *Module) debugRequests(w http.ResponseWriter, r *http.Request, tenant string) {
	bridge := r.URL.Query().Get("bridge")
	m.debugMu.Lock()
	defer m.debugMu.Unlock()
	entries := []debugRequest{}
	for i := len(m.requests) - 1; i >= 0; i-- {
		entry := m.requests[i]
		if entry.Tenant == tenant && (bridge == "" || entry.Bridge == bridge) {
			entries = append(entries, entry)
		}
	}
	output(w, 200, map[string]any{"success": true, "requests": entries})
}
