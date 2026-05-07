package filestate_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/filestate"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

func newStore(t *testing.T) (*filestate.Store, string) {
	t.Helper()
	tmp := t.TempDir()
	return filestate.New(filestate.Config{StateRoot: tmp}), tmp
}

func TestStoreImplementsPort(t *testing.T) {
	var _ outbound.StateStore = filestate.New(filestate.Config{StateRoot: "/tmp"})
}

func TestProjectScopedRoundTrip(t *testing.T) {
	s, root := newStore(t)
	fp := domain.Fingerprint("abc123def456")

	id, err := s.GetLast(context.Background(), fp)
	if err != nil {
		t.Fatal(err)
	}
	if !id.IsZero() {
		t.Error("fresh store should return zero id")
	}

	if err := s.SetLast(context.Background(), fp, "01HX..."); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetLast(context.Background(), fp)
	if err != nil {
		t.Fatal(err)
	}
	if got != "01HX..." {
		t.Errorf("GetLast = %q", got)
	}

	want := filepath.Join(root, "projects", string(fp), "last_change_id")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file at %q: %v", want, err)
	}
}

func TestGlobalRoundTrip(t *testing.T) {
	s, root := newStore(t)

	if err := s.SetGlobalLast(context.Background(), "g1"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetGlobalLast(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "g1" {
		t.Errorf("got %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "last_change_id")); err != nil {
		t.Errorf("expected global file: %v", err)
	}
}

func TestSetLastIsAtomic(t *testing.T) {
	s, root := newStore(t)
	fp := domain.Fingerprint("fp1")
	if err := s.SetLast(context.Background(), fp, "01H..."); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "projects", string(fp))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == "last_change_id.tmp" {
			t.Error("temp file leftover after SetLast")
		}
	}
}

func TestDirectoriesUse0700(t *testing.T) {
	s, root := newStore(t)
	fp := domain.Fingerprint("fp700")
	_ = s.SetLast(context.Background(), fp, "01H")
	dir := filepath.Join(root, "projects", string(fp))
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir perm = %o, want 0700", perm)
	}
}

func TestMetaJSONWritten(t *testing.T) {
	s, root := newStore(t)
	fp := domain.Fingerprint("fpmeta")
	meta := filestate.ProjectMeta{
		Project:   "ms-cotizacion",
		RepoRoot:  "/home/u/repo",
		RemoteURL: "git@github.com:org/repo.git",
	}
	if err := s.WriteProjectMeta(context.Background(), fp, meta); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadProjectMeta(context.Background(), fp)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != meta.Project || got.RemoteURL != meta.RemoteURL {
		t.Errorf("meta round-trip lost: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, "projects", string(fp), "meta.json")); err != nil {
		t.Errorf("expected meta.json: %v", err)
	}
}
