package fakes

import "context"

type FakeComposeRunner struct {
	ComposeVersion string
	UpErr          error
	DownErr        error
	UpCalls        int
	DownCalls      int
}

func NewFakeComposeRunner() *FakeComposeRunner {
	return &FakeComposeRunner{ComposeVersion: "Docker Compose version v2.32.1"}
}

func (f *FakeComposeRunner) Version(_ context.Context) (string, error) {
	return f.ComposeVersion, nil
}

func (f *FakeComposeRunner) Up(_ context.Context, _, _ string) error {
	f.UpCalls++
	return f.UpErr
}

func (f *FakeComposeRunner) Down(_ context.Context, _, _ string) error {
	f.DownCalls++
	return f.DownErr
}
