package orchestratorhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

func TestClientImplementsOrchestratorClient(t *testing.T) {
	var _ outbound.OrchestratorClient = orchestratorhttp.New(orchestratorhttp.Config{BaseURL: "http://x"})
}

func TestHealthzSucceedsOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/healthz" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	if err := c.Healthz(context.Background()); err != nil {
		t.Errorf("Healthz err: %v", err)
	}
}

func TestHealthzFailsOn500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	err := c.Healthz(context.Background())
	if err == nil {
		t.Error("expected error on 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status: %v", err)
	}
}

func TestHealthzFailsOnUnreachable(t *testing.T) {
	c := orchestratorhttp.New(orchestratorhttp.Config{
		BaseURL: "http://127.0.0.1:1",
		Timeout: 200 * time.Millisecond,
	})
	if err := c.Healthz(context.Background()); err == nil {
		t.Error("expected error on unreachable")
	}
}

func TestHealthzRespectsContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	if err := c.Healthz(ctx); err == nil {
		t.Error("expected ctx-deadline error")
	}
}
