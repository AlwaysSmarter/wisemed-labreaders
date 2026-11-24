// Package ecard hosts CNAS SDKs out of process. A card is never used concurrently.
package ecard

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

var sdkIDs = []string{"2.0.0.4", "cnas-wrapper-1.0.0.0", "2.0.0.3", "thinkit-1.3.1.4", "1.3.0.4", "1.3.0.2", "1.3.0.1", "1.2.0.1"}

type Settings struct {
	CEASOffline      bool     `json:"ceas_offline"`
	SDKOrder         []string `json:"sdk_order"`
	SDKRoot          string   `json:"sdk_root"`
	HostDir          string   `json:"host_dir"`
	StateDir         string   `json:"state_dir"`
	Reader           string   `json:"reader_name"`
	UMHost           string   `json:"um_host"`
	UMPort           int      `json:"um_port"`
	OperationTimeout int      `json:"operation_timeout_seconds"`
	JobTimeout       int      `json:"job_timeout_seconds"`
	MaxItems         int      `json:"max_items"`
	MaxItemBytes     int      `json:"max_item_bytes"`
	MaxJobBytes      int64    `json:"max_job_bytes"`
	MaxJobs          int      `json:"max_jobs"`
	RetentionHours   int      `json:"retention_hours"`
}

func readSettings(in map[string]interface{}) (Settings, error) {
	s := Settings{SDKOrder: append([]string(nil), sdkIDs...), SDKRoot: "./sdks", HostDir: "./host", StateDir: "./ecard-state", UMHost: "umceas.siui.ro", UMPort: 443, OperationTimeout: 180, JobTimeout: 7200, MaxItems: 100000, MaxItemBytes: 65536, MaxJobBytes: 256 << 20, MaxJobs: 32, RetentionHours: 24}
	b, e := json.Marshal(in)
	if e != nil {
		return s, e
	}
	if e = json.Unmarshal(b, &s); e != nil {
		return s, e
	}
	if len(s.SDKOrder) == 0 {
		return s, errors.New("sdk_order must not be empty")
	}
	seen := map[string]bool{}
	for _, id := range s.SDKOrder {
		known := false
		for _, v := range sdkIDs {
			if id == v {
				known = true
			}
		}
		if !known || seen[id] {
			return s, fmt.Errorf("invalid or repeated SDK %q", id)
		}
		seen[id] = true
	}
	if !validUMHost(s.UMHost) {
		return s, errors.New("um_host must be a hostname or IP address without scheme, port or path; configure um_port separately")
	}
	if s.UMPort < 1 || s.UMPort > 65535 || s.OperationTimeout < 10 || s.OperationTimeout > 600 || s.JobTimeout < 60 || s.JobTimeout > 86400 || s.MaxItems < 1 || s.MaxItems > 1000000 || s.MaxItemBytes < 1 || s.MaxItemBytes > 65536 || s.MaxJobBytes < 1 || s.MaxJobBytes > 1<<30 || s.MaxJobs < 1 || s.MaxJobs > 128 || s.RetentionHours < 1 || s.RetentionHours > 720 {
		return s, errors.New("settings outside supported bounds")
	}
	return s, nil
}

// The SDK accepts a bare DNS name or IP, not a web/TCP URL.
func validUMHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if len(label) == 0 || len(label) > 63 || !regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?$`).MatchString(label) {
			return false
		}
	}
	return true
}

type Contract struct {
	CIF          string `json:"cif"`
	CUI          string `json:"cui"`
	Number       string `json:"number"`
	Date         string `json:"date"`
	House        string `json:"house"`
	ProviderType string `json:"provider_type"`
}

func (c Contract) Validate() error {
	if _, err := time.Parse("2006-01-02", c.Date); err != nil {
		return errors.New("contract.date must be YYYY-MM-DD")
	}
	for _, v := range []string{c.CIF, c.CUI, c.Number, c.House, c.ProviderType} {
		if v == "" || len(v) > 128 || regexp.MustCompile(`[\r\n\x00]`).MatchString(v) {
			return errors.New("complete contract identity required")
		}
	}
	return nil
}

type Request struct {
	Expected    Identity        `json:"expected"`
	Operation   string          `json:"operation"`
	Contract    Contract        `json:"contract"`
	Fields      []string        `json:"fields,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
	CID         string          `json:"cid,omitempty"`
	Certificate string          `json:"certificate,omitempty"` // insured or mai
	Encoding    string          `json:"encoding,omitempty"`    // ascii (strict) or utf-8
	Confirm     bool            `json:"confirm,omitempty"`
}

