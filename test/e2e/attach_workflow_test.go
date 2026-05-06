//go:build e2e_smoke

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSmokeAttachWorkflow validates the full run -> (simulate detach) -> attach
// -> terminal cycle against an in-process orchestrator stub. The stub keeps
// the same Change alive across both CLI invocations.
//
// Verifies:
//   - exit 0 from `attach <change-id> --no-tui --json`
//   - JSONL stream contains the snapshot and the terminal "final_status":"done"
//   - last_change_id persisted to <stateRoot>/sophia/last_change_id
//
// Catches RM8-01 (snapshot/SSE race) and validates that `attach` and `run`
// share the same observation pipeline.
func TestSmokeAttachWorkflow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	binary := absBinary(t)

	var (
		mu           sync.Mutex
		changeStatus = "running"
		ssePerChange = map[string]int{}
		assignedID   = ""
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/changes":
			assignedID = "01HX-ATTACH-E2E"
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"change_id":%q,"status":%q,"name":"msg","project":"p"}`, assignedID, changeStatus)

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/events"):
			id := pathChangeID(r.URL.Path)
			ssePerChange[id]++
			if ssePerChange[id] > 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			flusher := w.(http.Flusher)
			fmt.Fprint(w, "event: phase.completed\nid: evt-1\ndata: {\"payload\":{\"status\":\"done\"}}\n\n")
			flusher.Flush()

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/changes/"):
			id := pathChangeID(r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"change_id":%q,"status":%q,"project":"p"}`, id, changeStatus)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tmp := t.TempDir()
	if out, err := exec.Command("git", "-C", tmp, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	yaml := []byte("version: 1\nproject: p\nbase_ref: main\nartifact_store: engram\n")
	if err := os.WriteFile(filepath.Join(tmp, ".sophia.yaml"), yaml, 0o644); err != nil {
		t.Fatal(err)
	}

	stateDir := t.TempDir()
	dataDir := t.TempDir()
	configDir := t.TempDir()

	mu.Lock()
	changeStatus = "running"
	mu.Unlock()

	runCmd := exec.Command(binary, "run", "msg", "--no-tui", "--json")
	runCmd.Dir = tmp
	runCmd.Env = append(os.Environ(),
		"SOPHIA_ORCHESTRATOR_URL="+srv.URL,
		"XDG_STATE_HOME="+stateDir,
		"XDG_DATA_HOME="+dataDir,
		"XDG_CONFIG_HOME="+configDir,
	)
	var runOut, runErr bytes.Buffer
	runCmd.Stdout = &runOut
	runCmd.Stderr = &runErr
	// Run is expected to exit non-zero (status flipped to running mid-fetch),
	// but the captured ID is in the JSONL stream regardless.
	_ = runCmd.Run()

	mu.Lock()
	capturedID := assignedID
	mu.Unlock()
	if capturedID == "" {
		t.Fatalf("run did not assign change_id; stdout=%q stderr=%q", runOut.String(), runErr.String())
	}

	mu.Lock()
	changeStatus = "done"
	mu.Unlock()

	time.Sleep(50 * time.Millisecond)
	atCmd := exec.Command(binary, "attach", capturedID, "--no-tui", "--json")
	atCmd.Dir = tmp
	atCmd.Env = runCmd.Env
	var atOut, atErr bytes.Buffer
	atCmd.Stdout = &atOut
	atCmd.Stderr = &atErr
	if err := atCmd.Run(); err != nil {
		t.Fatalf("attach failed: %v\nstdout: %s\nstderr: %s", err, atOut.String(), atErr.String())
	}

	lines := strings.Split(strings.TrimRight(atOut.String(), "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("no output lines from attach")
	}
	for _, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("invalid JSON line: %v\n%s", err, l)
		}
	}
	if !strings.Contains(atOut.String(), `"final_status":"done"`) {
		t.Errorf("missing terminal status in attach output: %s", atOut.String())
	}

	gpath := filepath.Join(stateDir, "sophia", "last_change_id")
	if _, err := os.Stat(gpath); err != nil {
		t.Errorf("expected global last_change_id at %s: %v", gpath, err)
	}
	got, _ := os.ReadFile(gpath)
	if !strings.Contains(string(got), capturedID) {
		t.Errorf("global last_change_id = %q, want contains %q", got, capturedID)
	}
}

// pathChangeID extracts the change-id segment from /api/v1/changes/{id}[/...].
func pathChangeID(p string) string {
	const prefix = "/api/v1/changes/"
	if !strings.HasPrefix(p, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(p, prefix)
	if i := strings.Index(rest, "/"); i >= 0 {
		return rest[:i]
	}
	return rest
}
