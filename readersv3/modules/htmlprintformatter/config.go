package htmlprintformatter

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const MaxTimeoutMS = 600000

type Settings struct {
	ChromePath       string
	MaxConcurrent    int
	MaxBodyBytes     int64
	DefaultTimeoutMS int
}

func ReadSettings(raw map[string]interface{}) (Settings, error) {
	s := Settings{MaxConcurrent: 2, MaxBodyBytes: 4 << 20, DefaultTimeoutMS: 120000}
	value := func(key, env string) string {
		if v, ok := os.LookupEnv(env); ok {
			return v
		}
		if v, ok := raw[key]; ok {
			return fmt.Sprint(v)
		}
		return ""
	}
	s.ChromePath = value("chrome_path", "CHROME_PATH")
	for _, item := range []struct {
		key, env string
		dst      *int
		max      int
	}{
		{"max_concurrent_renders", "MAX_CONCURRENT_RENDERS", &s.MaxConcurrent, 64},
		{"default_render_timeout_ms", "DEFAULT_RENDER_TIMEOUT_MS", &s.DefaultTimeoutMS, MaxTimeoutMS},
	} {
		if v := value(item.key, item.env); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > item.max {
				return s, fmt.Errorf("%s must be between 1 and %d", item.env, item.max)
			}
			*item.dst = n
		}
	}
	if v := value("max_body_bytes", "MAX_BODY_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 || n > 64<<20 {
			return s, fmt.Errorf("MAX_BODY_BYTES must be between 1 and 67108864")
		}
		s.MaxBodyBytes = n
	}
	return s, nil
}

type Request struct {
	WaitForPrintReady bool   `json:"wait_for_print_ready"`
	HTML              string `json:"html"`
	HTMLLink          string `json:"html_link"`
	BaseURL           string `json:"base_url"`
	TimeoutMS         int    `json:"timeout_ms"`
	ViewportWidth     int    `json:"viewport_width"`
	ViewportHeight    int    `json:"viewport_height"`
}

func (r *Request) validate(s Settings) error {
	if (strings.TrimSpace(r.HTML) == "") == (strings.TrimSpace(r.HTMLLink) == "") {
		return failure("invalid_request", "Provide exactly one of html or html_link", 400)
	}
	for _, v := range []string{r.HTMLLink, r.BaseURL} {
		if v == "" {
			continue
		}
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
			return failure("invalid_request", "html_link and base_url must be absolute HTTP(S) URLs without credentials", 400)
		}
	}
	if int64(len(r.HTML)) > s.MaxBodyBytes {
		return failure("body_too_large", "HTML exceeds max_body_bytes", 413)
	}
	if r.TimeoutMS == 0 {
		r.TimeoutMS = s.DefaultTimeoutMS
	}
	if r.ViewportWidth == 0 {
		r.ViewportWidth = 1280
	}
	if r.ViewportHeight == 0 {
		r.ViewportHeight = 900
	}
	if r.TimeoutMS < 1 || r.TimeoutMS > MaxTimeoutMS || r.ViewportWidth < 1 || r.ViewportWidth > 8192 || r.ViewportHeight < 1 || r.ViewportHeight > 8192 {
		return failure("invalid_request", "timeout_ms must be 1..600000; viewport dimensions must be 1..8192 (zero uses defaults)", 400)
	}
	return nil
}

type RenderError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *RenderError) Error() string { return e.Code + ": " + e.Message }
func failure(code, message string, status int) *RenderError {
	return &RenderError{code, message, status}
}
