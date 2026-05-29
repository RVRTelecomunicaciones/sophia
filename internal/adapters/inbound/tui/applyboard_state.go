package tui

import (
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/pkg/contract"
)

// MaxApplyBoardGroups bounds the in-memory group count (RM7-07). When the
// limit is exceeded, the OLDEST group (by insertion order) is evicted.
const MaxApplyBoardGroups = 50

// ApplySession is a TUI-internal representation of a spawned apply session
// (team-lead or implement).
type ApplySession struct {
	ID   string
	Role string // "team_lead" | "implement"
}

// ApplyTask is a TUI-internal representation of a task within a group.
type ApplyTask struct {
	ID                   string
	Status               string // "pending" | "running" | "escalated" | "error" | "done"
	Attempts             int
	LastError            string
	EscalationReason     string
	FinalEnvelopeSummary string
	BlockingRequirements []string
	ImplementSession     ApplySession
}

// ApplyGroup is a TUI-internal representation of a task group.
type ApplyGroup struct {
	ID              string
	Status          string // "pending" | "running" | "completed" | "failed" | "degraded"
	FailureReason   string
	FailedDep       string
	FailedDepErr    string
	ContinuedRun    bool
	TasksDone       int
	MaterializeErr  string
	TeamLeadSession ApplySession
	Tasks           []ApplyTask
}

// ApplyBoardState holds TUI-internal state derived from apply.* events.
// It is immutable: every mutation returns a new value. Groups are stored in
// insertion order and capped at MaxApplyBoardGroups via LRU eviction of the
// oldest entry.
type ApplyBoardState struct {
	boardID                string
	declaredGroupCount     int
	materializeTarget      string
	materializeStatus      string // "" | "running" | "completed" | "error"
	materializeGroupsCount int
	groups                 []ApplyGroup
}

// NewApplyBoardState returns an empty ApplyBoardState.
func NewApplyBoardState() ApplyBoardState {
	return ApplyBoardState{}
}

// BoardID returns the apply board id recorded from apply.board.created.
func (s ApplyBoardState) BoardID() string { return s.boardID }

// DeclaredGroupCount returns the group count declared at apply.board.created.
func (s ApplyBoardState) DeclaredGroupCount() int { return s.declaredGroupCount }

// GroupCount returns the number of groups currently tracked.
func (s ApplyBoardState) GroupCount() int { return len(s.groups) }

// Groups returns a deep copy of all groups in insertion order.
func (s ApplyBoardState) Groups() []ApplyGroup {
	out := make([]ApplyGroup, len(s.groups))
	for i, g := range s.groups {
		out[i] = cloneApplyGroup(g)
	}
	return out
}

// MaterializeTarget returns the materialization target path.
func (s ApplyBoardState) MaterializeTarget() string { return s.materializeTarget }

// MaterializeStatus returns the materialization status ("" | "running" | "completed" | "error").
func (s ApplyBoardState) MaterializeStatus() string { return s.materializeStatus }

// MaterializeGroupsCount returns the number of groups materialized.
func (s ApplyBoardState) MaterializeGroupsCount() int { return s.materializeGroupsCount }

// ApplyEvent processes a domain.Event and returns a new ApplyBoardState
// reflecting the change. The receiver is never mutated.
//
// All cases use contract.EventApply* constants — never raw string literals.
// Handlers are defensive: unknown group/task ids are created on-the-fly, and
// events that do not carry a group_id fall into a synthetic "<unknown>" group
// so no event is silently dropped.
func (s ApplyBoardState) ApplyEvent(ev domain.Event) ApplyBoardState {
	switch ev.Type {
	case contract.EventApplyBoardCreated:
		return s.applyBoardCreated(ev)
	case contract.EventApplyTeamLeadSpawned:
		return s.applyTeamLeadSpawned(ev)
	case contract.EventApplyTaskClaimed:
		return s.applyTaskClaimed(ev)
	case contract.EventApplyTaskClaimSkipped:
		return s.applyTaskClaimSkipped(ev)
	case contract.EventApplyTaskRetry:
		return s.applyTaskRetry(ev)
	case contract.EventApplyTaskEscalated:
		return s.applyTaskEscalated(ev)
	case contract.EventApplyGroupCompleted:
		return s.applyGroupCompleted(ev)
	case contract.EventApplyGroupFailed:
		return s.applyGroupFailed(ev)
	case contract.EventApplyGroupDegraded:
		return s.applyGroupDegraded(ev)
	case contract.EventApplyImplementSpawnFailed:
		return s.applyTaskError(ev)
	case contract.EventApplyImplementSpawnGovernorError:
		return s.applyTaskError(ev)
	case contract.EventApplyDispatchError:
		return s.applyTaskError(ev)
	case contract.EventApplyEnvelopeValidationFailed:
		return s.applyTaskError(ev)
	case contract.EventApplyMaterializeStarted:
		return s.applyMaterializeStarted(ev)
	case contract.EventApplyMaterializeCompleted:
		return s.applyMaterializeCompleted(ev)
	case contract.EventApplyMaterializeError:
		return s.applyMaterializeError(ev)
	default:
		return s
	}
}

