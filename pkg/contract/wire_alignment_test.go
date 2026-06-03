package contract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// cliEventsSourcePath is the CLI contract events file, relative to this package.
const cliEventsSourcePath = "events.go"

// applyBoardStatePath is the TUI ApplyBoardState implementation, relative to
// this package (sophia-cli/pkg/contract/).
const applyBoardStatePath = "../../internal/adapters/inbound/tui/applyboard_state.go"

// parseCLIEventConstants parses events.go in this package and returns a map of
// constName -> stringValue for every Event* constant.
func parseCLIEventConstants(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, cliEventsSourcePath, nil, 0)
	if err != nil {
		t.Fatalf("parse CLI events source: %v", err)
	}
	out := map[string]string{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, sp := range gd.Specs {
			vs, ok := sp.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Event") {
					continue
				}
				if i >= len(vs.Values) {
					continue
				}
				bl, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				v, err := strconv.Unquote(bl.Value)
				if err != nil {
					t.Fatalf("strconv.Unquote(%q): %v", bl.Value, err)
				}
				out[name.Name] = v
			}
		}
	}
	return out
}

// parseApplyBoardCases parses applyboard_state.go and returns the set of
// string values covered by case clauses in the ApplyEvent switch. It resolves
// contract.EventApply* selectors using cliConstants (constName → stringValue).
func parseApplyBoardCases(t *testing.T, cliConstants map[string]string) map[string]struct{} {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, applyBoardStatePath, nil, 0)
	if err != nil {
		t.Fatalf("parse applyboard_state.go: %v", err)
	}

	covered := map[string]struct{}{}

	// Walk the AST to find the ApplyEvent method and its switch statement.
	ast.Inspect(f, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}
		// Match the ApplyEvent method (receiver + name).
		if fd.Name.Name != "ApplyEvent" {
			return true
		}
		if fd.Recv == nil || len(fd.Recv.List) == 0 {
			return true
		}

		// Walk the body for SwitchStmt.
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			for _, stmt := range sw.Body.List {
				cc, ok := stmt.(*ast.CaseClause)
				if !ok || cc.List == nil {
					continue // default case
				}
				for _, expr := range cc.List {
					sel, ok := expr.(*ast.SelectorExpr)
					if !ok {
						continue
					}
					constName := sel.Sel.Name
					if v, ok := cliConstants[constName]; ok {
						covered[v] = struct{}{}
					}
				}
			}
			return false // found the switch — stop descending
		})
		return false // found ApplyEvent — stop ascending
	})

	return covered
}

// orchSourcePath is the path to the orchestrator's event-name constants file,
// relative to this package's directory (sophia-cli/pkg/contract/).
// Path: ../../../ = sophia-cli/ → 2026/ workspace root → sophia-orchestator repo.
// Requires both repos checked out side-by-side under a common workspace root.
const orchSourcePath = "../../../sophia-orchestator/internal/ports/inbound/event_types.go"

// parseOrchEvents walks the AST of the orch event_types.go and returns a map
// of const name -> string value for every constant whose name starts with "Event".
// Returns nil, skip-reason-string if the file cannot be read or parsed.
func parseOrchEvents(t *testing.T) (map[string]string, string) {
	t.Helper()
	if _, err := os.Stat(orchSourcePath); err != nil {
		return nil, "orch source not resolvable: " + err.Error()
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, orchSourcePath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse orch source: %v", err)
	}
	orchEvents := map[string]string{} // constName -> stringValue
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, sp := range gd.Specs {
			vs, ok := sp.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Event") {
					continue
				}
				if i >= len(vs.Values) {
					continue
				}
				bl, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				v, err := strconv.Unquote(bl.Value)
				if err != nil {
					t.Fatalf("strconv.Unquote(%q): %v", bl.Value, err)
				}
				orchEvents[name.Name] = v
			}
		}
	}
	return orchEvents, ""
}

