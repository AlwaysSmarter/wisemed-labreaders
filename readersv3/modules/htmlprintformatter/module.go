package htmlprintformatter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"wisemed-labreaders/readersv3/core/module"
)

type Module struct {
	rt       module.Runtime
	settings Settings
	mu       sync.Mutex
	renderer *Renderer
	requests sync.WaitGroup
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "html-print-formatter" }
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	var err error
	m.settings, err = ReadSettings(rt.ModuleSettings(m.ID()))
	if err != nil {
		return err
	}
	port := os.Getenv("PORT")
	if port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return errors.New("PORT must be between 1 and 65535")
		}
	}
	// Configure the shared listener; do not start a competing utility server.
	rt.RegisterService("http-server-options", func(s *http.Server) {
		if port != "" {
			host, _, _ := net.SplitHostPort(s.Addr)
			s.Addr = net.JoinHostPort(host, port)
		}
		s.ReadTimeout = 30 * time.Second
		s.WriteTimeout = (MaxTimeoutMS + 60000) * time.Millisecond
		s.IdleTimeout = 60 * time.Second
		s.MaxHeaderBytes = 64 << 10
	})
	for _, path := range []string{"/render", "/render/html", "/render/url", "/api/html-print-formatter/render", "/pdf", "/api/html-print-formatter/pdf"} {
		rt.Handle(path, http.HandlerFunc(m.handleRender))
	}
	rt.Handle("/api/html-print-formatter/token", http.HandlerFunc(m.handleToken))
	rt.Handle("/health", http.HandlerFunc(m.handleHealth))
	rt.Handle("/api/html-print-formatter/health", http.HandlerFunc(m.handleHealth))
	rt.AddMenu(module.MenuEntry{ID: "html-print-formatter", Group: "primary", Label: "HTML Print Formatter", Path: "/help/", Order: 10})
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	r, err := NewRenderer(ctx, m.settings, m.rt.Logf)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.renderer = r
	m.mu.Unlock()
	m.rt.Logf("html-print-formatter ready concurrency=%d timeout_ms=%d max_body_bytes=%d", m.settings.MaxConcurrent, m.settings.DefaultTimeoutMS, m.settings.MaxBodyBytes)
	<-ctx.Done()
	m.mu.Lock()
	m.renderer = nil
	m.mu.Unlock()
	m.requests.Wait()
	r.Close()
	return nil
}

func (m *Module) handleHealth(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeError(w, failure("method_not_allowed", "Use GET", 405), "")
		return
	}
	m.mu.Lock()
	ready := m.renderer != nil && m.renderer.Healthy()
	m.mu.Unlock()
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]interface{}{"ok": ready, "service": m.ID()})
}

func (m *Module) handleRender(w http.ResponseWriter, req *http.Request) {
	id := uuid.NewString()
	w.Header().Set("X-Request-ID", id)
	w.Header().Set("Cache-Control", "no-store")
	if req.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, failure("method_not_allowed", "Use POST", 405), id)
		return
	}
	if err := m.authorize(req); err != nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="html-print-formatter"`)
		writeError(w, err, id)
		return
	}
	ct, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil || ct != "application/json" {
		writeError(w, failure("unsupported_media_type", "Use Content-Type: application/json", 415), id)
		return
	}
	req.Body = http.MaxBytesReader(w, req.Body, m.settings.MaxBodyBytes)
	defer req.Body.Close()
	d := json.NewDecoder(req.Body)
	d.DisallowUnknownFields()
	var in Request
	err = d.Decode(&in)
	if err == nil {
		var extra interface{}
		if e := d.Decode(&extra); e != io.EOF {
			if e == nil {
				e = errors.New("only one JSON object is allowed")
			}
			err = e
		}
	}
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, failure("body_too_large", "JSON body exceeds max_body_bytes", 413), id)
		} else {
			writeError(w, failure("invalid_json", err.Error(), 400), id)
		}
		return
	}
	if err := in.validate(m.settings); err != nil {
		writeError(w, err, id)
		return
	}
	if (req.URL.Path == "/render/html" && in.HTML == "") || (req.URL.Path == "/render/url" && in.HTMLLink == "") {
		writeError(w, failure("invalid_request", "This endpoint requires html or html_link respectively", 400), id)
		return
	}
	m.mu.Lock()
	r := m.renderer
	if r != nil {
		m.requests.Add(1)
	}
	m.mu.Unlock()
	if r == nil {
		writeError(w, failure("browser_unavailable", "Chromium is starting or service is shutting down; check /health", 503), id)
		return
	}
	defer m.requests.Done()
	var html string
	var pdf []byte
	pdfRequest := req.URL.Path == "/pdf" || req.URL.Path == "/api/html-print-formatter/pdf"
	if pdfRequest {
		html, pdf, err = r.RenderPDF(req.Context(), in, id)
	} else {
		html, err = r.Render(req.Context(), in, id)
	}
	if err != nil {
		writeError(w, err, id)
		return
	}
	if pdfRequest {
		if req.URL.Path == "/api/html-print-formatter/pdf" {
			writeJSON(w, 200, map[string]interface{}{"ok": true, "html": html, "pdf_base64": base64.StdEncoding.EncodeToString(pdf), "filename": "rendered.pdf", "request_id": id})
		} else {
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", `attachment; filename="rendered.pdf"`)
			w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
			_, _ = w.Write(pdf)
		}
		return
	}
	if req.URL.Path == "/api/html-print-formatter/render" {
		writeJSON(w, 200, map[string]interface{}{"ok": true, "html": html, "request_id": id})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, html)
}

func writeError(w http.ResponseWriter, err error, id string) {
	var re *RenderError
	if !errors.As(err, &re) {
		re = failure("render_failed", err.Error(), 502)
	}
	writeJSON(w, re.Status, map[string]interface{}{"ok": false, "error": re, "request_id": id})
}
func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
