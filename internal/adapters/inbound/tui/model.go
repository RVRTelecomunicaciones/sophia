package tui

import (
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
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
	return m
}

// ApplyEvent integrates a single domain.Event into the model.
func (m Model) ApplyEvent(ev domain.Event) Model {
	switch ev.Type {
	case "phase.started":
		return m.applyPhaseStarted(ev)
	case "phase.completed":
		return m.applyPhaseCompleted(ev)
	case "approval.required":
		return m.applyApprovalRequired(ev)
	default:
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
