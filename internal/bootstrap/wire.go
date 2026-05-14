package bootstrap

import (
	"crypto/rand"
	"fmt"
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
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/ssestream"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/xdgpaths"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/yamlconfig"
	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain/trace"
	"github.com/RVRTelecomunicaciones/sophia/internal/infrastructure/httpclient"
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
	// APIKey overrides SOPHIA_API_KEY. Tests inject a programmable
	// resolver via APIKeyResolverOverride below; production leaves this
	// empty and lets New() read the env var.
	APIKey string
	// APIKeyResolverOverride lets tests inject a fully-built resolver
	// (e.g. backed by a non-os.Getenv source) without the binary picking
	// up the test process's real $SOPHIA_API_KEY. Production must leave
	// this nil.
	APIKeyResolverOverride *application.APIKeyResolver
}

// New is the composition root. It builds concrete outbound adapters,
// application services, and returns the configured root cobra command.
//
// API key plumbing (Phase 4 Task 4.2 / D-M10-02): the persistent
// --api-key flag is registered on the root command but its value isn't
// known until cobra runs ParseFlags. To keep the wiring deterministic
// the flag is resolved at command-execution time via PersistentPreRunE,
// not at bootstrap. SOPHIA_API_KEY env var IS read at bootstrap so
// header-bearing adapters can be built once.
func New(cfg Config) (*cobra.Command, error) {
	if cfg.LogWriter == nil {
		cfg.LogWriter = os.Stderr
	}
	if cfg.OrchestratorURL == "" {
		if v := os.Getenv(application.EnvOrchestratorURL); v != "" {
			cfg.OrchestratorURL = v
		} else {
			cfg.OrchestratorURL = DefaultOrchestratorURL
		}
	}
	logger := NewLogger(cfg.LogWriter, cfg.LogLevel)
	slog.SetDefault(logger)

	// ADR-0005 P2.2b: mint the CLI-invocation Trace ONCE. trace_id is
	// stable across every outbound HTTP/SSE call for this invocation;
	// each call rotates a fresh span_id (HTTP) or pins a stream-lifetime
	// span (SSE). On generation failure we proceed without a trace —
	// observability is best-effort, never a hard dependency.
	invocationTrace, traceErr := trace.New(rand.Reader)
	if traceErr != nil {
		logger.Warn("trace: failed to mint invocation trace, requests will not carry Traceparent",
			"error", traceErr.Error())
	} else {
		logger.Debug("trace: invocation trace minted",
			"trace_id", invocationTrace.TraceID,
			"span_id", invocationTrace.SpanID,
		)
	}

	keyResolver := cfg.APIKeyResolverOverride
	if keyResolver == nil {
		keyResolver = application.NewAPIKeyResolver(cfg.APIKey, os.Getenv)
	}
	apiKey := keyResolver.Resolve()

	// Trace-aware HTTP client shared by every adapter (P2.2b). The
	// RoundTripper rotates span_id per request under the stable
	// invocation trace_id, so server logs can correlate the whole CLI
	// run via trace_id and individual exchanges via span_id.
	sharedHTTP := httpclient.New(httpclient.Config{Trace: invocationTrace})

	compose := composeexec.New(composeexec.Config{})
	git := gitcli.New(gitcli.Config{})
	paths := xdgpaths.New(xdgpaths.Config{})
	orch := orchestratorhttp.New(orchestratorhttp.Config{
		BaseURL: cfg.OrchestratorURL,
		APIKey:  apiKey,
		HTTP:    sharedHTTP,
	})

	doctor := application.NewDoctorService(application.DoctorDeps{
		Compose: compose, Git: git, Paths: paths, Orch: orch,
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

	// Per-phase SSE stream client (Phase 4 Task 4.3). The client carries
	// the API key into every reconnect; key value is never logged. The
	// invocation Trace is passed so each Subscribe call pins a single
	// stream-lifetime span on every reconnect (P2.2b).
	stream := ssestream.New(ssestream.Config{
		BaseURL:    cfg.OrchestratorURL,
		Backoff:    ssestream.BackoffConfig{Min: time.Second, Max: 30 * time.Second},
		MaxRetries: ssestream.DefaultMaxRetries,
		Heartbeat:  ssestream.DefaultHeartbeat,
		APIKey:     apiKey,
		Trace:      invocationTrace,
	})

	browser := osbrowser.New(osbrowser.Config{})

	runnerFactory := func(sink inbound.EventSink) *application.Runner {
		return application.NewRunner(application.RunnerDeps{
			Orch:        orch,
			State:       state,
			Git:         git,
			Sink:        sink,
			EventStream: stream,
		}, application.RunnerOptions{})
	}

	lister := application.NewLister(application.ListerDeps{Orch: orch})
	approver := application.NewApprover(application.ApproverDeps{Orch: orch})
	aborter := application.NewAborter(application.AborterDeps{Orch: orch})

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
		Approver:        approver,
		Aborter:         aborter,
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
	root := cli.NewRoot(deps)

	// Persistent --api-key flag (D-M10-02). Resolution + auth gate runs
	// at command execution so subcommands that don't talk to the
	// orchestrator (--help, version) can run without a key.
	var apiKeyFlag string
	root.PersistentFlags().StringVar(&apiKeyFlag, "api-key", "", "API key for remote orchestrator (overrides SOPHIA_API_KEY)")
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if !needsAuth(cmd) {
			return nil
		}
		effectiveResolver := application.NewAPIKeyResolver(apiKeyFlag, os.Getenv)
		if cfg.APIKeyResolverOverride != nil && apiKeyFlag == "" {
			effectiveResolver = cfg.APIKeyResolverOverride
		}
		if err := effectiveResolver.Authorize(cfg.OrchestratorURL); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
		return nil
	}
	return root, nil
}

// needsAuth returns true for commands that talk to the orchestrator.
// Local-only commands (version, doctor, init, start, stop) skip the
// auth gate so they work without a key against any orchestrator URL.
func needsAuth(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "version", "doctor", "init", "start", "stop", "help":
		return false
	}
	return true
}
