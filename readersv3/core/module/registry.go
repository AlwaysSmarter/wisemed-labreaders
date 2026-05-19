package module

import "fmt"

type Factory func() Module

type Registry struct {
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{factories: map[string]Factory{}}
}

func (r *Registry) Register(id string, factory Factory) {
	r.factories[id] = factory
}

func (r *Registry) Build(id string) (Module, error) {
	factory, ok := r.factories[id]
	if !ok {
		return nil, fmt.Errorf("module %q is not registered", id)
	}
	return factory(), nil
}
