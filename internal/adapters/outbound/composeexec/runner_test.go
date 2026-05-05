package composeexec_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/composeexec"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

func TestComposeExecImplementsRunner(t *testing.T) {
	var _ outbound.ComposeRunner = composeexec.New(composeexec.Config{})
}

func TestComposeExecVersionRunsBinary(t *testing.T) {
	r := composeexec.New(composeexec.Config{
		DockerBinary: "echo",
		ExtraArgs:    []string{"Docker", "Compose", "version", "v2.32.1"},
	})
	out, err := r.Version(context.Background())
	if err != nil {
		t.Fatalf("Version err: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestUpRequiresProjectName(t *testing.T) {
	r := composeexec.New(composeexec.Config{DockerBinary: "echo"})
	if err := r.Up(context.Background(), "/p/compose.yaml", ""); err == nil {
		t.Error("expected error for empty project name")
	}
}

func TestDownRequiresComposePath(t *testing.T) {
	r := composeexec.New(composeexec.Config{DockerBinary: "echo"})
	if err := r.Down(context.Background(), "", "sophia"); err == nil {
		t.Error("expected error for empty compose path")
	}
}
