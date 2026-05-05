package domain_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestFingerprintIs16HexChars(t *testing.T) {
	fp := domain.ComputeFingerprint("ms-cotizacion", "/home/user/repo", "git@github.com:org/repo.git")
	if len(fp) != 16 {
		t.Errorf("len(fp)=%d, want 16; fp=%q", len(fp), fp)
	}
	for _, r := range fp {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Errorf("fingerprint contains non-hex char: %q", r)
		}
	}
}

func TestFingerprintIsDeterministic(t *testing.T) {
	a := domain.ComputeFingerprint("p", "/r", "u")
	b := domain.ComputeFingerprint("p", "/r", "u")
	if a != b {
		t.Errorf("fingerprint not deterministic: %q vs %q", a, b)
	}
}

func TestFingerprintDistinguishesInputs(t *testing.T) {
	cases := [][3]string{
		{"p", "/r", "u"},
		{"p2", "/r", "u"},
		{"p", "/r2", "u"},
		{"p", "/r", "u2"},
		{"p", "/r", ""},
	}
	seen := make(map[domain.Fingerprint]struct{})
	for _, c := range cases {
		fp := domain.ComputeFingerprint(c[0], c[1], c[2])
		if _, dup := seen[fp]; dup {
			t.Errorf("collision for %v", c)
		}
		seen[fp] = struct{}{}
	}
}

func TestFingerprintHandlesEmptyRemote(t *testing.T) {
	fp := domain.ComputeFingerprint("p", "/r", "")
	if len(fp) != 16 {
		t.Errorf("empty remote should still produce a 16-char fp, got %q", fp)
	}
}

func TestFingerprintZeroValue(t *testing.T) {
	var fp domain.Fingerprint
	if fp.String() != "" {
		t.Error("zero Fingerprint should stringify to empty")
	}
}
