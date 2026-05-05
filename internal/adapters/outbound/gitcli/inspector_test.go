package gitcli_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/gitcli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
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
