package astm

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"wisemed-labreaders/new/internal/shared/protocol"
)

var tagTokenRegexp = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_/-]{0,31}$`)

type QueryRequest struct {
	SampleID string
	Tags     []string
}

// ParseResultBatch parses ASTM-like records and extracts result rows.
// It is intentionally tolerant to vendor variations.
func ParseResultBatch(raw string) []protocol.ResultOutboxItemWire {
	raw = NormalizeMessage(raw)
	if raw == "" {
		return nil
	}
	rows := splitRecords(raw)
	var sampleID string
	items := make([]protocol.ResultOutboxItemWire, 0)
	for i, line := range rows {
		parts := strings.Split(line, "|")
		if len(parts) == 0 {
			continue
		}
		recType := strings.TrimSpace(parts[0])
		switch recType {
		case "O":
			sampleID = extractSampleID(parts)
		case "R":
			if len(parts) < 4 {
				continue
			}
			tag := extractTestTag(parts[2])
			val := strings.TrimSpace(parts[3])
			if tag == "" || val == "" {
				continue
			}
			unit := ""
			if len(parts) > 4 {
				unit = strings.TrimSpace(parts[4])
			}
			if sampleID == "" {
				sampleID = "UNKNOWN"
			}
			ref := fmt.Sprintf("astm-%d-%d", time.Now().UnixNano(), i)
			items = append(items, protocol.ResultOutboxItemWire{
				RefID:       ref,
				PatientID:   sampleID,
				SampleID:    sampleID,
				AnalyteTag:  tag,
				ResultValue: val,
				Unit:        unit,
				Meta:        map[string]interface{}{"protocol": ProtocolName()},
				ProducedAt:  time.Now().UTC(),
			})
		}
	}
	return items
}

// ParseQueryRequests extracts ASTM query records (Q) used by bidirectional analyzers.
func ParseQueryRequests(raw string) []QueryRequest {
	raw = NormalizeMessage(raw)
	if raw == "" {
		return nil
	}
	rows := splitRecords(raw)
	out := make([]QueryRequest, 0)
	for _, line := range rows {
		parts := strings.Split(line, "|")
		if len(parts) == 0 || strings.TrimSpace(parts[0]) != "Q" {
			continue
		}
		req := QueryRequest{
			SampleID: extractSampleID(parts),
			Tags:     extractQueryTags(parts),
		}
		if req.SampleID == "" {
			continue
		}
		out = append(out, req)
	}
	return out
}

func splitRecords(raw string) []string {
	raw = strings.ReplaceAll(raw, "\n", "\r")
	segs := strings.Split(raw, "\r")
	out := make([]string, 0, len(segs))
	for _, x := range segs {
		x = strings.TrimSpace(x)
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

func extractSampleID(parts []string) string {
	if len(parts) > 2 {
		id := strings.TrimSpace(parts[2])
		if id != "" {
			items := strings.Split(id, "^")
			for i := len(items) - 1; i >= 0; i-- {
				if strings.TrimSpace(items[i]) != "" {
					return strings.TrimSpace(items[i])
				}
			}
			return id
		}
	}
	if len(parts) > 3 {
		id := strings.TrimSpace(parts[3])
		if id != "" {
			return id
		}
	}
	return ""
}

func extractTestTag(testField string) string {
	testField = strings.TrimSpace(testField)
	if testField == "" {
		return ""
	}
	items := strings.Split(testField, "^")
	for i := len(items) - 1; i >= 0; i-- {
		v := strings.TrimSpace(items[i])
		if v != "" {
			return v
		}
	}
	return testField
}

func extractQueryTags(parts []string) []string {
	if len(parts) <= 4 {
		return nil
	}
	field := strings.TrimSpace(parts[4])
	if field == "" {
		return nil
	}
	candidate := strings.FieldsFunc(field, func(r rune) bool {
		return r == '^' || r == '\\' || r == ',' || r == ';'
	})
	tags := make([]string, 0, len(candidate))
	for _, c := range candidate {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c == "" || !tagTokenRegexp.MatchString(c) {
			continue
		}
		seen := false
		for _, x := range tags {
			if x == c {
				seen = true
				break
			}
		}
		if !seen {
			tags = append(tags, c)
		}
	}
	return tags
}
