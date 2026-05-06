package bootstrap

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/jsonsink"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/composeexec"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/filestate"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/gitcli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/sseprobe"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/xdgpaths"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/yamlconfig"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
)

// DefaultOrchestratorURL is used when neither flags nor env override it.
// V1 hardcodes localhost; V1.1+ will read it from <configRoot>/config.yaml.
const DefaultOrchestratorURL = "http://localhost:9080"

// Config controls bootstrap.New.
type Config struct {
	LogWriter       io.Writer  // nil ⇒ os.Stderr
	LogLevel        slog.Level // default Info
	OrchestratorURL string     // empty ⇒ DefaultOrchestratorURL
}

// New is the composition root. It builds concrete outbound adapters,
// application services, and returns the configured root cobra command.
func New(cfg Config) (*cobra.Command, error) {
	if cfg.LogWriter == nil {
		cfg.LogWriter = os.Stderr
	}
	if cfg.OrchestratorURL == "" {
		cfg.OrchestratorURL = DefaultOrchestratorURL
	}
	logger := NewLogger(cfg.LogWriter, cfg.LogLevel)
	slog.SetDefault(logger)

	compose := composeexec.New(composeexec.Config{})
	git := gitcli.New(gitcli.Config{})
	paths := xdgpaths.New(xdgpaths.Config{})
	orch := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: cfg.OrchestratorURL})
	sse := sseprobe.New(sseprobe.Config{BaseURL: cfg.OrchestratorURL})

	doctor := application.NewDoctorService(application.DoctorDeps{
		Compose: compose, Git: git, Paths: paths, Orch: orch, SSE: sse,
	})
	provisioner := application.NewProvisioner(application.ProvisionerDeps{
		Compose: compose,
		Paths:   paths,
		Materialize: func(dataRoot string, embed []byte, reset bool) (string, bool, error) {
			res, err := composeexec.Materialize(dataRoot, embed, reset)
			return res.Path, res.Wrote, err
		},
		Embedded: composeexec.EmbeddedComposeYAML,
	})

	// Resolve XDG paths once for state-aware adapters. Errors here mean
	// the binary still works for read-only commands; init/status will
	// re-resolve and fail with a clearer message.
	xdg, _ := paths.Resolve()

	projectStore := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{})
	state := filestate.New(filestate.Config{StateRoot: xdg.StateRoot})

	initializer := application.NewInitializer(application.InitializerDeps{
		Git:          git,
		ProjectStore: projectStore,
	})
	statusReader := application.NewStatusReader(application.StatusDeps{
		State:        state,
		Git:          git,
		ProjectStore: projectStore,
	})

	resolver := application.NewConfigResolver(application.ConfigResolverDeps{
		ProjectStore: projectStore,
		UserStore:    yamlconfig.NewUserStore(yamlconfig.UserConfig{}),
		Git:          git,
	})

	// JSON sink writes to stdout. The runner is constructed with this sink
	// for V1; M6 will swap to a TUI sink based on flags.
	sink := jsonsink.New(jsonsink.Config{Writer: os.Stdout})
	runner := application.NewRunner(application.RunnerDeps{
		Orch:  orch,
		State: state,
		Git:   git,
		Sink:  sink,
	}, application.RunnerOptions{})

	userConfigPath := filepath.Join(xdg.ConfigRoot, "config.yaml")

	info := NewVersionInfo()
	deps := cli.Deps{
		Doctor:         doctor,
		Provisioner:    provisioner,
		Initializer:    initializer,
		StatusReader:   statusReader,
		Runner:         runner,
		Resolver:       resolver,
		UserConfigPath: userConfigPath,
		Version:        info.Version,
		Commit:         info.Commit,
		BuildDate:      info.BuildDate,
	}
	return cli.NewRoot(deps), nil
}
