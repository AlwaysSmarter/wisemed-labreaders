package wisemedapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"wisemed-labreaders/readersv3/core/module"
)

type Module struct {
	rt module.Runtime

	mu       sync.RWMutex
	settings map[string]string
	client   *http.Client
}

type LoginRequest struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	MedicalUnitID string `json:"medical_unit_id"`
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
}

type LoginResponse struct {
	UserID             json.Number `json:"user_id"`
	Login              string      `json:"login"`
	FirstName          string      `json:"first_name"`
	LastName           string      `json:"last_name"`
	UserType           int         `json:"user_type"`
	UserEmail          string      `json:"user_email"`
	LoginToken         string      `json:"login_token"`
	UserPicture        string      `json:"user_picture"`
	MobilePrefix       string      `json:"user_mobile_country_prefix"`
	MobileNumber       string      `json:"user_mobile_number"`
	GlobalCacheVersion string      `json:"global_cache_version"`
}

type apiError struct {
	Message      string `json:"message"`
	ErrorCode    string `json:"error_code"`
	ErrorContext string `json:"error_context"`
}

type BootstrapClient struct {
	Settings   map[string]string
	CallerType string
	Client     *http.Client
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "wisemed-api" }

func NewBootstrapClient(settings map[string]string, callerType string) *BootstrapClient {
	items := map[string]string{}
	for k, v := range settings {
		items[k] = strings.TrimSpace(v)
	}
	applyBaseURLFallbackToSettings(items)
	if strings.TrimSpace(callerType) == "" {
		callerType = "Undefined"
	}
	return &BootstrapClient{
		Settings:   items,
		CallerType: callerType,
		Client:     &http.Client{Timeout: 20 * time.Second},
	}
}

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	m.settings = readStringSettings(rt.ModuleSettings(m.ID()))
	m.applyBaseURLFallback()
	m.client = &http.Client{Timeout: 20 * time.Second}
	rt.RegisterService("wisemed-api", m)
	rt.Handle("/api/wisemed/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":                   true,
			"module":               m.ID(),
			"configured":           m.IsConfigured(),
			"setup_complete":       m.SetupComplete(),
			"equipment_registered": m.HasEquipmentID(),
			"features":             []string{"administrative-login", "administrative-analyzer", "medicalunits", "wmanalyzertypes"},
		})
	}))
	return nil
}

func (m *Module) Settings() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]string, len(m.settings))
	for k, v := range m.settings {
		out[k] = v
	}
	return out
}

func (m *Module) PublicSettings() map[string]string {
	return m.Settings()
}

func (m *Module) IsConfigured() bool {
	settings := m.Settings()
	required := []string{"cfg_wisemed_protocol", "cfg_wisemed_ip", "cfg_wisemed_port", "cfg_wisemed_path", "cfg_wisemed_key"}
	for _, key := range required {
		if strings.TrimSpace(settings[key]) == "" {
			return false
		}
	}
	return true
}

func (m *Module) SetupComplete() bool {
	settings := m.Settings()
	if !m.IsConfigured() {
		return false
	}
	required := []string{"unitate_medicala_id", "tip_de_echipament_id", "cod_echipament", "numar_serial_echipament"}
	for _, key := range required {
		if strings.TrimSpace(settings[key]) == "" {
			return false
		}
	}
	return true
}

func (m *Module) HasEquipmentID() bool {
	value := strings.TrimSpace(m.Settings()["echipament_id"])
	if value == "" {
		return false
	}
	id, err := strconv.Atoi(value)
	return err == nil && id > 0
}

func (m *Module) SaveSetup(next map[string]string) (map[string]string, error) {
	m.mu.Lock()
	for key, value := range next {
		m.settings[key] = strings.TrimSpace(value)
	}
	m.mu.Unlock()
	if err := m.persistSettingsToConfig(); err != nil {
		return nil, err
	}
	return m.Settings(), nil
}

func (m *Module) Bootstrap() (map[string]interface{}, error) {
	resp := map[string]interface{}{
		"configured":           m.IsConfigured(),
		"setup_complete":       m.SetupComplete(),
		"equipment_registered": m.HasEquipmentID(),
		"settings":             m.PublicSettings(),
		"medical_units":        []map[string]interface{}{},
		"analyzer_types":       []map[string]interface{}{},
	}
	if !m.IsConfigured() {
		return resp, nil
	}
	medicalUnits, err := m.getJSONArray("/administrative/medicalunits")
	if err != nil {
		return nil, err
	}
	analyzerTypes, err := m.getJSONArray("/administrative/wmanalyzertypes")
	if err != nil {
		return nil, err
	}
	resp["medical_units"] = medicalUnits
	resp["analyzer_types"] = analyzerTypes
	return resp, nil
}