// TestWireAlignment_OrchEventsMirrored (E1, E2, E3, E4) verifies:
//   - E1: every Event* constant in the orch source is extracted via AST
//   - E2: every extracted orch event string exists in CLI knownEvents
//   - E3: every CLI-only key in knownEvents belongs to the documented allowlist
//   - E4: the test skips cleanly when the orch source is unreachable
func TestWireAlignment_OrchEventsMirrored(t *testing.T) {
	orchEvents, skipReason := parseOrchEvents(t)
	if orchEvents == nil {
		t.Skipf("cross-repo drift check skipped: %s", skipReason)
	}

	// E1: sanity — the parser must extract at least one event.
	if len(orchEvents) == 0 {
		t.Fatal("no Event* constants extracted from orch source; check orchSourcePath")
	}

	// E2: every orch event string must be in CLI knownEvents.
	for orchName, orchValue := range orchEvents {
		if _, ok := knownEvents[orchValue]; !ok {
			t.Errorf("drift: orch declares %s = %q but CLI knownEvents does NOT contain %q",
				orchName, orchValue, orchValue)
		}
	}

	// E3: every CLI knownEvents key must be either in the orch set OR in the
	// documented CLI-only allowlist. Unknown additions are drift in the other
	// direction.
	//
	// CLI-only allowlist: SSE protocol events that the orch never emits.
	// Any new CLI-only event must be explicitly added here with a comment.
	cliOnlyAllowlist := map[string]struct{}{
		"heartbeat": {}, // SSE keepalive — orch never emits, CLI reads
		"open":      {}, // SSE connection-live signal — orch never emits, CLI reads
	}
	orchValueSet := map[string]struct{}{}
	for _, v := range orchEvents {
		orchValueSet[v] = struct{}{}
	}
	for k := range knownEvents {
		if _, inOrch := orchValueSet[k]; inOrch {
			continue
		}
		if _, inAllowlist := cliOnlyAllowlist[k]; inAllowlist {
			continue
		}
		t.Errorf("CLI knownEvents has key %q that is neither in orch event set nor in the documented CLI-only allowlist", k)
	}
}

// TestKnownEvents_IsPackageMap (B1, B2) verifies at runtime that knownEvents
// is a non-empty map containing every mirrored + CLI-only event constant.
func TestKnownEvents_IsPackageMap(t *testing.T) {
	// B1: type assertion — knownEvents must be map[string]struct{}.
	got := reflect.TypeOf(knownEvents)
	want := reflect.TypeOf(map[string]struct{}{})
	if got != want {
		t.Fatalf("knownEvents type = %v, want %v", got, want)
	}

	// B2: knownEvents must contain every Mirrored + CLI-only constant.
	// Aspirational (EventTask*) and the deprecated alias are intentionally absent.
	mustBePresent := []string{
		// Section 1: Mirrored from orch.
		EventPhaseStarted,
		EventPhaseCompleted,
		EventPhaseCompletedWithConcerns,
		EventPhaseFailed,
		EventPhaseNeedsContext,
		EventApprovalRequired,
		EventApprovalResolved,
		EventGovernanceDecision,
		EventAgentDispatched,
		EventAgentEnvelopeReceived,
		EventApplyBoardCreated,
		EventApplyBoardSaveFailed,
		EventApplyWorktreeError,
		EventApplyGroupCompleted,
		EventApplyGroupFailed,
		EventApplyGroupDegraded,
		EventApplyTeamLeadSpawned,
		EventApplyImplementSpawnFailed,
		EventApplyImplementSpawnGovernorError,
		EventApplyTaskClaimed,
		EventApplyTaskClaimSkipped,
		EventApplyTaskEscalated,
		EventApplyTaskRetry,
		EventApplyDispatchError,
		EventApplyEnvelopeValidationFailed,
		EventRuntimeDispatchFailed,
		EventApplyMaterializeStarted,
		EventApplyMaterializeCompleted,
		EventApplyMaterializeError,
		EventMemoryArtifactPersistFailed,
		// Section 2: CLI-only.
		EventHeartbeat,
		EventOpen,
	}
	for _, ev := range mustBePresent {
		if _, ok := knownEvents[ev]; !ok {
			t.Errorf("knownEvents missing required key %q", ev)
		}
	}

	// Aspirational events must NOT be in the map.
	mustBeAbsent := []string{
		EventTaskCreated,
		EventTaskStarted,
		EventTaskCompleted,
		EventTaskFailed,
	}
	for _, ev := range mustBeAbsent {
		if _, ok := knownEvents[ev]; ok {
			t.Errorf("knownEvents unexpectedly contains aspirational key %q", ev)
		}
	}
}

// domainPhasePath is the canonical PhaseStatus source, relative to this package.
const domainPhasePath = "../../internal/domain/phase.go"

