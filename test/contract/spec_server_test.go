//go:build contract

package contract_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/pkg/contract"
)

// specServer is a synthetic orchestrator implementing exactly the wire
// surface defined by sophia-wire-v1 + pkg/contract. It exists to
// validate the cli's outbound HTTP + SSE clients against the canonical
// shapes — NOT to replace the real orchestrator. Coverage of "the orch
// implements the spec" is the orchestrator repo's own test suite.
//
// Behaviors are programmable per-test via the public fields. This keeps
// each test focused on one wire-level concern (auth, error envelope,
// SSE event ordering, …) rather than re-implementing orch state.
type specServer struct {
	mu sync.Mutex

	// requireAuth: when true, requests without a valid X-Sophia-API-Key
	// receive 401 + unauthorized envelope. When false, anonymous is
	// accepted (loopback mode, sophia-wire-v1 §3.2 / D-M10-02).
	requireAuth bool
	// validKey is compared verbatim against X-Sophia-API-Key. Empty
	// when requireAuth is false.
	validKey string

	// approveBehavior controls /phases/{id}/approve responses.
	approveBehavior approveBehavior
	// rejectBehavior mirrors approveBehavior for /reject.
	rejectBehavior approveBehavior
	// abortBehavior controls /changes/{id}/abort responses.
	abortBehavior abortBehavior

	// listLimitMax is the cap above which List returns
	// limit_too_large. Default 100 per sophia-wire-v1 §9.2.
	listLimitMax int

	// changes are returned by GET /changes and GET /changes/{id}.
	changes map[string]contract.ChangeResponse
	// phases are returned by GET /phases/{id}.
	phases map[string]contract.PhaseResponse
	// terminalPhases yield 410 phase_terminal_no_events on
	// /phases/{id}/events.
	terminalPhases map[string]bool
	// phaseEvents queues SSE events to emit for a given phase id.
	phaseEvents map[string][]sseEvent

	// onApprove / onReject / onAbort capture the latest input for
	// per-test assertions.
	onApprove func(phaseID string, body contract.ApprovalDecisionRequest)
	onReject  func(phaseID string, body contract.ApprovalDecisionRequest)
	onAbort   func(changeID string, body contract.AbortChangeRequest)

	// afterStream fires synchronously at the end of an SSE stream
	// (after all queued events were emitted). Tests use it to flip
	// change status / current_phase_id so the multiplexer's
	// post-stream snapshot sees the right state.
	afterStream func(phaseID string)
}

// approveBehavior is a small enum so tests can pick one of the
// idempotent / 4xx response paths without a mass of bools.
type approveBehavior int

const (
	approveOK              approveBehavior = iota // 200 status approved
	approveGateAlreadyDone                        // 409 gate_already_decided
	approveNotGated                               // 409 phase_not_gated
	approveApproverMissing                        // 400 approver_required
)

// abortBehavior controls /changes/{id}/abort.
type abortBehavior int

const (
	abortOK              abortBehavior = iota // 200
	abortAlreadyTerminal                      // 409 change_already_terminal
)

// sseEvent is one queued event for the SSE handler.
type sseEvent struct {
	Type    string
	ID      string
	Payload map[string]any
	// EmitAfter is the delay before emitting; tests use this to
	// interleave events with reconnect/timing checks.
	EmitAfter time.Duration
}

// newSpecServer returns a programmable spec-conformant server with
// safe defaults: anonymous allowed, no preset changes/phases,
// approveBehavior=OK.
func newSpecServer() *specServer {
	return &specServer{
		listLimitMax:   100,
		changes:        map[string]contract.ChangeResponse{},
		phases:         map[string]contract.PhaseResponse{},
		terminalPhases: map[string]bool{},
		phaseEvents:    map[string][]sseEvent{},
	}
}

