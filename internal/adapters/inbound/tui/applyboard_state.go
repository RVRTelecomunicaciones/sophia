package tui

import (
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// MaxApplyBoardGroups bounds the in-memory group count (RM7-07). When the
// limit is exceeded, the OLDEST group (by insertion order) is evicted.
const MaxApplyBoardGroups = 50

// ApplyAgent is a TUI-internal representation of a spawned agent.
type ApplyAgent struct {
	ID     string
	Role   string
	Status string
}

// ApplyTask is a TUI-internal representation of a task within a group.
type ApplyTask struct {
	ID           string
	GroupID      string
	FilesPattern string
	Status       string
	Agents       []ApplyAgent
}

// ApplyGroup is a TUI-internal representation of a task group.
type ApplyGroup struct {
	ID     string
	Tasks  []ApplyTask
	Agents []ApplyAgent // group-level agents (agent.spawned with no task_id)
}

// ApplyBoardState holds TUI-internal state derived from task.* and agent.*
// events. It is immutable: every mutation returns a new value. Groups are
// stored in insertion order and capped at MaxApplyBoardGroups via LRU eviction
// of the oldest entry.
type ApplyBoardState struct {
	groups []ApplyGroup
}

// NewApplyBoardState returns an empty ApplyBoardState.
func NewApplyBoardState() ApplyBoardState {
	return ApplyBoardState{}
}

// GroupCount returns the number of groups currently tracked.
func (s ApplyBoardState) GroupCount() int { return len(s.groups) }

// Groups returns a deep copy of all groups in insertion order.
func (s ApplyBoardState) Groups() []ApplyGroup {
	out := make([]ApplyGroup, len(s.groups))
	for i, g := range s.groups {
		out[i] = cloneGroup(g)
	}
	return out
}

// ApplyEvent processes a domain.Event and returns a new ApplyBoardState
// reflecting the change. The receiver is never mutated.
func (s ApplyBoardState) ApplyEvent(ev domain.Event) ApplyBoardState {
	switch ev.Type {
	case "task.started":
		return s.applyTaskStarted(ev)
	case "task.completed":
		return s.applyTaskCompleted(ev)
	case "agent.spawned":
		return s.applyAgentSpawned(ev)
	case "agent.completed":
		return s.applyAgentCompleted(ev)
	default:
		return s
	}
}

func (s ApplyBoardState) applyTaskStarted(ev domain.Event) ApplyBoardState {
	groupID, _ := ev.Payload["group_id"].(string)
	taskID, _ := ev.Payload["task_id"].(string)
	files, _ := ev.Payload["files_pattern"].(string)
	if groupID == "" || taskID == "" {
		return s
	}
	out := s.cloneGroups()
	gi := ensureGroup(&out, groupID)
	ti := ensureTask(&out[gi], taskID, groupID)
	out[gi].Tasks[ti].FilesPattern = files
	out[gi].Tasks[ti].Status = "running"
	return ApplyBoardState{groups: applyGroupsCap(out)}
}

func (s ApplyBoardState) applyTaskCompleted(ev domain.Event) ApplyBoardState {
	groupID, _ := ev.Payload["group_id"].(string)
	taskID, _ := ev.Payload["task_id"].(string)
	status, _ := ev.Payload["status"].(string)
	if groupID == "" || taskID == "" {
		return s
	}
	out := s.cloneGroups()
	gi := ensureGroup(&out, groupID)
	ti := ensureTask(&out[gi], taskID, groupID)
	if status == "" {
		status = "done"
	}
	out[gi].Tasks[ti].Status = status
	return ApplyBoardState{groups: applyGroupsCap(out)}
}

func (s ApplyBoardState) applyAgentSpawned(ev domain.Event) ApplyBoardState {
	agentID, _ := ev.Payload["agent_id"].(string)
	role, _ := ev.Payload["agent_role"].(string)
	groupID, _ := ev.Payload["group_id"].(string)
	taskID, _ := ev.Payload["task_id"].(string)
	if agentID == "" || groupID == "" {
		return s
	}
	out := s.cloneGroups()
	gi := ensureGroup(&out, groupID)
	agent := ApplyAgent{ID: agentID, Role: role, Status: "running"}
	if taskID == "" {
		out[gi].Agents = append(out[gi].Agents, agent)
	} else {
		ti := ensureTask(&out[gi], taskID, groupID)
		out[gi].Tasks[ti].Agents = append(out[gi].Tasks[ti].Agents, agent)
	}
	return ApplyBoardState{groups: applyGroupsCap(out)}
}

func (s ApplyBoardState) applyAgentCompleted(ev domain.Event) ApplyBoardState {
	agentID, _ := ev.Payload["agent_id"].(string)
	status, _ := ev.Payload["status"].(string)
	if agentID == "" {
		return s
	}
	if status == "" {
		status = "done"
	}
	out := s.cloneGroups()
	for gi := range out {
		for ti := range out[gi].Tasks {
			for ai := range out[gi].Tasks[ti].Agents {
				if out[gi].Tasks[ti].Agents[ai].ID == agentID {
					out[gi].Tasks[ti].Agents[ai].Status = status
				}
			}
		}
		for ai := range out[gi].Agents {
			if out[gi].Agents[ai].ID == agentID {
				out[gi].Agents[ai].Status = status
			}
		}
	}
	return ApplyBoardState{groups: out}
}

// cloneGroups returns a deep copy of the receiver's group slice.
// All mutation methods operate on this clone to preserve immutability.
func (s ApplyBoardState) cloneGroups() []ApplyGroup {
	out := make([]ApplyGroup, len(s.groups))
	for i, g := range s.groups {
		out[i] = cloneGroup(g)
	}
	return out
}

// cloneGroup deep-copies a group including its Task and Agent slices.
func cloneGroup(g ApplyGroup) ApplyGroup {
	cp := ApplyGroup{ID: g.ID}
	cp.Tasks = make([]ApplyTask, len(g.Tasks))
	for i, t := range g.Tasks {
		ct := t
		ct.Agents = append([]ApplyAgent(nil), t.Agents...)
		cp.Tasks[i] = ct
	}
	cp.Agents = append([]ApplyAgent(nil), g.Agents...)
	return cp
}

// ensureGroup returns the index of the group with the given ID, creating it
// (appended at the end) if not found.
func ensureGroup(groups *[]ApplyGroup, id string) int {
	for i, g := range *groups {
		if g.ID == id {
			return i
		}
	}
	*groups = append(*groups, ApplyGroup{ID: id})
	return len(*groups) - 1
}

// ensureTask returns the index of the task with the given ID inside a group,
// creating it (appended at the end) if not found.
func ensureTask(group *ApplyGroup, taskID, groupID string) int {
	for i, t := range group.Tasks {
		if t.ID == taskID {
			return i
		}
	}
	group.Tasks = append(group.Tasks, ApplyTask{ID: taskID, GroupID: groupID, Status: "pending"})
	return len(group.Tasks) - 1
}

// applyGroupsCap enforces the MaxApplyBoardGroups limit by evicting the oldest
// groups (insertion-order LRU). groups[excess:] creates a new slice header
// pointing into the same backing array; the evicted entries simply fall out of
// scope and are collected by the GC when no other reference holds them.
func applyGroupsCap(groups []ApplyGroup) []ApplyGroup {
	if len(groups) <= MaxApplyBoardGroups {
		return groups
	}
	excess := len(groups) - MaxApplyBoardGroups
	// Re-slice to drop oldest entries; copy to avoid holding the full backing array.
	trimmed := groups[excess:]
	result := make([]ApplyGroup, len(trimmed))
	copy(result, trimmed)
	return result
}
