//go:build e2e_smoke

package e2e_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSmokeRunAgainstStub spins up an in-process httptest stub of
// /api/v1/changes that emits a Change transitioning pending → running → done,
// then runs `./bin/sophia run "msg" --no-tui --json` from inside a temp git
// repo with .sophia.yaml. Verifies:
//   - exit 0
//   - JSONL stream contains the terminal "final_status":"done" line
//   - last_change_id persisted to <stateRoot>/sophia/last_change_id
//
// Validates the M4 auto_advance assumption: the stub auto-advances the Change
// without any client-side phase trigger. If a real orchestrator does NOT
// auto-advance, this test fails and we add the §5.2 compatibility mode.
func TestSmokeRunAgainstStub(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	binary := absBinary(t)

	// Stub orchestrator with auto-advance.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/changes":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"change_id":"01HX","status":"pending","name":"msg","project":"p"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/changes/"):
			calls++
			status := "running"
			if calls > 1 {
				status = "done"
			}
			_, _ = w.Write([]byte(`{"change_id":"01HX","status":"` + status + `"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Temp repo with .sophia.yaml.
	tmp := t.TempDir()
	if out, err := exec.Command("git", "-C", tmp, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	yaml := []byte("version: 1\nproject: p\nbase_ref: main\nartifact_store: engram\n")
	if err := os.WriteFile(filepath.Join(tmp, ".sophia.yaml"), yaml, 0o644); err != nil {
		t.Fatal(err)
	}

	// Isolate XDG state to a temp dir.
	stateDir := t.TempDir()
	cmd := exec.Command(binary, "run", "msg", "--no-tui", "--json")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(),
		"SOPHIA_ORCHESTRATOR_URL="+srv.URL,
		"XDG_STATE_HOME="+stateDir,
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_CONFIG_HOME="+t.TempDir(),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Verify each line is valid JSON.
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("no output lines")
	}
	for _, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("invalid JSON line: %v\n%s", err, l)
		}
	}
	if !strings.Contains(stdout.String(), `"final_status":"done"`) {
		t.Errorf("missing terminal status in output: %s", stdout.String())
	}

	// last_change_id persisted globally.
	gpath := filepath.Join(stateDir, "sophia", "last_change_id")
	if _, err := os.Stat(gpath); err != nil {
		t.Errorf("expected global last_change_id at %s: %v", gpath, err)
	}
}

func absBinary(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../bin/sophia")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("binary missing — run `make build` first: %v", err)
	}
	return abs
}
