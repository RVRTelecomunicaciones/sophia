package application_test

import (
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// envFromMap returns a programmable os.Getenv replacement for tests so
// the real $SOPHIA_API_KEY is never observed.
func envFromMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestAPIKeyResolver_FlagWinsOverEnv(t *testing.T) {
	r := application.NewAPIKeyResolver("flag-key", envFromMap(map[string]string{
		"SOPHIA_API_KEY": "env-key",
	}))
	if got := r.Resolve(); got != "flag-key" {
		t.Errorf("Resolve = %q, want flag-key", got)
	}
}

func TestAPIKeyResolver_FallsBackToEnv(t *testing.T) {
	r := application.NewAPIKeyResolver("", envFromMap(map[string]string{
		"SOPHIA_API_KEY": "env-key",
	}))
	if got := r.Resolve(); got != "env-key" {
		t.Errorf("Resolve = %q, want env-key", got)
	}
}

func TestAPIKeyResolver_EmptyWhenUnset(t *testing.T) {
	r := application.NewAPIKeyResolver("", envFromMap(nil))
	if r.HasKey() {
		t.Error("HasKey true with no flag/env")
	}
}

func TestAPIKeyResolver_AuthorizeAllowsLoopbackAnon(t *testing.T) {
	r := application.NewAPIKeyResolver("", envFromMap(nil))
	cases := []string{
		"http://localhost:9080",
		"http://127.0.0.1:9080",
		"http://[::1]:9080",
	}
	for _, url := range cases {
		if err := r.Authorize(url); err != nil {
			t.Errorf("loopback %q anon should succeed, got %v", url, err)
		}
	}
}

func TestAPIKeyResolver_AuthorizeRejectsRemoteAnon(t *testing.T) {
	r := application.NewAPIKeyResolver("", envFromMap(nil))
	err := r.Authorize("https://orch.example.com")
	if !errors.Is(err, domain.ErrAuthRequired) {
		t.Errorf("remote anon should ErrAuthRequired, got %v", err)
	}
}

func TestAPIKeyResolver_AuthorizeAllowsRemoteWithKey(t *testing.T) {
	r := application.NewAPIKeyResolver("key", envFromMap(nil))
	if err := r.Authorize("https://orch.example.com"); err != nil {
		t.Errorf("remote with key should succeed, got %v", err)
	}
}

func TestAPIKeyResolver_AuthorizeFailsClosedOnBadURL(t *testing.T) {
	r := application.NewAPIKeyResolver("", envFromMap(nil))
	if err := r.Authorize(":://broken"); !errors.Is(err, domain.ErrAuthRequired) {
		t.Errorf("unparseable URL should ErrAuthRequired (fail-closed), got %v", err)
	}
}

func TestIsLoopbackURL(t *testing.T) {
	cases := []struct {
		url     string
		want    bool
		wantErr bool
	}{
		{"http://localhost:9080", true, false},
		{"http://127.0.0.1", true, false},
		{"http://127.5.5.5", true, false},
		{"http://[::1]:8080", true, false},
		{"https://example.com", false, false},
		{"http://10.0.0.1", false, false},
		{"", false, true},
		{"://nope", false, true},
	}
	for _, tc := range cases {
		got, err := application.IsLoopbackURL(tc.url)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%q: want error, got nil", tc.url)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error %v", tc.url, err)
		}
		if got != tc.want {
			t.Errorf("%q: got %v, want %v", tc.url, got, tc.want)
		}
	}
}
