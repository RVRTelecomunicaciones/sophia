package fakes_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func TestFakePathResolverImplementsPort(t *testing.T) {
	var _ outbound.PathResolver = (*fakes.FakePathResolver)(nil)
}

func TestFakePathResolverReturnsConfiguredPaths(t *testing.T) {
	r := fakes.NewFakePathResolver()
	r.Paths = domain.XDGPaths{ConfigRoot: "/c", StateRoot: "/s", DataRoot: "/d"}
	got, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigRoot != "/c" {
		t.Errorf("ConfigRoot = %q", got.ConfigRoot)
	}
}

func TestFakePathResolverEnsureDirsRecords(t *testing.T) {
	r := fakes.NewFakePathResolver()
	_ = r.EnsureDirs(domain.XDGPaths{StateRoot: "/s"})
	if r.EnsureDirsCalls != 1 {
		t.Errorf("EnsureDirsCalls = %d", r.EnsureDirsCalls)
	}
}

func TestFakePathResolverValidateError(t *testing.T) {
	r := fakes.NewFakePathResolver()
	r.ValidateErr = fakes.ErrFakeValidate
	if err := r.ValidateDirs(domain.XDGPaths{}); err == nil {
		t.Error("expected error")
	}
}