func (m *Module) Login(req LoginRequest) (LoginResponse, error) {
	if !m.SetupComplete() {
		return LoginResponse{}, errors.New("WiseMED setup is incomplete")
	}
	settings := m.Settings()
	if strings.TrimSpace(req.MedicalUnitID) == "" {
		req.MedicalUnitID = settings["unitate_medicala_id"]
	}
	if strings.TrimSpace(req.DeviceID) == "" {
		req.DeviceID = m.rt.ReaderID()
	}
	if strings.TrimSpace(req.DeviceName) == "" {
		req.DeviceName = firstNonEmpty(readerString(m.rt, "analyzer_name"), readerString(m.rt, "label"), m.rt.ReaderID())
	}
	var raw interface{}
	if err := m.putJSON("/administrative/login", req, &raw); err != nil {
		return LoginResponse{}, err
	}
	resp, err := parseLoginResponse(raw)
	if err != nil {
		return LoginResponse{}, err
	}
	if strings.TrimSpace(resp.LoginToken) == "" {
		return LoginResponse{}, errors.New("administrative/login did not return login_token")
	}
	if _, err := m.SaveSetup(map[string]string{"login_token": strings.TrimSpace(resp.LoginToken)}); err != nil {
		return LoginResponse{}, err
	}
	return resp, nil
}