// parseContractPhaseStatusValues parses events.go in this package and returns
// the set of string values for every PhaseStatus* untyped string constant.
// This is the contract layer's re-declaration of the canonical enum.
func parseContractPhaseStatusValues(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, cliEventsSourcePath, nil, 0)
	if err != nil {
		t.Fatalf("parse contract events source: %v", err)
	}
	out := map[string]string{} // constName -> stringValue
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, sp := range gd.Specs {
			vs, ok := sp.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "PhaseStatus") {
					continue
				}
				if i >= len(vs.Values) {
					continue
				}
				bl, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				v, err := strconv.Unquote(bl.Value)
				if err != nil {
					t.Fatalf("strconv.Unquote(%q): %v", bl.Value, err)
				}
				out[name.Name] = v
			}
		}
	}
	return out
}

// parseDomainPhaseStatusValues parses internal/domain/phase.go and returns
// the set of string values for every typed PhaseStatus constant.
// These are the canonical values; all other declarations must match them.
func parseDomainPhaseStatusValues(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, domainPhasePath, nil, 0)
	if err != nil {
		t.Fatalf("parse domain phase source: %v", err)
	}
	out := map[string]string{} // constName -> stringValue
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, sp := range gd.Specs {
			vs, ok := sp.(*ast.ValueSpec)
			if !ok {
				continue
			}
			// Only collect constants whose type is PhaseStatus.
			if vs.Type == nil {
				continue
			}
			ident, ok := vs.Type.(*ast.Ident)
			if !ok || ident.Name != "PhaseStatus" {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				bl, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				v, err := strconv.Unquote(bl.Value)
				if err != nil {
					t.Fatalf("strconv.Unquote(%q): %v", bl.Value, err)
				}
				out[name.Name] = v
			}
		}
	}
	return out
}

// TestPhaseStatusDrift (D1–D4) guards against drift between the two
// PhaseStatus declarations that exist in this repo:
//
//   - D1: pkg/contract/events.go — untyped string constants (contract layer)
//   - D2: internal/domain/phase.go — typed PhaseStatus constants (canonical)
//
// The spec (sophia-wire-v1 §6.1) defines exactly 7 canonical values. Any
// addition, removal, or rename in EITHER file is a spec violation and fails CI.
//
// Why two files? Slice B re-declared the 7 values in events.go (as untyped
// string constants) instead of re-exporting the typed enum. Until that is
// refactored, this test enforces value parity so the two declarations cannot
// silently diverge. See ADR-0003 follow-up note.
func TestPhaseStatusDrift(t *testing.T) {
	// D1: parse both sources.
	contractVals := parseContractPhaseStatusValues(t)
	domainVals := parseDomainPhaseStatusValues(t)

	// D2: each parser must find at least one value — catch empty-parse bugs.
	if len(contractVals) == 0 {
		t.Fatal("no PhaseStatus* constants found in pkg/contract/events.go; check parseContractPhaseStatusValues")
	}
	if len(domainVals) == 0 {
		t.Fatal("no PhaseStatus typed constants found in internal/domain/phase.go; check parseDomainPhaseStatusValues")
	}

	// Canonical 7 values from sophia-wire-v1 §6.1.
	// "failed" is NOT listed — it is the phase.failed SSE event, not a status.
	canonical := map[string]struct{}{
		"pending":            {},
		"running":            {},
		"done":               {},
		"done_with_concerns": {},
		"blocked":            {},
		"needs_context":      {},
		"interrupted":        {},
	}

	// D3: contract/events.go values MUST equal the canonical set.
	contractValueSet := map[string]struct{}{}
	for _, v := range contractVals {
		contractValueSet[v] = struct{}{}
	}
	for v := range canonical {
		if _, ok := contractValueSet[v]; !ok {
			t.Errorf("contract/events.go is missing canonical PhaseStatus value %q", v)
		}
	}
	for v := range contractValueSet {
		if _, ok := canonical[v]; !ok {
			t.Errorf("contract/events.go has extra PhaseStatus value %q not in canonical set", v)
		}
	}

	// D4: domain/phase.go values MUST equal the canonical set.
	domainValueSet := map[string]struct{}{}
	for _, v := range domainVals {
		domainValueSet[v] = struct{}{}
	}
	for v := range canonical {
		if _, ok := domainValueSet[v]; !ok {
			t.Errorf("internal/domain/phase.go is missing canonical PhaseStatus value %q", v)
		}
	}
	for v := range domainValueSet {
		if _, ok := canonical[v]; !ok {
			t.Errorf("internal/domain/phase.go has extra PhaseStatus value %q not in canonical set", v)
		}
	}

	// D5: the two files MUST carry identical value sets — any divergence is
	// caught here even if both happen to drift from canonical in the same direction.
	for v := range contractValueSet {
		if _, ok := domainValueSet[v]; !ok {
			t.Errorf("VALUE PARITY FAIL: contract/events.go has PhaseStatus value %q but internal/domain/phase.go does not", v)
		}
	}
	for v := range domainValueSet {
		if _, ok := contractValueSet[v]; !ok {
			t.Errorf("VALUE PARITY FAIL: internal/domain/phase.go has PhaseStatus value %q but contract/events.go does not", v)
		}
	}
}

