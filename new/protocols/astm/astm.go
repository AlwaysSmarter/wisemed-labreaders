package astm

import "strings"

// NormalizeMessage keeps a shared ASTM normalization layer for all analyzers.
func NormalizeMessage(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\r")
	raw = strings.TrimSpace(raw)
	return raw
}

func ProtocolName() string {
	return "ASTM"
}

// BuildWorklistResponse creates a compact ASTM order response for query-based analyzers.
func BuildWorklistResponse(sampleID string, tags []string) string {
	normalizedTags := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(strings.ToUpper(t))
		if t != "" {
			normalizedTags = append(normalizedTags, "^^^"+t)
		}
	}
	joined := strings.Join(normalizedTags, `\`)
	return "H|\\^&|||WiseMED|||||P|1\rP|1\rO|1|" + sampleID + "||" + joined + "|R\rL|1|N\r"
}
