package bootstrap_test

import (
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/bootstrap"
)

func TestVersionInfoFallback(t *testing.T) {
	info := bootstrap.NewVersionInfo()
	if info.Version == "" || info.Commit == "" || info.BuildDate == "" {
		t.Errorf("VersionInfo has empty fields: %+v", info)
	}
	if !strings.HasPrefix(info.Version, "0.") && info.Version != "dev" {
		t.Errorf("unexpected version default: %q", info.Version)
	}
}
