package graphify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

const (
	// graphifyPackage is the hard-pinned version installed by Bootstrap.
	graphifyPackage = "graphifyy[mcp]==0.8.35"
)

// ExecGraphifyProber implements outbound.GraphifyProber using an injected
// ExecRunner so unit tests never spawn real subprocesses.
type ExecGraphifyProber struct {
	exec outbound.ExecRunner
}

// NewExecGraphifyProber constructs an ExecGraphifyProber.
func NewExecGraphifyProber(runner outbound.ExecRunner) *ExecGraphifyProber {
	return &ExecGraphifyProber{exec: runner}
}

// Probe checks for python3, uv, and graphify in sequence.
// The probe is degraded-first: if any dep is missing the result has
// Available=false and the missing tool is listed in MissingDeps.
// An error is returned only for unexpected OS-level failures.
func (p *ExecGraphifyProber) Probe(ctx context.Context) (outbound.ProberResult, error) {
	res := outbound.ProberResult{
		DetectedAt: time.Now(),
	}

	// Step 1: python3 --version
	_, _, exit, err := p.exec.Run(ctx, "python3", []string{"--version"}, outbound.ExecOpts{})
	if err != nil || exit != 0 {
		res.MissingDeps = append(res.MissingDeps, "python3")
		// Without python3 we can stop; uv and graphify won't work.
		return res, nil
	}
	res.PythonOK = true

	// Step 2: uv --version
	_, _, exit, err = p.exec.Run(ctx, "uv", []string{"--version"}, outbound.ExecOpts{})
	if err != nil || exit != 0 {
		res.MissingDeps = append(res.MissingDeps, "uv")
		// uv absent; graphify may still be installed via other means — keep probing.
	} else {
		res.UVOK = true
	}

	// Step 3: graphify --version
	stdout, _, exit, err := p.exec.Run(ctx, "graphify", []string{"--version"}, outbound.ExecOpts{})
	if err != nil || exit != 0 {
		res.MissingDeps = append(res.MissingDeps, "graphify")
		return res, nil
	}
	res.Version = strings.TrimSpace(string(stdout))
	res.Available = true

	return res, nil
}

// Bootstrap runs `uv tool install "graphifyy[mcp]==0.8.35"`. Returns a
// wrapped error that includes stderr when the subprocess exits non-zero.
func (p *ExecGraphifyProber) Bootstrap(ctx context.Context) error {
	args := []string{"tool", "install", graphifyPackage}
	_, stderr, exit, err := p.exec.Run(ctx, "uv", args, outbound.ExecOpts{})
	if err != nil || exit != 0 {
		stderrStr := strings.TrimSpace(string(stderr))
		if stderrStr == "" && err != nil {
			stderrStr = err.Error()
		}
		return fmt.Errorf("bootstrap graphify: uv tool install exited %d: %s", exit, stderrStr)
	}
	return nil
}
