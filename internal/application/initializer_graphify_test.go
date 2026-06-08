package application_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

// fakeGraphifyProber is a test double for outbound.GraphifyProber.
type fakeGraphifyProber struct {
	ProbeResult        outbound.ProberResult
	ProbeErr           error
	BootstrapFn        func(ctx context.Context) error
	BootstrapCallCount int
}

func (f *fakeGraphifyProber) Probe(_ context.Context) (outbound.ProberResult, error) {
	return f.ProbeResult, f.ProbeErr
}

func (f *fakeGraphifyProber) Bootstrap(ctx context.Context) error {
	f.BootstrapCallCount++
	if f.BootstrapFn != nil {
		return f.BootstrapFn(ctx)
	}
	return nil
}

func newInitWithProber(prober outbound.GraphifyProber) (
	*application.Initializer,
	*fakes.FakeGitInspector,
) {
	git := fakes.NewFakeGitInspector()
	store := fakes.NewFakeProjectConfigStore()
	init := application.NewInitializer(application.InitializerDeps{
		Git:          git,
		ProjectStore: store,
		Prober:       prober,
	})
	return init, git
}

// A.6 — Prober returns Available=true: Initializer logs INFO, does NOT call Bootstrap.
func TestInitializer_Run_GraphifyAvailable_NoBootstrap(t *testing.T) {
	prober := &fakeGraphifyProber{
		ProbeResult: outbound.ProberResult{Available: true, Version: "0.8.35"},
	}
	init, git := newInitWithProber(prober)
	git.Root = "/repo"

	_, err := init.Run(context.Background(), application.InitInput{
		Project:       "my-project",
		AutoBootstrap: false,
	})
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if prober.BootstrapCallCount != 0 {
		t.Errorf("Bootstrap called %d times; want 0 (graphify was available)", prober.BootstrapCallCount)
	}
}

// A.7 — Available=false AND --auto-bootstrap-graphify=true: Bootstrap called once, continues.
func TestInitializer_Run_AutoBootstrap_Success(t *testing.T) {
	prober := &fakeGraphifyProber{
		ProbeResult: outbound.ProberResult{
			Available:   false,
			MissingDeps: []string{"graphify"},
		},
		BootstrapFn: func(_ context.Context) error { return nil },
	}
	init, git := newInitWithProber(prober)
	git.Root = "/repo"

	_, err := init.Run(context.Background(), application.InitInput{
		Project:       "my-project",
		AutoBootstrap: true,
	})
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if prober.BootstrapCallCount != 1 {
		t.Errorf("Bootstrap called %d times; want 1", prober.BootstrapCallCount)
	}
}

// A.8 — Available=false AND --auto-bootstrap-graphify=false: WARN logged, exits 0, Bootstrap NOT called.
func TestInitializer_Run_Degraded_NoBootstrap(t *testing.T) {
	prober := &fakeGraphifyProber{
		ProbeResult: outbound.ProberResult{
			Available:   false,
			MissingDeps: []string{"graphify"},
		},
	}
	init, git := newInitWithProber(prober)
	git.Root = "/repo"

	_, err := init.Run(context.Background(), application.InitInput{
		Project:       "my-project",
		AutoBootstrap: false,
	})
	if err != nil {
		// Degraded mode must NOT return error (exit 0).
		t.Fatalf("Run: want nil error in degraded mode, got: %v", err)
	}
	if prober.BootstrapCallCount != 0 {
		t.Errorf("Bootstrap called %d times; want 0 (flag was false)", prober.BootstrapCallCount)
	}
}
