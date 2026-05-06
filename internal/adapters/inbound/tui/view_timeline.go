package tui

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

var pkgStyles = newStyles()

// View is the pure rendering function. Spec §6.3 inv 7: every untrusted
// string flows through pkgStyles.<Style>.Render to ensure ANSI escapes are
// emitted through the styled wrapper (not passed raw as leading output).
// View is pure: same Model → same output every time (no time.Now calls).
func View(m Model) string {
	var b strings.Builder

	header := fmt.Sprintf("Sophia · Change %s · %s", m.ChangeID(), m.ChangeStatus())
	b.WriteString(pkgStyles.header.Render(header))
	b.WriteString("\n\n")

	for _, row := range m.PhaseRows() {
		b.WriteString(renderPhaseRow(row))
		b.WriteString("\n")
	}

	for _, e := range m.Errors() {
		// Untrusted string flows through Render (§6.3 inv 7).
		b.WriteString(pkgStyles.errorLine.Render("error: " + e))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	if m.ConfirmingDetach() {
		b.WriteString(pkgStyles.confirmDialog.Render(" Detach? (y/n) "))
	} else {
		b.WriteString(pkgStyles.hint.Render("Q to detach · Ctrl+C confirm-then-detach"))
	}

	rendered := b.String()
	return truncateToWidth(rendered, m.Width())
}

func renderPhaseRow(row PhaseRow) string {
	statusStr := string(row.Status)
	style := pkgStyles.styleFor(statusStr)
	icon := pkgStyles.iconFor(statusStr)

	approval := ""
	if row.HasApproval {
		approval = " " + pkgStyles.approvalMark.Render(iconApproval)
	}

	// Duration: only show for completed phases (StartedAt + EndedAt both set).
	// Omitting time.Now() keeps View pure — no live duration while running.
	dur := ""
	if !row.StartedAt.IsZero() && !row.EndedAt.IsZero() {
		d := row.EndedAt.Sub(row.StartedAt).Round(1e9) // round to seconds
		if d > 0 {
			dur = fmt.Sprintf(" %s", d)
		}
	}

	conf := ""
	if row.Confidence > 0 {
		conf = fmt.Sprintf(" [%.2f]", row.Confidence)
	}

	body := fmt.Sprintf("%s %-9s %-8s%s%s", icon, row.Type, statusStr, dur, conf)
	rendered := style.Render(body)
	return rendered + approval
}

func truncateToWidth(s string, w int) string {
	if w <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if lipgloss.Width(line) > w {
			runes := []rune(line)
			if len(runes) > w {
				runes = runes[:w]
			}
			lines[i] = string(runes)
		}
	}
	return strings.Join(lines, "\n")
}
