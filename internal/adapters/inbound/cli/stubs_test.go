package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
)

func TestStubsAnnounceMilestone(t *testing.T) {
	cases := map[string]string{
		"run":     "M4",
		"attach":  "M8",
		"changes": "M8",
	}
	for sub, milestone := range cases {
		c := cli.NewRoot(cli.Deps{})
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetArgs([]string{sub})
		if err := c.Execute(); err != nil {
			t.Fatalf("%s err: %v", sub, err)
		}
		got := out.String()
		if !strings.Contains(got, "not implemented yet") || !strings.Contains(got, milestone) {
			t.Errorf("%s output = %q (want milestone %s)", sub, got, milestone)
		}
	}
}
