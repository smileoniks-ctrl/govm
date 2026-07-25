package services

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/smileoniks-ctrl/govm/internal/adapter/godev"
	"github.com/smileoniks-ctrl/govm/internal/adapter/local"
	"github.com/smileoniks-ctrl/govm/internal/install"
	"github.com/smileoniks-ctrl/govm/internal/lifecycle"
	"github.com/smileoniks-ctrl/govm/internal/loader"
	"github.com/smileoniks-ctrl/govm/internal/paths"
)

// Runtime is the composition root for all govm services. It wires
// together the loader, lifecycle, and install services with their
// production dependencies. main.go creates exactly one Runtime and
// passes it to CLI and TUI adapters.
type Runtime struct {
	Loader    *Loader
	Lifecycle *lifecycle.Service
	Install   *install.Service
	Paths     *paths.Resolver
}

// Loader wraps loader orchestration with production dependencies.
// Separated into its own type so it can carry method receivers
// for TUI/CLI convenience.
type Loader struct {
	deps loader.Dependencies
}

// Load is the production entry point for loading the version catalog.
func (l *Loader) Load(ctx context.Context) (loader.VersionCatalog, error) {
	return loader.LoadVersionCatalog(ctx, l.deps)
}

// NewRuntime constructs the production composition root with all
// services wired to their real dependencies.
func NewRuntime() (*Runtime, error) {
	resolver := paths.New()

	httpClient := &http.Client{Timeout: 10 * time.Second}
	releaseSource := godev.NewClient(httpClient, "https://go.dev/dl/?mode=json&include=all")
	versionScanner := local.NewVersionScanner(resolver)
	activeReader := local.NewActiveReader(resolver)
	platform := loader.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}

	loaderDeps := loader.Dependencies{
		ReleaseSource: releaseSource,
		LocalVersions: versionScanner,
		ActiveVersion: activeReader,
		Platform:      platform,
	}

	lifecycleSvc, err := lifecycle.New()
	if err != nil {
		return nil, fmt.Errorf("initialize lifecycle service: %w", err)
	}

	installSvc := install.NewService()

	return &Runtime{
		Loader:    &Loader{deps: loaderDeps},
		Lifecycle: lifecycleSvc,
		Install:   installSvc,
		Paths:     resolver,
	}, nil
}
