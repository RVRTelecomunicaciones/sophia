package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

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
		return m.ApplyEvent(approvalGateAsEvent(msg)), nil

	case ErrorMsg:
		text := ""
		if msg.Err != nil {
			text = msg.Err.Error()
		}
		return m.WithError(text), nil

	case CompleteMsg:
		return m.WithComplete(msg.Status), tea.Quit
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
	}
	return m, nil
}

// approvalGateAsEvent translates ApprovalGateMsg into a synthetic domain.Event
// so ApplyEvent can mark the affected phase row uniformly.
func approvalGateAsEvent(msg ApprovalGateMsg) domain.Event {
	return domain.Event{
		Type: "approval.required",
		Payload: map[string]any{
			"phase": string(msg.Gate.Phase),
		},
	}
}
