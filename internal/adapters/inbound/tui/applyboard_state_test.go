package tui_test

import (
	"fmt"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/pkg/contract"
)

// ---------------------------------------------------------------------------
// A-group: phantom event types MUST be ignored (A1-A4)
// ---------------------------------------------------------------------------

// TestApplyEvent_IgnoresTaskStarted — scenario A1
func TestApplyEvent_IgnoresTaskStarted(t *testing.T) {
	s := tui.NewApplyBoardState()
	s2 := s.ApplyEvent(domain.Event{
		Type:    "task.started",
		Payload: map[string]any{"group_id": "g1", "task_id": "t1"},
	})
	if s2.GroupCount() != 0 {
		t.Errorf("task.started must be ignored: got %d groups, want 0", s2.GroupCount())
	}
}

// TestApplyEvent_IgnoresTaskCompleted — scenario A2
func TestApplyEvent_IgnoresTaskCompleted(t *testing.T) {
	s := tui.NewApplyBoardState()
	s2 := s.ApplyEvent(domain.Event{
		Type:    "task.completed",
		Payload: map[string]any{"group_id": "g1", "task_id": "t1"},
	})
	if s2.GroupCount() != 0 {
		t.Errorf("task.completed must be ignored: got %d groups, want 0", s2.GroupCount())
	}
}

// TestApplyEvent_IgnoresAgentSpawned — scenario A3
func TestApplyEvent_IgnoresAgentSpawned(t *testing.T) {
	s := tui.NewApplyBoardState()
	s2 := s.ApplyEvent(domain.Event{
		Type:    "agent.spawned",
		Payload: map[string]any{"agent_id": "a1", "group_id": "g1"},
	})
	if s2.GroupCount() != 0 {
		t.Errorf("agent.spawned must be ignored: got %d groups, want 0", s2.GroupCount())
	}
}

// TestApplyEvent_IgnoresAgentCompleted — scenario A4
func TestApplyEvent_IgnoresAgentCompleted(t *testing.T) {
	s := tui.NewApplyBoardState()
	s2 := s.ApplyEvent(domain.Event{
		Type:    "agent.completed",
		Payload: map[string]any{"agent_id": "a1", "status": "done"},
	})
	if s2.GroupCount() != 0 {
		t.Errorf("agent.completed must be ignored: got %d groups, want 0", s2.GroupCount())
	}
}

// ---------------------------------------------------------------------------
// B-group: real apply.* event handlers (B1-B12)
// ---------------------------------------------------------------------------

// TestApplyBoardCreated_SeedsBoardAndGroupCount — scenario B1
func TestApplyBoardCreated_SeedsBoardAndGroupCount(t *testing.T) {
	s := tui.NewApplyBoardState().ApplyEvent(domain.Event{
		Type: contract.EventApplyBoardCreated,
		Payload: map[string]any{
			"board_id": "b1",
			"groups":   float64(3), // JSON numbers come as float64 in map[string]any
		},
	})

	if s.BoardID() != "b1" {
		t.Errorf("BoardID = %q, want %q", s.BoardID(), "b1")
	}
	if s.DeclaredGroupCount() != 3 {
		t.Errorf("DeclaredGroupCount = %d, want 3", s.DeclaredGroupCount())
	}
}

// TestApplyTeamLeadSpawned_RecordsSession — scenario B2
func TestApplyTeamLeadSpawned_RecordsSession(t *testing.T) {
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{
			Type:    contract.EventApplyBoardCreated,
			Payload: map[string]any{"board_id": "b1", "groups": float64(1)},
		}).
		ApplyEvent(domain.Event{
			Type: contract.EventApplyTeamLeadSpawned,
			Payload: map[string]any{
				"session_id": "s-lead-1",
				"group_id":   "g1",
			},
		})

	groups := s.Groups()
	if len(groups) != 1 || groups[0].ID != "g1" {
		t.Fatalf("expected group g1, got %+v", groups)
	}
	sess := groups[0].TeamLeadSession
	if sess.ID != "s-lead-1" {
		t.Errorf("TeamLeadSession.ID = %q, want %q", sess.ID, "s-lead-1")
	}
	if sess.Role != "team_lead" {
		t.Errorf("TeamLeadSession.Role = %q, want %q", sess.Role, "team_lead")
	}
}

