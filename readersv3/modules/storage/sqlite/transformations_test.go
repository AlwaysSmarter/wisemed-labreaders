package sqlite

import "testing"

func TestEvalFormulaMathFunctions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		formula string
		vars    map[string]float64
		want    string
	}{
		{name: "sqrt", formula: "SQRT(81)", want: "9"},
		{name: "sum", formula: "SUM(RES_FCAT, RES_FPT, 1)", vars: map[string]float64{"RES_FCAT": 2, "RES_FPT": 3}, want: "6"},
		{name: "avg", formula: "AVG(10, 20, 30)", want: "20"},
		{name: "round", formula: "ROUND(12.345, 2)", want: "12.35"},
		{name: "pow", formula: "POW(2, 3)", want: "8"},
		{name: "conditional", formula: "(SRC_RESULT > 40 'NEDETECTABIL':'DETECTABIL')", vars: map[string]float64{"SRC_RESULT": 27.7}, want: "DETECTABIL"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := evalFormula(normalizeFormulaSyntax(tc.formula), tc.vars)
			if err != nil {
				t.Fatalf("evalFormula error: %v", err)
			}
			if actual := formatFormulaValue(got, "raw"); actual != tc.want {
				t.Fatalf("formatFormulaValue() = %q, want %q", actual, tc.want)
			}
		})
	}
}