// TestApplyHandlerCoverage (E1, E2) — inverse drift guard.
//
// Asserts that every apply.* event constant the orchestrator declares has a
// corresponding handler case in ApplyBoardState.ApplyEvent. This is the
// inverse of TestWireAlignment_OrchEventsMirrored: instead of checking that
// the CLI *knows about* every orch event, it checks that ApplyBoard *handles*
// every apply.* event (or has a documented reason for not doing so).
//
// Apply.* events intentionally NOT handled by ApplyBoardState:
//
//   - apply.board.save_failed — orch persistence failure; no TUI state to update
//   - apply.worktree.error   — infrastructure error; surfaced via Timeline or error bar
//
// Any new apply.* event added to the orch that ApplyBoard should not handle
// must be added to intentionallyIgnored below with a documented reason.
func TestApplyHandlerCoverage(t *testing.T) {
	orchEvents, skipReason := parseOrchEvents(t)
	if orchEvents == nil {
		t.Skipf("cross-repo drift check skipped: %s", skipReason)
	}

	// Sanity: parser must find at least one event.
	if len(orchEvents) == 0 {
		t.Fatal("no Event* constants extracted from orch source; check orchSourcePath")
	}

	// Filter to apply.* events only.
	applyOrchEvents := map[string]string{}
	for name, value := range orchEvents {
		if strings.HasPrefix(value, "apply.") {
			applyOrchEvents[name] = value
		}
	}
	if len(applyOrchEvents) == 0 {
		t.Fatal("no apply.* events found in orch source; check parsing logic")
	}

	// Documented allowlist: apply.* events that ApplyBoard intentionally does
	// not handle. Each entry MUST have a comment explaining why.
	intentionallyIgnored := map[string]string{
		"apply.board.save_failed": "orch persistence failure; no TUI state to update — operator sees via error bar if present",
		"apply.worktree.error":    "infrastructure error; not per-group state — surfaced via Timeline error bar",
		// apply-build-feedback-loop: build events are mirrored in the contract
		// (knownEvents) so orch↔cli stay aligned, but the ApplyBoard does not
		// yet render a dedicated build view. Group pass/fail is already shown
		// via apply.group.completed/failed; a dedicated build-status row is a
		// follow-up UI task. Documented-ignored until then.
		"apply.build.started": "build verification start; group-level, no dedicated board row yet (follow-up UI)",
		"apply.build.passed":  "build passed; group completion already surfaced via apply.group.completed",
		"apply.build.failed":  "build failed; group failure already surfaced via apply.group.failed (stderr in event payload, follow-up UI)",
	}

	// Parse CLI constants (to resolve contract.EventApply* names → values).
	cliConstants := parseCLIEventConstants(t)
	if len(cliConstants) == 0 {
		t.Fatal("no Event* constants found in CLI events.go; check parseCLIEventConstants")
	}

	// Parse ApplyEvent switch cases in applyboard_state.go.
	handledCases := parseApplyBoardCases(t, cliConstants)
	if len(handledCases) == 0 {
		t.Fatal("no cases found in ApplyBoardState.ApplyEvent switch; check parseApplyBoardCases")
	}

	// Assert every orch apply.* event is either handled or intentionally ignored.
	for orchName, orchValue := range applyOrchEvents {
		if _, handled := handledCases[orchValue]; handled {
			continue
		}
		if reason, ignored := intentionallyIgnored[orchValue]; ignored {
			t.Logf("apply.* event %s = %q is intentionally not handled by ApplyBoard: %s", orchName, orchValue, reason)
			continue
		}
		t.Errorf("drift: orch emits %s = %q but ApplyBoardState.ApplyEvent has no handler case for it", orchName, orchValue)
	}
}

// domainChangePath is the canonical ChangeStatus source, relative to this package.
const domainChangePath = "../../internal/domain/change.go"

