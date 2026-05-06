package tui

import (
	"fmt"
	"strings"
)

// viewApplyBoard renders the ApplyBoard tree (groups → tasks → agents).
// Pure: same Model → same output.
func viewApplyBoard(m Model) string {
	var b strings.Builder

	header := fmt.Sprintf("Sophia · Change %s · ApplyBoard · %d groups",
		m.ChangeID(), m.ApplyBoard().GroupCount())
	b.WriteString(pkgStyles.header.Render(header))
	b.WriteString("\n\n")

	groups := m.ApplyBoard().Groups()
	if len(groups) == 0 {
		b.WriteString(pkgStyles.hint.Render("No tasks yet. Apply phase will populate this view."))
		b.WriteString("\n\n")
	} else {
		for _, g := range groups {
			b.WriteString(renderApplyGroup(g))
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

func renderApplyGroup(g ApplyGroup) string {
	var b strings.Builder
	b.WriteString(pkgStyles.header.Render(fmt.Sprintf("▼ %s", g.ID)))
	b.WriteString("\n")
	for _, a := range g.Agents {
		b.WriteString("  ")
		b.WriteString(renderApplyAgent(a))
		b.WriteString("\n")
	}
	for _, t := range g.Tasks {
		b.WriteString("  ")
		b.WriteString(renderApplyTask(t))
		b.WriteString("\n")
		for _, a := range t.Agents {
			b.WriteString("    ")
			b.WriteString(renderApplyAgent(a))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

func renderApplyTask(t ApplyTask) string {
	icon := pkgStyles.iconFor(t.Status)
	style := pkgStyles.styleFor(t.Status)
	files := ""
	if t.FilesPattern != "" {
		files = "  " + t.FilesPattern
	}
	body := fmt.Sprintf("%s %-12s %-8s%s", icon, t.ID, t.Status, files)
	return style.Render(body)
}

func renderApplyAgent(a ApplyAgent) string {
	icon := pkgStyles.iconFor(a.Status)
	style := pkgStyles.styleFor(a.Status)
	body := fmt.Sprintf("%s %-10s  %-12s  %s", icon, a.ID, a.Role, a.Status)
	return style.Render(body)
}
