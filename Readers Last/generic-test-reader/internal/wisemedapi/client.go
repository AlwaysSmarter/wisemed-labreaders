package wisemedapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"wisemed-labreaders/readerslast/generic-test-reader/internal/config"
	"wisemed-labreaders/readerslast/generic-test-reader/internal/model"
)

type Client struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Client {
	return &Client{cfg: cfg}
}

func (c *Client) ListMedicalUnits() ([]model.MedicalUnit, error) {
	payload, err := c.doJSON(http.MethodGet, "/administrative/medicalunits", nil)
	if err != nil {
		return nil, err
	}
	items := extractItems(payload)
	out := make([]model.MedicalUnit, 0, len(items))
	for _, item := range items {
		mu := model.MedicalUnit{
			ID:   intFrom(item, "medical_unit_id", "unitate_medicala_id", "id"),
			Code: strFrom(item, "medical_unit_code", "code", "cod"),
			Name: strFrom(item, "medical_unit_name", "nume_complet_unitate_medicala", "name", "nume", "denumire"),
			Raw:  item,
		}
		if mu.ID == 0 || mu.Name == "" {
			continue
		}
		out = append(out, mu)
	}
	return out, nil
}

func (c *Client) ListAnalyzerEquipmentTypes() ([]model.AnalyzerEquipmentType, error) {
	payload, err := c.doJSON(http.MethodGet, "/administrative/wmanalyzertypes", nil)
	if err != nil {
		return nil, err
	}
	items := extractItems(payload)
	out := make([]model.AnalyzerEquipmentType, 0, len(items))
	for _, item := range items {
		eqt := model.AnalyzerEquipmentType{
			ID:   intFrom(item, "analyzer_type_id", "tip_de_echipament_id", "id"),
			Name: strFrom(item, "analyzer_type_name", "name", "nume", "denumire"),
			Raw:  item,
		}
		if eqt.ID == 0 || eqt.Name == "" {
			continue
		}
		out = append(out, eqt)
	}
	return out, nil
}

type AnalyzerRegistrationRequest struct {
	Code             string      `json:"cod_echipament"`
	Name             string      `json:"nume_echipament"`
	APIKey           string      `json:"api_key_echipament,omitempty"`
	Manufacturer     string      `json:"producator_echipament"`
	AnalyzerType     int         `json:"tip_analizor"`
	SerialNo         string      `json:"numar_serial_echipament"`
	IP               string      `json:"ip"`
	Port             int         `json:"port"`
	Online           bool        `json:"online"`
	RacksNo          string      `json:"nr_rackuri"`
	PositionsPerRack string      `json:"pozitii_pe_rack"`
	NameOnReport     string      `json:"nume_pe_raport_final"`
	EquipmentID      int         `json:"echipament_id"`
	MedicalUnitID    int         `json:"unitate_medicala_id"`
	EquipmentTypeID  int         `json:"tip_de_echipament_id"`
	Analyses         interface{} `json:"analize"`
}

type AnalyzerRegistrationResponse struct {
	Name             string `json:"nume_echipament"`
	APIKey           string `json:"api_key_echipament"`
	Code             string `json:"cod_echipament"`
	Manufacturer     string `json:"producator_echipament"`
	AnalyzerType     int    `json:"tip_analizor"`
	SerialNo         string `json:"numar_serial_echipament"`
	IP               string `json:"ip"`
	Port             int    `json:"port"`
	Online           bool   `json:"online"`
	RacksNo          int    `json:"nr_rackuri"`
	PositionsPerRack int    `json:"pozitii_pe_rack"`
	NameOnReport     string `json:"nume_pe_raport_final"`
	EquipmentID      int    `json:"echipament_id"`
	MedicalUnitID    int    `json:"unitate_medicala_id"`
	EquipmentTypeID  int    `json:"tip_de_echipament_id"`
}

type LoginResponse struct {
	UserID      int                    `json:"user_id"`
	Login       string                 `json:"login"`
	FirstName   string                 `json:"first_name"`
	LastName    string                 `json:"last_name"`
	UserType    int                    `json:"user_type"`
	UserEmail   string                 `json:"user_email"`
	LoginToken  string                 `json:"login_token"`
	UserPicture string                 `json:"user_picture"`
	Raw         map[string]interface{} `json:"raw,omitempty"`
}

