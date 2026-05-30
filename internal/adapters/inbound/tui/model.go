package tui

import (
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/pkg/contract"
)

// DefaultWidth and DefaultHeight are conservative defaults for terminals
// that haven't yet sent a tea.WindowSizeMsg. The actual View truncates rows
// to fit Width.
const (
	DefaultWidth  = 80
	DefaultHeight = 24
)

// PhaseRow is the per-phase render state.
type PhaseRow struct {
	Type        domain.PhaseType
	ID          string
	Status      domain.PhaseStatus
	Confidence  float64
	StartedAt   time.Time
	EndedAt     time.Time
	HasApproval bool
}

// ViewType selects which TUI view is currently displayed.
type ViewType int

const (
	// ViewTimeline shows the 9-phase Timeline (M6 default view).
	ViewTimeline ViewType = iota
	// ViewApplyBoard shows the ApplyBoard groups → tasks → agents tree.
	ViewApplyBoard
)

// String returns a debug-friendly name.
func (v ViewType) String() string {
	switch v {
	case ViewTimeline:
		return "timeline"
	case ViewApplyBoard:
		return "applyboard"
	}
	return "unknown"
}

// ModelConfig configures the initial Model.
type ModelConfig struct {
	ChangeID domain.ChangeID
}

// Model is the immutable TUI state. All mutation methods (Apply*, With*)
// return a NEW Model and never modify the receiver.
type Model struct {
	changeID         domain.ChangeID
	changeStatus     domain.ChangeStatus
	currentPhaseID   string
	phases           [9]PhaseRow
	confirmingDetach bool
	detached         bool
	width            int
	height           int
	errors           []string

	// M7 additions:
	currentView ViewType
	bannerGate  *domain.ApprovalGate
	applyBoard  ApplyBoardState
}

// NewModel constructs a Model with all 9 phases in PhaseStatusPending.
func NewModel(cfg ModelConfig) Model {
	var rows [9]PhaseRow
	for i, pt := range domain.AllPhases() {
		rows[i] = PhaseRow{Type: pt, Status: domain.PhaseStatusPending}
	}
	return Model{
		changeID:     cfg.ChangeID,
		changeStatus: domain.ChangeStatusPending,
		phases:       rows,
		width:        DefaultWidth,
		height:       DefaultHeight,
	}
}

// ChangeID returns the Change being observed.
func (m Model) ChangeID() domain.ChangeID { return m.changeID }

// ChangeStatus returns the latest known top-level status.
func (m Model) ChangeStatus() domain.ChangeStatus { return m.changeStatus }

// CurrentPhaseID returns the phase ID currently flagged as running.
func (m Model) CurrentPhaseID() string { return m.currentPhaseID }

// PhaseRows returns a copy of the 9 phase render rows.
func (m Model) PhaseRows() []PhaseRow {
	out := make([]PhaseRow, 9)
	copy(out, m.phases[:])
	return out
}

// Width returns the terminal width.
func (m Model) Width() int { return m.width }

// Height returns the terminal height.
func (m Model) Height() int { return m.height }

// ConfirmingDetach reports whether the user is in the Ctrl+C confirm dialog.
func (m Model) ConfirmingDetach() bool { return m.confirmingDetach }

// Detached reports whether the user has detached the program.
func (m Model) Detached() bool { return m.detached }

// Errors returns a copy of the recorded non-fatal error messages.
func (m Model) Errors() []string {
	out := make([]string, len(m.errors))
	copy(out, m.errors)
	return out
}

// CurrentView returns the active view (Timeline or ApplyBoard).
func (m Model) CurrentView() ViewType { return m.currentView }

// WithCurrentView returns a new Model with the active view set to v.
func (m Model) WithCurrentView(v ViewType) Model {
	m.currentView = v
	return m
}

// BannerGate returns the active ApprovalGate (banner state) or nil if hidden.
func (m Model) BannerGate() *domain.ApprovalGate { return m.bannerGate }

// WithBannerGate returns a new Model whose banner state is set to gate.
// Pass nil to hide the banner.
func (m Model) WithBannerGate(gate *domain.ApprovalGate) Model {
	m.bannerGate = gate
	return m
}

// ApplyBoard returns the ApplyBoard state.
func (m Model) ApplyBoard() ApplyBoardState { return m.applyBoard }

// Resize returns a new Model with updated terminal dimensions.
func (m Model) Resize(width, height int) Model {
	if width <= 0 {
		width = DefaultWidth
	}
	if height <= 0 {
		height = DefaultHeight
	}
	m.width = width
	m.height = height
	return m
}

