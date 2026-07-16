package setup

import (
	"errors"
	"testing"
)

type failingShimResolver struct {
	err error
}

func (r failingShimResolver) ShimDir() (string, error) {
	return "", r.err
}

func TestNewWithResolverReturnsShimDirError(t *testing.T) {
	wantErr := errors.New("home directory unavailable")

	_, err := newWithResolver(failingShimResolver{err: wantErr})

	if !errors.Is(err, wantErr) {
		t.Fatalf("newWithResolver error = %v, want %v", err, wantErr)
	}
}