func (m *Module) EnsureEquipmentOnline(reader map[string]interface{}) (map[string]interface{}, error) {
	if !m.SetupComplete() {
		return nil, errors.New("WiseMED setup is incomplete")
	}
	payload := m.analyzerPayload(reader)
	resp := map[string]interface{}{}
	if err := m.putJSON("/administrative/analyzer", payload, &resp); err != nil {
		return nil, err
	}
	updates := map[string]string{}
	for _, key := range []string{
		"cod_echipament",
		"nume_echipament",
		"api_key_echipament",
		"numar_serial_echipament",
		"echipament_id",
		"unitate_medicala_id",
		"tip_de_echipament_id",
	} {
		if value := strings.TrimSpace(asString(resp[key])); value != "" {
			switch key {
			case "cod_echipament":
				updates["cod_echipament"] = value
			case "api_key_echipament":
				updates["api_key_echipament"] = value
			case "numar_serial_echipament":
				updates["numar_serial_echipament"] = value
			case "echipament_id":
				updates["echipament_id"] = value
			case "unitate_medicala_id":
				updates["unitate_medicala_id"] = value
			case "tip_de_echipament_id":
				updates["tip_de_echipament_id"] = value
			}
		}
	}
	if len(updates) > 0 {
		if _, err := m.SaveSetup(updates); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func (m *Module) FetchFileForAnalyzer(fileID, equipmentID string) (map[string]interface{}, error) {
	fileID = strings.TrimSpace(fileID)
	equipmentID = strings.TrimSpace(equipmentID)
	if fileID == "" {
		return nil, errors.New("file id is required")
	}
	if equipmentID == "" {
		return nil, errors.New("equipment id is required")
	}
	endpoint := "/fileforanalyzer/" + url.PathEscape(fileID) + "/" + url.PathEscape(equipmentID) + "/?XDEBUG_TRIGGER=debug"
	var raw interface{}
	if err := m.doJSON(http.MethodGet, endpoint, nil, &raw); err != nil {
		return nil, err
	}
	if resp, ok := raw.(map[string]interface{}); ok {
		return resp, nil
	}
	return map[string]interface{}{"data": raw}, nil
}

func (m *Module) applyBaseURLFallback() {
	m.mu.Lock()
	defer m.mu.Unlock()
	applyBaseURLFallbackToSettings(m.settings)
}

func (m *Module) persistSettingsToConfig() error {
	path := m.rt.ConfigPath()
	if strings.TrimSpace(path) == "" {
		return errors.New("config path unavailable")
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	cfg := map[string]interface{}{}
	if err := yaml.Unmarshal(blob, &cfg); err != nil {
		return err
	}
	modules, _ := cfg["modules"].(map[string]interface{})
	if modules == nil {
		modules = map[string]interface{}{}
		cfg["modules"] = modules
	}
	section := map[string]interface{}{}
	m.mu.RLock()
	for k, v := range m.settings {
		section[k] = v
	}
	m.mu.RUnlock()
	modules[m.ID()] = section
	updated, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, updated, 0o644)
}

func (m *Module) makeURL(path string) (string, error) {
	settings := m.Settings()
	protocol := strings.TrimSpace(settings["cfg_wisemed_protocol"])
	host := strings.TrimSpace(settings["cfg_wisemed_ip"])
	port := strings.TrimSpace(settings["cfg_wisemed_port"])
	basePath := strings.TrimSpace(settings["cfg_wisemed_path"])
	if protocol == "" || host == "" || port == "" || basePath == "" {
		return "", errors.New("WiseMED API configuration is incomplete")
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	return fmt.Sprintf("%s://%s:%s%s%s", protocol, host, port, strings.TrimRight(basePath, "/"), path), nil
}

func (m *Module) createJWT() (string, error) {
	settings := m.Settings()
	secret := strings.TrimSpace(settings["cfg_wisemed_key"])
	if secret == "" {
		return "", errors.New("WiseMED API key is missing")
	}
	headerJSON, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claimsJSON, _ := json.Marshal(map[string]interface{}{
		"caller_id":   "WM-Lab-Reader",
		"caller_type": m.callerType(),
		"exp":         time.Now().Add(5 * time.Minute).Unix(),
	})
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	claims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	unsigned := header + "." + claims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + signature, nil
}

func (m *Module) doJSON(method, path string, payload interface{}, out interface{}) error {
	return doJSONWithClient(m.client, m.Settings(), m.callerType(), method, path, payload, out)
}

func (m *Module) putJSON(path string, payload interface{}, out interface{}) error {
	return m.doJSON(http.MethodPut, path, payload, out)
}

func (m *Module) getJSONArray(path string) ([]map[string]interface{}, error) {
	var raw interface{}
	if err := m.doJSON(http.MethodGet, path, nil, &raw); err != nil {
		return nil, err
	}
	items, ok := raw.([]interface{})
	if !ok {
		return []map[string]interface{}{}, nil
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]interface{}); ok {
			out = append(out, row)
		}
	}
	return out, nil
}

func (m *Module) analyzerPayload(reader map[string]interface{}) map[string]interface{} {
	settings := m.Settings()
	addr := firstNonEmpty(localHTTPString(m.rt, "address"), "127.0.0.1:18080")
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host = "127.0.0.1"
		port = "18080"
	}
	return map[string]interface{}{
		"cod_echipament":          firstNonEmpty(settings["cod_echipament"], asString(reader["analyzer_code"]), m.rt.ReaderID()),
		"nume_echipament":         firstNonEmpty(asString(reader["analyzer_name"]), asString(reader["label"]), m.rt.ReaderID()),
		"api_key_echipament":      settings["api_key_echipament"],
		"producator_echipament":   "thinkIT",
		"tip_analizor":            m.callerType(),
		"numar_serial_echipament": settings["numar_serial_echipament"],
		"ip":                      host,
		"port":                    port,
		"online":                  true,
		"nr_rackuri":              "0",
		"pozitii_pe_rack":         "0",
		"echipament_id":           settings["echipament_id"],
		"unitate_medicala_id":     settings["unitate_medicala_id"],
		"tip_de_echipament_id":    settings["tip_de_echipament_id"],
	}
}

func (m *Module) callerType() string {
	switch strings.ToLower(strings.TrimSpace(analyzerString(m.rt, "protocol"))) {
	case "seegene-excel", "beosl-csv":
		return "Microbiology"
	case "cary60-uvvis", "generic-file":
		return "Biochemestry"
	case "astm":
		return "Immunology"
	default:
		return "Undefined"
	}
}

func readStringSettings(raw map[string]interface{}) map[string]string {
	out := map[string]string{}
	for k, v := range raw {
		switch t := v.(type) {
		case string:
			out[k] = t
		case int:
			out[k] = strconv.Itoa(t)
		case int64:
			out[k] = strconv.FormatInt(t, 10)
		case float64:
			out[k] = strconv.FormatFloat(t, 'f', -1, 64)
		case bool:
			if t {
				out[k] = "1"
			} else {
				out[k] = "0"
			}
		}
	}
	return out
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return strings.TrimSpace(item)
		}
	}
	return ""
}

func asString(value interface{}) string {
	switch t := value.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(value)
	}
}

