package fakes

import (
	"context"
	"sync"
)

type FakeComposeRunner struct {
	mu              sync.Mutex
	ComposeVersion  string
	UpErr           error
	DownErr         error
	UpCalls         int
	DownCalls       int
	LastComposePath string
	LastProjectName string
}

func NewFakeComposeRunner() *FakeComposeRunner {
	return &FakeComposeRunner{ComposeVersion: "Docker Compose version v2.32.1"}
}

func (f *FakeComposeRunner) Version(_ context.Context) (string, error) {
	return f.ComposeVersion, nil
}

func (f *FakeComposeRunner) Up(_ context.Context, composePath, projectName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.UpCalls++
	f.LastComposePath = composePath
	f.LastProjectName = projectName
	return f.UpErr
}

func (f *FakeComposeRunner) Down(_ context.Context, composePath, projectName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.DownCalls++
	f.LastComposePath = composePath
	f.LastProjectName = projectName
	return f.DownErr
}
