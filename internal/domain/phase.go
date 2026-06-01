package domain

import "time"

type PhaseType string

const (
	PhaseInit     PhaseType = "init"
	PhaseExplore  PhaseType = "explore"
	PhaseProposal PhaseType = "proposal"
	PhaseSpec     PhaseType = "spec"
	PhaseDesign   PhaseType = "design"
	PhaseTasks    PhaseType = "tasks"
	PhaseApply    PhaseType = "apply"
	PhaseVerify   PhaseType = "verify"
	PhaseArchive  PhaseType = "archive"
)

func AllPhases() []PhaseType {
	return []PhaseType{
		PhaseInit, PhaseExplore, PhaseProposal, PhaseSpec, PhaseDesign,
		PhaseTasks, PhaseApply, PhaseVerify, PhaseArchive,
	}
}

func (p PhaseType) IsValid() bool {
	switch p {
	case PhaseInit, PhaseExplore, PhaseProposal, PhaseSpec, PhaseDesign,
		PhaseTasks, PhaseApply, PhaseVerify, PhaseArchive:
		return true
	}
	return false
}

type PhaseStatus string

// Canonical phase-status set — aligned to sophia-wire-v1 §6.1 (7 values).
// "failed" is NOT a phase status; it is the phase.failed SSE event (§5.3).
// See docs/specs/sophia-wire-v1.sha256 for spec integrity.
const (
	PhaseStatusPending          PhaseStatus = "pending"
	PhaseStatusRunning          PhaseStatus = "running"
	PhaseStatusDone             PhaseStatus = "done"
	PhaseStatusDoneWithConcerns PhaseStatus = "done_with_concerns"
	PhaseStatusBlocked          PhaseStatus = "blocked"
	PhaseStatusNeedsContext     PhaseStatus = "needs_context"
	PhaseStatusInterrupted      PhaseStatus = "interrupted"
)

// IsTerminal reports whether the phase has reached a terminal state.
// Terminal states: done, done_with_concerns, blocked.
// Note: a blocked phase may represent either an approval gate or a hard
// failure; the distinction is carried by the phase.failed SSE event, not
// the phase status itself. Callers that need to distinguish failure from
// an approval wait MUST observe the phase.failed event (late subscribers
// cannot infer failure from GET /phases/{id} alone).
func (s PhaseStatus) IsTerminal() bool {
	return s == PhaseStatusDone || s == PhaseStatusDoneWithConcerns || s == PhaseStatusBlocked
}

type Phase struct {
	ID         string
	Type       PhaseType
	Status     PhaseStatus
	Confidence float64
	StartedAt  time.Time
	EndedAt    time.Time
}
