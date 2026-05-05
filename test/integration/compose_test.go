//go:build integration

// Package integration runs opt-in tests against a real Docker daemon.
// Run with: make test-integration
package integration_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/composeexec"
)

func dockerAvailable(t *testing.T) bool {
	t.Helper()
	if os.Getenv("CI") != "" {
		return false
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "info").CombinedOutput()
	if err != nil {
		t.Logf("docker info: %v\n%s", err, out)
		return false
	}
	return true
}

func TestComposeMaterializeAndVersion(t *testing.T) {
	if !dockerAvailable(t) {
		t.Skip("docker not available")
	}

	tmp := t.TempDir()
	res, err := composeexec.Materialize(tmp, composeexec.EmbeddedComposeYAML, false)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if !res.Wrote {
		t.Error("expected Wrote=true on first run")
	}

	r := composeexec.New(composeexec.Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	v, err := r.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v == "" {
		t.Error("Version empty")
	}
	t.Logf("compose: %s", v)
}
