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
