package wisemedapi

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
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
)

type DosimetryEntry struct {
	Serial string
	HP10   string
	HP007  string
}

type DosimetryResult struct {
	Success bool   `json:"success"`
	Serial  string `json:"serial"`
	Error   string `json:"error"`
}

type DosimetryResponse struct {
	Success bool              `json:"success"`
	Results []DosimetryResult `json:"results"`
}

type ServiceResultEntry struct {
	FSMID          string
	Result         string
	Interpretation string
	Conclusion     string
}

type Module struct {
	rt module.Runtime

	mu       sync.RWMutex
	settings map[string]string
	client   *http.Client
}

type analyteStore interface {
	ListAnalytes() ([]coremodel.Analyte, error)
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

func (m *Module) Start(ctx context.Context) error {
	if m.SetupComplete() {
		if _, err := m.EnsureEquipmentOnline(nil); err != nil {
			m.rt.Logf("wisemed-api startup sync failed: %v", err)
		}
	}
	<-ctx.Done()
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
	if err := m.putJSON("/administrative/analyzer?XDEBUG_TRIGGER=debug", payload, &resp); err != nil {
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

func (m *Module) SaveDosimetry(entries []DosimetryEntry) (*DosimetryResponse, error) {
	if len(entries) == 0 {
		return nil, errors.New("dosimetry entries are required")
	}
	var raw interface{}
	if err := m.doForm(http.MethodPatch, "/file/services/dosimetry/?XDEBUG_TRIGGER=debug", buildDosimetryForm(entries), &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, errors.New("dosimetry endpoint returned empty response")
	}
	blob, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var out DosimetryResponse
	if err := json.Unmarshal(blob, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (m *Module) SaveFileServiceResults(fileID string, entries []ServiceResultEntry) (map[string]interface{}, error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil, errors.New("file id is required")
	}
	form := url.Values{}
	index := 0
	for _, item := range entries {
		fsmID := strings.TrimSpace(item.FSMID)
		if fsmID == "" {
			continue
		}
		key := strconv.Itoa(index)
		form.Set("srv["+key+"][fsmid]", fsmID)
		form.Set("srv["+key+"][result]", strings.TrimSpace(item.Result))
		form.Set("srv["+key+"][interpretation]", strings.TrimSpace(item.Interpretation))
		form.Set("srv["+key+"][conclusion]", strings.TrimSpace(item.Conclusion))
		index++
	}
	if index == 0 {
		return nil, errors.New("no service results available to save")
	}
	endpoint := "/file/services/results/" + url.PathEscape(fileID) + "?XDEBUG_TRIGGER=debug"
	var raw interface{}
	if err := m.doForm(http.MethodPatch, endpoint, form, &raw); err != nil {
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
	targetURL, _ := makeURLFromSettings(m.Settings(), path)
	if m.verboseLevel() >= 4 {
		m.rt.Logf("wisemed-api request method=%s path=%s url=%s body=%s", method, path, targetURL, mustJSON(maskSecrets(payload)))
	}
	err := doJSONWithClient(m.client, m.Settings(), m.callerType(), method, path, payload, out)
	if m.verboseLevel() >= 4 {
		if err != nil {
			m.rt.Logf("wisemed-api response method=%s path=%s url=%s error=%v", method, path, targetURL, err)
		} else if m.verboseLevel() >= 5 {
			m.rt.Logf("wisemed-api response method=%s path=%s url=%s body=%s", method, path, targetURL, mustJSON(maskSecrets(out)))
		} else {
			m.rt.Logf("wisemed-api response method=%s path=%s url=%s status=ok", method, path, targetURL)
		}
	}
	return err
}

func (m *Module) doForm(method, path string, payload url.Values, out interface{}) error {
	targetURL, _ := makeURLFromSettings(m.Settings(), path)
	if m.verboseLevel() >= 4 {
		m.rt.Logf("wisemed-api form request method=%s path=%s url=%s body=%s", method, path, targetURL, sanitizeFormValues(payload).Encode())
	}
	err := doFormWithClient(m.client, m.Settings(), m.callerType(), method, path, payload, out)
	if m.verboseLevel() >= 4 {
		if err != nil {
			m.rt.Logf("wisemed-api form response method=%s path=%s url=%s error=%v", method, path, targetURL, err)
		} else if m.verboseLevel() >= 5 {
			m.rt.Logf("wisemed-api form response method=%s path=%s url=%s body=%s", method, path, targetURL, mustJSON(maskSecrets(out)))
		} else {
			m.rt.Logf("wisemed-api form response method=%s path=%s url=%s status=ok", method, path, targetURL)
		}
	}
	return err
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
	analytes := m.analytePayload()
	payload := map[string]interface{}{
		"cod_echipament":          firstNonEmpty(settings["cod_echipament"], asString(reader["analyzer_code"]), readerString(m.rt, "analyzer_code"), m.rt.ReaderID()),
		"nume_echipament":         firstNonEmpty(asString(reader["analyzer_name"]), asString(reader["label"]), readerString(m.rt, "analyzer_name"), readerString(m.rt, "label"), m.rt.ReaderID()),
		"producator_echipament":   firstNonEmpty(settings["producator_echipament"], firstToken(firstNonEmpty(asString(reader["analyzer_name"]), readerString(m.rt, "analyzer_name"), asString(reader["label"]), readerString(m.rt, "label")))),
		"tip_analizor":            analyzerTypeValue(settings, m.callerType()),
		"numar_serial_echipament": settings["numar_serial_echipament"],
		"online":                  true,
		"nr_rackuri":              intSetting(settings, "nr_rackuri", 0),
		"pozitii_pe_rack":         intSetting(settings, "pozitii_pe_rack", 0),
		"nume_pe_raport_final":    firstNonEmpty(settings["nume_pe_raport_final"], asString(reader["analyzer_name"]), readerString(m.rt, "analyzer_name"), asString(reader["label"]), readerString(m.rt, "label"), m.rt.ReaderID()),
		"unitate_medicala_id":     intSetting(settings, "unitate_medicala_id", 0),
		"tip_de_echipament_id":    intSetting(settings, "tip_de_echipament_id", 0),
		"analize":                 analytes,
	}
	if value := intSetting(settings, "echipament_id", 0); value > 0 {
		payload["echipament_id"] = value
	}
	return payload
}

func (m *Module) analytePayload() []map[string]interface{} {
	store := m.analyteStore()
	if store == nil {
		return []map[string]interface{}{}
	}
	items, err := store.ListAnalytes()
	if err != nil {
		m.rt.Logf("wisemed-api analyte payload load failed: %v", err)
		return []map[string]interface{}{}
	}
	equipmentID := intSetting(m.Settings(), "echipament_id", 0)
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		worklistLabel := strings.TrimSpace(asString(item.ProtocolOptions["worklist_label"]))
		unit := firstNonEmpty(item.ResultMeasureUnit, worklistLabel)
		row := map[string]interface{}{
			"pe_tag":                  strings.TrimSpace(item.Tag),
			"pe_codificare":           strings.TrimSpace(item.Code),
			"pe_codificare2":          "",
			"pe_formatare":            formatCode(item.ResultFormatting),
			"pe_tip_rezultat":         resultTypeCode(item.ResultType),
			"pe_ponderare_um":         item.ResultWeighting,
			"pe_um":                   unit,
			"pe_set_reactivi_buletin": strings.TrimSpace(item.ResultReagentsSet),
			"pe_nr_sincronizari":      0,
			"pe_activ":                boolToInt(item.Active),
			"pe_transformare":         analyteTransformationJSON(item),
		}
		if equipmentID > 0 {
			row["echipament_id"] = equipmentID
		}
		out = append(out, row)
	}
	return out
}

func (m *Module) analyteStore() analyteStore {
	service, ok := m.rt.Service("storage")
	if !ok {
		return nil
	}
	store, _ := service.(analyteStore)
	return store
}

func resultTypeCode(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "numeric", "number", "cantitativ", "quantitative":
		return 1
	case "text", "qualitative", "calitativ":
		return 2
	default:
		return 0
	}
}

func formatCode(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "numeric", "integer", "int", "decimal_0":
		return 1
	case "decimal_1":
		return 2
	case "decimal_2":
		return 3
	case "decimal_3":
		return 4
	case "decimal_4":
		return 5
	default:
		return 0
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func analyteTransformationJSON(item coremodel.Analyte) string {
	if len(item.Transformation) > 0 {
		blob, err := json.Marshal(item.Transformation)
		if err == nil {
			return string(blob)
		}
	}
	if raw, ok := item.ProtocolOptions["transformation"]; ok {
		blob, err := json.Marshal(raw)
		if err == nil {
			return string(blob)
		}
	}
	return ""
}

func intSetting(settings map[string]string, key string, fallback int) int {
	value := strings.TrimSpace(settings[key])
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func analyzerTypeValue(settings map[string]string, fallback string) interface{} {
	for _, key := range []string{"tip_analizor", "tip_analizor_id"} {
		if value := strings.TrimSpace(settings[key]); value != "" {
			if parsed, err := strconv.Atoi(value); err == nil {
				return parsed
			}
			return value
		}
	}
	return fallback
}

func firstToken(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
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

func buildDosimetryForm(entries []DosimetryEntry) url.Values {
	form := url.Values{}
	for idx, entry := range entries {
		if strings.TrimSpace(entry.Serial) == "" {
			continue
		}
		prefix := fmt.Sprintf("dos[%d]", idx)
		form.Set(prefix+"[serial]", strings.TrimSpace(entry.Serial))
		form.Set(prefix+"[hp_10]", strings.TrimSpace(entry.HP10))
		form.Set(prefix+"[hp_007]", strings.TrimSpace(entry.HP007))
	}
	return form
}

func (m *Module) verboseLevel() int {
	raw := asString(m.rt.ModuleSettings("logging")["verbose_level"])
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		value = 1
	}
	if value < 1 {
		value = 1
	}
	if value > 5 {
		value = 5
	}
	return value
}

func mustJSON(value interface{}) string {
	blob, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(blob)
}

func maskSecrets(value interface{}) interface{} {
	switch typed := value.(type) {
	case nil, string, bool, float64, float32, int, int64, int32, uint, uint64, uint32, json.Number:
		return typed
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, maskSecrets(item))
		}
		return out
	case map[string]interface{}:
		out := map[string]interface{}{}
		for key, item := range typed {
			out[key] = maskSecretValue(key, item)
		}
		return out
	case map[string]string:
		out := map[string]interface{}{}
		for key, item := range typed {
			out[key] = maskSecretValue(key, item)
		}
		return out
	case *interface{}:
		if typed == nil {
			return nil
		}
		return maskSecrets(*typed)
	default:
		blob, err := json.Marshal(value)
		if err != nil {
			return value
		}
		var generic interface{}
		if err := json.Unmarshal(blob, &generic); err != nil {
			return value
		}
		return maskSecrets(generic)
	}
}

func maskSecretValue(key string, value interface{}) interface{} {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "password", "login_token", "cfg_wisemed_key", "authorization", "api_key_echipament":
		return "***"
	default:
		return maskSecrets(value)
	}
}

func sanitizeFormValues(values url.Values) url.Values {
	out := url.Values{}
	for key, items := range values {
		copied := append([]string(nil), items...)
		if strings.Contains(strings.ToLower(key), "token") || strings.Contains(strings.ToLower(key), "password") {
			copied = []string{"***"}
		}
		for _, item := range copied {
			out.Add(key, item)
		}
	}
	return out
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
	case nil:
		return ""
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
	req.Header.Set("Authorization", "Bearer "+token)
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

func doFormWithClient(client *http.Client, settings map[string]string, callerType, method, path string, payload url.Values, out interface{}) error {
	target, err := makeURLFromSettings(settings, path)
	if err != nil {
		return err
	}
	encoded := ""
	if payload != nil {
		encoded = payload.Encode()
	}
	req, err := http.NewRequest(method, target, strings.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	token, err := createJWTForSettings(settings, callerType)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
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
	if out == nil {
		return nil
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		switch ptr := out.(type) {
		case *interface{}:
			*ptr = nil
			return nil
		default:
			return nil
		}
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
