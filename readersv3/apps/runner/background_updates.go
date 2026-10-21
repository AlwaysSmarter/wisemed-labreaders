package runner

import (
	"context"
	"errors"
	"log"
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/modules/builtin"
)

// Background checking lets data providers become available before the update
// server responds, retaining the common download/apply policy.
func registerRunnerModules(reg *module.Registry, opts RunOptions) {
	builtin.RegisterAll(reg)
	if opts.BackgroundStartupUpdates {
		reg.Register("runner-startup-updates", func() module.Module { return &backgroundUpdates{opts: opts} })
	}
}

type backgroundUpdates struct {
	opts  RunOptions
	cfg   *config.Config
	check func() error
}

func (b *backgroundUpdates) ID() string { return "runner-startup-updates" }
func (b *backgroundUpdates) Init(rt module.Runtime) error {
	var e error
	b.cfg, e = config.Load(rt.ConfigPath())
	return e
}
func (b *backgroundUpdates) Start(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return nil
	}
	check := b.check
	if check == nil {
		check = func() error { return checkForUpdates(b.cfg, b.opts) }
	}
	err := check()
	if errors.Is(err, errUpdateStarted) {
		return nil
	} // runtime drains and exits for updater
	if err != nil {
		log.Printf("app-updates: background startup check: %v", err)
	}
	<-ctx.Done()
	return nil
}
