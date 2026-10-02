package wisemedapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// WSSAccessToken uses an operator-configured relative API route. The WiseMED
// backend must implement it and authorize the device before returning {token:...}.
func (m *Module) WSSAccessToken(ctx context.Context, path string, payload map[string]interface{}) (string, error) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "?#") {
		return "", errors.New("configure a relative WiseMED token_path")
	}
	raw, _ := json.Marshal(payload)
	target, e := m.makeURL(path)
	if e != nil {
		return "", e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(raw))
	if e != nil {
		return "", e
	}
	token, e := m.createJWT()
	if e != nil {
		return "", e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := *m.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, e := client.Do(req)
	if e != nil {
		return "", errors.New("WiseMED WSS token request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", errors.New("WiseMED WSS token request rejected")
	}
	var result struct {
		Token string `json:"token"`
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 16384)).Decode(&result); e != nil || result.Token == "" {
		return "", errors.New("WiseMED did not return a WSS token")
	}
	return result.Token, nil
}
