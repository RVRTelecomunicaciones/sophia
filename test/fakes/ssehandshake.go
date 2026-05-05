package fakes

import "context"

// FakeSSEProber is a programmable SSEProber for tests.
type FakeSSEProber struct {
	ProbeErr   error
	ProbeCalls int
}

// NewFakeSSEProber returns a fake that succeeds by default.
func NewFakeSSEProber() *FakeSSEProber { return &FakeSSEProber{} }

// Probe records the call and returns the configured error.
func (f *FakeSSEProber) Probe(_ context.Context) error {
	f.ProbeCalls++
	return f.ProbeErr
}
