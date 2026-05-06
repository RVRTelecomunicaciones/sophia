package tui

import (
	tea "charm.land/bubbletea/v2"
)

// OpenBrowserMsg is dispatched by Update when the user presses [O] with a
// banner visible. It carries the URL of the gate. The Program-level message
// loop catches OpenBrowserMsg, calls Browser.Open(url) in a goroutine, and
// dispatches a BrowserOpenedMsg back into the loop with the result.
//
// Update itself is pure — it never touches I/O. This message type is the
// seam between the pure layer and the adapter layer.
type OpenBrowserMsg struct {
	URL string
}

// BrowserOpenedMsg is dispatched after a Browser.Open call returns. Err is
// non-nil when the open failed (validation error, OS error, etc.).
type BrowserOpenedMsg struct {
	Err error
}

// Update is the pure dispatch function. Returns the new Model and an
// optional tea.Cmd. Spec §2.2 / §4.5 — UI is event-driven, no tickers.
func Update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.Resize(msg.Width, msg.Height), nil

	case tea.KeyPressMsg:
		return updateKey(m, msg)

	case SnapshotMsg:
		return m.ApplySnapshot(msg.Change), nil

	case EventMsg:
		return m.ApplyEvent(msg.Event), nil

	case ApprovalGateMsg:
		// Set the banner directly with the structured Gate. The phase row
		// marker is already set by the matching EventMsg("approval.required")
		// that fires immediately before this in the runner's dispatchEvent.
		// Going through ApplyEvent would re-run applyBannerFromEvent and
		// discard URL/Reason/Risk/Policy because the synthetic event payload
		// doesn't carry those fields.
		cp := msg.Gate
		return m.WithBannerGate(&cp), nil

	case ErrorMsg:
		text := ""
		if msg.Err != nil {
			text = msg.Err.Error()
		}
		return m.WithError(text), nil

	case CompleteMsg:
		return m.WithComplete(msg.Status), tea.Quit

	case OpenBrowserMsg:
		// Pure layer — Program intercepts before Update sees this message.
		// Treat as no-op as a safety net.
		return m, nil

	case BrowserOpenedMsg:
		if msg.Err != nil {
			return m.WithError("browser: " + msg.Err.Error()), nil
		}
		return m, nil
	}
	return m, nil
}

func updateKey(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	action := classifyKey(msg, m.ConfirmingDetach())
	switch action {
	case ActionDetach, ActionConfirmYes:
		return m.WithConfirmingDetach(false).WithDetached(true), tea.Quit
	case ActionConfirmDetach:
		return m.WithConfirmingDetach(true), nil
	case ActionConfirmNo:
		return m.WithConfirmingDetach(false), nil
	case ActionToggleView:
		// D-M7-02: Tab cancels confirm AND toggles view.
		m = m.WithConfirmingDetach(false)
		switch m.CurrentView() {
		case ViewTimeline:
			return m.WithCurrentView(ViewApplyBoard), nil
		default:
			return m.WithCurrentView(ViewTimeline), nil
		}
	case ActionOpenBanner:
		gate := m.BannerGate()
		if gate == nil {
			// D-M7-03: [O] with no banner is a no-op.
			return m, nil
		}
		url := gate.URL
		// Emit a Cmd that produces an OpenBrowserMsg. The Program-level
		// loop intercepts and runs the actual Browser.Open call.
		return m, func() tea.Msg { return OpenBrowserMsg{URL: url} }
	}
	return m, nil
}
