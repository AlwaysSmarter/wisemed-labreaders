package astm

import "testing"

func TestResolveSpecimenCodeUsesMappingBeforeDefault(t *testing.T) {
	settings := map[string]interface{}{
		"specimen_code_default": "1",
		"specimen_code_map": map[string]interface{}{
			"ser":   "2",
			"urine": "3",
		},
	}
	if got := ResolveSpecimenCode(settings, " SER ", "9"); got != "2" {
		t.Fatalf("mapped specimen code = %q, want 2", got)
	}
	if got := ResolveSpecimenCode(settings, "plasma", "9"); got != "1" {
		t.Fatalf("default specimen code = %q, want 1", got)
	}
}
