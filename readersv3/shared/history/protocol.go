// Package history defines the archive protocol shared by Bridge and History.
package history

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

const ChunkSize = 256 * 1024
const MaxArtifactSize int64 = 4 << 30

var safeID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

func ValidID(s string) bool { return safeID.MatchString(s) }

type Artifact struct {
	Name        string `json:"name"`
	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}
type Record struct {
	CallID      string          `json:"call_id"`
	OperationID string          `json:"operation_id"`
	Operation   string          `json:"operation"`
	Owner       string          `json:"owner"`
	SourceIP    string          `json:"source_ip"`
	CreatedAt   string          `json:"created_at"`
	StartedAt   string          `json:"started_at,omitempty"`
	CompletedAt string          `json:"completed_at"`
	Status      string          `json:"status"`
	Details     json.RawMessage `json:"details,omitempty"`
	Artifacts   []Artifact      `json:"artifacts"`
}

func (r Record) Validate() error {
	if r.Status != "completed" && r.Status != "failed" && r.Status != "unknown" {
		return errors.New("invalid final status")
	}
	if !ValidID(r.CallID) || r.Operation == "" || len(r.Operation) > 128 || len(r.OperationID) > 128 || len(r.Artifacts) > 8 {
		return errors.New("invalid call identity or artifacts")
	}
	if _, e := time.Parse(time.RFC3339Nano, r.CreatedAt); e != nil {
		return errors.New("invalid created_at")
	}
	if _, e := time.Parse(time.RFC3339Nano, r.CompletedAt); e != nil {
		return errors.New("invalid completed_at")
	}
	seen := map[string]bool{}
	for _, a := range r.Artifacts {
		b, e := hex.DecodeString(a.SHA256)
		if (a.Filename != "" && !ValidFilename(a.Filename)) || (a.ContentType != "" && a.ContentType != "application/zip" && a.ContentType != "application/xml" && a.ContentType != "application/gzip" && a.ContentType != "application/octet-stream") || !ValidID(a.Name) || seen[a.Name] || a.Size < 0 || a.Size > MaxArtifactSize || e != nil || len(b) != 32 {
			return errors.New("invalid artifact")
		}
		seen[a.Name] = true
	}
	return nil
}
func Digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type Progress struct {
	Success bool             `json:"success"`
	Status  string           `json:"status"`
	Offsets map[string]int64 `json:"offsets,omitempty"`
}
