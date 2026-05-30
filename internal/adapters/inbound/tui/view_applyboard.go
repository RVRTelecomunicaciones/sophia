package tui

import (
	"fmt"
	"strings"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// viewApplyBoard renders the ApplyBoard panel.
//
// Design §6: header, empty-state, group tree with team-lead session, task list
// with implement-session + status + attempts + escalation summary, materialize
// block, per-group materialize errors. PR-2 full implementation.
func viewApplyBoard(m Model) string {
	var b strings.Builder

	board := m.ApplyBoard()

	// Header — includes materialize status when active.
	header := buildApplyBoardHeader(m.ChangeID(), board)
	b.WriteString(pkgStyles.header.Render(header))
	b.WriteString("\n\n")

	groups := board.Groups()
	if len(groups) == 0 && board.MaterializeStatus() == "" {
		// D1: empty state.
		b.WriteString(pkgStyles.hint.Render("No tasks yet. Apply phase will populate this view."))
		b.WriteString("\n\n")
	} else {
		// D2/D3: group tree.
		for _, g := range groups {
			b.WriteString(renderApplyGroup(g))
		}
		// D4: board-level materialize block.
		if ms := board.MaterializeStatus(); ms != "" {
			b.WriteString(renderMaterializeBlock(ms, board.MaterializeTarget(), board.MaterializeGroupsCount()))
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

// buildApplyBoardHeader constructs the header line, appending materialize
// status when it is active.
func buildApplyBoardHeader(changeID domain.ChangeID, board ApplyBoardState) string {
	base := fmt.Sprintf("Change %s · ApplyBoard · %d groups", changeID, board.GroupCount())
	if ms := board.MaterializeStatus(); ms != "" {
		base += fmt.Sprintf(" · materialize %s", ms)
		if t := board.MaterializeTarget(); t != "" {
			base += " " + t
		}
	}
	return base
}

// renderApplyGroup renders one group as a header line + team-lead session line
// + per-task lines (D2, D3). The group header uses styleFor/iconFor so that
// failed/degraded/escalated groups are visually distinct.
func renderApplyGroup(g ApplyGroup) string {
	var b strings.Builder

	icon := pkgStyles.iconFor(g.Status)
	style := pkgStyles.styleFor(g.Status)

	// Group header — for degraded groups also show failed_dep inline (D3, §6.3).
	groupHeader := fmt.Sprintf("▼ %s %s %s", icon, g.ID, g.Status)
	if g.Status == "degraded" && g.FailedDep != "" {
		groupHeader += fmt.Sprintf(" failed_dep=%s", g.FailedDep)
	}
	b.WriteString(style.Render(groupHeader))
	b.WriteString("\n")

	// Team-lead session line (§6.4).
	if g.TeamLeadSession.ID != "" {
		b.WriteString(pkgStyles.hint.Render(fmt.Sprintf("  ⌘ team_lead %s", g.TeamLeadSession.ID)))
		b.WriteString("\n")
	}

	// Per-task lines (§6.5, §6.6).
	for _, t := range g.Tasks {
		b.WriteString(renderApplyTask(t))
	}

	// Per-group materialize error (§6.7).
	if g.MaterializeErr != "" {
		b.WriteString(pkgStyles.errorLine.Render(fmt.Sprintf("  materialize error: %s", g.MaterializeErr)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	return b.String()
}

// renderApplyTask renders a single task row and its sub-lines.
func renderApplyTask(t ApplyTask) string {
	var b strings.Builder

	icon := pkgStyles.iconFor(t.Status)
	style := pkgStyles.styleFor(t.Status)

	line := fmt.Sprintf("  %s %-12s %-10s", icon, t.ID, t.Status)
	if t.Attempts > 0 {
		line += fmt.Sprintf(" attempts=%d", t.Attempts)
	}
	b.WriteString(style.Render(line))
	b.WriteString("\n")

	// Implement session line (§6.6).
	if t.ImplementSession.ID != "" {
		b.WriteString(pkgStyles.hint.Render(fmt.Sprintf("    ⌁ implement %s", t.ImplementSession.ID)))
		b.WriteString("\n")
	}

	// Escalation summary (§6.5 inline summary).
	if t.EscalationReason != "" {
		b.WriteString(pkgStyles.styleFor("escalated").Render(fmt.Sprintf("    escalation: %s", t.EscalationReason)))
		b.WriteString("\n")
	}

	return b.String()
}

// renderMaterializeBlock renders the board-level materialize progress footer
// (D4). Shows "running", "completed" (with count), or "error".
func renderMaterializeBlock(status, target string, groupsCount int) string {
	var b strings.Builder
	style := pkgStyles.styleFor(status)

	var line string
	switch status {
	case "completed":
		line = fmt.Sprintf("materialize completed: %s (%d groups)", target, groupsCount)
	case "running":
		line = fmt.Sprintf("materialize running: %s", target)
	case "error":
		line = fmt.Sprintf("materialize error: %s", target)
	default:
		line = fmt.Sprintf("materialize %s: %s", status, target)
	}
	b.WriteString(style.Render(line))
	b.WriteString("\n\n")
	return b.String()
}