// --- handlers ----------------------------------------------------------

func (s ApplyBoardState) applyBoardCreated(ev domain.Event) ApplyBoardState {
	boardID, _ := ev.Payload["board_id"].(string)
	groups, _ := ev.Payload["groups"].(int)
	// JSON numbers unmarshal as float64 in map[string]any.
	if groups == 0 {
		if f, ok := ev.Payload["groups"].(float64); ok {
			groups = int(f)
		}
	}
	return ApplyBoardState{
		boardID:            boardID,
		declaredGroupCount: groups,
		groups:             applyGroupsCap(s.cloneGroups()),
		materializeTarget:  s.materializeTarget,
		materializeStatus:  s.materializeStatus,
	}
}

func (s ApplyBoardState) applyTeamLeadSpawned(ev domain.Event) ApplyBoardState {
	sessionID, _ := ev.Payload["session_id"].(string)
	groupID, _ := ev.Payload["group_id"].(string)
	if groupID == "" {
		groupID = "<unknown>"
	}
	out := s.cloneGroups()
	gi := ensureApplyGroup(&out, groupID)
	out[gi].TeamLeadSession = ApplySession{ID: sessionID, Role: "team_lead"}
	return s.withGroups(out)
}

func (s ApplyBoardState) applyTaskClaimed(ev domain.Event) ApplyBoardState {
	taskID, _ := ev.Payload["task_id"].(string)
	sessionID, _ := ev.Payload["session_id"].(string)
	// apply.task.claimed carries NO group_id — attach to last-known group or <unknown>.
	out := s.cloneGroups()
	gi, ti := findTask(out, taskID)
	if gi < 0 {
		// Task not seen yet — create under <unknown> group.
		gi = ensureApplyGroup(&out, "<unknown>")
		ti = ensureApplyTask(&out[gi], taskID)
	}
	out[gi].Tasks[ti].Status = "running"
	out[gi].Tasks[ti].ImplementSession = ApplySession{ID: sessionID, Role: "implement"}
	return s.withGroups(out)
}

func (s ApplyBoardState) applyTaskClaimSkipped(ev domain.Event) ApplyBoardState {
	taskID, _ := ev.Payload["task_id"].(string)
	errMsg, _ := ev.Payload["err"].(string)
	if taskID == "" {
		return s
	}
	out := s.cloneGroups()
	gi, ti := findTask(out, taskID)
	if gi < 0 {
		gi = ensureApplyGroup(&out, "<unknown>")
		ti = ensureApplyTask(&out[gi], taskID)
	}
	out[gi].Tasks[ti].LastError = errMsg
	return s.withGroups(out)
}

func (s ApplyBoardState) applyTaskRetry(ev domain.Event) ApplyBoardState {
	taskID, _ := ev.Payload["task_id"].(string)
	attempts := toInt(ev.Payload["attempts"])
	if taskID == "" {
		return s
	}
	out := s.cloneGroups()
	gi, ti := findTask(out, taskID)
	if gi < 0 {
		gi = ensureApplyGroup(&out, "<unknown>")
		ti = ensureApplyTask(&out[gi], taskID)
	}
	out[gi].Tasks[ti].Attempts = attempts
	return s.withGroups(out)
}

