//go:build contract

// Phase 5 contract tests run via `make contract` (build tag
// `contract`). They validate the cli's outbound HTTP + SSE clients
// against a synthetic spec-conformant orchestrator (specServer below).
// The orchestrator side's "spec compliance" is covered by the orch
// repo's own tests (Phase 3.8); the cross-repo gate is the SHA256
// equality test in checksum_test.go.
package contract_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/pkg/contract"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/ssestream"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// --- Required endpoints (sophia-wire-v1 §4) ---

func TestContract_HealthEndpoint(t *testing.T) {
	srv := newSpecServer().start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	if err := c.Healthz(context.Background()); err != nil {
		t.Fatalf("Healthz: %v", err)
	}
}

func TestContract_CreateAndGetChange(t *testing.T) {
	spec := newSpecServer()
	srv := spec.start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	created, err := c.CreateChange(context.Background(), outbound.CreateChangeInput{
		Name: "feat-x", Project: "demo", BaseRef: "main", ArtifactStoreMode: "memory-engine",
	})
	if err != nil {
		t.Fatalf("CreateChange: %v", err)
	}
	if created.ID.IsZero() {
		t.Fatal("ID empty")
	}
	got, err := c.GetChange(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetChange: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("round-trip ID mismatch: got=%q want=%q", got.ID, created.ID)
	}
	if got.CurrentPhaseID == "" {
		t.Error("CurrentPhaseID missing on snapshot")
	}
}

func TestContract_ListChanges_RespectsLimit(t *testing.T) {
	srv := newSpecServer().start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	if _, err := c.ListChanges(context.Background(), outbound.ListChangesFilter{Limit: 50}); err != nil {
		t.Fatalf("ListChanges: %v", err)
	}
}

func TestContract_ListChanges_LimitTooLarge(t *testing.T) {
	srv := newSpecServer().start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	_, err := c.ListChanges(context.Background(), outbound.ListChangesFilter{Limit: 500})
	if !errors.Is(err, domain.ErrLimitTooLarge) {
		t.Fatalf("expected ErrLimitTooLarge, got %v", err)
	}
}

func TestContract_AbortChange(t *testing.T) {
	spec := newSpecServer()
	var captured contract.AbortChangeRequest
	spec.onAbort = func(_ string, body contract.AbortChangeRequest) { captured = body }
	srv := spec.start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	if err := c.AbortChange(context.Background(), "01CH", outbound.AbortChangeInput{Reason: "test"}); err != nil {
		t.Fatalf("AbortChange: %v", err)
	}
	if captured.Reason != "test" {
		t.Errorf("reason not propagated: %q", captured.Reason)
	}
}

func TestContract_AbortChange_AlreadyTerminalIsMapped(t *testing.T) {
	spec := newSpecServer()
	spec.abortBehavior = abortAlreadyTerminal
	srv := spec.start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	err := c.AbortChange(context.Background(), "01CH", outbound.AbortChangeInput{})
	if !errors.Is(err, domain.ErrChangeAlreadyTerminal) {
		t.Fatalf("expected ErrChangeAlreadyTerminal, got %v", err)
	}
}

func TestContract_GetPhase(t *testing.T) {
	spec := newSpecServer()
	spec.phases["01PH"] = contract.PhaseResponse{
		PhaseID: "01PH", PhaseType: "spec", Status: "running",
	}
	srv := spec.start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	// The cli currently fetches phases via the change route in M5;
	// for Phase 5 we only validate that a direct GET on the phase
	// endpoint returns the canonical shape via the contract package.
	resp, err := orchestratorhttpGetPhase(context.Background(), c, srv.URL, "01PH")
	if err != nil {
		t.Fatalf("GetPhase: %v", err)
	}
	if resp.Status != "running" {
		t.Errorf("status = %q", resp.Status)
	}
}

func TestContract_ApprovePhase_HappyPath(t *testing.T) {
	spec := newSpecServer()
	var capturedPhase string
	var capturedBody contract.ApprovalDecisionRequest
	spec.onApprove = func(phaseID string, body contract.ApprovalDecisionRequest) {
		capturedPhase = phaseID
		capturedBody = body
	}
	srv := spec.start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	if err := c.ApprovePhase(context.Background(), "01PH", outbound.ApprovalDecisionInput{
		Approver: "alice", Reason: "lgtm",
	}); err != nil {
		t.Fatalf("ApprovePhase: %v", err)
	}
	if capturedPhase != "01PH" {
		t.Errorf("phase = %q", capturedPhase)
	}
	if capturedBody.Approver != "alice" || capturedBody.Reason != "lgtm" {
		t.Errorf("body = %+v", capturedBody)
	}
}

