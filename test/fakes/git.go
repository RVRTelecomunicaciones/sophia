package fakes

import (
	"context"
	"errors"
)

type FakeGitInspector struct {
	GitVersion string
	NotARepo   bool
	Root       string
	Remote     string
	Branch     string
	Clean      bool
	DirtyCount int
}

func NewFakeGitInspector() *FakeGitInspector {
	return &FakeGitInspector{
		GitVersion: "git version 2.46.0",
		Root:       "/repo",
		Remote:     "git@github.com:org/repo.git",
		Branch:     "main",
		Clean:      true,
	}
}

func (f *FakeGitInspector) Version(_ context.Context) (string, error) {
	return f.GitVersion, nil
}

func (f *FakeGitInspector) RepoRoot(_ context.Context, _ string) (string, error) {
	if f.NotARepo {
		return "", errors.New("not a git repository")
	}
	return f.Root, nil
}

func (f *FakeGitInspector) RemoteURL(_ context.Context, _ string) (string, error) {
	return f.Remote, nil
}

func (f *FakeGitInspector) CurrentBranch(_ context.Context, _ string) (string, error) {
	return f.Branch, nil
}

func (f *FakeGitInspector) Status(_ context.Context, _ string) (bool, int, error) {
	return f.Clean, f.DirtyCount, nil
}
