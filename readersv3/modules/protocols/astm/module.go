package astm

import (
	"context"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "protocol-astm" }
func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	m.rt.AddMenu(module.MenuEntry{ID: "protocol-astm", Group: "admin", Label: "Protocol ASTM", Path: "/settings/protocol/astm", Order: 45})
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.rt.Logf("astm protocol module active")
	<-ctx.Done()
	return nil
}
