package bootstrap

import (
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/composeexec"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/gitcli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/xdgpaths"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
)

type Config struct {
	LogWriter io.Writer
	LogLevel  slog.Level
}

func New(cfg Config) (*cobra.Command, error) {
	if cfg.LogWriter == nil {
		cfg.LogWriter = os.Stderr
	}
	logger := NewLogger(cfg.LogWriter, cfg.LogLevel)
	slog.SetDefault(logger)

	compose := composeexec.New(composeexec.Config{})
	git := gitcli.New(gitcli.Config{})
	doctor := application.NewDoctorService(application.DoctorDeps{
		Compose: compose,
		Git:     git,
		// Paths/Orch/SSE wired in M2 Task 16 (bootstrap rewiring)
	})

	paths := xdgpaths.New(xdgpaths.Config{})
	provisioner := application.NewProvisioner(application.ProvisionerDeps{
		Compose: compose,
		Paths:   paths,
		Materialize: func(dataRoot string, embed []byte, reset bool) (string, bool, error) {
			res, err := composeexec.Materialize(dataRoot, embed, reset)
			return res.Path, res.Wrote, err
		},
		Embedded: composeexec.EmbeddedComposeYAML,
	})

	info := NewVersionInfo()
	deps := cli.Deps{
		Doctor:      doctor,
		Provisioner: provisioner,
		Version:     info.Version,
		Commit:      info.Commit,
		BuildDate:   info.BuildDate,
	}
	return cli.NewRoot(deps), nil
}