func (s ApplyBoardState) applyTaskEscalated(ev domain.Event) ApplyBoardState {
	taskID, _ := ev.Payload["task_id"].(string)
	attempts := toInt(ev.Payload["attempts"])
	reason, _ := ev.Payload["reason"].(string)
	finalSummary, _ := ev.Payload["final_envelope_summary"].(string)
	var blocking []string
	if raw, ok := ev.Payload["blocking_requirements"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				blocking = append(blocking, s)
			}
		}
	}
	if taskID == "" {
		return s
	}
	out := s.cloneGroups()
	gi, ti := findTask(out, taskID)
	if gi < 0 {
		gi = ensureApplyGroup(&out, "<unknown>")
		ti = ensureApplyTask(&out[gi], taskID)
	}
	out[gi].Tasks[ti].Status = "escalated"
	out[gi].Tasks[ti].Attempts = attempts
	out[gi].Tasks[ti].EscalationReason = reason
	out[gi].Tasks[ti].FinalEnvelopeSummary = finalSummary
	out[gi].Tasks[ti].BlockingRequirements = blocking
	return s.withGroups(out)
}

func (s ApplyBoardState) applyGroupCompleted(ev domain.Event) ApplyBoardState {
	groupID, _ := ev.Payload["group_id"].(string)
	tasksDone := toInt(ev.Payload["tasks_done"])
	if groupID == "" {
		groupID = "<unknown>"
	}
	out := s.cloneGroups()
	gi := ensureApplyGroup(&out, groupID)
	out[gi].Status = "completed"
	out[gi].TasksDone = tasksDone
	return s.withGroups(out)
}

func (s ApplyBoardState) applyGroupFailed(ev domain.Event) ApplyBoardState {
	groupID, _ := ev.Payload["group_id"].(string)
	reason, _ := ev.Payload["reason"].(string)
	if groupID == "" {
		groupID = "<unknown>"
	}
	out := s.cloneGroups()
	gi := ensureApplyGroup(&out, groupID)
	out[gi].Status = "failed"
	out[gi].FailureReason = reason
	return s.withGroups(out)
}

func (s ApplyBoardState) applyGroupDegraded(ev domain.Event) ApplyBoardState {
	groupID, _ := ev.Payload["group_id"].(string)
	failedDep, _ := ev.Payload["failed_dep"].(string)
	failedDepErr, _ := ev.Payload["failed_dep_err"].(string)
	continuedRun, _ := ev.Payload["continued_run"].(bool)
	if groupID == "" {
		groupID = "<unknown>"
	}
	out := s.cloneGroups()
	gi := ensureApplyGroup(&out, groupID)
	out[gi].Status = "degraded"
	out[gi].FailedDep = failedDep
	out[gi].FailedDepErr = failedDepErr
	out[gi].ContinuedRun = continuedRun
	return s.withGroups(out)
}

// applyTaskError is the shared handler for the 4 error-family events:
//
//	contract.EventApplyImplementSpawnFailed
//	contract.EventApplyImplementSpawnGovernorError
//	contract.EventApplyDispatchError
//	contract.EventApplyEnvelopeValidationFailed
//
// All carry {task_id, err}. Task status converges to "error".
func (s ApplyBoardState) applyTaskError(ev domain.Event) ApplyBoardState {
	taskID, _ := ev.Payload["task_id"].(string)
	errMsg, _ := ev.Payload["err"].(string)
	if taskID == "" {
		return s
	}
	out := s.cloneGroups()
	gi, ti := findTask(out, taskID)
	if gi < 0 {
		gi = ensureApplyGroup(&out, "<unknown>")
		ti = ensureApplyTask(&out[gi], taskID)
	}
	out[gi].Tasks[ti].Status = "error"
	out[gi].Tasks[ti].LastError = errMsg
	return s.withGroups(out)
}

func (s ApplyBoardState) applyMaterializeStarted(ev domain.Event) ApplyBoardState {
	targetPath, _ := ev.Payload["target_path"].(string)
	next := s.withGroups(s.cloneGroups())
	next.materializeTarget = targetPath
	next.materializeStatus = "running"
	return next
}

