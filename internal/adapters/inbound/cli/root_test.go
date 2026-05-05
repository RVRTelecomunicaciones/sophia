package cli_test

import (
	"bytes"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
)

func TestRootCommandHasName(t *testing.T) {
	c := cli.NewRoot(cli.Deps{})
	if c.Use != "sophia" {
		t.Errorf("root use = %q, want sophia", c.Use)
	}
}

func TestRootHelpOutput(t *testing.T) {
	c := cli.NewRoot(cli.Deps{})
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"--help"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		t.Error("expected help output")
	}
}
