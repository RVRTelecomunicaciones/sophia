// Package jsonsink implements inbound.EventSink by writing one JSON object
// per line to a configured io.Writer (typically os.Stdout). Used by
// `sophia run --no-tui --json` per spec §2.4.
package jsonsink

import (
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// Config configures the Sink.
type Config struct {
	Writer io.Writer
}

// Sink implements inbound.EventSink.
type Sink struct {
	mu sync.Mutex
	w  io.Writer
}

// New returns a Sink writing to cfg.Writer.
func New(cfg Config) *Sink { return &Sink{w: cfg.Writer} }

// OnSnapshot writes a snapshot line.
func (s *Sink) OnSnapshot(_ context.Context, c *domain.Change) error {
	phases := make([]map[string]any, 0, len(c.Phases))
	for _, p := range c.Phases {
		phases = append(phases, map[string]any{
			"id":         p.ID,
			"type":       string(p.Type),
			"status":     string(p.Status),
			"confidence": p.Confidence,
			"started_at": optionalTime(p.StartedAt),
			"ended_at":   optionalTime(p.EndedAt),
		})
	}
	return s.write(map[string]any{
		"type":             "snapshot",
		"change_id":        c.ID.String(),
		"name":             c.Name,
		"project":          c.Project,
		"status":           string(c.Status),
		"current_phase_id": c.CurrentPhaseID,
		"phases":           phases,
	})
}

// OnEvent writes an event line.
func (s *Sink) OnEvent(_ context.Context, ev domain.Event) error {
	return s.write(map[string]any{
		"type":    "event",
		"ev":      ev.Type,
		"ts":      optionalTime(ev.Timestamp),
		"payload": ev.Payload,
	})
}

// OnApprovalGate writes an approval-gate event line.
func (s *Sink) OnApprovalGate(_ context.Context, g domain.ApprovalGate) error {
	return s.write(map[string]any{
		"type": "event",
		"ev":   "approval.required",
		"payload": map[string]any{
			"gate_url":  g.URL,
			"reason":    g.Reason,
			"risk":      g.Risk,
			"policy":    g.Policy,
			"change_id": g.ChangeID.String(),
			"phase":     string(g.Phase),
		},
	})
}

// OnError writes an error line. Best-effort: never bubbles write errors up to
// the runner so that streaming continues for all sinks.
func (s *Sink) OnError(_ context.Context, e error) error {
	return s.write(map[string]any{
		"type":    "error",
		"message": e.Error(),
	})
}

// OnComplete writes a final terminal-status line.
func (s *Sink) OnComplete(_ context.Context, st domain.ChangeStatus) error {
	return s.write(map[string]any{
		"type":         "complete",
		"final_status": string(st),
	})
}

// Close is a no-op for stdout.
func (s *Sink) Close() error { return nil }

func (s *Sink) write(v map[string]any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.w.Write(b)
	return err
}

// optionalTime returns an RFC3339 string or nil for zero times, so JSONL lines
// don't carry meaningless `0001-01-01T00:00:00Z` entries.
func optionalTime(t interface{ IsZero() bool }) any {
	type zeroable interface {
		Format(string) string
		IsZero() bool
	}
	z, ok := t.(zeroable)
	if !ok || z.IsZero() {
		return nil
	}
	return z.Format("2006-01-02T15:04:05.000Z07:00")
}
