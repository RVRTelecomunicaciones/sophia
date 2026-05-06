package tui

// White-box tests for private helpers.
// Coexists with the package tui_test black-box tests in this directory.

import (
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// reattachHint ----------------------------------------------------------------

func TestReattachHintWithChangeID(t *testing.T) {
	got := reattachHint(domain.ChangeID("01HXABC"))
	want := "Detached. Reattach with: sophia attach 01HXABC"
	if got != want {
		t.Errorf("reattachHint = %q, want %q", got, want)
	}
}

func TestReattachHintEmptyChangeID(t *testing.T) {
	got := reattachHint(domain.ChangeID(""))
	want := "Detached."
	if got != want {
		t.Errorf("reattachHint = %q, want %q", got, want)
	}
}

// styleFor --------------------------------------------------------------------

func TestStyleForCoversAllStatuses(t *testing.T) {
	p := newStyles()
	for _, s := range []string{"pending", "running", "done", "failed", "blocked", "unknown"} {
		// Just ensure no panic and that a valid style is returned (non-zero value
		// check is not meaningful for lipgloss.Style — absence of panic is sufficient).
		_ = p.styleFor(s)
	}
}

// iconFor ---------------------------------------------------------------------

func TestIconForCoversAllStatuses(t *testing.T) {
	p := newStyles()
	cases := map[string]string{
		"pending": iconPending,
		"running": iconRunning,
		"done":    iconDone,
		"failed":  iconFailed,
		"blocked": iconBlocked,
		"unknown": iconPending, // default fallback
	}
	for status, want := range cases {
		if got := p.iconFor(status); got != want {
			t.Errorf("iconFor(%q) = %q, want %q", status, got, want)
		}
	}
}

// truncateToWidth -------------------------------------------------------------

func TestTruncateToWidthChopsLongLines(t *testing.T) {
	in := "This is a long line that exceeds the width\nshort"
	got := truncateToWidth(in, 10)
	lines := strings.Split(got, "\n")
	for _, line := range lines {
		if rc := len([]rune(line)); rc > 10 {
			t.Errorf("line %q has %d runes, want ≤10", line, rc)
		}
	}
}

func TestTruncateToWidthZeroWidthReturnsAsIs(t *testing.T) {
	in := "abc\nxyz"
	if got := truncateToWidth(in, 0); got != in {
		t.Errorf("zero width should not modify input: got %q, want %q", got, in)
	}
}
