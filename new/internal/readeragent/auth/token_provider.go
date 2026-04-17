package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

type TokenProvider struct {
	AuthURL     string
	ClientID    string
	SecretRef   string
	StaticToken string
	HTTPClient  *http.Client
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

func (p *TokenProvider) GetToken() (string, error) {
	if p.StaticToken != "" {
		return p.StaticToken, nil
	}
	if p.AuthURL == "" {
		return "", errors.New("missing auth url")
	}
	secret := os.Getenv(p.SecretRef)
	if secret == "" {
		return "", fmt.Errorf("missing auth secret in env var %s", p.SecretRef)
	}
	if p.HTTPClient == nil {
		p.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	payload := map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     p.ClientID,
		"client_secret": secret,
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, p.AuthURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("auth endpoint returned %d", resp.StatusCode)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}
	if tr.AccessToken == "" {
		return "", errors.New("empty access_token from auth")
	}
	return tr.AccessToken, nil
}
