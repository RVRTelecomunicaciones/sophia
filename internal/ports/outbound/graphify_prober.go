package outbound

import (
	"context"
	"time"
)

// GraphifyProber detects whether the Graphify toolchain (Python 3.10+, uv,
// and graphifyy[mcp]==0.8.35) is available on the current machine and can
// optionally install it. Implementations live in
// internal/adapters/outbound/graphify/prober.go. Unit tests supply a fake.
type GraphifyProber interface {
	// Probe checks for Python 3.10+, uv, and graphify availability. It never
	// runs a subprocess that modifies system state. Returns ProberResult on
	// success; err only on unexpected infrastructure failure (not on missing deps).
	Probe(ctx context.Context) (ProberResult, error)

	// Bootstrap installs graphifyy[mcp]==0.8.35 via
	// `uv tool install "graphifyy[mcp]==0.8.35"`. Called only when
	// --auto-bootstrap-graphify flag is supplied. Returns a wrapped error
	// including stderr output when the subprocess exits non-zero.
	Bootstrap(ctx context.Context) error
}

// ProberResult captures a single probe snapshot.
type ProberResult struct {
	// Available is true when Python 3.10+, uv, and graphify are all present.
	Available bool
	// Version is the output of `graphify --version` (trimmed). Empty when graphify is absent.
	Version string
	// PythonOK is true when `python3 --version` exits 0.
	PythonOK bool
	// UVOK is true when `uv --version` exits 0.
	UVOK bool
	// MissingDeps is the human-readable list of absent tools.
	// Entries may be: "python3", "uv", "graphify".
	MissingDeps []string
	// DetectedAt is the wall-clock time at which Probe was called.
	DetectedAt time.Time
}

// ExecOpts configures an ExecRunner.Run call.
type ExecOpts struct {
	// Dir is the working directory; empty means inherit from the parent process.
	Dir string
	// Env is extra environment variables in "KEY=VALUE" format; nil means inherit.
	Env []string
	// StdinBytes is optional stdin payload.
	StdinBytes []byte
}

// ExecRunner abstracts exec.Command so unit tests can inject a fake that
// returns canned stdout/exit-code without touching the OS.
// Implementations must be safe for concurrent use.
type ExecRunner interface {
	// Run executes name with args. Returns stdout, stderr, exit code, and any
	// os-level error (not the subprocess exit code error — callers check
	// exitCode != 0 for failure). err is non-nil only on OS-level launch failures.
	Run(ctx context.Context, name string, args []string, opts ExecOpts) (stdout []byte, stderr []byte, exitCode int, err error)
}
