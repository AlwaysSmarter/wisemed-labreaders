package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) handleServerCommand(msg Envelope) (map[string]interface{}, error) {
	command := strings.ToLower(strings.TrimSpace(asString(msg.Payload["command"])))
	args := mapPayload(msg.Payload["args"])

	switch command {
	case "server.status":
		return map[string]interface{}{
			"service":     "wsm-server",
			"connections": len(s.hub.Snapshot()),
			"now_utc":     time.Now().UTC(),
		}, nil
	case "wisemed.ensure_equipment_online":
		reader := mapPayload(args["reader"])
		if len(reader) == 0 {
			return nil, errors.New("reader payload is required")
		}
		return s.wiseMedPut("/administrative/analyzer?XDEBUG_TRIGGER=debug", reader)
	case "wisemed.fetch_file_for_analyzer":
		fileID := strings.TrimSpace(asString(args["file_id"]))
		equipmentID := strings.TrimSpace(asString(args["equipment_id"]))
		if fileID == "" {
			return nil, errors.New("file_id is required")
		}
		if equipmentID == "" {
			return nil, errors.New("equipment_id is required")
		}
		path := "/fileforanalyzer/" + url.PathEscape(fileID) + "/" + url.PathEscape(equipmentID) + "/?XDEBUG_TRIGGER=debug"
		return s.wiseMedGet(path)
	default:
		return nil, errors.New("unsupported server command")
	}
}

func (s *Server) wiseMedGet(path string) (map[string]interface{}, error) {
	return s.doWiseMedJSON(http.MethodGet, path, nil)
}

func (s *Server) wiseMedPut(path string, payload interface{}) (map[string]interface{}, error) {
	return s.doWiseMedJSON(http.MethodPut, path, payload)
}

func (s *Server) doWiseMedJSON(method, path string, payload interface{}) (map[string]interface{}, error) {
	baseURL := strings.TrimSpace(s.cfg.WiseMed.BaseURL)
	apiKey := strings.TrimSpace(s.cfg.WiseMed.APIKey)
	if baseURL == "" {
		return nil, errors.New("wisemed base_url is not configured")
	}
	if apiKey == "" {
		return nil, errors.New("wisemed api_key is not configured")
	}

	targetURL := strings.TrimRight(baseURL, "/") + path
	var body io.Reader
	if payload != nil {
		blob, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(blob)
	}

	req, err := http.NewRequest(method, targetURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	token, err := createWiseMedJWT(apiKey)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	blob, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("wisemed http %d: %s", resp.StatusCode, strings.TrimSpace(string(blob)))
	}
	if len(bytes.TrimSpace(blob)) == 0 {
		return map[string]interface{}{}, nil
	}

	var raw interface{}
	if err := json.Unmarshal(blob, &raw); err != nil {
		return nil, err
	}
	if item, ok := raw.(map[string]interface{}); ok {
		return item, nil
	}
	return map[string]interface{}{"data": raw}, nil
}

func createWiseMedJWT(secret string) (string, error) {
	now := time.Now().UTC()
	headerJSON, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claimsJSON, _ := json.Marshal(map[string]interface{}{
		"caller_id":   "WSM-Server",
		"caller_type": "WSMServer",
		"exp":         now.Add(5 * time.Minute).Unix(),
	})
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	claims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	unsigned := header + "." + claims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + signature, nil
}

func asString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		return ""
	}
}

func mapPayload(value interface{}) map[string]interface{} {
	item, _ := value.(map[string]interface{})
	if item == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(item))
	for key, val := range item {
		out[key] = val
	}
	return out
}
