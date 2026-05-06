package cli

import (
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// changeResponseFromDomain converts a domain.Change back to the
// orchestratorhttp wire shape. Used by `sophia changes --json` (Task 5) and
// `sophia status --json` (Task 7) so both commands emit identical JSON to
// what the orchestrator sends.
func changeResponseFromDomain(c *domain.Change) orchestratorhttp.ChangeResponse {
	r := orchestratorhttp.ChangeResponse{
		ChangeID:          c.ID.String(),
		Name:              c.Name,
		Project:           c.Project,
		BaseRef:           c.BaseRef,
		ArtifactStoreMode: c.ArtifactStoreMode,
		Status:            string(c.Status),
		CurrentPhaseID:    c.CurrentPhaseID,
		CreatedAt:         c.CreatedAt,
		UpdatedAt:         c.UpdatedAt,
	}
	if len(c.Phases) > 0 {
		r.Phases = make([]orchestratorhttp.PhaseDTO, len(c.Phases))
		for i, p := range c.Phases {
			r.Phases[i] = orchestratorhttp.PhaseDTO{
				ID:         p.ID,
				Type:       string(p.Type),
				Status:     string(p.Status),
				Confidence: p.Confidence,
				StartedAt:  p.StartedAt,
				EndedAt:    p.EndedAt,
			}
		}
	}
	return r
}
