package orchestratorhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// TestApprovePhase_PostsCanonicalBodyWithAuth asserts:
//   - canonical path /api/v1/phases/{id}/approve
//   - JSON body {approver, reason}
//   - X-Sophia-API-Key header injected when configured
func TestApprovePhase_PostsCanonicalBodyWithAuth(t *testing.T) {
	var (
		gotPath    string
		gotKey     string
		gotApprove bool
		gotReason  string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-Sophia-API-Key")
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Approver string `json:"approver"`
			Reason   string `json:"reason"`
		}
		_ = json.Unmarshal(raw, &body)
		gotApprove = body.Approver == "alice"
		gotReason = body.Reason
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL, APIKey: "secret"})
	err := c.ApprovePhase(context.Background(), "01PHASE", outbound.ApprovalDecisionInput{
		Approver: "alice", Reason: "lgtm",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/phases/01PHASE/approve" {
		t.Errorf("path = %q", gotPath)
	}
	if gotKey != "secret" {
		t.Errorf("X-Sophia-API-Key = %q", gotKey)
	}
	if !gotApprove {
		t.Error("approver not parsed")
	}
	if gotReason != "lgtm" {
		t.Errorf("reason = %q", gotReason)
	}
}

func TestRejectPhase_PostsCanonicalPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL, APIKey: "k"})
	if err := c.RejectPhase(context.Background(), "01PHASE", outbound.ApprovalDecisionInput{Approver: "a"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/phases/01PHASE/reject" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestAbortChange_PostsCanonicalPath(t *testing.T) {
	var (
		gotPath string
		gotBody string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	if err := c.AbortChange(context.Background(), domain.ChangeID("01CH"), outbound.AbortChangeInput{
		Reason: "user requested",
	}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/changes/01CH/abort" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, "user requested") {
		t.Errorf("body = %q", gotBody)
	}
}

// TestApprovePhase_GateAlreadyDecidedMaps asserts the wire envelope's
// `code: gate_already_decided` is mapped via errors.Is to the domain
// sentinel — the CLI relies on this for idempotency.
func TestApprovePhase_GateAlreadyDecidedMaps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"gate_already_decided","error":"already decided"}`))
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	err := c.ApprovePhase(context.Background(), "01P", outbound.ApprovalDecisionInput{Approver: "a"})
	if !errors.Is(err, domain.ErrGateAlreadyDecided) {
		t.Errorf("expected ErrGateAlreadyDecided, got %v", err)
	}
}

// TestAbortChange_AlreadyTerminalMaps mirrors the approve idempotency
// path for the abort verb.
func TestAbortChange_AlreadyTerminalMaps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"change_already_terminal","error":"terminal"}`))
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	err := c.AbortChange(context.Background(), "01CH", outbound.AbortChangeInput{})
	if !errors.Is(err, domain.ErrChangeAlreadyTerminal) {
		t.Errorf("expected ErrChangeAlreadyTerminal, got %v", err)
	}
}

// TestClient_AuthHeaderOmittedWhenAnon asserts no X-Sophia-API-Key
// header is sent when APIKey is empty (loopback / anon mode). The key
// MUST never be logged or echoed; this test enforces "absent ≠ empty".
func TestClient_AuthHeaderOmittedWhenAnon(t *testing.T) {
	var headerSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, headerSeen = r.Header["X-Sophia-Api-Key"]
		// Note: r.Header.Get normalises case but we want the literal
		// presence check.
		if v := r.Header.Get("X-Sophia-API-Key"); v != "" {
			headerSeen = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	_ = c.Healthz(context.Background())
	if headerSeen {
		t.Error("X-Sophia-API-Key MUST NOT be sent when APIKey is empty")
	}
}
