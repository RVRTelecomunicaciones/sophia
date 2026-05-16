package orchestratorhttp_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

func TestCreateRequestJSONFieldsAreSnakeCase(t *testing.T) {
	r := orchestratorhttp.CreateChangeRequest{
		Name:              "test",
		Project:           "p",
		BaseRef:           "main",
		ArtifactStoreMode: "memory-engine",
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"name":"test"`, `"project":"p"`, `"base_ref":"main"`, `"artifact_store_mode":"memory-engine"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %q in %s", want, b)
		}
	}
}

func TestChangeResponseDecodes(t *testing.T) {
	body := `{
		"change_id":"01HX-abc",
		"name":"test",
		"project":"p",
		"base_ref":"main",
		"artifact_store_mode":"memory-engine",
		"status":"running",
		"current_phase_id":"phase-1",
		"phases":[
			{"id":"phase-1","type":"explore","status":"running","confidence":0.5,"started_at":"2026-05-05T12:00:00Z"}
		],
		"created_at":"2026-05-05T11:00:00Z",
		"updated_at":"2026-05-05T12:00:00Z"
	}`
	var resp orchestratorhttp.ChangeResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	got := resp.ToDomain()
	if got.ID.String() != "01HX-abc" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.Status != domain.ChangeStatusRunning {
		t.Errorf("Status = %q", got.Status)
	}
	if got.CurrentPhaseID != "phase-1" {
		t.Errorf("CurrentPhaseID = %q", got.CurrentPhaseID)
	}
	if len(got.Phases) != 1 {
		t.Fatalf("Phases len = %d", len(got.Phases))
	}
	if got.Phases[0].Type != domain.PhaseExplore {
		t.Errorf("phase Type = %q", got.Phases[0].Type)
	}
	if got.Phases[0].Confidence != 0.5 {
		t.Errorf("phase Confidence = %f", got.Phases[0].Confidence)
	}
	if got.Phases[0].StartedAt.IsZero() {
		t.Error("StartedAt should not be zero")
	}
	want := time.Date(2026, 5, 5, 11, 0, 0, 0, time.UTC)
	if !got.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, want)
	}
}

func TestChangeResponseTolerantOfMissingFields(t *testing.T) {
	body := `{"change_id":"01HX","status":"pending"}`
	var resp orchestratorhttp.ChangeResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	got := resp.ToDomain()
	if got.ID.String() != "01HX" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.Status != domain.ChangeStatusPending {
		t.Errorf("Status = %q", got.Status)
	}
	if got.Phases != nil {
		t.Errorf("Phases should be nil when absent, got %v", got.Phases)
	}
}

func TestListResponseDecodes(t *testing.T) {
	body := `{"items":[{"change_id":"a","status":"done"},{"change_id":"b","status":"running"}],"total":2}`
	var resp orchestratorhttp.ListChangesResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("Items len = %d", len(resp.Items))
	}
	if resp.Total != 2 {
		t.Errorf("Total = %d", resp.Total)
	}
}
