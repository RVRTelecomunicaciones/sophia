//go:build e2e_smoke

package e2e_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestSmokeVersion(t *testing.T) {
	cmd := exec.Command("../../bin/sophia", "version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("sophia version failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "sophia") {
		t.Errorf("output missing sophia: %s", out.String())
	}
}