// TestApplyTaskClaimed_MarksRunningWithImplementSession — scenario B3
func TestApplyTaskClaimed_MarksRunningWithImplementSession(t *testing.T) {
	// Pre-seed group and task so claimed knows where to put the task.
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{
			Type:    contract.EventApplyTeamLeadSpawned,
			Payload: map[string]any{"session_id": "s-lead", "group_id": "g1"},
		}).
		ApplyEvent(domain.Event{
			Type: contract.EventApplyTaskClaimed,
			Payload: map[string]any{
				"task_id":    "t1",
				"session_id": "s-impl-1",
			},
		})

	groups := s.Groups()
	if len(groups) == 0 {
		t.Fatal("expected at least one group")
	}
	// Find t1 across groups.
	var found *tui.ApplyTask
	for _, g := range groups {
		for i := range g.Tasks {
			if g.Tasks[i].ID == "t1" {
				cp := g.Tasks[i]
				found = &cp
			}
		}
	}
	if found == nil {
		t.Fatal("task t1 not found in state")
	}
	if found.Status != "running" {
		t.Errorf("task Status = %q, want running", found.Status)
	}
	if found.ImplementSession.ID != "s-impl-1" {
		t.Errorf("ImplementSession.ID = %q, want s-impl-1", found.ImplementSession.ID)
	}
	if found.ImplementSession.Role != "implement" {
		t.Errorf("ImplementSession.Role = %q, want implement", found.ImplementSession.Role)
	}
}

// TestApplyTaskRetry_SetsAttempts — scenario B4
func TestApplyTaskRetry_SetsAttempts(t *testing.T) {
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{
			Type:    contract.EventApplyTeamLeadSpawned,
			Payload: map[string]any{"session_id": "s-lead", "group_id": "g1"},
		}).
		ApplyEvent(domain.Event{
			Type:    contract.EventApplyTaskClaimed,
			Payload: map[string]any{"task_id": "t1", "session_id": "s1"},
		}).
		ApplyEvent(domain.Event{
			Type: contract.EventApplyTaskRetry,
			Payload: map[string]any{
				"task_id":  "t1",
				"attempts": float64(2),
			},
		})

	groups := s.Groups()
	var found *tui.ApplyTask
	for _, g := range groups {
		for i := range g.Tasks {
			if g.Tasks[i].ID == "t1" {
				cp := g.Tasks[i]
				found = &cp
			}
		}
	}
	if found == nil {
		t.Fatal("task t1 not found")
	}
	if found.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", found.Attempts)
	}
}

// TestApplyTaskEscalated_RecordsEscalationContext — scenario B5
func TestApplyTaskEscalated_RecordsEscalationContext(t *testing.T) {
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{
			Type:    contract.EventApplyTeamLeadSpawned,
			Payload: map[string]any{"session_id": "s-lead", "group_id": "g1"},
		}).
		ApplyEvent(domain.Event{
			Type:    contract.EventApplyTaskClaimed,
			Payload: map[string]any{"task_id": "t1", "session_id": "s1"},
		}).
		ApplyEvent(domain.Event{
			Type: contract.EventApplyTaskEscalated,
			Payload: map[string]any{
				"task_id":                "t1",
				"attempts":               float64(3),
				"reason":                 "blocked",
				"final_envelope_summary": "spec context blocked",
				"blocking_requirements":  []any{"R1"},
			},
		})

	groups := s.Groups()
	var found *tui.ApplyTask
	for _, g := range groups {
		for i := range g.Tasks {
			if g.Tasks[i].ID == "t1" {
				cp := g.Tasks[i]
				found = &cp
			}
		}
	}
	if found == nil {
		t.Fatal("task t1 not found")
	}
	if found.Status != "escalated" {
		t.Errorf("Status = %q, want escalated", found.Status)
	}
	if found.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", found.Attempts)
	}
	if found.EscalationReason != "blocked" {
		t.Errorf("EscalationReason = %q, want blocked", found.EscalationReason)
	}
	if found.FinalEnvelopeSummary != "spec context blocked" {
		t.Errorf("FinalEnvelopeSummary = %q", found.FinalEnvelopeSummary)
	}
	if len(found.BlockingRequirements) != 1 || found.BlockingRequirements[0] != "R1" {
		t.Errorf("BlockingRequirements = %v, want [R1]", found.BlockingRequirements)
	}
}

