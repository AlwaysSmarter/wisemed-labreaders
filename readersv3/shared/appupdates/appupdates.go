package appupdates

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Claims struct {
	AppID    string   `json:"app_id,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
	IssuedAt int64    `json:"iat,omitempty"`
	Expires  int64    `json:"exp,omitempty"`
}

type CheckRequest struct {
	AppID          string `json:"app_id"`
	CurrentVersion string `json:"current_version,omitempty"`
	OS             string `json:"os,omitempty"`
	Arch           string `json:"arch,omitempty"`
	Channel        string `json:"channel,omitempty"`
}

type CheckResponse struct {
	OK              bool   `json:"ok"`
	Status          string `json:"status"`
	Message         string `json:"message,omitempty"`
	AppID           string `json:"app_id,omitempty"`
	CurrentVersion  string `json:"current_version,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
	Mandatory       bool   `json:"mandatory,omitempty"`
	DownloadURL     string `json:"download_url,omitempty"`
	ChecksumSHA256  string `json:"checksum_sha256,omitempty"`
	FileName        string `json:"file_name,omitempty"`
	ReleaseNotes    string `json:"release_notes,omitempty"`
	VersionRecordID int64  `json:"version_record_id,omitempty"`
}

type Client struct {
	BaseURL string
	APIKey  string
	AppID   string
	Channel string
	Client  *http.Client
}

type DownloadProgress struct {
	ReceivedBytes int64
	TotalBytes    int64
	Percent       float64
}

func NewClient(baseURL, apiKey, appID, channel string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:  strings.TrimSpace(apiKey),
		AppID:   strings.TrimSpace(appID),
		Channel: strings.TrimSpace(channel),
		Client:  &http.Client{Timeout: 30 * time.Minute},
	}
}

func (c *Client) Check(currentVersion, osName, arch string) (CheckResponse, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return CheckResponse{}, errors.New("update base url is empty")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return CheckResponse{}, errors.New("update api key is empty")
	}
	token, err := SignJWT(c.APIKey, Claims{
		AppID:    c.AppID,
		Scopes:   []string{"check", "download"},
		IssuedAt: time.Now().Unix(),
		Expires:  time.Now().Add(10 * time.Minute).Unix(),
	})
	if err != nil {
		return CheckResponse{}, err
	}
	payload := CheckRequest{
		AppID:          c.AppID,
		CurrentVersion: strings.TrimSpace(currentVersion),
		OS:             firstNonEmpty(strings.TrimSpace(osName), runtime.GOOS),
		Arch:           firstNonEmpty(strings.TrimSpace(arch), runtime.GOARCH),
		Channel:        c.Channel,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return CheckResponse{}, err
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/public/check-update", bytes.NewReader(body))
	if err != nil {
		return CheckResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "wisemed-readersv3-updater/1.0")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return CheckResponse{}, err
	}
	defer resp.Body.Close()
	var out CheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return CheckResponse{}, err
	}
	if resp.StatusCode >= 300 {
		return out, fmt.Errorf(firstNonEmpty(out.Message, "update check failed"))
	}
	return out, nil
}

func (c *Client) Download(downloadURL, targetDir string) (string, string, error) {
	return c.DownloadWithProgress(downloadURL, targetDir, nil)
}

func (c *Client) DownloadWithProgress(downloadURL, targetDir string, onProgress func(DownloadProgress)) (string, string, error) {
	if strings.TrimSpace(downloadURL) == "" {
		return "", "", errors.New("download url is empty")
	}
	if strings.TrimSpace(targetDir) == "" {
		targetDir = "."
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", "", err
	}
	token, err := SignJWT(c.APIKey, Claims{
		AppID:    c.AppID,
		Scopes:   []string{"download"},
		IssuedAt: time.Now().Unix(),
		Expires:  time.Now().Add(30 * time.Minute).Unix(),
	})
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequest(http.MethodGet, resolveURL(c.BaseURL, downloadURL), nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "wisemed-readersv3-updater/1.0")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		blob, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("download failed: %s", strings.TrimSpace(string(blob)))
	}
	fileName := downloadFileName(resp, downloadURL)
	targetPath := filepath.Join(targetDir, fileName)
	tmpPath := targetPath + ".part"
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", "", err
	}
	hasher := sha256.New()
	source := io.Reader(resp.Body)
	if onProgress != nil {
		source = &progressReader{
			reader:     resp.Body,
			total:      resp.ContentLength,
			onProgress: onProgress,
		}
	}
	if _, err := io.Copy(io.MultiWriter(f, hasher), source); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return "", "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(targetPath, 0o755)
	}
	return targetPath, hex.EncodeToString(hasher.Sum(nil)), nil
}

