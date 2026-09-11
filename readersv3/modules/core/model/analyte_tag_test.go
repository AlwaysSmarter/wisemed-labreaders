package model

import "testing"

func TestAnalyteTagPreservesIdentity(t *testing.T) {
	for _, tag := range []string{"NEU#", "NEU%", "P-LCR", "RDW-CV", "A/B", "A.B", "A+B", "A_B", "A(B)"} {
		if got := NormalizeAnalyteTag("  " + tag + "  "); got != tag {
			t.Errorf("%q became %q", tag, got)
		}
	}
}