func readerString(rt module.Runtime, key string) string {
	service, ok := rt.Service("reader-config")
	if !ok {
		return ""
	}
	cfg, ok := service.(map[string]interface{})
	if !ok {
		return ""
	}
	return strings.TrimSpace(asString(cfg[key]))
}

func analyzerString(rt module.Runtime, key string) string {
	service, ok := rt.Service("analyzer-config")
	if !ok {
		return ""
	}
	cfg, ok := service.(map[string]interface{})
	if !ok {
		return ""
	}
	return strings.TrimSpace(asString(cfg[key]))
}

func localHTTPString(rt module.Runtime, key string) string {
	settings := rt.ModuleSettings("local-http")
	return strings.TrimSpace(asString(settings[key]))
}

func (c *BootstrapClient) ListMedicalUnits() ([]map[string]interface{}, error) {
	var raw interface{}
	if err := doJSONWithClient(c.httpClient(), c.Settings, c.CallerType, http.MethodGet, "/administrative/medicalunits", nil, &raw); err != nil {
		return nil, err
	}
	return normalizeJSONArray(raw), nil
}

func (c *BootstrapClient) ListEquipmentTypes() ([]map[string]interface{}, error) {
	var raw interface{}
	if err := doJSONWithClient(c.httpClient(), c.Settings, c.CallerType, http.MethodGet, "/administrative/wmanalyzertypes", nil, &raw); err != nil {
		return nil, err
	}
	return normalizeJSONArray(raw), nil
}

