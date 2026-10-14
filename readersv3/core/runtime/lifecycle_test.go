package runtime

import (
	"context"
	"errors"
	"testing"

	"wisemed-labreaders/readersv3/core/module"
)

type lifecycleModule struct {
	started chan struct{}
	stopped chan struct{}
	fail    bool
}

func (*lifecycleModule) ID() string                { return "lifecycle-test" }
func (*lifecycleModule) Init(module.Runtime) error { return nil }
func (m *lifecycleModule) Start(ctx context.Context) error {
	close(m.started)
	defer close(m.stopped)
	if m.fail {
		return errors.New("start failed")
	}
	<-ctx.Done()
	return nil
}
func TestStartWaitsForShutdown(t *testing.T) {
	for _, fail := range []bool{false, true} {
		m := &lifecycleModule{make(chan struct{}), make(chan struct{}), false}
		n := &lifecycleModule{make(chan struct{}), make(chan struct{}), fail}
		a := &App{modules: []module.Module{m, n}}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- a.Start(ctx) }()
		<-m.started
		<-n.started
		if !fail {
			cancel()
		}
		err := <-done
		cancel()
		if (err != nil) != fail {
			t.Fatalf("unexpected error %v", err)
		}
		for _, stopped := range []chan struct{}{m.stopped, n.stopped} {
			select {
			case <-stopped:
			default:
				t.Fatal("returned before module cleanup")
			}
		}
	}
}
