package tui_integration_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// TestTUITabTogglesToApplyBoard sends a task.started event, presses Tab,
// and asserts the ApplyBoard view replaces the Timeline.
func TestTUITabTogglesToApplyBoard(t *testing.T) {
	tm := teatest.NewTestModel(
		t,
		newRoot(domain.ChangeID("01HXTAB")),
		teatest.WithInitialTermSize(120, 40),
	)
	defer tm.Quit() //nolint:errcheck

	// Feed a task.started event so the ApplyBoard has content.
	tm.Send(tui.EventMsg{Event: domain.Event{
		Type: "task.started",
		Payload: map[string]any{
			"group_id":      "g1",
			"task_id":       "t1",
			"files_pattern": "internal/**",
		},
	}})

	// Initially we're in Timeline — assert phase names appear.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "explore")
	}, teatest.WithDuration(2*time.Second))

	// Press Tab — switch to ApplyBoard.
	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab})

	// ApplyBoard should now show the header and task content.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		s := string(b)
		return strings.Contains(s, "ApplyBoard") &&
			strings.Contains(s, "g1") &&
			strings.Contains(s, "t1")
	}, teatest.WithDuration(2*time.Second))

	// Press Tab again — back to Timeline.
	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "explore")
	}, teatest.WithDuration(2*time.Second))
}

// TestTUIApprovalBannerAppearsAndOpensBrowser sends an approval.required event
// with gate_url set (so the banner URL is populated), presses [O], and asserts
// the banner is rendered and the browser-open callback is invoked.
//
// We use a rootShim that mimics the Program's OpenBrowserMsg interception so
// teatest can verify the [O] flow without a real osbrowser.
//
// Note: ApprovalGateMsg routes through approvalGateAsEvent which only passes
// "phase" in the payload — it does NOT propagate gate_url. We therefore use
// EventMsg{approval.required} with gate_url in the payload, which flows through
// applyBannerFromEvent and correctly populates bannerGate.URL.
func TestTUIApprovalBannerAppearsAndOpensBrowser(t *testing.T) {
	openedURLCh := make(chan string, 1)
	tm := teatest.NewTestModel(
		t,
		newRootWithBrowser(domain.ChangeID("01HXBANNER"), func(url string) error {
			openedURLCh <- url
			return nil
		}),
		teatest.WithInitialTermSize(120, 40),
	)
	defer tm.Quit() //nolint:errcheck

	// Send approval.required with gate_url so bannerGate.URL is populated.
	tm.Send(tui.EventMsg{Event: domain.Event{
		Type: "approval.required",
		Payload: map[string]any{
			"gate_url": "https://gov.local/approvals/abc",
			"phase":    "apply",
			"risk":     "medium",
			"reason":   "policy says no apply without tasks approved",
		},
	}})

	// Banner should render at the top.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Approval required")
	}, teatest.WithDuration(2*time.Second))

	// Press [O].
	tm.Send(tea.KeyPressMsg{Code: 'o', Text: "o"})

	// Assert the browser open callback was called with the expected URL.
	select {
	case url := <-openedURLCh:
		if url != "https://gov.local/approvals/abc" {
			t.Errorf("opened URL = %q", url)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("browser open was not called within 2s")
	}
}

// rootShim mimics the Program's interception of OpenBrowserMsg for teatest.
// When Update receives an OpenBrowserMsg it calls openCallback (the test's
// verification hook) in a goroutine — same pattern Program uses for
// handleOpenBrowser — then returns without forwarding to tui.Update (which
// treats OpenBrowserMsg as a no-op anyway).
type rootShim struct {
	state        tui.Model
	openCallback func(url string) error
}

func newRootWithBrowser(id domain.ChangeID, openCallback func(url string) error) tea.Model {
	return rootShim{
		state:        tui.NewModel(tui.ModelConfig{ChangeID: id}),
		openCallback: openCallback,
	}
}

func (m rootShim) Init() tea.Cmd { return nil }

func (m rootShim) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Intercept OpenBrowserMsg before tui.Update sees it.
	if om, ok := msg.(tui.OpenBrowserMsg); ok {
		if m.openCallback != nil {
			go func(url string) {
				_ = m.openCallback(url)
			}(om.URL)
		}
		return m, nil
	}
	newState, cmd := tui.Update(m.state, msg)
	m.state = newState
	return m, cmd
}

func (m rootShim) View() tea.View { return tea.NewView(tui.View(m.state)) }
