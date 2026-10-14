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
	mu       sync.Mutex
	key      [32]byte
	token    string
	acquired time.Time
}

func ocspConfigKey(cfg settings) [32]byte { data, _ := json.Marshal(cfg); return sha256.Sum256(data) }
func ocspHeaders(h http.Header, token string) http.Header {
	h.Set("OSCP_RESPONSE", token)
	h.Set("SessionID", token)
	h.Set("Content-Type", "text/xml; charset=utf-8")
	h.Set("SOAPAction", `""`)
	return h
}
