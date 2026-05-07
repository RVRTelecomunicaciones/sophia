package gitcli_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/gitcli"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

func TestGitCLIImplementsInspector(t *testing.T) {
	var _ outbound.GitInspector = gitcli.New(gitcli.Config{})
}

func TestGitCLIVersionWithEcho(t *testing.T) {
	g := gitcli.New(gitcli.Config{Binary: "echo", VersionArgs: []string{"git", "version", "2.46.0"}})
	v, err := g.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v == "" {
		t.Error("expected non-empty version")
	}
}

func TestGitCLIRepoRootReturnsBinaryOutput(t *testing.T) {
	g := gitcli.New(gitcli.Config{Binary: "echo"})
	out, err := g.RepoRoot(context.Background(), "")
	if err != nil {
		t.Fatalf("RepoRoot err: %v", err)
	}
	// echo emits its args; we passed "rev-parse --show-toplevel" → that's the output.
	if !strings.Contains(out, "rev-parse") {
		t.Errorf("RepoRoot output = %q, expected to contain 'rev-parse'", out)
	}
}

func TestGitCLIRepoRootErrorWhenBinaryFails(t *testing.T) {
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("/usr/bin/false (or equivalent) not available")
	}
	g := gitcli.New(gitcli.Config{Binary: "false"})
	_, err := g.RepoRoot(context.Background(), "")
	if err == nil {
		t.Error("expected error when binary exits non-zero")
	}
}

func TestGitCLIRemoteURLReturnsBinaryOutput(t *testing.T) {
	g := gitcli.New(gitcli.Config{Binary: "echo"})
	out, err := g.RemoteURL(context.Background(), "")
	if err != nil {
		t.Fatalf("RemoteURL err: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty RemoteURL output")
	}
}

func TestGitCLIRemoteURLSwallowsErrorReturnsEmpty(t *testing.T) {
	// RemoteURL is documented to return ("", nil) on error so the caller
	// can fall back gracefully (no remote configured is a normal state).
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("/usr/bin/false (or equivalent) not available")
	}
	g := gitcli.New(gitcli.Config{Binary: "false"})
	out, err := g.RemoteURL(context.Background(), "")
	if err != nil {
		t.Errorf("RemoteURL must swallow errors and return empty, got err: %v", err)
	}
	if out != "" {
		t.Errorf("RemoteURL on error = %q, want empty", out)
	}
}

func TestGitCLICurrentBranchReturnsBinaryOutput(t *testing.T) {
	g := gitcli.New(gitcli.Config{Binary: "echo"})
	out, err := g.CurrentBranch(context.Background(), "")
	if err != nil {
		t.Fatalf("CurrentBranch err: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty CurrentBranch output")
	}
}

func TestGitCLIStatusCleanWhenOutputEmpty(t *testing.T) {
	// `true` exits 0 with no stdout — Status interprets empty as clean.
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("/usr/bin/true not available")
	}
	g := gitcli.New(gitcli.Config{Binary: "true"})
	clean, count, err := g.Status(context.Background(), "")
	if err != nil {
		t.Fatalf("Status err: %v", err)
	}
	if !clean {
		t.Error("Status should report clean when stdout is empty")
	}
	if count != 0 {
		t.Errorf("dirty count = %d, want 0", count)
	}
}

func TestGitCLIStatusDirtyWhenOutputNonEmpty(t *testing.T) {
	// echo emits its args (one line) → Status reports dirty with 1 line.
	g := gitcli.New(gitcli.Config{Binary: "echo"})
	clean, count, err := g.Status(context.Background(), "")
	if err != nil {
		t.Fatalf("Status err: %v", err)
	}
	if clean {
		t.Error("Status should report dirty when stdout has lines")
	}
	if count != 1 {
		t.Errorf("dirty count = %d, want 1 (one echo line)", count)
	}
}

func TestGitCLIStatusErrorWhenBinaryFails(t *testing.T) {
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("/usr/bin/false not available")
	}
	g := gitcli.New(gitcli.Config{Binary: "false"})
	_, _, err := g.Status(context.Background(), "")
	if err == nil {
		t.Error("expected error when binary exits non-zero")
	}
}