// parseContractChangeStatusValues extracts every ChangeStatus* string constant
// from pkg/contract/events.go (untyped contract-layer constants).
func parseContractChangeStatusValues(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, cliEventsSourcePath, nil, 0)
	if err != nil {
		t.Fatalf("parse contract events source: %v", err)
	}
	out := map[string]string{} // constName -> stringValue
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, sp := range gd.Specs {
			vs, ok := sp.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "ChangeStatus") {
					continue
				}
				if i >= len(vs.Values) {
					continue
				}
				bl, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				v, err := strconv.Unquote(bl.Value)
				if err != nil {
					t.Fatalf("strconv.Unquote(%q): %v", bl.Value, err)
				}
				out[name.Name] = v
			}
		}
	}
	return out
}

// parseDomainChangeStatusValues extracts every typed ChangeStatus constant from
// internal/domain/change.go (the canonical typed enum).
func parseDomainChangeStatusValues(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, domainChangePath, nil, 0)
	if err != nil {
		t.Fatalf("parse domain change source: %v", err)
	}
	out := map[string]string{} // constName -> stringValue
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, sp := range gd.Specs {
			vs, ok := sp.(*ast.ValueSpec)
			if !ok {
				continue
			}
			// Only collect constants whose type is ChangeStatus.
			if vs.Type == nil {
				continue
			}
			ident, ok := vs.Type.(*ast.Ident)
			if !ok || ident.Name != "ChangeStatus" {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				bl, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				v, err := strconv.Unquote(bl.Value)
				if err != nil {
					t.Fatalf("strconv.Unquote(%q): %v", bl.Value, err)
				}
				out[name.Name] = v
			}
		}
	}
	return out
}

// TestChangeStatusDrift guards against drift between the two ChangeStatus
// declarations in this repo, mirroring TestPhaseStatusDrift:
//
//   - pkg/contract/events.go — untyped string constants (contract layer)
//   - internal/domain/change.go — typed ChangeStatus constants (canonical)
//
// The spec (sophia-wire-v1 §505) defines exactly 6 canonical ChangeStatus
// values. Unlike PhaseStatus, ChangeStatus DOES include "failed" and "aborted"
// — they are legitimate terminal change statuses. Any addition, removal, or
// rename in either file is a spec violation and fails CI.
func TestChangeStatusDrift(t *testing.T) {
	contractVals := parseContractChangeStatusValues(t)
	domainVals := parseDomainChangeStatusValues(t)

	if len(contractVals) == 0 {
		t.Fatal("no ChangeStatus* constants found in pkg/contract/events.go; check parseContractChangeStatusValues")
	}
	if len(domainVals) == 0 {
		t.Fatal("no ChangeStatus typed constants found in internal/domain/change.go; check parseDomainChangeStatusValues")
	}

	// Canonical 6 values from sophia-wire-v1 §505.
	canonical := map[string]struct{}{
		"pending": {},
		"running": {},
		"done":    {},
		"blocked": {},
		"failed":  {},
		"aborted": {},
	}

	contractValueSet := map[string]struct{}{}
	for _, v := range contractVals {
		contractValueSet[v] = struct{}{}
	}
	for v := range canonical {
		if _, ok := contractValueSet[v]; !ok {
			t.Errorf("contract/events.go is missing canonical ChangeStatus value %q", v)
		}
	}
	for v := range contractValueSet {
		if _, ok := canonical[v]; !ok {
			t.Errorf("contract/events.go has extra ChangeStatus value %q not in canonical set", v)
		}
	}

	domainValueSet := map[string]struct{}{}
	for _, v := range domainVals {
		domainValueSet[v] = struct{}{}
	}
	for v := range canonical {
		if _, ok := domainValueSet[v]; !ok {
			t.Errorf("internal/domain/change.go is missing canonical ChangeStatus value %q", v)
		}
	}
	for v := range domainValueSet {
		if _, ok := canonical[v]; !ok {
			t.Errorf("internal/domain/change.go has extra ChangeStatus value %q not in canonical set", v)
		}
	}

	// Value parity between the two files — catches same-direction drift.
	for v := range contractValueSet {
		if _, ok := domainValueSet[v]; !ok {
			t.Errorf("VALUE PARITY FAIL: contract/events.go has ChangeStatus value %q but internal/domain/change.go does not", v)
		}
	}
	for v := range domainValueSet {
		if _, ok := contractValueSet[v]; !ok {
			t.Errorf("VALUE PARITY FAIL: internal/domain/change.go has ChangeStatus value %q but contract/events.go does not", v)
		}
	}
}