// TestApplyGroupCompleted_MarksDoneWithCount — scenario B6
func TestApplyGroupCompleted_MarksDoneWithCount(t *testing.T) {
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{
			Type:    contract.EventApplyTeamLeadSpawned,
			Payload: map[string]any{"session_id": "s-lead", "group_id": "g1"},
		}).
		ApplyEvent(domain.Event{
			Type: contract.EventApplyGroupCompleted,
			Payload: map[string]any{
				"group_id":   "g1",
				"tasks_done": float64(2),
			},
		})

	groups := s.Groups()
	if len(groups) != 1 || groups[0].ID != "g1" {
		t.Fatalf("expected group g1, got %+v", groups)
	}
	if groups[0].Status != "completed" {
		t.Errorf("Status = %q, want completed", groups[0].Status)
	}
	if groups[0].TasksDone != 2 {
		t.Errorf("TasksDone = %d, want 2", groups[0].TasksDone)
	}
}

// TestApplyGroupFailed_RecordsReason — scenario B7
func TestApplyGroupFailed_RecordsReason(t *testing.T) {
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{
			Type:    contract.EventApplyTeamLeadSpawned,
			Payload: map[string]any{"session_id": "s-lead", "group_id": "g1"},
		}).
		ApplyEvent(domain.Event{
			Type: contract.EventApplyGroupFailed,
			Payload: map[string]any{
				"group_id": "g1",
				"reason":   "dispatch timeout",
			},
		})

	groups := s.Groups()
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Status != "failed" {
		t.Errorf("Status = %q, want failed", groups[0].Status)
	}
	if groups[0].FailureReason != "dispatch timeout" {
		t.Errorf("FailureReason = %q, want dispatch timeout", groups[0].FailureReason)
	}
}

// TestApplyGroupDegraded_RecordsFailedDep — scenario B8
func TestApplyGroupDegraded_RecordsFailedDep(t *testing.T) {
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{
			Type: contract.EventApplyGroupDegraded,
			Payload: map[string]any{
				"group_id":       "g1",
				"failed_dep":     "g0",
				"failed_dep_err": "envelope blocked",
				"continued_run":  true,
			},
		})

	groups := s.Groups()
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g.Status != "degraded" {
		t.Errorf("Status = %q, want degraded", g.Status)
	}
	if g.FailedDep != "g0" {
		t.Errorf("FailedDep = %q, want g0", g.FailedDep)
	}
	if g.FailedDepErr != "envelope blocked" {
		t.Errorf("FailedDepErr = %q, want envelope blocked", g.FailedDepErr)
	}
	if !g.ContinuedRun {
		t.Error("ContinuedRun should be true")
	}
}

// TestApplyMaterializeStarted_RecordsTarget — scenario B9
func TestApplyMaterializeStarted_RecordsTarget(t *testing.T) {
	s := tui.NewApplyBoardState().ApplyEvent(domain.Event{
		Type: contract.EventApplyMaterializeStarted,
		Payload: map[string]any{
			"target_path": "/repo/x",
		},
	})

	if s.MaterializeTarget() != "/repo/x" {
		t.Errorf("MaterializeTarget = %q, want /repo/x", s.MaterializeTarget())
	}
	if s.MaterializeStatus() != "running" {
		t.Errorf("MaterializeStatus = %q, want running", s.MaterializeStatus())
	}
}

