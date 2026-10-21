package runner

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBackgroundUpdateCheckKeepsRuntimeAlive(t *testing.T) {
	for _, checkErr := range []error{nil, errors.New("offline")} {
		ctx, cancel := context.WithCancel(context.Background())
		checked := make(chan struct{})
		done := make(chan error, 1)
		b := &backgroundUpdates{check: func() error { close(checked); return checkErr }}
		go func() { done <- b.Start(ctx) }()
		<-checked
		select {
		case <-done:
			t.Fatal("update check stopped serving runtime")
		case <-time.After(10 * time.Millisecond):
		}
		cancel()
		if e := <-done; e != nil {
			t.Fatal(e)
		}
	}
}
func TestBackgroundAppliedUpdateDrainsRuntime(t *testing.T) {
	b := &backgroundUpdates{check: func() error { return errUpdateStarted }}
	if e := b.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
}
