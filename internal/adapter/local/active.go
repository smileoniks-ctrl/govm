package local

import (
	"context"

	"github.com/smileoniks-ctrl/govm/internal/paths"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

type ActiveReader struct {
	registry *FilesystemRegistry
}

func NewActiveReader(resolver *paths.Resolver) *ActiveReader {
	return &ActiveReader{registry: NewRegistry(resolver)}
}

func (r *ActiveReader) ReadActive(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path, err := r.registry.resolver.ActiveVersionFile()
	if err != nil {
		return "", err
	}
	return utils.ReadActiveVersion(path)
}

func (r *ActiveReader) GetFromPath(ctx context.Context) string {
	return utils.GetCurrentGoVersion()
}
