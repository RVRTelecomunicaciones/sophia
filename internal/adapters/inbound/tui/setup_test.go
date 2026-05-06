package tui_test

import (
	"os"
	"testing"
)

// TestMain forces a no-color environment for deterministic golden assertions.
// NOTE: In lipgloss v2, Style.Render() always emits full-fidelity ANSI at the
// render layer — NO_COLOR is honoured only at the print/writer layer. Our tests
// use strings.Contains which handles ANSI-wrapped text correctly, so this env
// var is kept as documentation intent and for future colorprofile.Writer usage.
func TestMain(m *testing.M) {
	_ = os.Setenv("NO_COLOR", "1")
	os.Exit(m.Run())
}
