package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Action is the discrete user action a keypress maps to.
type Action int

const (
	ActionNone Action = iota
	ActionDetach
	ActionConfirmDetach
	ActionConfirmYes
	ActionConfirmNo
	ActionToggleView // M7: Tab toggles Timeline ↔ ApplyBoard
	ActionOpenBanner // M7: [O] opens the approval gate URL in a browser
)

// classifyKey reads a tea.KeyPressMsg and returns the Action it triggers.
func classifyKey(msg tea.KeyPressMsg, confirming bool) Action {
	keyStr := strings.ToLower(keyPressString(msg))
	switch keyStr {
	case "q":
		return ActionDetach
	case "ctrl+c":
		if confirming {
			return ActionConfirmYes
		}
		return ActionConfirmDetach
	case "y":
		if confirming {
			return ActionConfirmYes
		}
		return ActionNone
	case "n":
		if confirming {
			return ActionConfirmNo
		}
		return ActionNone
	case "tab":
		// D-M7-02: Tab in confirm mode cancels confirm AND toggles view.
		// Update reads this Action and clears confirmingDetach before
		// flipping currentView.
		return ActionToggleView
	case "o":
		// [O] only does something useful when the banner is visible.
		// Update gates on m.BannerGate() != nil.
		return ActionOpenBanner
	}
	if confirming {
		return ActionConfirmNo
	}
	return ActionNone
}

// keyPressString returns the canonical lowercase form of a tea.KeyPressMsg.
//
// In v2, KeyPressMsg.String() returns Key.Text when non-empty (e.g. "q"),
// or falls back to Key.Keystroke() which builds "ctrl+c", "enter", etc.
func keyPressString(msg tea.KeyPressMsg) string {
	return msg.String()
}
