package tui_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
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

func TestUpdateTabTogglesView(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, cmd := tui.Update(m, keyPressTab())
	if m2.CurrentView() != tui.ViewApplyBoard {
		t.Errorf("after Tab from Timeline, view = %v", m2.CurrentView())
	}
	if cmd != nil {
		t.Errorf("Tab should not produce a Cmd")
	}
	m3, _ := tui.Update(m2, keyPressTab())
	if m3.CurrentView() != tui.ViewTimeline {
		t.Errorf("after second Tab, view = %v", m3.CurrentView())
	}
}

func TestUpdateTabInConfirmModeCancelsAndToggles(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).WithConfirmingDetach(true)
	m2, _ := tui.Update(m, keyPressTab())
	if m2.ConfirmingDetach() {
		t.Error("Tab should cancel confirm mode (D-M7-02)")
	}
	if m2.CurrentView() != tui.ViewApplyBoard {
		t.Error("Tab should also toggle view after canceling confirm (D-M7-02)")
	}
}

func TestUpdateOKeyWithBannerEmitsOpenBrowserMsg(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{URL: "https://gov.local/x"})
	m2, cmd := tui.Update(m, keyPress("o"))
	if cmd == nil {
		t.Fatal("[O] with banner visible should produce a Cmd that emits OpenBrowserMsg")
	}
	got := cmd()
	openMsg, ok := got.(tui.OpenBrowserMsg)
	if !ok {
		t.Fatalf("Cmd return = %T, want OpenBrowserMsg", got)
	}
	if openMsg.URL != "https://gov.local/x" {
		t.Errorf("OpenBrowserMsg.URL = %q", openMsg.URL)
	}
	// Model unchanged.
	if m2.BannerGate() == nil {
		t.Error("[O] should not clear banner on its own")
	}
}

func TestUpdateOKeyWithoutBannerIsNoOp(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, cmd := tui.Update(m, keyPress("o"))
	if cmd != nil {
		t.Errorf("[O] without banner should be a no-op (D-M7-03)")
	}
	_ = m2
}

func TestUpdateBrowserOpenedMsgWithErrorAddsErrorLine(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, cmd := tui.Update(m, tui.BrowserOpenedMsg{Err: errors.New("boom")})
	if cmd != nil {
		t.Errorf("BrowserOpenedMsg should not produce a Cmd")
	}
	errs := m2.Errors()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "browser") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an error line mentioning 'browser'; got %v", errs)
	}
}

func TestUpdateBrowserOpenedMsgWithNilIsNoOp(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, _ := tui.Update(m, tui.BrowserOpenedMsg{Err: nil})
	if len(m2.Errors()) != 0 {
		t.Errorf("BrowserOpenedMsg{nil} should not append errors; got %v", m2.Errors())
	}
}

// keyPressTab constructs a Tab keystroke per Bubble Tea v2 API.
func keyPressTab() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyTab}
}

// Regression for the ApprovalGateMsg URL-loss bug discovered during M7 Task 10.
// ApprovalGateMsg{Gate{URL,Reason,...}} → Model.BannerGate() must preserve all
// fields from the Gate. Previously, the synthetic event round-trip via
// approvalGateAsEvent only copied `phase`, losing URL/Reason/Risk/Policy.
func TestUpdateApprovalGateMsgPreservesAllFields(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	gate := domain.ApprovalGate{
		URL:      "https://gov.local/approvals/abc",
		Reason:   "policy says no apply",
		Risk:     "medium",
		Policy:   "require_approval",
		ChangeID: domain.ChangeID("01HX"),
		Phase:    domain.PhaseApply,
		TraceID:  "trace-xyz",
	}
	m2, _ := tui.Update(m, tui.ApprovalGateMsg{Gate: gate})

	got := m2.BannerGate()
	if got == nil {
		t.Fatal("ApprovalGateMsg should set BannerGate")
	}
	if got.URL != "https://gov.local/approvals/abc" {
		t.Errorf("URL = %q (lost in roundtrip?)", got.URL)
	}
	if got.Reason != "policy says no apply" {
		t.Errorf("Reason = %q", got.Reason)
	}
	if got.Risk != "medium" {
		t.Errorf("Risk = %q", got.Risk)
	}
	if got.Policy != "require_approval" {
		t.Errorf("Policy = %q", got.Policy)
	}
	if got.Phase != domain.PhaseApply {
		t.Errorf("Phase = %q", got.Phase)
	}
	if got.TraceID != "trace-xyz" {
		t.Errorf("TraceID = %q", got.TraceID)
	}
}
