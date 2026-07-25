// Package services composes the application services that share installed
// version state.
package services

import (
	"github.com/smileoniks-ctrl/govm/internal/install"
	"github.com/smileoniks-ctrl/govm/internal/lifecycle"
	"github.com/smileoniks-ctrl/govm/internal/paths"
	"github.com/smileoniks-ctrl/govm/internal/state"
)

// Runtime owns the process-wide services that mutate installed Go versions.
// All services share one resolver and coordinator so recovery and locking are
// consistent across install, activation, and deletion.
type Runtime struct {
	Resolver  *paths.Resolver
	Install   *install.Service
	Lifecycle *lifecycle.Service
}

// NewRuntime constructs a production runtime.
func NewRuntime() (*Runtime, error) {
	resolver := paths.New()
	coordinator := state.NewCoordinator(resolver)
	installer := install.NewServiceWithResolverAndCoordinator(resolver, coordinator)
	lifecycleService, err := lifecycle.NewService(resolver, coordinator)
	if err != nil {
		return nil, err
	}
	return &Runtime{
		Resolver:  resolver,
		Install:   installer,
		Lifecycle: lifecycleService,
	}, nil
}