type progressReader struct {
	reader       io.Reader
	total        int64
	received     int64
	lastReported int64
	onProgress   func(DownloadProgress)
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.received += int64(n)
		shouldReport := r.total <= 0 || r.received == r.total || r.received-r.lastReported >= 256*1024
		if shouldReport && r.onProgress != nil {
			percent := 0.0
			if r.total > 0 {
				percent = (float64(r.received) / float64(r.total)) * 100
			}
			r.onProgress(DownloadProgress{
				ReceivedBytes: r.received,
				TotalBytes:    r.total,
				Percent:       percent,
			})
			r.lastReported = r.received
		}
	}
	return n, err
}

func SignJWT(secret string, claims Claims) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", errors.New("jwt secret is empty")
	}
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	sig := signHMACSHA256(secret, body)
	return body + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func VerifyJWT(secret, token string) (Claims, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Claims{}, errors.New("missing token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, errors.New("invalid token format")
	}
	body := parts[0] + "." + parts[1]
	expected := signHMACSHA256(secret, body)
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, errors.New("invalid token signature")
	}
	if !hmac.Equal(expected, got) {
		return Claims{}, errors.New("invalid token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, errors.New("invalid token payload")
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, errors.New("invalid token payload")
	}
	if claims.Expires > 0 && time.Now().Unix() > claims.Expires {
		return Claims{}, errors.New("token expired")
	}
	return claims, nil
}

func CompareVersions(a, b string) int {
	left := tokenizeVersion(a)
	right := tokenizeVersion(b)
	limit := len(left)
	if len(right) > limit {
		limit = len(right)
	}
	for i := 0; i < limit; i++ {
		la := tokenAt(left, i)
		rb := tokenAt(right, i)
		if la.isNumber && rb.isNumber {
			if la.num < rb.num {
				return -1
			}
			if la.num > rb.num {
				return 1
			}
			continue
		}
		if la.raw < rb.raw {
			return -1
		}
		if la.raw > rb.raw {
			return 1
		}
	}
	return 0
}

func NormalizeVersion(v string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V"))
}

func ResolveBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func resolveURL(baseURL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	baseURL = ResolveBaseURL(baseURL)
	if baseURL == "" {
		return raw
	}
	if strings.HasPrefix(raw, "/") {
		return baseURL + raw
	}
	return baseURL + "/" + raw
}

func signHMACSHA256(secret, value string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

type versionToken struct {
	raw      string
	num      int64
	isNumber bool
}

func tokenizeVersion(v string) []versionToken {
	clean := NormalizeVersion(v)
	if clean == "" {
		return nil
	}
	clean = strings.NewReplacer("-", ".", "_", ".", "+", ".").Replace(clean)
	parts := strings.Split(clean, ".")
	out := make([]versionToken, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			out = append(out, versionToken{raw: ""})
			continue
		}
		if n, err := strconv.ParseInt(part, 10, 64); err == nil {
			out = append(out, versionToken{raw: part, num: n, isNumber: true})
			continue
		}
		out = append(out, versionToken{raw: strings.ToLower(part)})
	}
	return out
}

func tokenAt(items []versionToken, index int) versionToken {
	if index < 0 || index >= len(items) {
		return versionToken{raw: "", num: 0, isNumber: true}
	}
	return items[index]
}

func downloadFileName(resp *http.Response, rawURL string) string {
	cd := strings.TrimSpace(resp.Header.Get("Content-Disposition"))
	if strings.Contains(strings.ToLower(cd), "filename=") {
		for _, part := range strings.Split(cd, ";") {
			part = strings.TrimSpace(part)
			if !strings.HasPrefix(strings.ToLower(part), "filename=") {
				continue
			}
			name := strings.Trim(strings.TrimPrefix(part, "filename="), `"`)
			name = filepath.Base(strings.TrimSpace(name))
			if name != "" && name != "." && name != string(filepath.Separator) {
				return name
			}
		}
	}
	if parsed, err := url.Parse(rawURL); err == nil {
		base := filepath.Base(parsed.Path)
		if base != "" && base != "." && base != "/" {
			return base
		}
	}
	return fmt.Sprintf("update-%d.bin", time.Now().Unix())
}

func (c *Client) httpClient() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return &http.Client{Timeout: 90 * time.Second}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
