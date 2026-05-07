package fakes

import (
	"errors"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// ErrFakeValidate is a sentinel for ValidateDirs failure tests.
var ErrFakeValidate = errors.New("fake validate failure")

// FakePathResolver is a programmable PathResolver for tests.
type FakePathResolver struct {
	Paths           domain.XDGPaths
	ResolveErr      error
	EnsureErr       error
	ValidateErr     error
	EnsureDirsCalls int
}

// NewFakePathResolver returns sensible defaults.
func NewFakePathResolver() *FakePathResolver {
	return &FakePathResolver{
		Paths: domain.XDGPaths{
			ConfigRoot: "/home/u/.config/sophia",
			StateRoot:  "/home/u/.local/state/sophia",
			DataRoot:   "/home/u/.local/share/sophia",
			CacheRoot:  "/home/u/.cache/sophia",
		},
	}
}

func (f *FakePathResolver) Resolve() (domain.XDGPaths, error) {
	return f.Paths, f.ResolveErr
}

func (f *FakePathResolver) EnsureDirs(_ domain.XDGPaths) error {
	f.EnsureDirsCalls++
	return f.EnsureErr
}

func (f *FakePathResolver) ValidateDirs(_ domain.XDGPaths) error {
	return f.ValidateErr
}
