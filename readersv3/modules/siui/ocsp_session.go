package siui

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

const ocspMaxAge = 150 * time.Second

// A process-local session is shared only by clients with the same configuration.
type ocspSession struct {
	mu             sync.Mutex
	key            [32]byte
	token          string
	acquired       time.Time
	lastAttempt    time.Time
	lastFailed     bool
	lastDurationMS int64
}

func ocspConfigKey(cfg settings) [32]byte { data, _ := json.Marshal(cfg); return sha256.Sum256(data) }
func ocspHeaders(h http.Header, token string) http.Header {
	h.Set("OSCP_RESPONSE", token)
	h.Set("SessionID", token)
	h.Set("Content-Type", "text/xml; charset=utf-8")
	h.Set("SOAPAction", `""`)
	return h
}

// Never wait for the token mutex: authentication can take up to two minutes.
func (s *ocspSession) dashboardState(cfg settings) map[string]any {
	if !s.mu.TryLock() {
		return map[string]any{"state": "authenticating"}
	}
	defer s.mu.Unlock()
	state := "not_tested"
	out := map[string]any{}
	if s.key == ocspConfigKey(cfg) {
		if !s.lastAttempt.IsZero() {
			state = "expired"
			out["last_attempt"] = s.lastAttempt.UTC().Format(time.RFC3339)
			out["duration_ms"] = s.lastDurationMS
		}
		if s.lastFailed {
			state = "failed"
		}
		if !s.acquired.IsZero() {
			out["last_success"] = s.acquired.UTC().Format(time.RFC3339)
		}
		age := time.Since(s.acquired)
		if s.token != "" && age >= 0 && age <= ocspMaxAge {
			state = "cached"
			out["remaining_seconds"] = int((ocspMaxAge - age).Seconds())
		}
	}
	out["state"] = state
	return out
}
