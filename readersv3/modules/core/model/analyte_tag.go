package model

import (
	"strings"
	"unicode"
)

// NormalizeAnalyteTag preserves punctuation because it is part of the analyzer
// identity (for example NEU# and NEU% are different tests).
func NormalizeAnalyteTag(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return '_'
		}
		return unicode.ToUpper(r)
	}, strings.TrimSpace(value))
}
