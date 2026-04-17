package protocol

import "time"

type Envelope struct {
	Type string `json:"type"`
}

type ReaderHelloMessage struct {
	Type         string    `json:"type"`
	ReaderID     string    `json:"reader_id"`
	AnalyzerCode string    `json:"analyzer_code"`
	AnalyzerName string    `json:"analyzer_name"`
	AnalyzerType string    `json:"analyzer_type"`
	LicenseCode  string    `json:"license_code"`
	CreatedAt    time.Time `json:"created_at"`
}

type RegistrationStateMessage struct {
	Type          string                 `json:"type"`
	ReaderID      string                 `json:"reader_id"`
	Registered    bool                   `json:"registered"`
	SetupComplete bool                   `json:"setup_complete"`
	Profile       map[string]interface{} `json:"profile,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
}

type CommandMessage struct {
	Type          string                 `json:"type"`
	CommandID     string                 `json:"command_id"`
	CorrelationID string                 `json:"correlation_id"`
	Command       string                 `json:"command"`
	Args          map[string]interface{} `json:"args"`
	IssuedAt      time.Time              `json:"issued_at"`
}

type CommandResultMessage struct {
	Type          string                 `json:"type"`
	CommandID     string                 `json:"command_id"`
	CorrelationID string                 `json:"correlation_id"`
	Success       bool                   `json:"success"`
	Data          map[string]interface{} `json:"data,omitempty"`
	Error         string                 `json:"error,omitempty"`
	HandledAt     time.Time              `json:"handled_at"`
}

type HeartbeatMessage struct {
	Type      string                 `json:"type"`
	ReaderID  string                 `json:"reader_id"`
	Status    string                 `json:"status"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

type ResultBatchMessage struct {
	Type      string                 `json:"type"`
	ReaderID  string                 `json:"reader_id"`
	Items     []ResultOutboxItemWire `json:"items"`
	CreatedAt time.Time              `json:"created_at"`
}

type ResultBatchAckMessage struct {
	Type         string   `json:"type"`
	ReaderID     string   `json:"reader_id"`
	AcceptedRefs []string `json:"accepted_refs"`
}

type ResultOutboxItemWire struct {
	RefID       string                 `json:"ref_id"`
	PatientID   string                 `json:"patient_id"`
	SampleID    string                 `json:"sample_id"`
	AnalyteTag  string                 `json:"analyte_tag"`
	ResultValue string                 `json:"result_value"`
	Unit        string                 `json:"unit"`
	Meta        map[string]interface{} `json:"meta,omitempty"`
	ProducedAt  time.Time              `json:"produced_at"`
}

const (
	MsgTypeReaderHello   = "reader_hello"
	MsgTypeCommand       = "command"
	MsgTypeCommandResult = "command_result"
	MsgTypeHeartbeat     = "heartbeat"
	MsgTypeResultBatch   = "result_batch"
	MsgTypeResultAck     = "result_ack"
	MsgTypeRegisterState = "registration_state"
)
