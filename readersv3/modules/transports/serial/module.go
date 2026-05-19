package serial

import (
	"context"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

func New() module.Module                       { return &Module{} }
func (m *Module) ID() string                   { return "transport-serial" }
func (m *Module) Init(rt module.Runtime) error { m.rt = rt; return nil }
func (m *Module) Start(ctx context.Context) error {
	m.rt.Logf("serial transport configured")
	<-ctx.Done()
	return nil
}
