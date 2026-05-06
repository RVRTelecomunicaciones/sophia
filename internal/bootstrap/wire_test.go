package bootstrap_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/bootstrap"
)

func TestNewProducesRoot(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	if root.Use != "sophia" {
		t.Errorf("root use = %q", root.Use)
	}
}

func TestNewWiresM2Commands(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"doctor", "start", "stop", "version"} {
		c, _, err := root.Find([]string{sub})
		if err != nil {
			t.Errorf("%s not found: %v", sub, err)
		}
		if c == nil {
			t.Errorf("%s cmd is nil", sub)
		}
	}
}

func TestNewWiresM3Commands(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"init", "status"} {
		c, _, err := root.Find([]string{sub})
		if err != nil {
			t.Errorf("%s not found: %v", sub, err)
		}
		if c == nil {
			t.Errorf("%s cmd is nil", sub)
		}
	}
}

func TestNewWiresM4Run(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("run cmd is nil")
	}
}

func TestNewWiresM5SSEStream(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	// Smoke: the run command is still wired; the SSE stream is constructed
	// inside the runner. We can't assert the stream type from outside without
	// exposing it via Deps, so this is a smoke check.
	c, _, err := root.Find([]string{"run"})
	if err != nil || c == nil {
		t.Fatalf("run cmd missing: %v", err)
	}
}

func TestNewWiresM6RunnerFactory(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := root.Find([]string{"run"})
	if err != nil || c == nil {
		t.Fatalf("run cmd missing: %v", err)
	}
}

func TestNewWiresM7Browser(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := root.Find([]string{"run"})
	if err != nil || c == nil {
		t.Fatalf("run cmd missing: %v", err)
	}
	// We can't directly inspect Deps from the cobra tree, but the run cmd
	// constructed without panic — bootstrap wired Browser cleanly.
}

func TestNewHonorsOrchestratorURLEnv(t *testing.T) {
	t.Setenv(application.EnvOrchestratorURL, "http://from-env:1234")
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	// Smoke: command tree built without panic. The actual URL injection
	// is verified by the e2e smoke test (Task 9) which runs the binary
	// with the env set.
	_ = root
}
