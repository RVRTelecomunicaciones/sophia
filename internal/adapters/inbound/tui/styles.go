package tui

import (
	lipgloss "charm.land/lipgloss/v2"
)

type stylePalette struct {
	header        lipgloss.Style
	phasePending  lipgloss.Style
	phaseRunning  lipgloss.Style
	phaseDone     lipgloss.Style
	phaseFailed   lipgloss.Style
	phaseBlocked  lipgloss.Style
	approvalMark  lipgloss.Style
	confirmDialog lipgloss.Style
	hint          lipgloss.Style
	errorLine     lipgloss.Style
}

const (
	iconPending  = " "
	iconRunning  = "▶"
	iconDone     = "✓"
	iconFailed   = "✗"
	iconBlocked  = "■"
	iconApproval = "!"
)

func newStyles() stylePalette {
	return stylePalette{
		header: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")),
		phasePending: lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")),
		phaseRunning: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("11")),
		phaseDone: lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")),
		phaseFailed: lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")),
		phaseBlocked: lipgloss.NewStyle().
			Foreground(lipgloss.Color("13")),
		approvalMark: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("9")),
		confirmDialog: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("4")),
		hint: lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")),
		errorLine: lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")),
	}
}

func (p stylePalette) styleFor(status string) lipgloss.Style {
	switch status {
	case "running":
		return p.phaseRunning
	case "done", "completed":
		return p.phaseDone
	case "failed", "error":
		return p.phaseFailed
	case "blocked", "escalated", "degraded":
		return p.phaseBlocked
	default:
		return p.phasePending
	}
}

func (p stylePalette) iconFor(status string) string {
	switch status {
	case "running":
		return iconRunning
	case "done", "completed":
		return iconDone
	case "failed", "error":
		return iconFailed
	case "blocked", "escalated", "degraded":
		return iconBlocked
	default:
		return iconPending
	}
}