func (r *Request) Validate() error {
	switch r.Operation {
	case "readers", "status", "read", "certificates", "sign", "activate", "change-pin", "reset-pin", "write", "enroll":
	default:
		return errors.New("unsupported operation")
	}
	if r.Operation != "readers" && r.Operation != "status" {
		if e := r.Contract.Validate(); e != nil {
			return e
		}
	}
	if r.Operation == "activate" || r.Operation == "change-pin" || r.Operation == "reset-pin" || r.Operation == "write" || r.Operation == "enroll" {
		if !r.Confirm {
			return errors.New("explicit confirm required")
		}
	}
	if r.Operation == "sign" {
		if e := validateExpected(r.Expected); e != nil {
			return e
		}
	}
	if r.Encoding == "" {
		r.Encoding = "ascii"
	}
	if r.Encoding != "ascii" && r.Encoding != "utf-8" {
		return errors.New("encoding must be ascii or utf-8")
	}
	if r.Certificate == "" {
		r.Certificate = "insured"
	}
	if r.Certificate != "insured" && r.Certificate != "mai" {
		return errors.New("certificate must be insured or mai")
	}
	if len(r.Fields) > 64 {
		return errors.New("too many fields")
	}
	for _, v := range r.Fields {
		if !regexp.MustCompile(`^[A-I][0-9]{1,2}$`).MatchString(v) || v == "G2" || v == "G3" || v == "G4" {
			return errors.New("invalid card field")
		}
	}
	if r.Operation == "write" && (r.CID == "" || len(r.Data) == 0 || len(r.Fields) == 0) {
		return errors.New("write requires cid, fields and data")
	}
	return nil
}

type Item struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}
type Job struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Operation string `json:"operation"`
	SDK       string `json:"sdk,omitempty"`
	Total     int    `json:"total"`
	Completed int    `json:"completed"`
	Created   int64  `json:"created"`
	Error     string `json:"error,omitempty"`
}
type Reply struct {
	InputKind     string          `json:"input_kind,omitempty"`
	Diagnostics   json.RawMessage `json:"diagnostics,omitempty"`
	ExceptionType string          `json:"exception_type,omitempty"`
	SDKIdentity   json.RawMessage `json:"sdk_identity,omitempty"`
	HostVersion   string          `json:"host_version,omitempty"`
	Message       string          `json:"message,omitempty"`
	SDKName       string          `json:"sdk_name,omitempty"`
	Stage         string          `json:"stage,omitempty"`
	State         json.RawMessage `json:"state,omitempty"`
	OK            bool            `json:"ok"`
	Code          string          `json:"code,omitempty"`
	SDKCode       int             `json:"sdk_code,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
	Event         string          `json:"event,omitempty"`
	Prompt        string          `json:"prompt,omitempty"`
	PINAttempted  bool            `json:"pin_attempted,omitempty"`
}

// Reads try every configured SDK on failure with a fresh session/PIN prompt.
// Signing preparation retains restricted fallback; mutations are never replayed.
func canFallback(op string, r Reply, pinSeen bool) bool {
	if invalidatesPIN(r) {
		return false
	}
	if op == "read" {
		return true
	}
	if (op != "read" && op != "certificates" && op != "sign") || pinSeen || r.PINAttempted {
		return false
	}
	if r.Code == "sdk_unavailable" || r.Code == "sdk_incompatible" {
		return true
	}
	switch r.SDKCode {
	case -6, -17, -33, -34, -51, -68, -69:
		return true
	}
	return false
}
