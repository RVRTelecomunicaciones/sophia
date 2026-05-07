package cli_test

import (
	"bytes"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/cli"
)

func TestStopCommandSucceeds(t *testing.T) {
	deps, compose := newStartDeps()
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"stop"})
	if err := c.Execute(); err != nil {
		t.Fatalf("stop err: %v", err)
	}
	if compose.DownCalls != 1 {
		t.Errorf("DownCalls = %d", compose.DownCalls)
	}
}
