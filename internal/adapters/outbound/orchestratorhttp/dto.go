package orchestratorhttp

import (
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// CreateChangeRequest mirrors the body of POST /api/v1/changes.
type CreateChangeRequest struct {
	Name              string `json:"name"`
	Project           string `json:"project"`
	BaseRef           string `json:"base_ref"`
	ArtifactStoreMode string `json:"artifact_store_mode"`
}

// PhaseDTO mirrors a phase entry in the orchestrator's snapshot response.
type PhaseDTO struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Status     string    `json:"status"`
	Confidence float64   `json:"confidence"`
	StartedAt  time.Time `json:"started_at,omitzero"`
	EndedAt    time.Time `json:"ended_at,omitzero"`
}

// ChangeResponse mirrors the response shape of POST /api/v1/changes and
// GET /api/v1/changes/{id}. All fields are tolerant of absence per spec
// §3.2 (forward-compatible).
type ChangeResponse struct {
	ChangeID          string     `json:"change_id"`
	Name              string     `json:"name,omitempty"`
	Project           string     `json:"project,omitempty"`
	BaseRef           string     `json:"base_ref,omitempty"`
	ArtifactStoreMode string     `json:"artifact_store_mode,omitempty"`
	Status            string     `json:"status,omitempty"`
	CurrentPhaseID    string     `json:"current_phase_id,omitempty"`
	Phases            []PhaseDTO `json:"phases,omitempty"`
	CreatedAt         time.Time  `json:"created_at,omitzero"`
	UpdatedAt         time.Time  `json:"updated_at,omitzero"`
}

// ToDomain converts a ChangeResponse to the pure domain type.
func (r *ChangeResponse) ToDomain() *domain.Change {
	c := &domain.Change{
		ID:                domain.ChangeID(r.ChangeID),
		Name:              r.Name,
		Project:           r.Project,
		BaseRef:           r.BaseRef,
		ArtifactStoreMode: r.ArtifactStoreMode,
		Status:            domain.ChangeStatus(r.Status),
		CurrentPhaseID:    r.CurrentPhaseID,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
	}
	if len(r.Phases) > 0 {
		c.Phases = make([]domain.Phase, len(r.Phases))
		for i, p := range r.Phases {
			c.Phases[i] = domain.Phase{
				ID:         p.ID,
				Type:       domain.PhaseType(p.Type),
				Status:     domain.PhaseStatus(p.Status),
				Confidence: p.Confidence,
				StartedAt:  p.StartedAt,
				EndedAt:    p.EndedAt,
			}
		}
	}
	return c
}

// ListChangesResponse mirrors GET /api/v1/changes.
type ListChangesResponse struct {
	Items []ChangeResponse `json:"items"`
	Total int              `json:"total"`
}
