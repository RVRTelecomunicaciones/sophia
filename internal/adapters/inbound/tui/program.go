package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// ProgramConfig configures NewProgram.
type ProgramConfig struct {
	ChangeID domain.ChangeID
	Output   io.Writer // nil ⇒ os.Stdout (resolved by tea.WithOutput)
	Input    io.Reader // nil ⇒ os.Stdin

	// Browser is the outbound.Browser used by [O] in the approval banner.
	// nil ⇒ pressing [O] surfaces an error line ("browser: not configured").
	Browser outbound.Browser
}

// Program owns the Bubble Tea program plus its Bridge.
type Program struct {
	mu      sync.Mutex
	tea     *tea.Program
	bridge  *Bridge
	closed  bool
	running bool // true once Run() has been called

	// M7: latest pure-Model state, updated on every Update call. Snapshot()
	// returns a copy so tests can inspect without racing the program loop.
	stateMu sync.Mutex
	state   Model

	browser outbound.Browser
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

// rootModel implements bubbletea v2 Model by delegating to pure Update/View.
//
// publish: called after every Update with the new pure-Model state, so
// Program.Snapshot() can return the latest. nil-safe.
//
// openBrowser: called when an OpenBrowserMsg arrives. Spawns a goroutine to
// call Browser.Open and Send a BrowserOpenedMsg back into the program.
// nil-safe (handled in rootModel.Update with an error line fallback).
type rootModel struct {
	state       Model
	publish     func(Model)
	openBrowser func(url string)
}

func (rm rootModel) Init() tea.Cmd { return nil }

func (rm rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Intercept OpenBrowserMsg BEFORE the pure Update sees it: dispatch the
	// browser open in a goroutine; the pure Update gets a no-op pass.
	if om, ok := msg.(OpenBrowserMsg); ok {
		if rm.openBrowser != nil {
			rm.openBrowser(om.URL)
		} else {
			// No Browser configured — record an error line in the model.
			rm.state = rm.state.WithError("browser: not configured")
			if rm.publish != nil {
				rm.publish(rm.state)
			}
		}
		return rm, nil
	}

	newState, cmd := Update(rm.state, msg)
	rm.state = newState
	if rm.publish != nil {
		rm.publish(rm.state)
	}
	return rm, cmd
}

func (rm rootModel) View() tea.View {
	return tea.NewView(View(rm.state))
}

// NewProgram constructs a Program, wiring the Bubble Tea program to the Bridge
// via teaSender.
func NewProgram(cfg ProgramConfig) (*Program, error) {
	initial := NewModel(ModelConfig{ChangeID: cfg.ChangeID})

	p := &Program{
		state:   initial,
		browser: cfg.Browser,
	}

	var openBrowserFn func(string)
	if cfg.Browser != nil {
		openBrowserFn = func(url string) {
			p.handleOpenBrowser(url)
		}
	}

	root := rootModel{
		state: initial,
		publish: func(m Model) {
			p.stateMu.Lock()
			p.state = m
			p.stateMu.Unlock()
		},
		openBrowser: openBrowserFn,
	}

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

	p.tea = teaProg
	p.bridge = bridge

	return p, nil
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

	defer func() {
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
	}()

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

// handleOpenBrowser runs the browser open in a goroutine and sends a
// BrowserOpenedMsg back into the program loop with the result.
// Only called when p.browser != nil (enforced by NewProgram).
func (p *Program) handleOpenBrowser(url string) {
	browser := p.browser
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := browser.Open(ctx, url)
		p.mu.Lock()
		running := p.running
		p.mu.Unlock()
		if !running {
			return
		}
		p.tea.Send(BrowserOpenedMsg{Err: err})
	}()
}

// SendForTest forwards a tea.Msg into the program loop. Test-only.
func (p *Program) SendForTest(msg any) {
	p.mu.Lock()
	running := p.running
	p.mu.Unlock()
	if !running || p.tea == nil {
		return
	}
	p.tea.Send(msg)
}

// Snapshot returns a copy of the latest pure-Model state observed by the
// program. Tests use this to inspect side effects without racing.
func (p *Program) Snapshot() Model {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	return p.state
}

// reattachHint returns the user-facing reattach instruction printed AFTER
// the bubbletea program exits. Spec §2.2.
func reattachHint(id domain.ChangeID) string {
	if id.IsZero() {
		return "Detached."
	}
	return fmt.Sprintf("Detached. Reattach with: sophia attach %s", id)
}