func (c *BootstrapClient) RegisterEquipment(payload map[string]interface{}) (map[string]interface{}, error) {
	resp := map[string]interface{}{}
	if err := doJSONWithClient(c.httpClient(), c.Settings, c.CallerType, http.MethodPut, "/administrative/analyzer", payload, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *BootstrapClient) httpClient() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func applyBaseURLFallbackToSettings(settings map[string]string) {
	rawHost := strings.TrimSpace(settings["cfg_wisemed_ip"])
	rawPath := strings.TrimSpace(settings["cfg_wisemed_path"])
	if rawHost != "" {
		if strings.Contains(rawHost, "://") {
			if parsed, err := url.Parse(rawHost); err == nil {
				if parsed.Scheme != "" && strings.TrimSpace(settings["cfg_wisemed_protocol"]) == "" {
					settings["cfg_wisemed_protocol"] = parsed.Scheme
				}
				if parsed.Hostname() != "" {
					settings["cfg_wisemed_ip"] = parsed.Hostname()
				}
				if parsed.Port() != "" && strings.TrimSpace(settings["cfg_wisemed_port"]) == "" {
					settings["cfg_wisemed_port"] = parsed.Port()
				}
				if parsed.Path != "" && parsed.Path != "/" {
					settings["cfg_wisemed_path"] = joinAPIPaths(parsed.Path, rawPath)
				}
			}
		} else if strings.Contains(rawHost, "/") {
			hostPart := rawHost
			pathPart := ""
			if idx := strings.Index(rawHost, "/"); idx >= 0 {
				hostPart = rawHost[:idx]
				pathPart = rawHost[idx:]
			}
			settings["cfg_wisemed_ip"] = strings.TrimSpace(hostPart)
			if pathPart != "" {
				settings["cfg_wisemed_path"] = joinAPIPaths(pathPart, rawPath)
			}
		}
	}
	if strings.TrimSpace(settings["cfg_wisemed_protocol"]) != "" {
		if strings.TrimSpace(settings["cfg_wisemed_path"]) == "" {
			settings["cfg_wisemed_path"] = "/api"
		}
		return
	}
	baseURL := strings.TrimSpace(settings["base_url"])
	if baseURL == "" {
		return
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return
	}
	settings["cfg_wisemed_protocol"] = parsed.Scheme
	if host := parsed.Hostname(); host != "" {
		settings["cfg_wisemed_ip"] = host
	}
	if port := parsed.Port(); port != "" {
		settings["cfg_wisemed_port"] = port
	}
	path := strings.TrimSpace(parsed.Path)
	if path == "" {
		path = "/api"
	}
	settings["cfg_wisemed_path"] = path
}

func doJSONWithClient(client *http.Client, settings map[string]string, callerType, method, path string, payload interface{}, out interface{}) error {
	target, err := makeURLFromSettings(settings, path)
	if err != nil {
		return err
	}
	var body io.Reader
	if payload != nil {
		blob, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(blob)
	}
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	token, err := createJWTForSettings(settings, callerType)
	if err != nil {
		return err
	}
	req.Header.Set("authorization", token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := apiError{}
		if err := json.Unmarshal(raw, &apiErr); err == nil && strings.TrimSpace(apiErr.Message) != "" {
			return errors.New(apiErr.Message)
		}
		if trimmed := strings.TrimSpace(string(raw)); trimmed != "" {
			return errors.New(trimmed)
		}
		return fmt.Errorf("%d %s", resp.StatusCode, resp.Status)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func makeURLFromSettings(settings map[string]string, path string) (string, error) {
	applyBaseURLFallbackToSettings(settings)
	protocol := strings.TrimSpace(settings["cfg_wisemed_protocol"])
	host := strings.TrimSpace(settings["cfg_wisemed_ip"])
	port := strings.TrimSpace(settings["cfg_wisemed_port"])
	basePath := strings.TrimSpace(settings["cfg_wisemed_path"])
	if protocol == "" || host == "" || port == "" || basePath == "" {
		return "", errors.New("WiseMED API configuration is incomplete")
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	return fmt.Sprintf("%s://%s:%s%s%s", protocol, host, port, strings.TrimRight(basePath, "/"), path), nil
}

func joinAPIPaths(parts ...string) string {
	out := ""
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.HasPrefix(part, "/") {
			part = "/" + part
		}
		out += strings.TrimRight(part, "/")
	}
	if out == "" {
		return "/api"
	}
	return out
}

func createJWTForSettings(settings map[string]string, callerType string) (string, error) {
	secret := strings.TrimSpace(settings["cfg_wisemed_key"])
	if secret == "" {
		return "", errors.New("WiseMED API key is missing")
	}
	if strings.TrimSpace(callerType) == "" {
		callerType = "Undefined"
	}
	headerJSON, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claimsMap := map[string]interface{}{
		"caller_id":   "WM-Lab-Reader",
		"caller_type": callerType,
		"exp":         time.Now().Add(5 * time.Minute).Unix(),
	}
	if token := strings.TrimSpace(settings["login_token"]); token != "" {
		claimsMap["lt"] = token
	}
	claimsJSON, _ := json.Marshal(claimsMap)
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	claims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	unsigned := header + "." + claims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + signature, nil
}

func normalizeJSONArray(raw interface{}) []map[string]interface{} {
	if wrapper, ok := raw.(map[string]interface{}); ok {
		for _, key := range []string{"rows", "data", "items", "result"} {
			if nested, ok := wrapper[key]; ok {
				return normalizeJSONArray(nested)
			}
		}
	}
	items, ok := raw.([]interface{})
	if !ok {
		return []map[string]interface{}{}
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]interface{}); ok {
			out = append(out, row)
		}
	}
	return out
}

func parseLoginResponse(raw interface{}) (LoginResponse, error) {
	item, ok := raw.(map[string]interface{})
	if !ok {
		blob, err := json.Marshal(raw)
		if err != nil {
			return LoginResponse{}, err
		}
		if err := json.Unmarshal(blob, &item); err != nil {
			return LoginResponse{}, err
		}
	}
	loginToken := firstNonEmpty(
		strings.TrimSpace(asString(item["login_token"])),
		strings.TrimSpace(asString(item["token"])),
		strings.TrimSpace(asString(item["lt"])),
	)
	return LoginResponse{
		UserID:             json.Number(strings.TrimSpace(asString(item["user_id"]))),
		Login:              strings.TrimSpace(asString(item["login"])),
		FirstName:          strings.TrimSpace(asString(item["first_name"])),
		LastName:           strings.TrimSpace(asString(item["last_name"])),
		UserType:           intFrom(item, "user_type"),
		UserEmail:          strings.TrimSpace(asString(item["user_email"])),
		LoginToken:         loginToken,
		UserPicture:        strings.TrimSpace(asString(item["user_picture"])),
		MobilePrefix:       strings.TrimSpace(asString(item["user_mobile_country_prefix"])),
		MobileNumber:       strings.TrimSpace(asString(item["user_mobile_number"])),
		GlobalCacheVersion: strings.TrimSpace(asString(item["global_cache_version"])),
	}, nil
}

func intFrom(m map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch x := v.(type) {
			case int:
				return x
			case int64:
				return int(x)
			case float64:
				return int(x)
			case json.Number:
				if n, err := x.Int64(); err == nil {
					return int(n)
				}
			case string:
				n, err := strconv.Atoi(strings.TrimSpace(x))
				if err == nil {
					return n
				}
			}
		}
	}
	return 0
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