// WithConfirmingDetach toggles the detach-confirmation dialog.
func (m Model) WithConfirmingDetach(v bool) Model {
	m.confirmingDetach = v
	return m
}

// WithDetached marks the program as detached.
func (m Model) WithDetached(v bool) Model {
	m.detached = v
	return m
}

// WithError appends a non-fatal error message to the model.
func (m Model) WithError(msg string) Model {
	m.errors = append(append([]string(nil), m.errors...), msg)
	return m
}

// WithComplete records the terminal status.
func (m Model) WithComplete(st domain.ChangeStatus) Model {
	m.changeStatus = st
	return m
}

// ApplySnapshot replaces phase rows from a domain.Change snapshot.
func (m Model) ApplySnapshot(c *domain.Change) Model {
	if c == nil {
		return m
	}
	m.changeStatus = c.Status
	m.currentPhaseID = c.CurrentPhaseID

	// Reset phase rows to pending (snapshot is authoritative).
	for i, pt := range domain.AllPhases() {
		m.phases[i] = PhaseRow{Type: pt, Status: domain.PhaseStatusPending}
	}
	for _, p := range c.Phases {
		idx := indexOfPhase(p.Type)
		if idx < 0 {
			continue
		}
		m.phases[idx] = PhaseRow{
			Type:       p.Type,
			ID:         p.ID,
			Status:     p.Status,
			Confidence: p.Confidence,
			StartedAt:  p.StartedAt,
			EndedAt:    p.EndedAt,
		}
	}
	// M7: clear banner if snapshot shows we've moved past the gated phase.
	if m.bannerGate != nil {
		if pt, ok := currentPhaseType(c); ok {
			if isPhaseAfter(pt, m.bannerGate.Phase) {
				m.bannerGate = nil
			}
		}
	}
	return m
}

// ApplyEvent integrates a single domain.Event into the model.
//
// The set of event types it understands grew in M7 and Phase 4 / M10:
//
//   - phase.started, phase.completed, phase.failed — Timeline phase transitions
//   - approval.required                    — sets bannerGate AND marks phase row
//   - approval.resolved                    — clears bannerGate (M7)
//   - apply.board.created … apply.materialize.error — 16 real apply events feed ApplyBoard
//   - agent.dispatched                     — Timeline/dispatch concern; tolerated, no-op for ApplyBoard
//   - open                                 — sophia-wire-v1 §5.3 open event: no-op
//   - heartbeat                            — already filtered by Runner; no-op here
//
// Side-effect on banner clearing:
//   - phase.started for any phase whose ordinal is STRICTLY GREATER than
//     bannerGate.Phase clears the banner (forward progress, D-M7-07).
func (m Model) ApplyEvent(ev domain.Event) Model {
	switch ev.Type {
	case "phase.started":
		m = m.applyPhaseStarted(ev)
		m = m.maybeClearBannerOnForwardProgress(ev)
		return m
	case "phase.completed", "phase.failed", "phase.completed_with_concerns", "phase.needs_context":
		return m.applyPhaseCompleted(ev)
	case "approval.required":
		m = m.applyApprovalRequired(ev)
		m = m.applyBannerFromEvent(ev)
		return m
	case "approval.resolved":
		m.bannerGate = nil
		return m
	case contract.EventApplyBoardCreated,
		contract.EventApplyTeamLeadSpawned,
		contract.EventApplyTaskClaimed,
		contract.EventApplyTaskClaimSkipped,
		contract.EventApplyTaskRetry,
		contract.EventApplyTaskEscalated,
		contract.EventApplyGroupCompleted,
		contract.EventApplyGroupFailed,
		contract.EventApplyGroupDegraded,
		contract.EventApplyImplementSpawnFailed,
		contract.EventApplyImplementSpawnGovernorError,
		contract.EventApplyDispatchError,
		contract.EventApplyEnvelopeValidationFailed,
		contract.EventApplyMaterializeStarted,
		contract.EventApplyMaterializeCompleted,
		contract.EventApplyMaterializeError:
		m.applyBoard = m.applyBoard.ApplyEvent(ev)
		return m
	default:
		// Unknown event types (agent.dispatched, orch-internal extensions,
		// future events) are tolerated: Timeline / ApplyBoard unchanged.
		// Forward-compat policy from sophia-wire-v1 §10.
		return m
	}
}

func (m Model) applyPhaseStarted(ev domain.Event) Model {
	pt := phaseTypeFromPayload(ev.Payload)
	if pt == "" {
		return m
	}
	idx := indexOfPhase(pt)
	if idx < 0 {
		return m
	}
	row := m.phases[idx]
	row.Status = domain.PhaseStatusRunning
	if id, ok := ev.Payload["phase_id"].(string); ok {
		row.ID = id
		m.currentPhaseID = id
	}
	if !ev.Timestamp.IsZero() {
		row.StartedAt = ev.Timestamp
	}
	m.phases[idx] = row
	return m
}

