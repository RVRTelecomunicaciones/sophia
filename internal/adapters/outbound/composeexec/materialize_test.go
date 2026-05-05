package composeexec_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/composeexec"
)

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestMaterializeWritesWhenAbsent(t *testing.T) {
	root := t.TempDir()
	res, err := composeexec.Materialize(root, composeexec.EmbeddedComposeYAML, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Wrote {
		t.Error("expected Wrote=true on first run")
	}
	got, err := os.ReadFile(filepath.Join(root, "compose", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if sha(got) != sha(composeexec.EmbeddedComposeYAML) {
		t.Error("file content does not match embedded")
	}
}

func TestMaterializeNoOpWhenSame(t *testing.T) {
	root := t.TempDir()
	_, _ = composeexec.Materialize(root, composeexec.EmbeddedComposeYAML, false)
	res, err := composeexec.Materialize(root, composeexec.EmbeddedComposeYAML, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Wrote {
		t.Error("expected Wrote=false on re-run with same bytes")
	}
}

func TestMaterializeAbortsOnUserEdit(t *testing.T) {
	root := t.TempDir()
	_, _ = composeexec.Materialize(root, composeexec.EmbeddedComposeYAML, false)

	target := filepath.Join(root, "compose", "compose.yaml")
	if err := os.WriteFile(target, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	newEmbed := append([]byte{}, composeexec.EmbeddedComposeYAML...)
	newEmbed = append(newEmbed, []byte("\n# upgrade\n")...)
	_, err := composeexec.Materialize(root, newEmbed, false)
	if err == nil {
		t.Fatal("expected ErrUserEdited when file diverges and reset=false")
	}
	if err != composeexec.ErrUserEdited {
		t.Errorf("expected ErrUserEdited, got %v", err)
	}
}

func TestMaterializeBacksUpAndOverwritesWhenReset(t *testing.T) {
	root := t.TempDir()
	_, _ = composeexec.Materialize(root, composeexec.EmbeddedComposeYAML, false)

	target := filepath.Join(root, "compose", "compose.yaml")
	_ = os.WriteFile(target, []byte("user-edit\n"), 0o644)

	newEmbed := append([]byte{}, composeexec.EmbeddedComposeYAML...)
	newEmbed = append(newEmbed, []byte("\n# v2\n")...)
	res, err := composeexec.Materialize(root, newEmbed, true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Wrote {
		t.Error("expected Wrote=true with reset=true")
	}
	prev, err := os.ReadFile(filepath.Join(root, "compose", "compose.yaml.previous"))
	if err != nil {
		t.Fatalf("expected .previous backup: %v", err)
	}
	if string(prev) != "user-edit\n" {
		t.Errorf(".previous = %q", prev)
	}
	got, _ := os.ReadFile(target)
	if sha(got) != sha(newEmbed) {
		t.Error("target was not overwritten with new bytes")
	}
}

func TestMaterializeUpgradePathDetectsLastEmbeddedHash(t *testing.T) {
	root := t.TempDir()
	v1 := composeexec.EmbeddedComposeYAML
	_, _ = composeexec.Materialize(root, v1, false)

	v2 := append([]byte{}, v1...)
	v2 = append(v2, []byte("\n# v2\n")...)
	res, err := composeexec.Materialize(root, v2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Wrote {
		t.Error("expected automatic upgrade when last_embedded matches current file")
	}
}
