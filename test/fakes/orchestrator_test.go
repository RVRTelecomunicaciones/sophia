package fakes_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func TestFakeOrchestratorImplementsClient(t *testing.T) {
	var _ outbound.OrchestratorClient = (*fakes.FakeOrchestrator)(nil)
}

func TestFakeOrchestratorHealthz(t *testing.T) {
	c := fakes.NewFakeOrchestrator()
	if err := c.Healthz(context.Background()); err != nil {
		t.Fatalf("Healthz err: %v", err)
	}
	c.HealthzErr = errors.New("down")
	if err := c.Healthz(context.Background()); err == nil {
		t.Error("expected error")
	}
}

func TestFakeOrchestratorCreateAndGet(t *testing.T) {
	c := fakes.NewFakeOrchestrator()
	change, err := c.CreateChange(context.Background(), outbound.CreateChangeInput{Name: "n", Project: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if change.ID.IsZero() {
		t.Fatal("expected non-zero id")
	}
	got, err := c.GetChange(context.Background(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "n" || got.Project != "p" {
		t.Errorf("round-trip lost fields: %+v", got)
	}
}

func TestFakeOrchestratorListFilters(t *testing.T) {
	c := fakes.NewFakeOrchestrator()
	c.SeedChange(&domain.Change{ID: "a", Project: "x", Status: domain.ChangeStatusRunning})
	c.SeedChange(&domain.Change{ID: "b", Project: "y", Status: domain.ChangeStatusDone})
	got, err := c.ListChanges(context.Background(), outbound.ListChangesFilter{Project: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("filter failed: %+v", got)
	}
}
