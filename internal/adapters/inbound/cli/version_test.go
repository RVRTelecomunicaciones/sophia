package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/cli"
)

func TestVersionCommandPrintsAllFields(t *testing.T) {
	c := cli.NewRoot(cli.Deps{Version: "0.1.0", Commit: "abc1234", BuildDate: "2026-05-05"})
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"version"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"0.1.0", "abc1234", "2026-05-05"} {
		if !strings.Contains(got, want) {
			t.Errorf("version output missing %q: %q", want, got)
		}
	}
}
