package tui_test

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestUpdateSnapshotMsgUpdatesModel(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	change := &domain.Change{
		ID:             domain.ChangeID("01HX"),
		Status:         domain.ChangeStatusRunning,
		CurrentPhaseID: "p-explore",
		Phases: []domain.Phase{
			{ID: "p-explore", Type: domain.PhaseExplore, Status: domain.PhaseStatusRunning},
		},
	}

	m2, cmd := tui.Update(m, tui.SnapshotMsg{Change: change})
	if m2.ChangeStatus() != domain.ChangeStatusRunning {
		t.Errorf("status = %q", m2.ChangeStatus())
	}
	if cmd != nil {
		t.Errorf("snapshot should not produce a Cmd")
	}
}

func TestUpdateEventMsgUpdatesModel(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})

	m2, cmd := tui.Update(m, tui.EventMsg{Event: domain.Event{
		Type:    "phase.started",
		Payload: map[string]any{"phase_type": "explore", "phase_id": "p-1"},
	}})
	if cmd != nil {
		t.Errorf("event should not produce a Cmd")
	}
	if m2.CurrentPhaseID() != "p-1" {
		t.Errorf("CurrentPhaseID = %q", m2.CurrentPhaseID())
	}
}

func TestUpdateErrorMsgRecordsError(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, _ := tui.Update(m, tui.ErrorMsg{Err: errors.New("boom")})
	errs := m2.Errors()
	if len(errs) != 1 || errs[0] != "boom" {
		t.Errorf("errors = %v", errs)
	}
}

func TestUpdateCompleteMsgQuits(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, cmd := tui.Update(m, tui.CompleteMsg{Status: domain.ChangeStatusDone})
	if m2.ChangeStatus() != domain.ChangeStatusDone {
		t.Errorf("status = %q", m2.ChangeStatus())
	}
	if cmd == nil {
		t.Fatal("CompleteMsg should produce tea.Quit Cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("Cmd return = %T, want tea.QuitMsg", cmd())
	}
}

func TestUpdateWindowSizeMsgUpdatesDimensions(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, cmd := tui.Update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m2.Width() != 120 || m2.Height() != 40 {
		t.Errorf("after WindowSizeMsg w=%d h=%d", m2.Width(), m2.Height())
	}
	if cmd != nil {
		t.Errorf("WindowSizeMsg should not produce a Cmd")
	}
}

func TestUpdateQKeyDetaches(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, cmd := tui.Update(m, keyPress("q"))
	if !m2.Detached() {
		t.Error("Q should mark model detached")
	}
	if cmd == nil {
		t.Fatal("Q should return a tea.Quit Cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("Cmd return = %T, want tea.QuitMsg", cmd())
	}
}

func TestUpdateCtrlCFirstPressEntersConfirm(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, cmd := tui.Update(m, keyPress("ctrl+c"))
	if !m2.ConfirmingDetach() {
		t.Error("first Ctrl+C should set ConfirmingDetach=true")
	}
	if m2.Detached() {
		t.Error("first Ctrl+C should NOT detach")
	}
	if cmd != nil {
		t.Errorf("first Ctrl+C should not produce a Cmd")
	}
}

func TestUpdateCtrlCSecondPressDetaches(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).WithConfirmingDetach(true)
	m2, cmd := tui.Update(m, keyPress("ctrl+c"))
	if !m2.Detached() {
		t.Error("second Ctrl+C should detach")
	}
	if cmd == nil {
		t.Fatal("second Ctrl+C should return tea.Quit Cmd")
	}
}

func TestUpdateYConfirmsDetach(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).WithConfirmingDetach(true)
	m2, cmd := tui.Update(m, keyPress("y"))
	if !m2.Detached() {
		t.Error("y should detach when in confirm mode")
	}
	if cmd == nil {
		t.Fatal("y should return tea.Quit Cmd")
	}
}

func TestUpdateNCancelsConfirm(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).WithConfirmingDetach(true)
	m2, cmd := tui.Update(m, keyPress("n"))
	if m2.ConfirmingDetach() {
		t.Error("n should cancel confirm")
	}
	if m2.Detached() {
		t.Error("n must not detach")
	}
	if cmd != nil {
		t.Error("n should not produce a Cmd")
	}
}

func TestUpdateUnknownKeyInConfirmCancels(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).WithConfirmingDetach(true)
	m2, cmd := tui.Update(m, keyPress("x"))
	if m2.ConfirmingDetach() {
		t.Error("unrecognized key in confirm mode should cancel (D-M6-04)")
	}
	if cmd != nil {
		t.Error("unrecognized key should not produce a Cmd")
	}
}

// keyPress builds a tea.KeyPressMsg for the given canonical key name.
//
// v2 API findings:
//   - tea.KeyPressMsg is defined as: type KeyPressMsg Key
//   - tea.Key has fields: Text string, Mod KeyMod, Code rune, ShiftedCode rune, BaseCode rune, IsRepeat bool
//   - Key.String() returns Text if non-empty, otherwise falls back to Keystroke()
//   - Keystroke() builds: "ctrl+" prefix when ModCtrl set, then the Code rune
//   - For plain chars ("q", "y", "n", "x"): set Code = rune and Text = string for String() to return the char
//   - For ctrl combos ("ctrl+c"): set Code = 'c', Mod = tea.ModCtrl, Text = "" (Keystroke returns "ctrl+c")
func keyPress(name string) tea.KeyPressMsg {
	switch name {
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	default:
		// Single printable character — Text drives String().
		r := rune(name[0])
		return tea.KeyPressMsg{Code: r, Text: name}
	}
}
