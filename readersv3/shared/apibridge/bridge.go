// Package apibridge adapts a JSON WSS command to the existing local HTTP mux.
// It never opens a loopback/network HTTP connection and never renders a UI.
package apibridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const MaxBody = 4 * 1024 * 1024

// Principal is server-attested identity, not taken from caller HTTP headers.
type Principal struct {
	Subject      string
	TenantID     string
	ConnectionID string
	Admin        bool
}
type principalKey struct{}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok && p.Subject != "" && p.TenantID != ""
}

type Request struct {
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	Body        json.RawMessage `json:"body,omitempty"`
	BodyBase64  string          `json:"body_base64,omitempty"`
	ContentType string          `json:"content_type,omitempty"`
}
type Response struct {
	Kind        string            `json:"kind"`
	Status      int               `json:"status"`
	ContentType string            `json:"content_type"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        interface{}       `json:"body,omitempty"`
	BodyBase64  string            `json:"body_base64,omitempty"`
}

func Invoke(ctx context.Context, handler http.Handler, p Principal, in Request) (out Response, err error) {
	if p.Subject == "" || p.TenantID == "" {
		return out, errors.New("verified WSS principal required")
	}
	u, e := url.ParseRequestURI(in.Path)
	if e != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(in.Path, "/") || strings.HasPrefix(in.Path, "//") || strings.ContainsAny(u.Path, "\\\x00") || strings.Contains(u.Path, "/../") || strings.Contains(u.Path, "/./") {
		return out, errors.New("invalid relative API path")
	}
	if !(strings.HasPrefix(u.Path, "/api/") || u.Path == "/barcode/print") {
		return out, errors.New("only data API routes are supported")
	}
	// Do not recursively bridge via the debug forwarding endpoint or create cookies.
	if strings.HasPrefix(u.Path, "/api/wss/bridge") || u.Path == "/api/wss/open" || u.Path == "/api/wss/debug" || u.Path == "/api/session/login" || u.Path == "/api/session/logout" {
		return out, errors.New("route not available through WSS")
	}
	method := strings.ToUpper(in.Method)
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD":
	default:
		return out, errors.New("unsupported API method")
	}
	body := []byte(in.Body)
	if in.BodyBase64 != "" {
		if len(body) > 0 {
			return out, errors.New("use body or body_base64")
		}
		if len(in.BodyBase64) > MaxBody*2 {
			return out, errors.New("request body too large")
		}
		body, e = base64.StdEncoding.DecodeString(in.BodyBase64)
		if e != nil {
			return out, errors.New("invalid body_base64")
		}
	}
	if len(body) > MaxBody {
		return out, errors.New("request body too large")
	}
	ct := in.ContentType
	if ct == "" {
		ct = "application/json"
	}
	if strings.ContainsAny(ct, "\r\n") {
		return out, errors.New("invalid content type")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, principalKey{}, p)
	req, e := http.NewRequestWithContext(ctx, method, in.Path, bytes.NewReader(body))
	if e != nil {
		return out, e
	}
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Accept", "application/json")
	req.RemoteAddr = "wss:" + p.ConnectionID
	recorder := &boundedRecorder{headers: make(http.Header)}
	defer func() {
		if recover() != nil {
			out = Response{}
			err = errors.New("API handler failed")
		}
	}()
	handler.ServeHTTP(recorder, req)
	if recorder.overflow {
		return out, errors.New("API response exceeds WSS bridge limit")
	}
	contentType := recorder.headers.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return out, errors.New("UI/HTML responses are not transported over WSS")
	}
	status := recorder.status
	if status == 0 {
		status = 200
	}
	out = Response{Kind: "api.response", Status: status, ContentType: contentType, Headers: map[string]string{}}
	for _, k := range []string{"Content-Disposition", "ETag", "Location"} {
		if v := recorder.headers.Get(k); v != "" {
			out.Headers[k] = v
		}
	}
	if recorder.body.Len() > 0 {
		if json.Valid(recorder.body.Bytes()) {
			var v interface{}
			d := json.NewDecoder(bytes.NewReader(recorder.body.Bytes()))
			d.UseNumber()
			if e = d.Decode(&v); e != nil {
				return out, e
			}
			if v == nil {
				// Keep explicit JSON null distinct from an empty HTTP response.
				v = json.RawMessage("null")
			}
			out.Body = v
		} else {
			out.BodyBase64 = base64.StdEncoding.EncodeToString(recorder.body.Bytes())
		}
	}
	return out, nil
}

type boundedRecorder struct {
	headers  http.Header
	status   int
	body     bytes.Buffer
	overflow bool
}

func (r *boundedRecorder) Header() http.Header { return r.headers }
func (r *boundedRecorder) WriteHeader(n int) {
	if r.status == 0 {
		r.status = n
	}
}
func (r *boundedRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = 200
	}
	if r.body.Len()+len(b) > MaxBody {
		r.overflow = true
		return 0, io.ErrShortBuffer
	}
	return r.body.Write(b)
}
