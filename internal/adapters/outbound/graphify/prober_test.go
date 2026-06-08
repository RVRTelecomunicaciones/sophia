package graphify_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/graphify"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// fakeExecRunner is a test double for outbound.ExecRunner that returns
// canned results without spawning subprocesses.
type fakeExecRunner struct {
	calls     []execCall
	responses map[string]execResponse
}

type execCall struct {
	Name string
	Args []string
}

type execResponse struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Err      error
}

func (f *fakeExecRunner) Run(_ context.Context, name string, args []string, _ outbound.ExecOpts) ([]byte, []byte, int, error) {
	key := name
	if len(args) > 0 {
		key = name + " " + args[0]
	}
	f.calls = append(f.calls, execCall{Name: name, Args: args})
	r, ok := f.responses[key]
	if !ok {
		return nil, nil, 0, nil
	}
	return r.Stdout, r.Stderr, r.ExitCode, r.Err
}

// A.1 — Probe returns Available=true, Version set when all deps present.
func TestExecGraphifyProber_Probe_Available(t *testing.T) {
	runner := &fakeExecRunner{
		responses: map[string]execResponse{
			"python3 --version":  {Stdout: []byte("Python 3.12.0"), ExitCode: 0},
			"uv --version":       {Stdout: []byte("uv 0.4.1"), ExitCode: 0},
			"graphify --version": {Stdout: []byte("0.8.35"), ExitCode: 0},
		},
	}
	prober := graphify.NewExecGraphifyProber(runner)

	res, err := prober.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: unexpected error: %v", err)
	}
	if !res.Available {
		t.Errorf("Available = false; want true")
	}
	if res.Version != "0.8.35" {
		t.Errorf("Version = %q; want %q", res.Version, "0.8.35")
	}
	if len(res.MissingDeps) != 0 {
		t.Errorf("MissingDeps = %v; want empty", res.MissingDeps)
	}
	if !res.PythonOK {
		t.Errorf("PythonOK = false; want true")
	}
}

// A.2 — Probe returns Available=false, MissingDeps=["python3"] when python3 exits non-zero.
func TestExecGraphifyProber_Probe_MissingPython(t *testing.T) {
	runner := &fakeExecRunner{
		responses: map[string]execResponse{
			"python3 --version": {ExitCode: 1, Err: errors.New("python3 not found")},
		},
	}
	prober := graphify.NewExecGraphifyProber(runner)

	res, err := prober.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: unexpected error: %v", err)
	}
	if res.Available {
		t.Errorf("Available = true; want false")
	}
	if res.PythonOK {
		t.Errorf("PythonOK = true; want false")
	}
	if len(res.MissingDeps) == 0 {
		t.Fatalf("MissingDeps is empty; want at least one entry")
	}
	found := false
	for _, d := range res.MissingDeps {
		if d == "python3" {
			found = true
		}
	}
	if !found {
		t.Errorf("MissingDeps = %v; want to include %q", res.MissingDeps, "python3")
	}
}

// A.3 — Probe returns Available=false, MissingDeps=["graphify"] when python3 OK but graphify missing.
func TestExecGraphifyProber_Probe_MissingGraphify(t *testing.T) {
	runner := &fakeExecRunner{
		responses: map[string]execResponse{
			"python3 --version":  {Stdout: []byte("Python 3.12.0"), ExitCode: 0},
			"uv --version":       {Stdout: []byte("uv 0.4.1"), ExitCode: 0},
			"graphify --version": {ExitCode: 127, Err: errors.New("graphify: command not found")},
		},
	}
	prober := graphify.NewExecGraphifyProber(runner)

	res, err := prober.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: unexpected error: %v", err)
	}
	if res.Available {
		t.Errorf("Available = true; want false")
	}
	if !res.PythonOK {
		t.Errorf("PythonOK = false; want true (python3 was present)")
	}
	found := false
	for _, d := range res.MissingDeps {
		if d == "graphify" {
			found = true
		}
	}
	if !found {
		t.Errorf("MissingDeps = %v; want to include %q", res.MissingDeps, "graphify")
	}
}

// A.4 — Bootstrap returns nil when uv tool install exits 0.
func TestExecGraphifyProber_Bootstrap_Success(t *testing.T) {
	runner := &fakeExecRunner{
		responses: map[string]execResponse{
			"uv tool": {Stdout: []byte("installed"), ExitCode: 0},
		},
	}
	prober := graphify.NewExecGraphifyProber(runner)

	err := prober.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap: unexpected error: %v", err)
	}
}

// A.5 — Bootstrap returns wrapped error containing stderr when uv exits non-zero.
func TestExecGraphifyProber_Bootstrap_Failure(t *testing.T) {
	runner := &fakeExecRunner{
		responses: map[string]execResponse{
			"uv tool": {
				Stderr:   []byte("error: uv not found"),
				ExitCode: 1,
				Err:      errors.New("exit status 1"),
			},
		},
	}
	prober := graphify.NewExecGraphifyProber(runner)

	err := prober.Bootstrap(context.Background())
	if err == nil {
		t.Fatal("Bootstrap: expected error, got nil")
	}
	// The error message must include the stderr output.
	if !strings.Contains(err.Error(), "uv not found") {
		t.Errorf("error %q does not mention stderr content %q", err.Error(), "uv not found")
	}
}
