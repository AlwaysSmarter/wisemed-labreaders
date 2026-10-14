package htmlprintformatter

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsAndValidation(t *testing.T) {
	for _, key := range []string{"CHROME_PATH", "MAX_CONCURRENT_RENDERS", "MAX_BODY_BYTES", "DEFAULT_RENDER_TIMEOUT_MS"} {
		t.Setenv(key, "")
	}
	s, err := ReadSettings(nil)
	if err != nil || s.MaxConcurrent != 2 || s.MaxBodyBytes != 4194304 || s.DefaultTimeoutMS != 120000 {
		t.Fatalf("%+v %v", s, err)
	}
	t.Setenv("MAX_CONCURRENT_RENDERS", "3")
	t.Setenv("DEFAULT_RENDER_TIMEOUT_MS", "45000")
	s, err = ReadSettings(map[string]interface{}{"max_concurrent_renders": 7})
	if err != nil || s.MaxConcurrent != 3 || s.DefaultTimeoutMS != 45000 {
		t.Fatalf("%+v %v", s, err)
	}
	for _, req := range []Request{{}, {HTML: "x", HTMLLink: "https://example.org"}, {HTMLLink: "file:///etc/passwd"}, {HTML: "x", BaseURL: "javascript:alert(1)"}, {HTML: "x", TimeoutMS: -1}, {HTML: "x", TimeoutMS: 600001}, {HTML: "x", ViewportWidth: 99999}, {HTML: "x", ViewportHeight: -1}} {
		if req.validate(s) == nil {
			t.Fatalf("accepted %+v", req)
		}
	}
	t.Setenv("MAX_CONCURRENT_RENDERS", "0")
	if _, err = ReadSettings(nil); err == nil {
		t.Fatal("accepted zero concurrency")
	}
}

func TestBasePreservesSource(t *testing.T) {
	for _, source := range []string{`<!DOCTYPE html><html><head><script>const x='<base href="bad">';</script></head><body>x</body></html>`, `<!DOCTYPE html><html><script>1 < 2</script></html>`, `<!DOCTYPE html><title>x</title>`, `<html><head><template><base href="ignored"></template></head></html>`} {
		out := withBase(source, "https://example.org/?a=1&b=2")
		if strings.Replace(out, `<base href="https://example.org/?a=1&amp;b=2">`, "", 1) != source {
			t.Fatalf("source rewritten: %s", out)
		}
	}
	source := `<!DOCTYPE html><HTML><HEAD><BASE HREF='/old'><script>1<2</script></HEAD></HTML>`
	if withBase(source, "https://example.org") != source {
		t.Fatal("existing base modified")
	}
}

func TestHTTPValidation(t *testing.T) {
	s := Settings{MaxConcurrent: 1, MaxBodyBytes: 128, DefaultTimeoutMS: 1000}
	m := &Module{settings: s}
	for _, tc := range []struct {
		method, ct, body string
		status           int
		code             string
	}{
		{"GET", "application/json", `{}`, 405, "method_not_allowed"},
		{"POST", "text/plain", `{}`, 415, "unsupported_media_type"},
		{"POST", "application/json", `{"html":`, 400, "invalid_json"},
		{"POST", "application/json", `{"html":"x","unknown":true}`, 400, "invalid_json"},
		{"POST", "application/json", `{"html":"x"} {}`, 400, "invalid_json"},
		{"POST", "application/json", `{}`, 400, "invalid_request"},
		{"POST", "application/json", `{"html":"` + strings.Repeat("x", 150) + `"}`, 413, "body_too_large"},
		{"POST", "application/json", `{"html":"x"}`, 503, "browser_unavailable"},
	} {
		req := httptest.NewRequest(tc.method, "/render", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", tc.ct)
		w := httptest.NewRecorder()
		m.handleRender(w, req)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) || w.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%+v: %d %s", tc, w.Code, w.Body.String())
		}
	}
}
