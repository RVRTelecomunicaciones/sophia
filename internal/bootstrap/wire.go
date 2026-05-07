package bootstrap

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/composeexec"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/filestate"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/gitcli"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/osbrowser"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/sseprobe"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/ssestream"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/xdgpaths"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/yamlconfig"
	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/inbound"
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
		// SOPHIA_ORCHESTRATOR_URL takes effect at bootstrap; --orchestrator-url
		// arrives in M5 once the orch client supports per-call rebinding.
		if v := os.Getenv(application.EnvOrchestratorURL); v != "" {
			cfg.OrchestratorURL = v
		} else {
			cfg.OrchestratorURL = DefaultOrchestratorURL
		}
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
		Orch:         orch,
		State:        state,
		Git:          git,
		ProjectStore: projectStore,
	}, application.StatusOptions{})

	resolver := application.NewConfigResolver(application.ConfigResolverDeps{
		ProjectStore: projectStore,
		UserStore:    yamlconfig.NewUserStore(yamlconfig.UserConfig{}),
		Git:          git,
	})

	// SSE stream client (M5): consumes /api/v1/changes/{id}/events with
	// reconnect, Last-Event-ID, and 60s heartbeat watchdog per spec §5.7.
	stream := ssestream.New(ssestream.Config{
		BaseURL:    cfg.OrchestratorURL,
		Backoff:    ssestream.BackoffConfig{Min: time.Second, Max: 30 * time.Second},
		MaxRetries: ssestream.DefaultMaxRetries,
		Heartbeat:  ssestream.DefaultHeartbeat,
	})

	browser := osbrowser.New(osbrowser.Config{})

	// RunnerFactory builds a Runner with the caller-provided sink. The sink is
	// chosen at command time: TUI bridge in default mode, jsonsink in --no-tui --json mode.
	runnerFactory := func(sink inbound.EventSink) *application.Runner {
		return application.NewRunner(application.RunnerDeps{
			Orch:        orch,
			State:       state,
			Git:         git,
			Sink:        sink,
			EventStream: stream,
		}, application.RunnerOptions{})
	}

	// Lister implements `sophia changes` (M8). Pure pass-through over
	// OrchestratorClient.ListChanges; project-default resolution lives in
	// cli/changes.go.
	lister := application.NewLister(application.ListerDeps{Orch: orch})

	// AttacherFactory mirrors RunnerFactory for `sophia attach` (M8). Each
	// invocation gets its own Runner+Attacher pair so lifecycles stay
	// isolated.
	attacherFactory := func(sink inbound.EventSink) *application.Attacher {
		runner := application.NewRunner(application.RunnerDeps{
			Orch:        orch,
			State:       state,
			Git:         git,
			Sink:        sink,
			EventStream: stream,
		}, application.RunnerOptions{})
		return application.NewAttacher(application.AttacherDeps{
			Orch:   orch,
			State:  state,
			Git:    git,
			Runner: runner,
		})
	}

	userConfigPath := filepath.Join(xdg.ConfigRoot, "config.yaml")

	info := NewVersionInfo()
	deps := cli.Deps{
		Doctor:          doctor,
		Provisioner:     provisioner,
		Initializer:     initializer,
		StatusReader:    statusReader,
		Lister:          lister,
		Orch:            orch,
		RunnerFactory:   runnerFactory,
		AttacherFactory: attacherFactory,
		Resolver:        resolver,
		Browser:         browser,
		UserConfigPath:  userConfigPath,
		Version:         info.Version,
		Commit:          info.Commit,
		BuildDate:       info.BuildDate,
	}
	return cli.NewRoot(deps), nil
}