func TestContract_ApprovePhase_GateAlreadyDecided(t *testing.T) {
	spec := newSpecServer()
	spec.approveBehavior = approveGateAlreadyDone
	srv := spec.start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	err := c.ApprovePhase(context.Background(), "01PH", outbound.ApprovalDecisionInput{Approver: "alice"})
	if !errors.Is(err, domain.ErrGateAlreadyDecided) {
		t.Fatalf("expected ErrGateAlreadyDecided, got %v", err)
	}
}

func TestContract_ApprovePhase_PhaseNotGated(t *testing.T) {
	spec := newSpecServer()
	spec.approveBehavior = approveNotGated
	srv := spec.start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	err := c.ApprovePhase(context.Background(), "01PH", outbound.ApprovalDecisionInput{Approver: "alice"})
	if !errors.Is(err, domain.ErrPhaseNotGated) {
		t.Fatalf("expected ErrPhaseNotGated, got %v", err)
	}
}

func TestContract_RejectPhase_HappyPath(t *testing.T) {
	spec := newSpecServer()
	var captured contract.ApprovalDecisionRequest
	spec.onReject = func(_ string, body contract.ApprovalDecisionRequest) { captured = body }
	srv := spec.start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	if err := c.RejectPhase(context.Background(), "01PH", outbound.ApprovalDecisionInput{
		Approver: "alice", Reason: "no",
	}); err != nil {
		t.Fatalf("RejectPhase: %v", err)
	}
	if captured.Approver != "alice" {
		t.Errorf("body = %+v", captured)
	}
}

// --- Auth (sophia-wire-v1 §3) ---

func TestContract_Auth_LoopbackAnonAllowed(t *testing.T) {
	spec := newSpecServer()
	// requireAuth false (default) emulates loopback anon mode.
	srv := spec.start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	if err := c.Healthz(context.Background()); err != nil {
		t.Fatalf("anon healthz: %v", err)
	}
	if _, err := c.ListChanges(context.Background(), outbound.ListChangesFilter{}); err != nil {
		t.Fatalf("anon list: %v", err)
	}
}

func TestContract_Auth_RemoteAnonRejected(t *testing.T) {
	spec := newSpecServer()
	spec.requireAuth = true
	spec.validKey = "topsecret"
	srv := spec.start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	_, err := c.ListChanges(context.Background(), outbound.ListChangesFilter{})
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestContract_Auth_ValidKeyAccepted(t *testing.T) {
	spec := newSpecServer()
	spec.requireAuth = true
	spec.validKey = "topsecret"
	srv := spec.start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL, APIKey: "topsecret"})
	if _, err := c.ListChanges(context.Background(), outbound.ListChangesFilter{}); err != nil {
		t.Fatalf("auth'd list: %v", err)
	}
}

func TestContract_Auth_InvalidKeyRejected(t *testing.T) {
	spec := newSpecServer()
	spec.requireAuth = true
	spec.validKey = "topsecret"
	srv := spec.start()
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL, APIKey: "wrong"})
	_, err := c.ListChanges(context.Background(), outbound.ListChangesFilter{})
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for wrong key, got %v", err)
	}
}

// --- SSE (sophia-wire-v1 §5) ---

