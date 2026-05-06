package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// ProgramConfig configures NewProgram.
type ProgramConfig struct {
	ChangeID domain.ChangeID
	Output   io.Writer // nil ⇒ os.Stdout (resolved by tea.WithOutput)
	Input    io.Reader // nil ⇒ os.Stdin
}

// Program owns the Bubble Tea program plus its Bridge.
type Program struct {
	mu      sync.Mutex
	tea     *tea.Program
	bridge  *Bridge
	closed  bool
	running bool // true once Run() has been called
}

// teaSender adapts *tea.Program to the Bridge's Sender interface.
// The bridge Sender uses Send(m any); tea.Program.Send takes tea.Msg which is
// an alias for interface{} — so the call is safe without a type assertion.
type teaSender struct {
	p *tea.Program
}

func (s *teaSender) Send(m any) {
	if s.p == nil {
		return
	}
	s.p.Send(m)
}

// rootModel implements the bubbletea v2 Model interface by delegating to the
// pure Update/View functions.
//
// v2 Init() signature: Init() tea.Cmd (returns only a Cmd, NOT (Model, Cmd)).
type rootModel struct {
	state Model
}

func (rm rootModel) Init() tea.Cmd {
	return nil
}

func (rm rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	newState, cmd := Update(rm.state, msg)
	rm.state = newState
	return rm, cmd
}

func (rm rootModel) View() tea.View {
	return tea.NewView(View(rm.state))
}

// NewProgram constructs a Program, wiring the Bubble Tea program to the Bridge
// via teaSender.
func NewProgram(cfg ProgramConfig) (*Program, error) {
	root := rootModel{state: NewModel(ModelConfig{ChangeID: cfg.ChangeID})}

	opts := []tea.ProgramOption{
		// Disable the signal handler so that our Update logic controls Ctrl+C
		// semantics (confirm-then-detach per spec §2.2).
		tea.WithoutSignalHandler(),
	}

	if cfg.Output != nil {
		opts = append(opts, tea.WithOutput(cfg.Output))
		// When output is explicitly redirected (e.g. io.Discard in tests),
		// disable terminal input too — the program is running headless and
		// calling Open(/dev/tty) would fail in a non-TTY environment.
		// In production (cfg.Output == nil), the tea defaults apply and the
		// real terminal is used for keyboard input.
		if cfg.Input == nil {
			opts = append(opts, tea.WithInput(nil))
		}
	}
	if cfg.Input != nil {
		opts = append(opts, tea.WithInput(cfg.Input))
	}

	teaProg := tea.NewProgram(root, opts...)
	sender := &teaSender{p: teaProg}
	bridge := NewBridge(BridgeConfig{Sender: sender})

	return &Program{
		tea:    teaProg,
		bridge: bridge,
	}, nil
}

// Bridge returns the EventSink-implementing bridge.
func (p *Program) Bridge() *Bridge { return p.bridge }

// Run starts the Bubble Tea event loop and blocks until it exits.
// If ctx is cancelled, the program is gracefully quit.
// Returns the reattach hint (non-empty only when the user detached) and an
// error if the program exited abnormally.
func (p *Program) Run(ctx context.Context) (string, error) {
	p.mu.Lock()
	p.running = true
	p.mu.Unlock()

	stopWatcher := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			p.tea.Quit()
		case <-stopWatcher:
		}
	}()
	defer close(stopWatcher)

	finalModel, err := p.tea.Run()
	if err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return "", fmt.Errorf("tui: program ended with error: %w", err)
	}

	if rm, ok := finalModel.(rootModel); ok && rm.state.Detached() {
		return reattachHint(rm.state.ChangeID()), nil
	}
	return "", nil
}

// Close stops the program and tears down the bridge. Idempotent.
// If Run() has not been called, the tea program is never started so Quit() is
// not called (calling Quit/Send on an unstarted tea.Program blocks until it
// starts). The bridge is still closed so pending goroutines drain cleanly.
func (p *Program) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	// Only quit the tea program if it was actually started — Send blocks on an
	// unstarted program.
	if p.tea != nil && p.running {
		p.tea.Quit()
	}
	if p.bridge != nil {
		_ = p.bridge.Close()
	}
	return nil
}

// reattachHint returns the user-facing reattach instruction printed AFTER
// the bubbletea program exits. Spec §2.2.
func reattachHint(id domain.ChangeID) string {
	if id.IsZero() {
		return "Detached."
	}
	return fmt.Sprintf("Detached. Reattach with: sophia attach %s", id)
}
