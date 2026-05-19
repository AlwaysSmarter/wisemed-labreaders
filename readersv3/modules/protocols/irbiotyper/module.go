package irbiotyper

import "wisemed-labreaders/readersv3/core/module"

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "protocol-ir-biotyper" }
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.AddMenu(module.MenuEntry{ID: "protocol-irbt", Group: "admin", Label: "Protocol IR Biotyper", Path: "/settings/protocol/irbt", Order: 45})
	return nil
}