func (s ApplyBoardState) applyMaterializeCompleted(ev domain.Event) ApplyBoardState {
	targetPath, _ := ev.Payload["target_path"].(string)
	count := toInt(ev.Payload["groups_materialized"])
	next := s.withGroups(s.cloneGroups())
	next.materializeTarget = targetPath
	next.materializeStatus = "completed"
	next.materializeGroupsCount = count
	return next
}

func (s ApplyBoardState) applyMaterializeError(ev domain.Event) ApplyBoardState {
	groupID, _ := ev.Payload["group_id"].(string)
	errMsg, _ := ev.Payload["err"].(string)
	if groupID == "" {
		groupID = "<unknown>"
	}
	out := s.cloneGroups()
	gi := ensureApplyGroup(&out, groupID)
	out[gi].MaterializeErr = errMsg
	next := s.withGroups(out)
	next.materializeStatus = "error"
	return next
}

// --- defensive helpers -------------------------------------------------

// ensureApplyGroup returns the index of the group with the given ID,
// creating it (appended at the end) if not found. A blank id is normalized
// to "<unknown>".
func ensureApplyGroup(groups *[]ApplyGroup, id string) int {
	if id == "" {
		id = "<unknown>"
	}
	for i, g := range *groups {
		if g.ID == id {
			return i
		}
	}
	*groups = append(*groups, ApplyGroup{ID: id})
	return len(*groups) - 1
}

// ensureApplyTask returns the index of the task with the given ID inside a
// group, creating it (Status = "pending") if not found.
func ensureApplyTask(group *ApplyGroup, taskID string) int {
	for i, t := range group.Tasks {
		if t.ID == taskID {
			return i
		}
	}
	group.Tasks = append(group.Tasks, ApplyTask{ID: taskID, Status: "pending"})
	return len(group.Tasks) - 1
}

// findTask searches all groups for taskID and returns (groupIndex, taskIndex).
// Returns (-1, -1) when not found.
func findTask(groups []ApplyGroup, taskID string) (int, int) {
	for gi := range groups {
		for ti := range groups[gi].Tasks {
			if groups[gi].Tasks[ti].ID == taskID {
				return gi, ti
			}
		}
	}
	return -1, -1
}

// --- immutability helpers -----------------------------------------------

// cloneGroups returns a deep copy of the receiver's group slice.
func (s ApplyBoardState) cloneGroups() []ApplyGroup {
	out := make([]ApplyGroup, len(s.groups))
	for i, g := range s.groups {
		out[i] = cloneApplyGroup(g)
	}
	return out
}

// cloneApplyGroup deep-copies a group including its Task and BlockingRequirements slices.
func cloneApplyGroup(g ApplyGroup) ApplyGroup {
	cp := g
	cp.Tasks = make([]ApplyTask, len(g.Tasks))
	for i, t := range g.Tasks {
		ct := t
		if t.BlockingRequirements != nil {
			ct.BlockingRequirements = append([]string(nil), t.BlockingRequirements...)
		}
		cp.Tasks[i] = ct
	}
	return cp
}

// withGroups returns a new ApplyBoardState with the same scalar fields but
// the given groups slice (after LRU cap). It is the canonical way to produce
// a mutated value from a handler.
func (s ApplyBoardState) withGroups(groups []ApplyGroup) ApplyBoardState {
	return ApplyBoardState{
		boardID:                s.boardID,
		declaredGroupCount:     s.declaredGroupCount,
		materializeTarget:      s.materializeTarget,
		materializeStatus:      s.materializeStatus,
		materializeGroupsCount: s.materializeGroupsCount,
		groups:                 applyGroupsCap(groups),
	}
}

// applyGroupsCap enforces the MaxApplyBoardGroups limit by evicting the oldest
// groups (insertion-order LRU). A copy is returned to avoid holding the full
// backing array when eviction occurs.
func applyGroupsCap(groups []ApplyGroup) []ApplyGroup {
	if len(groups) <= MaxApplyBoardGroups {
		return groups
	}
	excess := len(groups) - MaxApplyBoardGroups
	trimmed := groups[excess:]
	result := make([]ApplyGroup, len(trimmed))
	copy(result, trimmed)
	return result
}

// --- utility -----------------------------------------------------------

// toInt coerces an any value to int, supporting both int and float64
// (JSON numbers unmarshal as float64 in map[string]any). Returns 0 for
// unsupported types.
func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	}
	return 0
}
