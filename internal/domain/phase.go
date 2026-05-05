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

const (
	PhaseStatusPending PhaseStatus = "pending"
	PhaseStatusRunning PhaseStatus = "running"
	PhaseStatusDone    PhaseStatus = "done"
	PhaseStatusBlocked PhaseStatus = "blocked"
	PhaseStatusFailed  PhaseStatus = "failed"
)

func (s PhaseStatus) IsTerminal() bool {
	return s == PhaseStatusDone || s == PhaseStatusBlocked || s == PhaseStatusFailed
}

type Phase struct {
	ID         string
	Type       PhaseType
	Status     PhaseStatus
	Confidence float64
	StartedAt  time.Time
	EndedAt    time.Time
}
