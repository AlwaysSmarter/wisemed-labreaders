package fileimportbase

import (
	"errors"
	"testing"
	"wisemed-labreaders/readersv3/core/module"
	model "wisemed-labreaders/readersv3/modules/core/model"
	"wisemed-labreaders/readersv3/modules/wisemedapi"
)

type identityAPI struct{ file string }

func (*identityAPI) SetupComplete() bool { return true }
func (a *identityAPI) SaveFileServiceResults(id string, _ []wisemedapi.ServiceResultEntry) (map[string]interface{}, error) {
	a.file = id
	return map[string]interface{}{}, nil
}

type identityRuntime struct {
	module.Runtime
	current *model.Order
}

func (r identityRuntime) Service(string) (interface{}, bool) {
	if r.current != nil {
		return r, true
	}
	return nil, false
}
func (identityRuntime) Logf(string, ...interface{})                  {}
func (identityRuntime) ModuleSettings(string) map[string]interface{} { return nil }
func (r identityRuntime) GetOrder(int64) (model.Order, error)        { return *r.current, nil }
func TestSendUsesCorrectedFileID(t *testing.T) {
	api := &identityAPI{}
	order := model.Order{ID: 1, SampleID: "00123", FileID: "OLD", Meta: map[string]interface{}{"id_correction": map[string]interface{}{"new_id": "00123"}}}
	bundles := []model.OrderBundle{{Order: order, Analyses: []model.OrderAnalysisBundle{{Analysis: model.OrderAnalysis{ID: 1, WiseMEDFSMID: "new-fsm", ResultValue: "100"}}}}}
	if _, err := saveOrderBundlesToWiseMED(api, bundles, identityRuntime{}); err != nil {
		t.Fatal(err)
	}
	if api.file != "00123" {
		t.Fatal("sent to old file", api.file)
	}
	api.file = ""
	current := order
	current.SampleID = "456"
	if _, err := saveOrderBundlesToWiseMED(api, bundles, identityRuntime{current: &current}); !errors.Is(err, model.ErrOrderIdentityConflict) {
		t.Fatal(err)
	}
	if api.file != "" {
		t.Fatal("stale bundle was sent")
	}
}