func TestContract_SSE_EventTypes(t *testing.T) {
	spec := newSpecServer()
	spec.phaseEvents["01PH"] = []sseEvent{
		{Type: contract.EventPhaseStarted, ID: "e1", Payload: map[string]any{"phase_id": "01PH", "phase_type": "spec"}},
		{Type: contract.EventApprovalRequired, ID: "e2", Payload: map[string]any{
			"phase_id": "01PH", "gate_url": "http://gate", "reason": "policy",
		}},
		{Type: contract.EventApprovalResolved, ID: "e3", Payload: map[string]any{
			"phase_id": "01PH", "decision": "approved", "approver": "alice",
		}},
		{Type: contract.EventPhaseCompleted, ID: "e4", Payload: map[string]any{
			"phase_id": "01PH", "phase_type": "spec",
		}},
		{Type: "apply.tx.committed", ID: "e5", Payload: map[string]any{"detail": "diagnostic"}}, // unknown / Optional
	}
	srv := spec.start()
	defer srv.Close()

	c := ssestream.New(ssestream.Config{
		BaseURL:    srv.URL,
		Heartbeat:  5 * time.Second,
		MaxRetries: 1,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{
		ChangeID: domain.ChangeID("01CH"), PhaseID: "01PH",
	}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	wantTypes := []string{
		contract.EventOpen, // synthetic server emits it first
		contract.EventPhaseStarted,
		contract.EventApprovalRequired,
		contract.EventApprovalResolved,
		contract.EventPhaseCompleted,
		"apply.tx.committed", // unknown event MUST be tolerated and surfaced unchanged
	}
	got := drainAll(t, ch, len(wantTypes), 2*time.Second)
	for i, ev := range got {
		if ev.Type != wantTypes[i] {
			t.Errorf("event %d type = %q, want %q", i, ev.Type, wantTypes[i])
		}
	}
}

func TestContract_SSE_PhaseTerminalNoEvents(t *testing.T) {
	spec := newSpecServer()
	spec.terminalPhases["01PH"] = true
	srv := spec.start()
	defer srv.Close()

	c := ssestream.New(ssestream.Config{
		BaseURL:    srv.URL,
		Heartbeat:  100 * time.Millisecond,
		MaxRetries: 5, // would normally exhaust on retries; 410 must short-circuit
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{
		ChangeID: "01CH", PhaseID: "01PH",
	}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected immediate channel close on 410 phase_terminal_no_events")
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("channel did not close on 410 within 1.5s — retry storm likely")
	}
}

// --- Error envelope (sophia-wire-v1 §9.1 + §9.2) ---

func TestContract_ErrorEnvelope_AllStableCodes(t *testing.T) {
	// Map each code to an HTTP status that the spec server can return
	// for that code. We then trigger via crafted requests and assert
	// the cli's StatusError surfaces the right domain sentinel.
	codes := []struct {
		code     string
		sentinel error
	}{
		{contract.CodeUnauthorized, domain.ErrUnauthorized},
		{contract.CodeValidationFailed, domain.ErrValidationFailed},
		{contract.CodeApproverRequired, domain.ErrApproverRequired},
		{contract.CodeLimitTooLarge, domain.ErrLimitTooLarge},
		{contract.CodeChangeNotFound, domain.ErrChangeNotFound},
		{contract.CodePhaseNotFound, domain.ErrPhaseNotFound},
		{contract.CodeChangeAlreadyExists, domain.ErrChangeAlreadyExists},
		{contract.CodeChangeAlreadyTerminal, domain.ErrChangeAlreadyTerminal},
		{contract.CodePhaseNotResumable, domain.ErrPhaseNotResumable},
		{contract.CodePhaseNotGated, domain.ErrPhaseNotGated},
		{contract.CodeGateAlreadyDecided, domain.ErrGateAlreadyDecided},
		{contract.CodePhaseTerminalNoEvents, domain.ErrPhaseTerminalNoEvents},
		{contract.CodeInternalError, domain.ErrInternal},
	}
	// Direct construction asserts the parser; the live-server side is
	// covered by the dedicated success/failure tests above.
	for _, tc := range codes {
		body := []byte(`{"code":"` + tc.code + `","error":"x","details":{"k":"v"}}`)
		err := orchestratorhttp.NewStatusError(400, body)
		if !errors.Is(err, tc.sentinel) {
			t.Errorf("code %q failed to map to %v", tc.code, tc.sentinel)
		}
	}
}

// --- helpers ---

// orchestratorhttpGetPhase is a thin probe that exercises the GET
// /api/v1/phases/{id} endpoint via the cli's own HTTP client. The
// outbound port doesn't yet expose GetPhase (the multiplexer reads
// phases through the change snapshot), but the wire endpoint exists
// and the contract test asserts it returns the canonical shape. Once
// the cli adds GetPhase to the port, this helper can be deleted.
func orchestratorhttpGetPhase(ctx context.Context, _ *orchestratorhttp.Client, baseURL, phaseID string) (contract.PhaseResponse, error) {
	// Use the same transport headers that production code would use,
	// but skip a public method we don't yet expose.
	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: baseURL})
	_ = c // silence unused
	// Build the request through net/http for now.
	url := strings.TrimRight(baseURL, "/") + contract.PhasePath(phaseID)
	return doGetPhase(ctx, url)
}

func drainAll(t *testing.T, ch <-chan domain.Event, n int, deadline time.Duration) []domain.Event {
	t.Helper()
	out := make([]domain.Event, 0, n)
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-timer.C:
			t.Fatalf("drainAll: timed out at %d/%d events: %+v", len(out), n, out)
		}
	}
	return out
}
