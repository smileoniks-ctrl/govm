package local

import (
	"context"

	"github.com/smileoniks-ctrl/govm/internal/paths"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

type ActiveReader struct {
	resolver *paths.Resolver
}

func NewActiveReader(resolver *paths.Resolver) *ActiveReader {
	return &ActiveReader{resolver: resolver}
}

func (r *ActiveReader) ReadActive(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	path, err := r.resolver.ActiveVersionFile()
	if err != nil {
		return "", err
	}

	version, err := utils.ReadActiveVersion(path)
	if err != nil {
		return "", err
	}

	return version, nil
}

func (r *ActiveReader) GetFromPath(ctx context.Context) string {
	return utils.GetCurrentGoVersion()
}
