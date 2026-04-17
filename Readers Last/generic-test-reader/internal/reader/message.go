package reader

import "time"

type Envelope struct {
	Type          string                 `json:"type"`
	RequestID     string                 `json:"request_id,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	ConnectionID  string                 `json:"connection_id,omitempty"`
	Target        *Target                `json:"target,omitempty"`
	Broadcast     bool                   `json:"broadcast,omitempty"`
	Payload       map[string]interface{} `json:"payload,omitempty"`
	Timestamp     time.Time              `json:"timestamp,omitempty"`
}

type Target struct {
	Mode         string `json:"mode,omitempty"`
	ConnectionID string `json:"connection_id,omitempty"`
	ClientType   string `json:"client_type,omitempty"`
	ReaderID     string `json:"reader_id,omitempty"`
	Topic        string `json:"topic,omitempty"`
}
