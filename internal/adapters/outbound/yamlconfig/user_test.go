package yamlconfig_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/yamlconfig"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/spf13/afero"
)

func TestUserStoreImplementsPort(t *testing.T) {
	var _ outbound.UserConfigStore = yamlconfig.NewUserStore(yamlconfig.UserConfig{FS: afero.NewMemMapFs()})
}

func TestUserStoreRoundTrip(t *testing.T) {
	fs := afero.NewMemMapFs()
	s := yamlconfig.NewUserStore(yamlconfig.UserConfig{FS: fs})
	cfg := &domain.UserConfig{Version: 1, OrchestratorURL: "http://localhost:9080", TimeoutSeconds: 30}
	if err := s.Write(context.Background(), "/cfg/config.yaml", cfg); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(context.Background(), "/cfg/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got.OrchestratorURL != "http://localhost:9080" || got.TimeoutSeconds != 30 {
		t.Errorf("round-trip lost: %+v", got)
	}
}

func TestUserStoreReadMissing(t *testing.T) {
	fs := afero.NewMemMapFs()
	s := yamlconfig.NewUserStore(yamlconfig.UserConfig{FS: fs})
	_, err := s.Read(context.Background(), "/cfg/config.yaml")
	if err != domain.ErrConfigMissing {
		t.Errorf("expected ErrConfigMissing, got %v", err)
	}
}

func TestUserStoreWriteUses0600(t *testing.T) {
	fs := afero.NewMemMapFs()
	s := yamlconfig.NewUserStore(yamlconfig.UserConfig{FS: fs})
	cfg := &domain.UserConfig{Version: 1, OrchestratorURL: "http://x", TimeoutSeconds: 5}
	if err := s.Write(context.Background(), "/cfg/config.yaml", cfg); err != nil {
		t.Fatal(err)
	}
	fi, err := fs.Stat("/cfg/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 0600", perm)
	}
}