// TestApplyMaterializeCompleted_RecordsCount — scenario B10
func TestApplyMaterializeCompleted_RecordsCount(t *testing.T) {
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{
			Type:    contract.EventApplyMaterializeStarted,
			Payload: map[string]any{"target_path": "/repo/x"},
		}).
		ApplyEvent(domain.Event{
			Type: contract.EventApplyMaterializeCompleted,
			Payload: map[string]any{
				"target_path":         "/repo/x",
				"groups_materialized": float64(3),
			},
		})

	if s.MaterializeStatus() != "completed" {
		t.Errorf("MaterializeStatus = %q, want completed", s.MaterializeStatus())
	}
	if s.MaterializeGroupsCount() != 3 {
		t.Errorf("MaterializeGroupsCount = %d, want 3", s.MaterializeGroupsCount())
	}
}

// TestApplyMaterializeError_RecordsPerGroup — scenario B11
func TestApplyMaterializeError_RecordsPerGroup(t *testing.T) {
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{
			Type:    contract.EventApplyTeamLeadSpawned,
			Payload: map[string]any{"session_id": "s-lead", "group_id": "g1"},
		}).
		ApplyEvent(domain.Event{
			Type: contract.EventApplyMaterializeError,
			Payload: map[string]any{
				"group_id": "g1",
				"err":      "fs locked",
			},
		})

	groups := s.Groups()
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].MaterializeErr != "fs locked" {
		t.Errorf("MaterializeErr = %q, want fs locked", groups[0].MaterializeErr)
	}
	if s.MaterializeStatus() != "error" {
		t.Errorf("board MaterializeStatus = %q, want error", s.MaterializeStatus())
	}
}

