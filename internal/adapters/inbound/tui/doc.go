// Package tui implements inbound.EventSink as a second adapter — alongside
// jsonsink — that forwards events into a Bubble Tea v2 program. The package
// is split into:
//
//   - bridge.go:       cap-256 buffered EventSink that calls program.Send,
//                      with a drop policy that protects phase.* / approval.*
//                      events while shedding heartbeats first under pressure.
//   - model.go:        pure Model state — no I/O, no tickers.
//   - update.go:       pure Update function for tea.Msg dispatch.
//   - keybindings.go:  key → action map (Q, Ctrl+C, etc.).
//   - styles.go:       lipgloss styles for status icons and headers.
//   - view_timeline.go: Timeline View() rendering 9 SDD phases.
//   - program.go:      tea.NewProgram assembly + Run() entry point.
//
// Spec invariants honored:
//   - §6.3 inv 7: lipgloss.Style.Render is the only rendering path —
//     no raw user-supplied strings are ever printf'd into the terminal.
//   - §4.5: heartbeats are dropped first; phase.* / approval.* never dropped.
//   - §2.2: Q detaches; Ctrl+C confirms before detach; never cancels the Change.
//
// M6 scope only — ApplyBoard view, full ApprovalGate banner, and Tab toggle
// are M7.
package tui
