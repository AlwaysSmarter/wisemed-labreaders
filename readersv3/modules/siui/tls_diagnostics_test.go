package siui

import (
	"strings"
	"testing"
)

func TestSecureFailureDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		flags uint32
		terms []string
	}{
		{0x20, []string{"expirat", "data calculatorului", "0x00000020"}},
		{0x28, []string{"expirat", "autoritatea", "0x00000028"}},
		{0x10, []string{"numele certificatului"}},
		{0x80000000, []string{"Schannel", "tokenului"}},
		{0, []string{"nu a furnizat", "Schannel"}},
	} {
		message := secureFailureMessage(tc.flags)
		for _, term := range tc.terms {
			if !strings.Contains(message, term) {
				t.Fatalf("missing diagnostic %q in %q", term, message)
			}
		}
	}
}
