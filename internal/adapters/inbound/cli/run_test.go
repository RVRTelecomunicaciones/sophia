package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newRunDeps(t *testing.T, sinkBuf *bytes.Buffer) (cli.Deps, *fakes.FakeOrchestrator) {
	t.Helper()
	orch := fakes.NewFakeOrchestrator()
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	pc := fakes.NewFakeProjectConfigStore()
	uc := fakes.NewFakeUserConfigStore()

	// Pre-populate .sophia.yaml so resolver finds the project.
	_ = pc.Write(context.Background(), "/repo/.sophia.yaml", &domain.ProjectConfig{
		Project: "ms-cotizacion", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})

	resolver := application.NewConfigResolver(application.ConfigResolverDeps{
		ProjectStore: pc, UserStore: uc, Git: git,
	})

	return cli.Deps{
		Resolver: resolver,
		Runner: application.NewRunner(application.RunnerDeps{
			Orch:  orch,
			State: state,
			Git:   git,
			Sink:  newTestSink(sinkBuf),
		}, application.RunnerOptions{
			PollMin: time.Millisecond,
			PollMax: 5 * time.Millisecond,
		}),
	}, orch
}

func newTestSink(w *bytes.Buffer) *testSink {
	return &testSink{w: w}
}

type testSink struct{ w *bytes.Buffer }

func (s *testSink) OnSnapshot(_ context.Context, c *domain.Change) error {
	_, err := s.w.WriteString("snap:" + c.ID.String() + ":" + string(c.Status) + "\n")
	return err
}
func (s *testSink) OnEvent(_ context.Context, _ domain.Event) error               { return nil }
func (s *testSink) OnApprovalGate(_ context.Context, _ domain.ApprovalGate) error { return nil }
func (s *testSink) OnError(_ context.Context, _ error) error                      { return nil }
func (s *testSink) OnComplete(_ context.Context, st domain.ChangeStatus) error {
	_, err := s.w.WriteString("done:" + string(st) + "\n")
	return err
}
func (s *testSink) Close() error { return nil }

func TestRunCommandRequiresMessage(t *testing.T) {
	deps, _ := newRunDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"run", "--no-tui", "--json"})
	if err := c.Execute(); err == nil {
		t.Error("expected error when message missing")
	}
}

func TestRunCommandSucceedsWithMessage(t *testing.T) {
	t.Setenv(application.EnvOrchestratorURL, "")
	t.Setenv(application.EnvProject, "")
	t.Setenv(application.EnvBaseRef, "")
	var sinkBuf bytes.Buffer
	deps, orch := newRunDeps(t, &sinkBuf)
	first := true
	orch.TickHook = func(c *domain.Change) {
		if first {
			c.Status = domain.ChangeStatusRunning
			first = false
		} else {
			c.Status = domain.ChangeStatusDone
		}
	}
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"run", "test message", "--no-tui", "--json"})
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

// TestRunCommandReturnsExitErrorOnFailure asserts that a Failed terminal status
// surfaces as a non-nil error from cobra. The strict ExitError.Code assertion
// is intentionally deferred to Task 8, where main.go is wired to extract the
// code via errors.As and exit accordingly.
func TestRunCommandReturnsExitErrorOnFailure(t *testing.T) {
	t.Setenv(application.EnvOrchestratorURL, "")
	t.Setenv(application.EnvProject, "")
	t.Setenv(application.EnvBaseRef, "")
	var sinkBuf bytes.Buffer
	deps, orch := newRunDeps(t, &sinkBuf)
	orch.TickHook = func(c *domain.Change) { c.Status = domain.ChangeStatusFailed }

	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"run", "msg", "--no-tui", "--json"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}
