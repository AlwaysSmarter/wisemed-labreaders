package debugreplay

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Terminator string

const (
	TerminatorEOF Terminator = "eof"
	TerminatorOUT Terminator = "out"
	TerminatorEOT Terminator = "eot"
)

type Step struct {
	Index          int        `json:"index"`
	Line           int        `json:"line"`
	Input          []byte     `json:"-"`
	ExpectedOutput []byte     `json:"-"`
	Terminator     Terminator `json:"terminator"`
}

type StepResult struct {
	Index          int        `json:"index"`
	Line           int        `json:"line"`
	Terminator     Terminator `json:"terminator"`
	InputPreview   string     `json:"input_preview"`
	ExpectedOutput string     `json:"expected_output"`
	ActualOutput   string     `json:"actual_output"`
	Passed         bool       `json:"passed"`
	Error          string     `json:"error,omitempty"`
	DurationMS     int64      `json:"duration_ms"`
}

type Result struct {
	Runner     string       `json:"runner"`
	ScriptName string       `json:"script_name"`
	Passed     bool         `json:"passed"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt time.Time    `json:"finished_at"`
	Steps      []StepResult `json:"steps"`
}

type Runner interface {
	RunDebugReplay(ctx context.Context, scriptName string, steps []Step) (Result, error)
}

var tokenBytes = []struct {
	token string
	value []byte
}{
	{token: "<CRLF>", value: []byte{'\r', '\n'}},
	{token: "<ENQ>", value: []byte{0x05}},
	{token: "<ACK>", value: []byte{0x06}},
	{token: "<NAK>", value: []byte{0x15}},
	{token: "<EOT>", value: []byte{0x04}},
	{token: "<STX>", value: []byte{0x02}},
	{token: "<ETX>", value: []byte{0x03}},
	{token: "<ETB>", value: []byte{0x17}},
	{token: "<CR>", value: []byte{'\r'}},
	{token: "<LF>", value: []byte{'\n'}},
	{token: "<TAB>", value: []byte{'\t'}},
	{token: "<NUL>", value: []byte{0x00}},
}

func DecodeTokens(value string) []byte {
	decoded := value
	for _, item := range tokenBytes {
		decoded = strings.ReplaceAll(decoded, item.token, string(item.value))
	}
	return []byte(decoded)
}

func EncodeTokens(payload []byte) string {
	if len(payload) == 0 {
		return "(empty)"
	}
	replacer := strings.NewReplacer(
		"\r\n", "<CRLF>",
		"\r", "<CR>",
		"\n", "<LF>\n",
		"\t", "<TAB>",
		string([]byte{0x05}), "<ENQ>",
		string([]byte{0x06}), "<ACK>",
		string([]byte{0x15}), "<NAK>",
		string([]byte{0x04}), "<EOT>",
		string([]byte{0x02}), "<STX>",
		string([]byte{0x03}), "<ETX>",
		string([]byte{0x17}), "<ETB>",
		string([]byte{0x00}), "<NUL>",
	)
	return replacer.Replace(string(payload))
}

func HexPreview(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	return strings.TrimSpace(hex.Dump(payload))
}

func DescribePayload(payload []byte) string {
	text := EncodeTokens(payload)
	hexText := HexPreview(payload)
	if hexText == "" {
		return text
	}
	return fmt.Sprintf("%s\nHEX:\n%s", text, hexText)
}
