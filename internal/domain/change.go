package domain

import "time"

type ChangeID string

func (id ChangeID) String() string { return string(id) }
func (id ChangeID) IsZero() bool   { return string(id) == "" }

type ChangeStatus string

const (
	ChangeStatusPending ChangeStatus = "pending"
	ChangeStatusRunning ChangeStatus = "running"
	ChangeStatusDone    ChangeStatus = "done"
	ChangeStatusBlocked ChangeStatus = "blocked"
	ChangeStatusFailed  ChangeStatus = "failed"
)

func (s ChangeStatus) IsTerminal() bool {
	return s == ChangeStatusDone || s == ChangeStatusBlocked || s == ChangeStatusFailed
}

type Change struct {
	ID                ChangeID
	Name              string
	Project           string
	BaseRef           string
	ArtifactStoreMode string
	Status            ChangeStatus
	CurrentPhaseID    string
	Phases            []Phase
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
