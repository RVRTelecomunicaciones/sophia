package fakes_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func TestFakeGitImplementsGitInspector(t *testing.T) {
	var _ outbound.GitInspector = (*fakes.FakeGitInspector)(nil)
}

func TestFakeGitVersion(t *testing.T) {
	g := fakes.NewFakeGitInspector()
	g.GitVersion = "git version 2.46.0"
	v, err := g.Version(context.Background())
	if err != nil {
		t.Fatalf("Version err: %v", err)
	}
	if v != "git version 2.46.0" {
		t.Errorf("Version = %q", v)
	}
}

func TestFakeGitRepoRootError(t *testing.T) {
	g := fakes.NewFakeGitInspector()
	g.NotARepo = true
	_, err := g.RepoRoot(context.Background(), "/anywhere")
	if err == nil {
		t.Error("expected error when NotARepo")
	}
}

func TestFakeGitStatus(t *testing.T) {
	g := fakes.NewFakeGitInspector()
	g.Clean = false
	g.DirtyCount = 3
	clean, n, err := g.Status(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if clean || n != 3 {
		t.Errorf("got clean=%v n=%d", clean, n)
	}
}
