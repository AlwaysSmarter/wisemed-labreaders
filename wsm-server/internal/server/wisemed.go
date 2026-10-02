package server

import (
	"bytes"
	"context"
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
	"wisemed-labreaders/serverlast/wsm-server/internal/config"
)

func (s *Server) handleServerCommand(ctx context.Context, tenant config.Tenant, conn *Connection, msg Envelope) (map[string]interface{}, error) {
	command := strings.ToLower(strings.TrimSpace(asString(msg.Payload["command"])))
	args := mapPayload(msg.Payload["args"])

	switch command {
	case "equipment.status":
		ids := []string{}
		if id := asString(args["equipment_id"]); id != "" {
			ids = append(ids, id)
		}
		if list, ok := args["equipment_ids"].([]interface{}); ok {
			for _, id := range list {
				ids = append(ids, asString(id))
			}
		}
		if len(ids) == 0 || len(ids) > 100 {
			return nil, errors.New("1..100 equipment IDs required")
		}
		results := make([]map[string]interface{}, 0, len(ids))
		for _, id := range ids {
			if !config.ValidID(id) {
				return nil, errors.New("invalid equipment ID")
			}
			results = append(results, s.equipmentStatus(conn.TenantID, id))
		}
		return map[string]interface{}{"equipment": results}, nil
	case "equipment.list":
		return map[string]interface{}{"connections": s.hub.Snapshot(conn.TenantID)}, nil
	case "server.status":
		return map[string]interface{}{
			"service":     "wsm-server",
			"tenant_id":   conn.TenantID,
			"connections": len(s.hub.Snapshot(conn.TenantID)),
			"now_utc":     time.Now().UTC(),
		}, nil
	case "wisemed.ensure_equipment_online":
		reader := mapPayload(args["reader"])
		if len(reader) == 0 {
			return nil, errors.New("reader payload is required")
		}
		return s.doWiseMedJSON(ctx, tenant.WiseMed, http.MethodPut, "/administrative/analyzer", reader)
	case "wisemed.fetch_file_for_analyzer":
		fileID := strings.TrimSpace(asString(args["file_id"]))
		equipmentID := strings.TrimSpace(asString(args["equipment_id"]))
		if fileID == "" {
			return nil, errors.New("file_id is required")
		}
		if equipmentID == "" {
			return nil, errors.New("equipment_id is required")
		}
		path := "/fileforanalyzer/" + url.PathEscape(fileID) + "/" + url.PathEscape(equipmentID) + "/"
		return s.doWiseMedJSON(ctx, tenant.WiseMed, http.MethodGet, path, nil)
	default:
		return nil, errors.New("unsupported server command")
	}
}

func (s *Server) doWiseMedJSON(ctx context.Context, upstream config.WiseMed, method, path string, payload interface{}) (map[string]interface{}, error) {
	baseURL := strings.TrimSpace(upstream.BaseURL)
	apiKey := strings.TrimSpace(upstream.APIKey)
	if baseURL == "" || apiKey == "" {
		return nil, errors.New("WiseMED proxy is not configured for this tenant")
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

	req, err := http.NewRequestWithContext(ctx, method, targetURL, body)
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

	resp, err := s.upstreamClient.Do(req)
	if err != nil {
		return nil, errors.New("WiseMED upstream request failed")
	}
	defer resp.Body.Close()

	blob, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(blob) > 4*1024*1024 {
		return nil, errors.New("WiseMED response too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("WiseMED HTTP %d", resp.StatusCode)
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
