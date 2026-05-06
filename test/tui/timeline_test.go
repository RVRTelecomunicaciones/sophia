package tui_integration_test

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("NO_COLOR", "1")
	os.Exit(m.Run())
}

// TestTimelineSnapshotRenders sends a SnapshotMsg and asserts the explore
// row shows up running.
func TestTimelineSnapshotRenders(t *testing.T) {
	tm := teatest.NewTestModel(
		t,
		newRoot(domain.ChangeID("01HXABC")),
		teatest.WithInitialTermSize(120, 40),
	)
	defer tm.Quit() //nolint:errcheck

	tm.Send(tui.SnapshotMsg{Change: &domain.Change{
		ID:             domain.ChangeID("01HXABC"),
		Status:         domain.ChangeStatusRunning,
		CurrentPhaseID: "p-explore",
		Phases: []domain.Phase{
			{ID: "p-explore", Type: domain.PhaseExplore, Status: domain.PhaseStatusRunning},
		},
	}})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		s := string(b)
		return strings.Contains(s, "explore") &&
			(strings.Contains(s, "running") || strings.Contains(s, "▶"))
	}, teatest.WithDuration(2*time.Second))
}

// TestTimelinePhaseStartedEvent dispatches an EventMsg and verifies the
// row updates.
func TestTimelinePhaseStartedEvent(t *testing.T) {
	tm := teatest.NewTestModel(
		t,
		newRoot(domain.ChangeID("01HXABC")),
		teatest.WithInitialTermSize(120, 40),
	)
	defer tm.Quit() //nolint:errcheck

	tm.Send(tui.EventMsg{Event: domain.Event{
		Type:    "phase.started",
		Payload: map[string]any{"phase_type": "proposal", "phase_id": "p-1"},
	}})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		s := string(b)
		return strings.Contains(s, "proposal") &&
			(strings.Contains(s, "running") || strings.Contains(s, "▶"))
	}, teatest.WithDuration(2*time.Second))
}

// TestTimelineQDetaches presses Q and expects the program to exit cleanly.
func TestTimelineQDetaches(t *testing.T) {
	tm := teatest.NewTestModel(
		t,
		newRoot(domain.ChangeID("01HXABC")),
		teatest.WithInitialTermSize(120, 40),
	)

	tm.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})

	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	final := tm.FinalModel(t)
	rm, ok := final.(rootModelExposed)
	if !ok {
		t.Fatalf("final model = %T, want rootModelExposed", final)
	}
	if !rm.State().Detached() {
		t.Error("final model should be detached after Q")
	}
}

// TestTimelineCtrlCConfirmThenDetach presses Ctrl+C twice.
func TestTimelineCtrlCConfirmThenDetach(t *testing.T) {
	tm := teatest.NewTestModel(
		t,
		newRoot(domain.ChangeID("01HXABC")),
		teatest.WithInitialTermSize(120, 40),
	)

	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Detach?")
	}, teatest.WithDuration(time.Second))

	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	final := tm.FinalModel(t)
	rm, ok := final.(rootModelExposed)
	if !ok {
		t.Fatalf("final model = %T", final)
	}
	if !rm.State().Detached() {
		t.Error("final model should be detached after second Ctrl+C")
	}
}

// rootModelExposed exposes the inner Model so tests can assert on state.
type rootModelExposed interface {
	State() tui.Model
}

// wrappedModel implements tea.Model for v2 (Init returns tea.Cmd, View returns tea.View).
type wrappedModel struct {
	state tui.Model
}

func (m wrappedModel) State() tui.Model { return m.state }

func (m wrappedModel) Init() tea.Cmd {
	return nil
}

func (m wrappedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	newState, cmd := tui.Update(m.state, msg)
	m.state = newState
	return m, cmd
}

func (m wrappedModel) View() tea.View {
	return tea.NewView(tui.View(m.state))
}

func newRoot(id domain.ChangeID) tea.Model {
	return wrappedModel{state: tui.NewModel(tui.ModelConfig{ChangeID: id})}
}
