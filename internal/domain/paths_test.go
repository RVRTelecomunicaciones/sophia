package domain_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

func TestXDGPathsZeroValue(t *testing.T) {
	var p domain.XDGPaths
	if p.ConfigRoot != "" || p.StateRoot != "" || p.DataRoot != "" || p.CacheRoot != "" {
		t.Errorf("zero XDGPaths should have empty fields, got %+v", p)
	}
}

func TestXDGPathsAllFields(t *testing.T) {
	p := domain.XDGPaths{
		ConfigRoot: "/c", StateRoot: "/s", DataRoot: "/d", CacheRoot: "/x",
	}
	if p.ConfigRoot != "/c" || p.StateRoot != "/s" || p.DataRoot != "/d" || p.CacheRoot != "/x" {
		t.Errorf("fields lost: %+v", p)
	}
}
