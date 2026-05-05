//go:build e2e_smoke

package e2e_test

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"
)

func TestSmokeDoctorJSON(t *testing.T) {
	cmd := exec.Command("../../bin/sophia", "doctor", "--json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Doctor may return non-zero when checks fail (e.g. CI without docker).
	// We only require that --json produces parseable JSON on stdout.
	_ = cmd.Run()

	var report struct {
		Summary struct{ OK, Info, Warn, Fail int }
		Checks  []map[string]any
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor --json produced invalid JSON: %v\nstdout: %s\nstderr: %s",
			err, stdout.String(), stderr.String())
	}
	if len(report.Checks) < 3 {
		t.Errorf("expected at least 3 checks, got %d", len(report.Checks))
	}
}
