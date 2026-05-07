package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func newLister(orch *fakes.FakeOrchestrator) *application.Lister {
	return application.NewLister(application.ListerDeps{Orch: orch})
}

func TestListerForwardsFilters(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	orch.SeedChange(&domain.Change{ID: "01H1", Project: "p1", Status: domain.ChangeStatusRunning})
	orch.SeedChange(&domain.Change{ID: "01H2", Project: "p2", Status: domain.ChangeStatusDone})
	orch.SeedChange(&domain.Change{ID: "01H3", Project: "p1", Status: domain.ChangeStatusDone})

	l := newLister(orch)
	got, err := l.List(context.Background(), application.ListInput{
		Project: "p1",
		Status:  string(domain.ChangeStatusDone),
		Limit:   10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 change, got %d", len(got))
	}
	if got[0].ID != "01H3" {
		t.Errorf("ID = %q, want 01H3", got[0].ID)
	}
}

func TestListerEmptyProjectMeansNoFilter(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	orch.SeedChange(&domain.Change{ID: "01H1", Project: "p1", Status: domain.ChangeStatusRunning})
	orch.SeedChange(&domain.Change{ID: "01H2", Project: "p2", Status: domain.ChangeStatusRunning})

	l := newLister(orch)
	// Project="" → no project filter is forwarded. Lister never invents a
	// default; the CLI is responsible for resolving project defaults before
	// calling List.
	got, err := l.List(context.Background(), application.ListInput{
		Project: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 changes (no project filter), got %d", len(got))
	}
}

func TestListerDefaultLimitIs10(t *testing.T) {
	// The Lister itself does NOT impose a default — the CLI layer does.
	// This test asserts that List(Limit=0) forwards Limit=0 to the orchestrator.
	orch := fakes.NewFakeOrchestrator()
	var seen outbound.ListChangesFilter
	orch.OnListChanges = func(f outbound.ListChangesFilter) {
		seen = f
	}
	orch.SeedChange(&domain.Change{ID: "01H1", Project: "p"})

	l := newLister(orch)
	if _, err := l.List(context.Background(), application.ListInput{Project: "p"}); err != nil {
		t.Fatal(err)
	}
	if seen.Limit != 0 {
		t.Errorf("Limit forwarded as %d; expected 0 (cli applies default, not Lister)", seen.Limit)
	}
}

func TestListerSurfacesOrchestratorError(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	orch.ListErr = errors.New("boom")

	l := newLister(orch)
	_, err := l.List(context.Background(), application.ListInput{Project: "p"})
	if err == nil {
		t.Fatal("expected error from orchestrator")
	}
}
