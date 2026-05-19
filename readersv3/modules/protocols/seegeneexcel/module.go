package seegeneexcel

import "wisemed-labreaders/readersv3/core/module"

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "protocol-seegene-excel" }
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.AddMenu(module.MenuEntry{ID: "protocol-seegene", Group: "admin", Label: "Protocol Seegene", Path: "/settings/protocol/seegene", Order: 45})
	return nil
}
