package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newRunDeps(t *testing.T, sinkBuf *bytes.Buffer) (cli.Deps, *fakes.FakeOrchestrator, *fakes.FakeEventStream) {
	t.Helper()
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	pc := fakes.NewFakeProjectConfigStore()
	uc := fakes.NewFakeUserConfigStore()

	_ = pc.Write(context.Background(), "/repo/.sophia.yaml", &domain.ProjectConfig{
		Project: "ms-cotizacion", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})

	resolver := application.NewConfigResolver(application.ConfigResolverDeps{
		ProjectStore: pc, UserStore: uc, Git: git,
	})

	factory := func(sink inbound.EventSink) *application.Runner {
		return application.NewRunner(application.RunnerDeps{
			Orch:        orch,
			State:       state,
			Git:         git,
			Sink:        sink,
			EventStream: stream,
		}, application.RunnerOptions{})
	}

	return cli.Deps{
		Resolver:         resolver,
		RunnerFactory:    factory,
		JSONSinkOverride: newTestSink(sinkBuf),
	}, orch, stream
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
	deps, _, _ := newRunDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"run", "--no-tui", "--json"})
	if err := c.Execute(); err == nil {
		t.Error("expected error when message missing")
	}
}

func TestRunCommandJSONLModeSucceeds(t *testing.T) {
	t.Setenv(application.EnvOrchestratorURL, "")
	t.Setenv(application.EnvProject, "")
	t.Setenv(application.EnvBaseRef, "")
	var sinkBuf bytes.Buffer
	deps, orch, stream := newRunDeps(t, &sinkBuf)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
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

func TestRunCommandNoTUIWithoutJSONFails(t *testing.T) {
	deps, _, _ := newRunDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"run", "msg", "--no-tui"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected error when --no-tui used without --json")
	}
	if !strings.Contains(err.Error(), "--json") {
		t.Errorf("error should mention --json: %v", err)
	}
}

func TestRunCommandJSONWithoutNoTUIFails(t *testing.T) {
	deps, _, _ := newRunDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"run", "msg", "--json"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected error when --json used without --no-tui")
	}
	if !strings.Contains(err.Error(), "--no-tui") {
		t.Errorf("error should mention --no-tui: %v", err)
	}
}

func TestRunCommandDefaultModeStartsTUI(t *testing.T) {
	t.Setenv(application.EnvOrchestratorURL, "")
	t.Setenv(application.EnvProject, "")
	t.Setenv(application.EnvBaseRef, "")
	var sinkBuf bytes.Buffer
	deps, orch, stream := newRunDeps(t, &sinkBuf)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	deps.TUIOutput = &bytes.Buffer{}

	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"run", "test message"})
	if err := c.Execute(); err != nil {
		t.Fatalf("default mode should not error on natural exit: %v", err)
	}
	// JSONSinkOverride must NOT have received the snapshot — TUI bridge did.
	if strings.Contains(sinkBuf.String(), "snap:") {
		t.Errorf("default mode should use TUI sink, not jsonsink override: %s", sinkBuf.String())
	}
}

func TestRunCommandReturnsExitErrorOnFailure(t *testing.T) {
	t.Setenv(application.EnvOrchestratorURL, "")
	t.Setenv(application.EnvProject, "")
	t.Setenv(application.EnvBaseRef, "")
	var sinkBuf bytes.Buffer
	deps, orch, stream := newRunDeps(t, &sinkBuf)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusFailed)
			stream.Close(target)
		}()
	}

	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"run", "msg", "--no-tui", "--json"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}
