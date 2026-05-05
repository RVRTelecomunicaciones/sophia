package domain_test

import (
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestSentinelErrorsAreDistinct(t *testing.T) {
	all := []error{
		domain.ErrConfigMissing,
		domain.ErrChangeNotFound,
		domain.ErrUnreachable,
		domain.ErrInvalidYAML,
		domain.ErrNotARepo,
		domain.ErrInvalidURL,
	}
	seen := make(map[string]bool)
	for _, e := range all {
		if e == nil {
			t.Fatal("sentinel error is nil")
		}
		if seen[e.Error()] {
			t.Errorf("duplicate sentinel: %q", e.Error())
		}
		seen[e.Error()] = true
	}
}

func TestSentinelErrorsAreErrorsAsTarget(t *testing.T) {
	wrapped := errors.Join(domain.ErrConfigMissing, errors.New("context"))
	if !errors.Is(wrapped, domain.ErrConfigMissing) {
		t.Error("ErrConfigMissing should be reachable via errors.Is")
	}
}
