package wisemedai

import (
	"encoding/json"
	"errors"
	"os"
	"time"
)

type Settings struct {
	AllowedHosts          []string `json:"connector_allowed_hosts"`
	OpenAIKeyFile         string   `json:"openai_key_file"`
	DatabaseURL           string   `json:"database_url"`
	DatabaseURLEnv        string   `json:"database_url_env"`
	EncryptionKeyFile     string   `json:"encryption_key_file"`
	BootstrapEmail        string   `json:"bootstrap_email"`
	BootstrapPasswordFile string   `json:"bootstrap_password_file"`
	Provider              string   `json:"provider"`
	Model                 string   `json:"model"`
	OpenAIKeyEnv          string   `json:"openai_key_env"`
	MonthlyBudget         float64  `json:"monthly_budget_usd"`
	InputPrice            float64  `json:"input_usd_per_million"`
	OutputPrice           float64  `json:"output_usd_per_million"`
	RetentionDays         int      `json:"retention_days"`
	SecureCookies         bool     `json:"secure_cookies"`
	WiseMEDLogin          bool     `json:"wisemed_login_enabled"`
	WSMIdentity           bool     `json:"wsm_identity_enabled"`
}

func readSettings(raw map[string]interface{}) (Settings, error) {
	s := Settings{DatabaseURLEnv: "WISEMED_AI_DATABASE_URL", OpenAIKeyFile: "./ai-openai.key", EncryptionKeyFile: "./ai-encryption.key", BootstrapEmail: "admin@wisemed.local", BootstrapPasswordFile: "./ai-bootstrap-password", Provider: "mock", Model: "gpt-4.1-mini", OpenAIKeyEnv: "OPENAI_API_KEY", MonthlyBudget: 10, InputPrice: 0.4, OutputPrice: 1.6, RetentionDays: 30}
	b, e := json.Marshal(raw)
	if e != nil {
		return s, e
	}
	if e = json.Unmarshal(b, &s); e != nil {
		return s, e
	}
	if v := os.Getenv(s.DatabaseURLEnv); v != "" {
		s.DatabaseURL = v
	}
	if s.Provider != "mock" && s.Provider != "openai" {
		return s, errors.New("provider must be mock or openai")
	}
	if s.RetentionDays < 1 || s.MonthlyBudget <= 0 || s.InputPrice < 0 || s.OutputPrice < 0 {
		return s, errors.New("invalid retention/budget/prices")
	}
	return s, nil
}

const requestTimeout = 55 * time.Second
