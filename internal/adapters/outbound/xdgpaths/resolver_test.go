package xdgpaths_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/xdgpaths"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

func TestResolverImplementsPort(t *testing.T) {
	var _ outbound.PathResolver = xdgpaths.New(xdgpaths.Config{})
}

func TestResolveHonorsXDGEnv(t *testing.T) {
	r := xdgpaths.New(xdgpaths.Config{
		Env: map[string]string{
			"XDG_CONFIG_HOME": "/c",
			"XDG_STATE_HOME":  "/s",
			"XDG_DATA_HOME":   "/d",
			"XDG_CACHE_HOME":  "/x",
			"HOME":            "/home/u",
		},
	})
	p, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if p.ConfigRoot != "/c/sophia" {
		t.Errorf("ConfigRoot = %q", p.ConfigRoot)
	}
	if p.StateRoot != "/s/sophia" {
		t.Errorf("StateRoot = %q", p.StateRoot)
	}
	if p.DataRoot != "/d/sophia" {
		t.Errorf("DataRoot = %q", p.DataRoot)
	}
	if p.CacheRoot != "/x/sophia" {
		t.Errorf("CacheRoot = %q", p.CacheRoot)
	}
}

func TestResolveDefaultsFromHome(t *testing.T) {
	r := xdgpaths.New(xdgpaths.Config{
		Env: map[string]string{"HOME": "/home/u"},
	})
	p, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"ConfigRoot": p.ConfigRoot,
		"StateRoot":  p.StateRoot,
		"DataRoot":   p.DataRoot,
		"CacheRoot":  p.CacheRoot,
	}
	for k, v := range cases {
		if !strings.HasPrefix(v, "/home/u/") {
			t.Errorf("%s = %q does not start under HOME", k, v)
		}
		if !strings.HasSuffix(v, "/sophia") {
			t.Errorf("%s = %q does not end with /sophia", k, v)
		}
	}
}

func TestResolveErrorsWithoutHome(t *testing.T) {
	r := xdgpaths.New(xdgpaths.Config{Env: map[string]string{}})
	if _, err := r.Resolve(); err == nil {
		t.Error("expected error when HOME unset and no XDG vars")
	}
}

func TestEnsureDirsCreatesAt0700(t *testing.T) {
	tmp := t.TempDir()
	r := xdgpaths.New(xdgpaths.Config{Env: map[string]string{"HOME": tmp}})
	p, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.EnsureDirs(p); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{p.ConfigRoot, p.StateRoot, p.DataRoot, p.CacheRoot} {
		fi, err := os.Stat(d)
		if err != nil {
			t.Fatalf("dir %q missing: %v", d, err)
		}
		if perm := fi.Mode().Perm(); perm != 0o700 {
			t.Errorf("%q perm = %o, want 0700", d, perm)
		}
	}
}

func TestValidateDirsReturnsErrorOnMissing(t *testing.T) {
	tmp := t.TempDir()
	r := xdgpaths.New(xdgpaths.Config{Env: map[string]string{"HOME": tmp}})
	p, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateDirs(p); err == nil {
		t.Error("expected error before EnsureDirs")
	}
	if err := r.EnsureDirs(p); err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateDirs(p); err != nil {
		t.Errorf("expected nil after EnsureDirs, got %v", err)
	}
}

func TestValidateDirsRejectsLoosePerms(t *testing.T) {
	tmp := t.TempDir()
	r := xdgpaths.New(xdgpaths.Config{Env: map[string]string{"HOME": tmp}})
	p, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p.ConfigRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(p.StateRoot, 0o700)
	_ = os.MkdirAll(p.DataRoot, 0o700)
	_ = os.MkdirAll(p.CacheRoot, 0o700)
	if err := r.ValidateDirs(p); err == nil {
		t.Error("expected error on loose perms")
	}
}

func TestPathsDoNotEscapeHome(t *testing.T) {
	tmp := t.TempDir()
	r := xdgpaths.New(xdgpaths.Config{Env: map[string]string{"HOME": tmp}})
	p, _ := r.Resolve()
	abs, _ := filepath.Abs(tmp)
	for _, d := range []string{p.ConfigRoot, p.StateRoot, p.DataRoot, p.CacheRoot} {
		if !strings.HasPrefix(d, abs) {
			t.Errorf("%q escapes HOME=%q", d, abs)
		}
	}
}
