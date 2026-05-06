package orchestratorhttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

func TestCreateChangePostsBodyAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q", r.Method)
		}
		if r.URL.Path != "/api/v1/changes" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q", ct)
		}
		var got orchestratorhttp.CreateChangeRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.Name != "msg" || got.Project != "p" || got.BaseRef != "main" || got.ArtifactStoreMode != "engram" {
			t.Errorf("body = %+v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"change_id":"01HX","name":"msg","project":"p","status":"pending"}`)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	got, err := c.CreateChange(context.Background(), outbound.CreateChangeInput{
		Name: "msg", Project: "p", BaseRef: "main", ArtifactStoreMode: "engram",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID.String() != "01HX" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.Status != domain.ChangeStatusPending {
		t.Errorf("Status = %q", got.Status)
	}
}

func TestCreateChangeMapsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":"down"}`)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	_, err := c.CreateChange(context.Background(), outbound.CreateChangeInput{Name: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrUnreachable) {
		t.Errorf("expected ErrUnreachable, got %v", err)
	}
}

func TestGetChangeFetchesAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/changes/01HX" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"change_id":"01HX","status":"running","current_phase_id":"phase-1"}`)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	got, err := c.GetChange(context.Background(), domain.ChangeID("01HX"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.ChangeStatusRunning {
		t.Errorf("Status = %q", got.Status)
	}
	if got.CurrentPhaseID != "phase-1" {
		t.Errorf("CurrentPhaseID = %q", got.CurrentPhaseID)
	}
}

func TestGetChangeNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	_, err := c.GetChange(context.Background(), domain.ChangeID("nope"))
	if !errors.Is(err, domain.ErrChangeNotFound) {
		t.Errorf("expected ErrChangeNotFound, got %v", err)
	}
}

func TestListChangesQueryString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.ParseQuery(r.URL.RawQuery)
		if q.Get("project") != "p" {
			t.Errorf("project = %q", q.Get("project"))
		}
		if q.Get("status") != "running" {
			t.Errorf("status = %q", q.Get("status"))
		}
		if q.Get("limit") != "5" {
			t.Errorf("limit = %q", q.Get("limit"))
		}
		if q.Get("offset") != "10" {
			t.Errorf("offset = %q", q.Get("offset"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[{"change_id":"a","status":"running"}],"total":1}`)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	got, err := c.ListChanges(context.Background(), outbound.ListChangesFilter{
		Project: "p", Status: "running", Limit: 5, Offset: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID.String() != "a" {
		t.Errorf("got = %+v", got)
	}
}

func TestListChangesNoFiltersOmitsEmptyParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.ParseQuery(r.URL.RawQuery)
		if q.Get("project") != "" {
			t.Errorf("project should be empty, got %q", q.Get("project"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[],"total":0}`)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	if _, err := c.ListChanges(context.Background(), outbound.ListChangesFilter{}); err != nil {
		t.Fatal(err)
	}
}

func TestDoJSONRejectsOversizedBody(t *testing.T) {
	// Server sends body larger than the 1 MiB cap.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Write 1 MiB + 1 byte of valid-looking JSON wrapper. The shape doesn't
		// matter — we expect to fail before decode.
		w.WriteHeader(http.StatusOK)
		// Pad inside a JSON string so it stays valid-ish if it WERE truncated cleanly.
		_, _ = io.WriteString(w, `{"change_id":"x","name":"`)
		_, _ = w.Write(bytes.Repeat([]byte("a"), 1<<20))
		_, _ = io.WriteString(w, `"}`)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	_, err := c.GetChange(context.Background(), domain.ChangeID("x"))
	if err == nil {
		t.Fatal("expected error on oversized body")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected 'exceeds' in error, got: %v", err)
	}
}
