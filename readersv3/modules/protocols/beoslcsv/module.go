package beoslcsv

import "wisemed-labreaders/readersv3/core/module"

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "protocol-beosl-csv" }
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.AddMenu(module.MenuEntry{ID: "protocol-beosl", Group: "admin", Label: "Protocol BEOSL", Path: "/settings/protocol/beosl", Order: 45})
	return nil
}
