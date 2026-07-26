package local

import (
	"context"

	"github.com/smileoniks-ctrl/govm/internal/paths"
)

type VersionScanner struct {
	registry *FilesystemRegistry
}

func NewVersionScanner(resolver *paths.Resolver) *VersionScanner {
	return &VersionScanner{registry: NewRegistry(resolver)}
}

func (s *VersionScanner) ScanInstalled(ctx context.Context) (map[string]string, error) {
	toolchains, err := s.registry.List(ctx)
	if err != nil {
		return nil, err
	}

	installed := make(map[string]string, len(toolchains))
	for _, toolchain := range toolchains {
		installed[toolchain.Version] = toolchain.Path
	}
	return installed, nil
}
