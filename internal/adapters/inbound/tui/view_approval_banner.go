package tui

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// bannerStyle is the lipgloss style for the approval banner box.
var bannerStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("9")). // bright red — attention
	Padding(0, 1).
	MarginBottom(1)

// renderApprovalBanner returns the rendered banner for the given gate.
// Banner stays visible until Update clears m.bannerGate via approval.resolved,
// forward-progress phase.started, or snapshot showing past phase.
//
// Spec §2.2 layout: Phase / Risk / Reason / Policy / Gate / Status / [O].
// Spec §6.3 inv 7: every untrusted string is concatenated into a body string
// then rendered through bannerStyle.Render — lipgloss treats this as a single
// styled block; embedded ANSI in user input is not re-evaluated.
func renderApprovalBanner(_ Model, gate domain.ApprovalGate) string {
	var b strings.Builder
	b.WriteString("Approval required by governance\n\n")
	b.WriteString(fmt.Sprintf("Phase: %-12s Risk: %s\n",
		stringOrDash(string(gate.Phase)),
		stringOrDash(gate.Risk),
	))
	b.WriteString(fmt.Sprintf("Reason: %s\n", stringOrDash(gate.Reason)))
	b.WriteString(fmt.Sprintf("Policy: %s\n", stringOrDash(gate.Policy)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Gate: %s\n", stringOrDash(gate.URL)))
	b.WriteString("Status: waiting\n")
	b.WriteString("\n")
	b.WriteString("[O] Open in browser")

	return bannerStyle.Render(b.String())
}

// stringOrDash returns "—" for empty strings so the banner stays aligned.
func stringOrDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
