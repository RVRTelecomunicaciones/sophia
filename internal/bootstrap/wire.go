package bootstrap

import (
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/composeexec"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/gitcli"
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
	xdg := newXDGValidator()
	doctor := application.NewDoctorService(compose, git, xdg)

	info := NewVersionInfo()
	deps := cli.Deps{
		Doctor:    doctor,
		Version:   info.Version,
		Commit:    info.Commit,
		BuildDate: info.BuildDate,
	}
	return cli.NewRoot(deps), nil
}

func newXDGValidator() application.XDGValidator {
	return func(_ string) error {
		return nil
	}
}
