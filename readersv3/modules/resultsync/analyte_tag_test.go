package resultsync

import (
	"testing"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
)

func TestWiseMEDMatchesSpecialTagsSeparately(t *testing.T) {
	analyses := []coremodel.OrderAnalysisBundle{
		{Analysis: coremodel.OrderAnalysis{AnalyteTag: "NEU#"}},
		{Analysis: coremodel.OrderAnalysis{AnalyteTag: "NEU%"}},
		{Analysis: coremodel.OrderAnalysis{AnalyteTag: "NEU"}},
	}
	payload := map[string]interface{}{"o_tests": []interface{}{
		map[string]interface{}{"t_tag": "NEU#", "t_sm_id": "abs", "t_fsm_id": "file-abs"},
		map[string]interface{}{"t_tag": "NEU%", "t_sm_id": "pct", "t_fsm_id": "file-pct"},
	}}
	got := applyWiseMEDTestsOnly(analyses, payload)
	if len(got) != 2 || got[0].WiseMEDSMID != "abs" || got[1].WiseMEDSMID != "pct" || got[0].WiseMEDFSMID != "file-abs" || got[1].WiseMEDFSMID != "file-pct" {
		t.Fatalf("incorrect mapping: %+v", got)
	}
}
