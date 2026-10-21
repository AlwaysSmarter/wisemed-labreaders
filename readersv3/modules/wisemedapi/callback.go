package wisemedapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// CallbackDestination binds queued work to the configured WiseMED instance.
func (m *Module) CallbackDestination() (string, error) {
	return callbackDestination(m.Settings())
}
func callbackDestination(settings map[string]string) (string, error) {
	base, err := makeURLFromSettings(settings, "")
	if err != nil {
		return "", err
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid WiseMED API destination")
	}
	return strings.TrimRight(base, "/"), nil
}

// SendCallback never follows redirects or logs patient data, tokens or bodies.
func (m *Module) SendCallback(ctx context.Context, destination, method, path, id, lt string, body []byte) (int, error) {
	settings := m.Settings()
	base, err := callbackDestination(settings)
	if err != nil || base != destination {
		return 0, errors.New("WiseMED destination changed or unavailable")
	}
	if method != http.MethodPost && method != http.MethodPut {
		return 0, errors.New("unsupported callback method")
	}
	u, err := url.Parse(path)
	if err != nil || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || strings.Contains(path, "\\") {
		return 0, errors.New("invalid callback path")
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return 0, errors.New("invalid callback path")
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
	if err != nil {
		return 0, errors.New("invalid callback request")
	}
	// Only the token explicitly supplied for this job may establish user context.
	delete(settings, "login_token")
	delete(settings, "login_user_id")
	if lt != "" {
		settings["login_token"] = lt
	}
	token, err := createJWTForSettings(settings, m.callerType())
	if err != nil {
		return 0, errors.New("WiseMED authentication unavailable")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", id)
	client := *m.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return 0, errors.New("WiseMED callback transport failed")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 65536))
	return resp.StatusCode, nil
}
