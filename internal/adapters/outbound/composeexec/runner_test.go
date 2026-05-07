package composeexec_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/composeexec"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
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

func TestUpRunsBinarySuccessfully(t *testing.T) {
	// echo accepts any args and exits 0 — proxy for a successful compose up.
	r := composeexec.New(composeexec.Config{DockerBinary: "echo"})
	if err := r.Up(context.Background(), "/p/compose.yaml", "sophia"); err != nil {
		t.Errorf("Up should succeed when binary exits 0: %v", err)
	}
}

func TestUpReturnsErrorWhenBinaryFails(t *testing.T) {
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("/usr/bin/false not available")
	}
	r := composeexec.New(composeexec.Config{DockerBinary: "false"})
	err := r.Up(context.Background(), "/p/compose.yaml", "sophia")
	if err == nil {
		t.Error("expected error when binary exits non-zero")
	}
	if !strings.Contains(err.Error(), "compose up") {
		t.Errorf("error should be wrapped with 'compose up' context: %v", err)
	}
}

func TestDownRunsBinarySuccessfully(t *testing.T) {
	r := composeexec.New(composeexec.Config{DockerBinary: "echo"})
	if err := r.Down(context.Background(), "/p/compose.yaml", "sophia"); err != nil {
		t.Errorf("Down should succeed when binary exits 0: %v", err)
	}
}

func TestDownReturnsErrorWhenBinaryFails(t *testing.T) {
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("/usr/bin/false not available")
	}
	r := composeexec.New(composeexec.Config{DockerBinary: "false"})
	err := r.Down(context.Background(), "/p/compose.yaml", "sophia")
	if err == nil {
		t.Error("expected error when binary exits non-zero")
	}
	if !strings.Contains(err.Error(), "compose down") {
		t.Errorf("error should be wrapped with 'compose down' context: %v", err)
	}
}
