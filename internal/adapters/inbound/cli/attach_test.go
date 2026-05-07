package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/inbound"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func newAttachDeps(t *testing.T, sinkBuf *bytes.Buffer) (cli.Deps, *fakes.FakeOrchestrator, *fakes.FakeEventStream) {
	t.Helper()
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	pc := fakes.NewFakeProjectConfigStore()
	uc := fakes.NewFakeUserConfigStore()

	_ = pc.Write(context.Background(), "/repo/.sophia.yaml", &domain.ProjectConfig{
		Project: "ms-x", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})

	resolver := application.NewConfigResolver(application.ConfigResolverDeps{
		ProjectStore: pc, UserStore: uc, Git: git,
	})

	factory := func(sink inbound.EventSink) *application.Attacher {
		runner := application.NewRunner(application.RunnerDeps{
			Orch: orch, State: state, Git: git, Sink: sink, EventStream: stream,
		}, application.RunnerOptions{})
		return application.NewAttacher(application.AttacherDeps{
			Orch: orch, State: state, Git: git, Runner: runner,
		})
	}

	return cli.Deps{
		Resolver:         resolver,
		AttacherFactory:  factory,
		Orch:             orch,
		JSONSinkOverride: newTestSink(sinkBuf),
	}, orch, stream
}

func TestAttachCommandRequiresChangeID(t *testing.T) {
	deps, _, _ := newAttachDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"attach", "--no-tui", "--json"})
	if err := c.Execute(); err == nil {
		t.Error("expected error when change-id missing")
	}
}

func TestAttachCommandJSONLModeSucceeds(t *testing.T) {
	t.Setenv(application.EnvOrchestratorURL, "")
	t.Setenv(application.EnvProject, "")
	t.Setenv(application.EnvBaseRef, "")

	var sinkBuf bytes.Buffer
	deps, orch, stream := newAttachDeps(t, &sinkBuf)
	orch.SeedChange(&domain.Change{ID: "ATT-1", Project: "ms-x", Status: domain.ChangeStatusRunning})

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"attach", "ATT-1", "--no-tui", "--json"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sinkBuf.String(), "snap:") {
		t.Errorf("sink missing snapshot: %s", sinkBuf.String())
	}
	if !strings.Contains(sinkBuf.String(), "done") {
		t.Errorf("sink missing terminal status: %s", sinkBuf.String())
	}
}

func TestAttachCommandNoTUIWithoutJSONFails(t *testing.T) {
	deps, _, _ := newAttachDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"attach", "ATT-1", "--no-tui"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected error when --no-tui used without --json")
	}
	if !strings.Contains(err.Error(), "--json") {
		t.Errorf("error should mention --json: %v", err)
	}
}

func TestAttachCommandJSONWithoutNoTUIFails(t *testing.T) {
	deps, _, _ := newAttachDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"attach", "ATT-1", "--json"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected error when --json used without --no-tui")
	}
}

func TestAttachCommandPropagatesExitCode3OnNotFound(t *testing.T) {
	deps, _, _ := newAttachDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"attach", "MISSING", "--no-tui", "--json"})

	err := c.Execute()
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3", exit.Code)
	}
}

func TestAttachCommandPropagatesExitCode0OnDone(t *testing.T) {
	deps, orch, stream := newAttachDeps(t, &bytes.Buffer{})
	orch.SeedChange(&domain.Change{ID: "ATT-OK", Status: domain.ChangeStatusRunning, Project: "ms-x"})
	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"attach", "ATT-OK", "--no-tui", "--json"})
	if err := c.Execute(); err != nil {
		t.Errorf("expected nil err on done, got %v", err)
	}
}

// D-M8-13: when the snapshot already shows a phase blocked on approval and no
// SSE event ever arrives (mock orchestrator never pushes), the
// --approval-timeout MUST start at attach time and exit code MUST be 5.
func TestAttachJSONLEagerArmsTimeoutOnPendingApproval(t *testing.T) {
	deps, orch, stream := newAttachDeps(t, &bytes.Buffer{})
	orch.SeedChange(&domain.Change{
		ID:      "ATT-PEND",
		Project: "ms-x",
		Status:  domain.ChangeStatusRunning,
		Phases: []domain.Phase{
			{ID: "p1", Type: "implement", Status: domain.PhaseStatusBlocked},
		},
	})
	stream.OnSubscribe = func(_ outbound.StreamTarget) {}

	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"attach", "ATT-PEND", "--no-tui", "--json", "--approval-timeout", "40ms"})

	err := c.Execute()
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 5 {
		t.Errorf("Code = %d, want 5 (approval-timeout)", exit.Code)
	}
}
