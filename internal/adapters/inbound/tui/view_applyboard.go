package tui

import (
	"fmt"
	"strings"
)

// viewApplyBoard renders the ApplyBoard panel.
//
// PR-1: minimal placeholder rendering against the new model structs.
// PR-2 redesigns this view for the full group → task → session tree layout
// per design §6.
func viewApplyBoard(m Model) string {
	var b strings.Builder

	board := m.ApplyBoard()
	header := fmt.Sprintf("Sophia · Change %s · ApplyBoard · %d groups",
		m.ChangeID(), board.GroupCount())
	b.WriteString(pkgStyles.header.Render(header))
	b.WriteString("\n\n")

	groups := board.Groups()
	if len(groups) == 0 && board.MaterializeStatus() == "" {
		b.WriteString(pkgStyles.hint.Render("No tasks yet. Apply phase will populate this view."))
		b.WriteString("\n\n")
	} else {
		for _, g := range groups {
			b.WriteString(renderApplyGroupPlaceholder(g))
		}
		if board.MaterializeStatus() != "" {
			fmt.Fprintf(&b, "materialize %s %s\n", board.MaterializeStatus(), board.MaterializeTarget())
		}
	}

	for _, e := range m.Errors() {
		b.WriteString(pkgStyles.errorLine.Render("error: " + e))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	if m.ConfirmingDetach() {
		b.WriteString(pkgStyles.confirmDialog.Render(" Detach? (y/n) "))
	} else {
		b.WriteString(pkgStyles.hint.Render("Tab Timeline · Q detach · Ctrl+C confirm-detach"))
	}

	return truncateToWidth(b.String(), m.Width())
}

// renderApplyGroupPlaceholder renders a group with its tasks in the minimal
// PR-1 placeholder style. PR-2 replaces this with the full tree view.
func renderApplyGroupPlaceholder(g ApplyGroup) string {
	var b strings.Builder

	statusMarker := applyStatusIcon(g.Status)
	b.WriteString(pkgStyles.header.Render(fmt.Sprintf("▼ %s %s", g.ID, statusMarker)))
	b.WriteString("\n")

	if g.TeamLeadSession.ID != "" {
		fmt.Fprintf(&b, "  ⌘ team_lead %s\n", g.TeamLeadSession.ID)
	}

	for _, t := range g.Tasks {
		icon := applyStatusIcon(t.Status)
		line := fmt.Sprintf("  %s %-12s %-10s", icon, t.ID, t.Status)
		if t.Attempts > 0 {
			line += fmt.Sprintf(" attempts=%d", t.Attempts)
		}
		b.WriteString(pkgStyles.styleFor(t.Status).Render(line))
		b.WriteString("\n")
		if t.ImplementSession.ID != "" {
			fmt.Fprintf(&b, "    ⌁ implement %s\n", t.ImplementSession.ID)
		}
		if t.EscalationReason != "" {
			fmt.Fprintf(&b, "    escalation: %s\n", t.EscalationReason)
		}
	}

	if g.MaterializeErr != "" {
		b.WriteString(pkgStyles.errorLine.Render(fmt.Sprintf("  materialize error: %s", g.MaterializeErr)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	return b.String()
}

// applyStatusIcon returns a visual icon for an apply status value.
func applyStatusIcon(status string) string {
	switch status {
	case "running":
		return iconRunning
	case "completed", "done":
		return iconDone
	case "failed", "error":
		return iconFailed
	case "escalated", "degraded":
		return iconBlocked
	default:
		return iconPending
	}
}