// TestErrorFamily_MarksTaskError — scenario B12
// Table test over all 4 error-family events.
func TestErrorFamily_MarksTaskError(t *testing.T) {
	errorEvents := []struct {
		name      string
		eventType string
	}{
		{"ImplementSpawnFailed", contract.EventApplyImplementSpawnFailed},
		{"ImplementSpawnGovernorError", contract.EventApplyImplementSpawnGovernorError},
		{"DispatchError", contract.EventApplyDispatchError},
		{"EnvelopeValidationFailed", contract.EventApplyEnvelopeValidationFailed},
	}

	for _, tt := range errorEvents {
		t.Run(tt.name, func(t *testing.T) {
			taskID := "t-" + tt.name
			s := tui.NewApplyBoardState().ApplyEvent(domain.Event{
				Type: tt.eventType,
				Payload: map[string]any{
					"task_id": taskID,
					"err":     "something went wrong",
				},
			})

			groups := s.Groups()
			var found *tui.ApplyTask
			for _, g := range groups {
				for i := range g.Tasks {
					if g.Tasks[i].ID == taskID {
						cp := g.Tasks[i]
						found = &cp
					}
				}
			}
			if found == nil {
				t.Fatalf("[%s] task %s not found in state", tt.name, taskID)
			}
			if found.Status != "error" {
				t.Errorf("[%s] Status = %q, want error", tt.name, found.Status)
			}
			if found.LastError != "something went wrong" {
				t.Errorf("[%s] LastError = %q, want 'something went wrong'", tt.name, found.LastError)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// C-group: defensive out-of-order handling (C1-C4)
// ---------------------------------------------------------------------------

// TestDefensive_TaskClaimedBeforeBoardCreated — scenario C1
// apply.task.claimed arrives with no prior apply.board.created.
func TestDefensive_TaskClaimedBeforeBoardCreated(t *testing.T) {
	s := tui.NewApplyBoardState()
	// Must not panic.
	s2 := s.ApplyEvent(domain.Event{
		Type: contract.EventApplyTaskClaimed,
		Payload: map[string]any{
			"task_id":    "t1",
			"session_id": "s1",
		},
	})

	groups := s2.Groups()
	if len(groups) == 0 {
		t.Fatal("task should be recorded under a defensive group")
	}
	var found *tui.ApplyTask
	for _, g := range groups {
		for i := range g.Tasks {
			if g.Tasks[i].ID == "t1" {
				cp := g.Tasks[i]
				found = &cp
			}
		}
	}
	if found == nil {
		t.Fatal("task t1 not found in state after out-of-order claimed")
	}
	if found.Status != "running" {
		t.Errorf("Status = %q, want running", found.Status)
	}
}

// TestDefensive_UnknownGroupCreatedOnTheFly — scenario C2
// apply.group.completed for a group id that was never seen before.
func TestDefensive_UnknownGroupCreatedOnTheFly(t *testing.T) {
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{
			Type:    contract.EventApplyTeamLeadSpawned,
			Payload: map[string]any{"session_id": "s-lead", "group_id": "g1"},
		}).
		ApplyEvent(domain.Event{
			Type: contract.EventApplyGroupCompleted,
			Payload: map[string]any{
				"group_id":   "g99",
				"tasks_done": float64(0),
			},
		})

	groups := s.Groups()
	found := false
	for _, g := range groups {
		if g.ID == "g99" && g.Status == "completed" {
			found = true
		}
	}
	if !found {
		t.Errorf("group g99 should have been created on-the-fly and marked completed; groups = %+v", groups)
	}
}

// TestMaxGroupsCap_EvictsOldest — scenario C3
func TestMaxGroupsCap_EvictsOldest(t *testing.T) {
	s := tui.NewApplyBoardState()
	for i := 0; i <= tui.MaxApplyBoardGroups; i++ {
		groupID := fmt.Sprintf("g%03d", i)
		s = s.ApplyEvent(domain.Event{
			Type: contract.EventApplyTeamLeadSpawned,
			Payload: map[string]any{
				"session_id": "s-lead",
				"group_id":   groupID,
			},
		})
	}

	if got := s.GroupCount(); got != tui.MaxApplyBoardGroups {
		t.Errorf("GroupCount = %d, want %d", got, tui.MaxApplyBoardGroups)
	}
	// Oldest group (g000) should have been evicted.
	for _, g := range s.Groups() {
		if g.ID == "g000" {
			t.Errorf("oldest group g000 should have been evicted but still present")
		}
	}
	// Newest group should still be present.
	newest := fmt.Sprintf("g%03d", tui.MaxApplyBoardGroups)
	found := false
	for _, g := range s.Groups() {
		if g.ID == newest {
			found = true
		}
	}
	if !found {
		t.Errorf("newest group %s should still be present after LRU eviction", newest)
	}
}

// TestImmutability_ReceiverUnchangedAfterApplyEvent — scenario C4
func TestImmutability_ReceiverUnchangedAfterApplyEvent(t *testing.T) {
	s0 := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{
			Type:    contract.EventApplyTeamLeadSpawned,
			Payload: map[string]any{"session_id": "s-lead", "group_id": "g1"},
		})

	originalCount := s0.GroupCount()

	s1 := s0.ApplyEvent(domain.Event{
		Type: contract.EventApplyGroupCompleted,
		Payload: map[string]any{
			"group_id":   "g2",
			"tasks_done": float64(1),
		},
	})

	// s0 must be unchanged.
	if s0.GroupCount() != originalCount {
		t.Errorf("receiver mutated: GroupCount went from %d to %d", originalCount, s0.GroupCount())
	}

	// s1 must reflect the new group.
	if s1.GroupCount() != originalCount+1 {
		t.Errorf("s1 GroupCount = %d, want %d", s1.GroupCount(), originalCount+1)
	}

	// Verify the original group's state is intact in s0.
	s0Groups := s0.Groups()
	if len(s0Groups) != originalCount {
		t.Errorf("s0.Groups() len = %d, want %d", len(s0Groups), originalCount)
	}
	for _, g := range s0Groups {
		if g.ID == "g2" {
			t.Error("s0 should not contain g2 which was added to s1")
		}
	}
}

// TestNewApplyBoardStateIsEmpty confirms fresh state is zero-valued.
func TestNewApplyBoardStateIsEmpty(t *testing.T) {
	s := tui.NewApplyBoardState()
	if got := s.GroupCount(); got != 0 {
		t.Errorf("fresh GroupCount = %d, want 0", got)
	}
	if got := s.Groups(); len(got) != 0 {
		t.Errorf("fresh Groups() = %v, want []", got)
	}
	if got := s.BoardID(); got != "" {
		t.Errorf("fresh BoardID = %q, want empty", got)
	}
	if got := s.MaterializeStatus(); got != "" {
		t.Errorf("fresh MaterializeStatus = %q, want empty", got)
	}
}
