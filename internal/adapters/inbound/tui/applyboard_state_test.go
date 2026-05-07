package tui_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

func TestNewApplyBoardStateIsEmpty(t *testing.T) {
	s := tui.NewApplyBoardState()
	if got := s.GroupCount(); got != 0 {
		t.Errorf("fresh GroupCount = %d, want 0", got)
	}
	if got := s.Groups(); len(got) != 0 {
		t.Errorf("fresh Groups() = %v, want []", got)
	}
}

func TestApplyBoardStateApplyTaskStartedCreatesGroupAndTask(t *testing.T) {
	s := tui.NewApplyBoardState()
	s2 := s.ApplyEvent(domain.Event{
		Type: "task.started",
		Payload: map[string]any{
			"group_id":      "g1",
			"task_id":       "t1",
			"files_pattern": "internal/**/*.go",
		},
	})

	groups := s2.Groups()
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if groups[0].ID != "g1" {
		t.Errorf("group ID = %q", groups[0].ID)
	}
	if len(groups[0].Tasks) != 1 {
		t.Fatalf("tasks in g1 = %d, want 1", len(groups[0].Tasks))
	}
	task := groups[0].Tasks[0]
	if task.ID != "t1" {
		t.Errorf("task ID = %q", task.ID)
	}
	if task.FilesPattern != "internal/**/*.go" {
		t.Errorf("FilesPattern = %q", task.FilesPattern)
	}
	if task.Status != "running" {
		t.Errorf("Status = %q, want running", task.Status)
	}
}

func TestApplyBoardStateApplyTaskCompletedUpdatesStatus(t *testing.T) {
	s := tui.NewApplyBoardState().ApplyEvent(domain.Event{
		Type: "task.started",
		Payload: map[string]any{
			"group_id": "g1",
			"task_id":  "t1",
		},
	}).ApplyEvent(domain.Event{
		Type: "task.completed",
		Payload: map[string]any{
			"group_id": "g1",
			"task_id":  "t1",
			"status":   "done",
		},
	})

	groups := s.Groups()
	if groups[0].Tasks[0].Status != "done" {
		t.Errorf("Status after completed = %q, want done", groups[0].Tasks[0].Status)
	}
}

func TestApplyBoardStateApplyAgentSpawnedAttachesToTask(t *testing.T) {
	s := tui.NewApplyBoardState().ApplyEvent(domain.Event{
		Type: "task.started",
		Payload: map[string]any{
			"group_id": "g1",
			"task_id":  "t1",
		},
	}).ApplyEvent(domain.Event{
		Type: "agent.spawned",
		Payload: map[string]any{
			"agent_role": "team_lead",
			"agent_id":   "a1",
			"group_id":   "g1",
			"task_id":    "t1",
		},
	})

	task := s.Groups()[0].Tasks[0]
	if len(task.Agents) != 1 {
		t.Fatalf("agents in t1 = %d, want 1", len(task.Agents))
	}
	if task.Agents[0].ID != "a1" || task.Agents[0].Role != "team_lead" {
		t.Errorf("agent = %+v", task.Agents[0])
	}
	if task.Agents[0].Status != "running" {
		t.Errorf("agent.Status = %q, want running", task.Agents[0].Status)
	}
}

func TestApplyBoardStateApplyAgentCompletedUpdatesAgentStatus(t *testing.T) {
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}}).
		ApplyEvent(domain.Event{Type: "agent.spawned", Payload: map[string]any{"agent_id": "a1", "agent_role": "worker", "group_id": "g1", "task_id": "t1"}}).
		ApplyEvent(domain.Event{Type: "agent.completed", Payload: map[string]any{"agent_id": "a1", "status": "done"}})

	got := s.Groups()[0].Tasks[0].Agents[0]
	if got.Status != "done" {
		t.Errorf("agent.Status = %q, want done", got.Status)
	}
}

func TestApplyBoardStateGroupsOrderIsInsertionOrder(t *testing.T) {
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g3", "task_id": "t3"}}).
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}}).
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g2", "task_id": "t2"}})

	groups := s.Groups()
	want := []string{"g3", "g1", "g2"}
	for i, g := range groups {
		if g.ID != want[i] {
			t.Errorf("groups[%d] = %q, want %q", i, g.ID, want[i])
		}
	}
}

func TestApplyBoardStateAgentWithoutTaskIDIsAttachedAtGroupLevel(t *testing.T) {
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}}).
		ApplyEvent(domain.Event{
			Type: "agent.spawned",
			Payload: map[string]any{
				"agent_role": "team_lead",
				"agent_id":   "a1",
				"group_id":   "g1",
				// no task_id
			},
		})

	g := s.Groups()[0]
	if len(g.Agents) != 1 {
		t.Fatalf("group-level agents = %d, want 1", len(g.Agents))
	}
	if g.Agents[0].Role != "team_lead" {
		t.Errorf("group-level agent role = %q", g.Agents[0].Role)
	}
}

func TestApplyBoardStateUnknownEventIsNoOp(t *testing.T) {
	s := tui.NewApplyBoardState().ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}})
	before := s.GroupCount()
	s2 := s.ApplyEvent(domain.Event{Type: "phase.started", Payload: map[string]any{"phase_type": "apply"}})
	if s2.GroupCount() != before {
		t.Errorf("phase.started should not affect ApplyBoard state; group count changed: %d → %d", before, s2.GroupCount())
	}
}

func TestApplyBoardStateLazyCreatesGroupOnAgentSpawn(t *testing.T) {
	s := tui.NewApplyBoardState().ApplyEvent(domain.Event{
		Type: "agent.spawned",
		Payload: map[string]any{
			"agent_id":   "a1",
			"agent_role": "worker",
			"group_id":   "g1",
			"task_id":    "t1",
		},
	})

	groups := s.Groups()
	if len(groups) != 1 || groups[0].ID != "g1" {
		t.Fatalf("groups = %+v", groups)
	}
	if len(groups[0].Tasks) != 1 || groups[0].Tasks[0].ID != "t1" {
		t.Fatalf("tasks = %+v", groups[0].Tasks)
	}
	if len(groups[0].Tasks[0].Agents) != 1 {
		t.Fatalf("agents = %+v", groups[0].Tasks[0].Agents)
	}
}

func TestApplyBoardStateCapsGroupsAt50(t *testing.T) {
	s := tui.NewApplyBoardState()
	for i := 0; i < 60; i++ {
		s = s.ApplyEvent(domain.Event{
			Type: "task.started",
			Payload: map[string]any{
				"group_id": groupID(i),
				"task_id":  "t1",
			},
		})
	}
	if got := s.GroupCount(); got != 50 {
		t.Errorf("GroupCount after 60 inserts = %d, want 50", got)
	}
	// The 10 oldest groups should have been evicted.
	for _, g := range s.Groups() {
		if g.ID == groupID(0) || g.ID == groupID(9) {
			t.Errorf("oldest group %q should have been evicted", g.ID)
		}
	}
}

func TestApplyBoardStateImmutability(t *testing.T) {
	s1 := tui.NewApplyBoardState()
	s2 := s1.ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}})
	if s1.GroupCount() != 0 {
		t.Error("ApplyEvent mutated the receiver")
	}
	_ = s2
}

func groupID(i int) string {
	switch i {
	case 0:
		return "g0"
	case 9:
		return "g9"
	}
	return "g" + itoa3(i)
}

func itoa3(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}
