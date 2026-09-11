package resultsync

import (
	"errors"
	"testing"
	"wisemed-labreaders/readersv3/core/module"
	model "wisemed-labreaders/readersv3/modules/core/model"
)

type identityRuntime struct{ module.Runtime }

func (identityRuntime) Logf(string, ...interface{})                  {}
func (identityRuntime) ModuleSettings(string) map[string]interface{} { return nil }

type identityLookup struct{ file string }

func (*identityLookup) HasEquipmentID() bool        { return true }
func (*identityLookup) Settings() map[string]string { return nil }
func (l *identityLookup) FetchFileForAnalyzer(id, _ string) (map[string]interface{}, error) {
	l.file = id
	return nil, errors.New("test stops after lookup")
}
func TestManualFileIDBypassesAnalyzerNormalization(t *testing.T) {
	m := &Module{rt: identityRuntime{}}
	lookup := &identityLookup{}
	order := model.Order{SampleID: "NAME", Meta: map[string]interface{}{"id_correction": map[string]interface{}{"new_id": "00123"}}}
	m.processOrder(syncSettings{}, order, nil, "4", lookup)
	if lookup.file != "00123" {
		t.Fatalf("lookup used %q", lookup.file)
	}
}
