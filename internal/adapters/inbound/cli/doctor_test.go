package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func newDoctorDeps() cli.Deps {
	return cli.Deps{
		Doctor: application.NewDoctorService(application.DoctorDeps{
			Compose: fakes.NewFakeComposeRunner(),
			Git:     fakes.NewFakeGitInspector(),
			Paths:   fakes.NewFakePathResolver(),
			Orch:    fakes.NewFakeOrchestrator(),
			SSE:     fakes.NewFakeSSEProber(),
		}),
	}
}

func TestDoctorCommandPrintsTable(t *testing.T) {
	c := cli.NewRoot(newDoctorDeps())
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"doctor"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"Docker", "Git", "XDG"} {
		if !strings.Contains(got, want) {
			t.Errorf("doctor output missing %q: %s", want, got)
		}
	}
}

func TestDoctorCommandJSON(t *testing.T) {
	c := cli.NewRoot(newDoctorDeps())
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"doctor", "--json"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	var report struct {
		Summary struct{ OK, Info, Warn, Fail int }
		Checks  []map[string]any
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(report.Checks) < 3 {
		t.Errorf("expected at least 3 checks, got %d", len(report.Checks))
	}
}