func (c *Client) RegisterAnalyzer(req AnalyzerRegistrationRequest) (*AnalyzerRegistrationResponse, error) {
	payload, err := c.doJSON(http.MethodPut, "/administrative/analyzer", req)
	if err != nil {
		return nil, err
	}
	m, ok := payload.(map[string]interface{})
	if !ok {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
	}
	out := AnalyzerRegistrationResponse{
		Name:             strFrom(m, "nume_echipament"),
		APIKey:           strFrom(m, "api_key_echipament"),
		Code:             strFrom(m, "cod_echipament"),
		Manufacturer:     strFrom(m, "producator_echipament"),
		AnalyzerType:     intFrom(m, "tip_analizor"),
		SerialNo:         strFrom(m, "numar_serial_echipament"),
		IP:               strFrom(m, "ip"),
		Port:             intFrom(m, "port"),
		Online:           boolFrom(m, "online"),
		RacksNo:          intFrom(m, "nr_rackuri"),
		PositionsPerRack: intFrom(m, "pozitii_pe_rack"),
		NameOnReport:     strFrom(m, "nume_pe_raport_final"),
		EquipmentID:      intFrom(m, "echipament_id"),
		MedicalUnitID:    intFrom(m, "unitate_medicala_id"),
		EquipmentTypeID:  intFrom(m, "tip_de_echipament_id"),
	}
	return &out, nil
}

func (c *Client) AdministrativeLogin(username, password string, medicalUnitID int) (*LoginResponse, error) {
	payload, err := c.doJSON(http.MethodPut, "/administrative/login", map[string]interface{}{
		"username":        username,
		"password":        password,
		"medical_unit_id": medicalUnitID,
	})
	if err != nil {
		return nil, err
	}
	m, ok := payload.(map[string]interface{})
	if !ok {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
	}
	out := LoginResponse{
		UserID:      intFrom(m, "user_id"),
		Login:       strFrom(m, "login"),
		FirstName:   strFrom(m, "first_name"),
		LastName:    strFrom(m, "last_name"),
		UserType:    intFrom(m, "user_type"),
		UserEmail:   strFrom(m, "user_email"),
		LoginToken:  strFrom(m, "login_token", "token", "lt"),
		UserPicture: strFrom(m, "user_picture"),
		Raw:         m,
	}
	if out.LoginToken == "" {
		return nil, fmt.Errorf("administrative/login did not return login_token")
	}
	return &out, nil
}

func (c *Client) doJSON(method, path string, body interface{}) (interface{}, error) {
	token, err := c.signJWT()
	if err != nil {
		return nil, err
	}
	var rawBody []byte
	if body != nil {
		rawBody, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequest(method, c.cfg.APIBaseURL()+path, bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s returned status %d", path, resp.StatusCode)
	}

	var payload interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) signJWT() (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"caller_id":   c.cfg.WiseMedAPI.JWTCallerID,
		"caller_type": c.cfg.WiseMedAPI.JWTCallerType,
		"iss":         c.cfg.WiseMedAPI.JWTISS,
		"ist":         c.cfg.WiseMedAPI.JWTIST,
		"iat":         now.Unix(),
		"exp":         now.Add(10 * time.Minute).Unix(),
		"lt":          c.cfg.WiseMedAPI.LoginToken,
		"jti":         fmt.Sprintf("wmr-%d", now.UnixNano()),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(c.cfg.WiseMedAPI.JWTSecret))
}

func extractItems(payload interface{}) []map[string]interface{} {
	switch x := payload.(type) {
	case []interface{}:
		return mapsFromArray(x)
	case map[string]interface{}:
		for _, key := range []string{"rows", "data", "items", "medicalunits", "medical_units", "result", "results"} {
			if arr, ok := x[key].([]interface{}); ok {
				return mapsFromArray(arr)
			}
		}
		return []map[string]interface{}{x}
	default:
		return nil
	}
}

func mapsFromArray(in []interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(in))
	for _, item := range in {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

func strFrom(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch x := v.(type) {
			case string:
				if strings.TrimSpace(x) != "" {
					return x
				}
			}
		}
	}
	return ""
}

func intFrom(m map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch x := v.(type) {
			case float64:
				return int(x)
			case int:
				return x
			case string:
				var n int
				_, err := fmt.Sscanf(x, "%d", &n)
				if err == nil {
					return n
				}
			}
		}
	}
	return 0
}

func boolFrom(m map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch x := v.(type) {
			case bool:
				return x
			case string:
				switch strings.ToLower(strings.TrimSpace(x)) {
				case "1", "true", "on", "yes":
					return true
				}
			case float64:
				return x != 0
			case int:
				return x != 0
			}
		}
	}
	return false
}