// start wraps the server in an httptest.NewServer the test can address.
// Caller is responsible for srv.Close().
func (s *specServer) start() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc(contract.RouteHealth, s.handleHealth)
	mux.HandleFunc(contract.RouteChanges, s.handleChangesCollection) // GET list, POST create
	mux.HandleFunc("/api/v1/changes/", s.handleChangesByID)          // /{id}, /{id}/abort
	mux.HandleFunc("/api/v1/phases/", s.handlePhasesByID)            // /{id}, /{id}/approve, /{id}/reject, /{id}/events

	return httptest.NewServer(mux)
}

// requireAuthOK validates the X-Sophia-API-Key header per
// sophia-wire-v1 §3.1. Returns true when the request is allowed; when
// false, it has already written the unauthorized envelope.
func (s *specServer) requireAuthOK(w http.ResponseWriter, r *http.Request) bool {
	if !s.requireAuth {
		return true
	}
	got := r.Header.Get(contract.HeaderAPIKey)
	if got == "" {
		writeError(w, http.StatusUnauthorized, contract.CodeUnauthorized, "missing api key", nil)
		return false
	}
	if got != s.validKey {
		writeError(w, http.StatusUnauthorized, contract.CodeUnauthorized, "invalid api key", nil)
		return false
	}
	return true
}

// --- /api/v1/health ---

func (s *specServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	// /health is always public per sophia-wire-v1 §4.1, regardless of
	// requireAuth — clients need to probe before authenticating.
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, contract.CodeValidationFailed, "method not allowed", nil)
		return
	}
	writeJSON(w, http.StatusOK, contract.HealthResponse{Status: "ok", Version: contract.Version})
}

// --- /api/v1/changes (collection) ---

func (s *specServer) handleChangesCollection(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuthOK(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListChanges(w, r)
	case http.MethodPost:
		s.handleCreateChange(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, contract.CodeValidationFailed, "method not allowed", nil)
	}
}

func (s *specServer) handleListChanges(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, contract.CodeValidationFailed, "limit not an integer", nil)
			return
		}
		if n > s.listLimitMax {
			writeError(w, http.StatusBadRequest, contract.CodeLimitTooLarge,
				"limit exceeds maximum",
				map[string]any{"limit": n, "max_limit": s.listLimitMax})
			return
		}
		limit = n
	}
	s.mu.Lock()
	items := make([]contract.ChangeResponse, 0, len(s.changes))
	for _, c := range s.changes {
		items = append(items, c)
	}
	s.mu.Unlock()
	if limit < len(items) {
		items = items[:limit]
	}
	writeJSON(w, http.StatusOK, contract.ListChangesResponse{Items: items, Total: len(items), Limit: limit})
}

func (s *specServer) handleCreateChange(w http.ResponseWriter, r *http.Request) {
	var req contract.CreateChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, contract.CodeValidationFailed, "invalid body: "+err.Error(), nil)
		return
	}
	if req.Name == "" || req.Project == "" {
		writeError(w, http.StatusBadRequest, contract.CodeValidationFailed,
			"name and project required", nil)
		return
	}
	id := fmt.Sprintf("01CONTRACT%04d", len(s.changes)+1)
	resp := contract.ChangeResponse{
		ChangeID: id, Name: req.Name, Project: req.Project,
		BaseRef: req.BaseRef, ArtifactStoreMode: req.ArtifactStoreMode,
		Status: "running", CurrentPhaseID: id + "PHASE",
	}
	s.mu.Lock()
	s.changes[id] = resp
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, resp)
}

// --- /api/v1/changes/{id}/(abort) ---

func (s *specServer) handleChangesByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuthOK(w, r) {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/changes/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, contract.CodeChangeNotFound, "change_id required", nil)
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "abort" {
		s.handleAbort(w, r, id)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, contract.CodeValidationFailed, "method not allowed", nil)
		return
	}
	s.mu.Lock()
	resp, ok := s.changes[id]
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, contract.CodeChangeNotFound, "no such change", nil)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *specServer) handleAbort(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, contract.CodeValidationFailed, "method not allowed", nil)
		return
	}
	var body contract.AbortChangeRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	if s.onAbort != nil {
		s.onAbort(id, body)
	}
	switch s.abortBehavior {
	case abortAlreadyTerminal:
		writeError(w, http.StatusConflict, contract.CodeChangeAlreadyTerminal, "change is terminal", nil)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "aborting"})
	}
}

