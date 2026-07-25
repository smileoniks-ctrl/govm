package local

import (
	"context"

	"github.com/smileoniks-ctrl/govm/internal/paths"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

type VersionScanner struct {
	resolver *paths.Resolver
}

func NewVersionScanner(resolver *paths.Resolver) *VersionScanner {
	return &VersionScanner{resolver: resolver}
}

func (s *VersionScanner) ScanInstalled(ctx context.Context) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dir, err := s.resolver.VersionsDir()
	if err != nil {
		return nil, err
	}

	installed, err := utils.ScanInstalledVersions(dir)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return installed, nil
}
