// Package osbrowser implements outbound.Browser by shelling out to the OS-
// native URL handler (`open` on macOS, `xdg-open` on Linux, `cmd /c start`
// on Windows). It performs strict URL validation BEFORE invoking any
// subprocess: only http:// and https:// URLs are accepted. javascript:,
// data:, file:, vbscript:, and any other scheme are rejected with
// ErrInvalidScheme.
//
// Spec invariants honored:
//
//   - §6.3 #3: subprocess + URL validation. The validated URL is the only
//     value that reaches exec.Command; the schemes whitelist is enforced
//     before fork.
//   - §7.2 M7 DoD: implements outbound.Browser; wired into the TUI program
//     so [O] in the approval banner opens the gate URL.
//
// Out of scope: opening the user's preferred browser explicitly (we delegate
// to the OS), supporting per-platform browser flags, and probing whether a
// handler exists before fork.
package osbrowser