// --- /api/v1/phases/{id}/(approve|reject|events) ---

func (s *specServer) handlePhasesByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuthOK(w, r) {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/phases/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, contract.CodePhaseNotFound, "phase_id required", nil)
		return
	}
	id := parts[0]
	switch {
	case len(parts) == 2 && parts[1] == "approve":
		s.handleApprove(w, r, id)
	case len(parts) == 2 && parts[1] == "reject":
		s.handleReject(w, r, id)
	case len(parts) == 2 && parts[1] == "events":
		s.handleEvents(w, r, id)
	case len(parts) == 1:
		s.handlePhaseGet(w, r, id)
	default:
		writeError(w, http.StatusNotFound, contract.CodePhaseNotFound, "unknown phase route", nil)
	}
}

func (s *specServer) handlePhaseGet(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, contract.CodeValidationFailed, "method not allowed", nil)
		return
	}
	s.mu.Lock()
	resp, ok := s.phases[id]
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, contract.CodePhaseNotFound, "no such phase", nil)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *specServer) handleApprove(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, contract.CodeValidationFailed, "method not allowed", nil)
		return
	}
	var body contract.ApprovalDecisionRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	if s.onApprove != nil {
		s.onApprove(id, body)
	}
	switch s.approveBehavior {
	case approveGateAlreadyDone:
		writeError(w, http.StatusConflict, contract.CodeGateAlreadyDecided, "gate already decided", nil)
	case approveNotGated:
		writeError(w, http.StatusConflict, contract.CodePhaseNotGated, "phase not awaiting approval", nil)
	case approveApproverMissing:
		writeError(w, http.StatusBadRequest, contract.CodeApproverRequired, "approver required", nil)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
	}
}

func (s *specServer) handleReject(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, contract.CodeValidationFailed, "method not allowed", nil)
		return
	}
	var body contract.ApprovalDecisionRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	if s.onReject != nil {
		s.onReject(id, body)
	}
	switch s.rejectBehavior {
	case approveGateAlreadyDone:
		writeError(w, http.StatusConflict, contract.CodeGateAlreadyDecided, "gate already decided", nil)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
	}
}

// handleEvents emits the queued sseEvent slice for the phase, then
// closes the connection. Phases listed in terminalPhases short-circuit
// to 410 phase_terminal_no_events instead of streaming.
func (s *specServer) handleEvents(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, contract.CodeValidationFailed, "method not allowed", nil)
		return
	}
	s.mu.Lock()
	terminal := s.terminalPhases[id]
	queue := append([]sseEvent(nil), s.phaseEvents[id]...)
	s.mu.Unlock()
	if terminal {
		writeError(w, http.StatusGone, contract.CodePhaseTerminalNoEvents,
			"phase is terminal", map[string]any{"phase_id": id, "status": "blocked"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Always emit the open event first per sophia-wire-v1 §5.3.
	fmt.Fprintf(w, "id: %s\nevent: %s\ndata: {\"phase_id\":%q}\n\n",
		"open-1", contract.EventOpen, id)
	flusher.Flush()

	for i, ev := range queue {
		if ev.EmitAfter > 0 {
			time.Sleep(ev.EmitAfter)
		}
		payload, _ := json.Marshal(map[string]any{
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"payload":   ev.Payload,
		})
		evID := ev.ID
		if evID == "" {
			evID = fmt.Sprintf("ev-%d", i)
		}
		fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", evID, ev.Type, payload)
		flusher.Flush()
	}
	if s.afterStream != nil {
		s.afterStream(id)
	}
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string, details map[string]any) {
	writeJSON(w, status, contract.ErrorResponse{Code: code, Error: msg, Details: details})
}