func (m Model) applyPhaseCompleted(ev domain.Event) Model {
	pt := phaseTypeFromPayload(ev.Payload)
	if pt == "" {
		return m
	}
	idx := indexOfPhase(pt)
	if idx < 0 {
		return m
	}
	row := m.phases[idx]
	if statusStr, ok := ev.Payload["status"].(string); ok {
		row.Status = domain.PhaseStatus(statusStr)
	} else {
		row.Status = domain.PhaseStatusDone
	}
	if conf, ok := ev.Payload["confidence"].(float64); ok {
		row.Confidence = conf
	}
	if !ev.Timestamp.IsZero() {
		row.EndedAt = ev.Timestamp
	}
	m.phases[idx] = row
	return m
}

func (m Model) applyApprovalRequired(ev domain.Event) Model {
	pt := phaseTypeFromPayload(ev.Payload)
	if pt == "" {
		if ph, ok := ev.Payload["phase"].(string); ok {
			pt = domain.PhaseType(ph)
		}
	}
	if pt == "" {
		return m
	}
	idx := indexOfPhase(pt)
	if idx < 0 {
		return m
	}
	m.phases[idx].HasApproval = true
	return m
}

// applyBannerFromEvent constructs an ApprovalGate from an approval.required
// event payload and sets it as the current bannerGate. Spec §5.4 payload
// keys: gate_url, reason, risk, policy, phase, change_id.
func (m Model) applyBannerFromEvent(ev domain.Event) Model {
	gate := domain.ApprovalGate{TraceID: ev.TraceID}
	if ev.Payload == nil {
		m.bannerGate = &gate
		return m
	}
	gate.URL, _ = ev.Payload["gate_url"].(string)
	gate.Reason, _ = ev.Payload["reason"].(string)
	gate.Risk, _ = ev.Payload["risk"].(string)
	gate.Policy, _ = ev.Payload["policy"].(string)
	if ph, ok := ev.Payload["phase"].(string); ok {
		gate.Phase = domain.PhaseType(ph)
	} else if ph, ok := ev.Payload["phase_type"].(string); ok {
		gate.Phase = domain.PhaseType(ph)
	}
	if cid, ok := ev.Payload["change_id"].(string); ok {
		gate.ChangeID = domain.ChangeID(cid)
	}
	m.bannerGate = &gate
	return m
}

// maybeClearBannerOnForwardProgress clears bannerGate when ev is a
// phase.started for a phase strictly later than bannerGate.Phase.
func (m Model) maybeClearBannerOnForwardProgress(ev domain.Event) Model {
	if m.bannerGate == nil {
		return m
	}
	pt := phaseTypeFromPayload(ev.Payload)
	if pt == "" {
		return m
	}
	if isPhaseAfter(pt, m.bannerGate.Phase) {
		m.bannerGate = nil
	}
	return m
}

// isPhaseAfter reports whether candidate's ordinal in domain.AllPhases() is
// strictly greater than reference's ordinal. Returns false if either phase
// is unknown.
func isPhaseAfter(candidate, reference domain.PhaseType) bool {
	c := indexOfPhase(candidate)
	r := indexOfPhase(reference)
	if c < 0 || r < 0 {
		return false
	}
	return c > r
}

// currentPhaseType returns the PhaseType of the snapshot's CurrentPhaseID,
// or ("", false) if no phase row in the snapshot matches.
func currentPhaseType(c *domain.Change) (domain.PhaseType, bool) {
	if c == nil {
		return "", false
	}
	for _, p := range c.Phases {
		if p.ID == c.CurrentPhaseID {
			return p.Type, true
		}
	}
	return "", false
}

// indexOfPhase returns the index of pt in domain.AllPhases(), or -1.
func indexOfPhase(pt domain.PhaseType) int {
	for i, candidate := range domain.AllPhases() {
		if candidate == pt {
			return i
		}
	}
	return -1
}

// phaseTypeFromPayload reads the canonical "phase_type" field. Spec §5.4
// defines the field name; older or alternate sources may use "phase" — we
// accept both for tolerance.
func phaseTypeFromPayload(payload map[string]any) domain.PhaseType {
	if payload == nil {
		return ""
	}
	if s, ok := payload["phase_type"].(string); ok {
		return domain.PhaseType(s)
	}
	if s, ok := payload["phase"].(string); ok {
		return domain.PhaseType(s)
	}
	return ""
}
