// Package contract holds the cross-repo wire-conformance test suite for
// sophia-wire-v1. Tests in this package fall into two tiers:
//
//   - Lightweight invariants (this file) that run in the default
//     `make test` so every CI run catches drift between the cli and
//     orchestrator spec mirrors.
//   - Build-tag `contract` tests that boot a synthetic spec-conformant
//     orchestrator and exercise the cli's outbound HTTP + SSE clients
//     against it. Run via `make contract`.
//
// Phase 5 / D-M10-16 release blocker: the cli's
// docs/specs/sophia-wire-v1.sha256 MUST equal the orchestrator's
// equivalent file at the to-be-tagged commit. This file enforces that
// invariant locally; CI enforcement lands in Phase 7.
package contract_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// EnvOrchRepo names the env var clients can set to point at the
// orchestrator repo when its checkout lives outside `../sophia-orchestator`.
const EnvOrchRepo = "SOPHIA_ORCHESTRATOR_REPO"

// defaultOrchRepoRel is the conventional sister-checkout location.
// Both repos are typically cloned side-by-side under the same parent
// directory.
const defaultOrchRepoRel = "../sophia-orchestator"

// TestSpecChecksum_LocalRecordedMatchesActual asserts that the cli
// repo's sophia-wire-v1.sha256 matches a freshly-computed digest of
// sophia-wire-v1.md. A mismatch means the .sha256 file was not
// regenerated after a spec edit; the release blocker (D-M10-16) fires.
func TestSpecChecksum_LocalRecordedMatchesActual(t *testing.T) {
	cliRoot := repoRoot(t)
	specPath := filepath.Join(cliRoot, "docs/specs/sophia-wire-v1.md")
	checksumPath := filepath.Join(cliRoot, "docs/specs/sophia-wire-v1.sha256")

	want := readFirstField(t, checksumPath)
	got := sha256OfFile(t, specPath)

	if want != got {
		t.Fatalf("cli spec checksum drift\n  recorded: %s\n  actual:   %s\n  → run `shasum -a 256 %s > %s`",
			want, got, specPath, checksumPath)
	}
}

// TestSpecChecksum_CrossRepoMatchesOrchestrator asserts the cli and
// orchestrator carry byte-identical sophia-wire-v1.md mirrors. This is
// the strongest cross-repo invariant we can check without booting both
// processes — the spec IS the contract (D-M10-04).
//
// SKIPS when the orchestrator repo is not findable; CI is expected to
// either set SOPHIA_ORCHESTRATOR_REPO or check out both repos
// side-by-side.
func TestSpecChecksum_CrossRepoMatchesOrchestrator(t *testing.T) {
	orchRoot := orchestratorRepo(t)
	if orchRoot == "" {
		t.Skip("orchestrator repo not found; set SOPHIA_ORCHESTRATOR_REPO or check out side-by-side at " + defaultOrchRepoRel)
	}

	cliRoot := repoRoot(t)
	cliSpec := filepath.Join(cliRoot, "docs/specs/sophia-wire-v1.md")
	orchSpec := filepath.Join(orchRoot, "docs/specs/sophia-wire-v1.md")

	cliHash := sha256OfFile(t, cliSpec)
	orchHash := sha256OfFile(t, orchSpec)

	if cliHash != orchHash {
		t.Fatalf("cross-repo spec divergence\n  cli  (%s): %s\n  orch (%s): %s\n  → re-mirror the spec and regenerate the .sha256 files",
			cliSpec, cliHash, orchSpec, orchHash)
	}

	// Also assert each repo's recorded .sha256 matches the actual
	// content. A repo can't merge with a stale .sha256 even if the
	// content matches the other repo.
	cliRecorded := readFirstField(t, filepath.Join(cliRoot, "docs/specs/sophia-wire-v1.sha256"))
	orchRecorded := readFirstField(t, filepath.Join(orchRoot, "docs/specs/sophia-wire-v1.sha256"))
	if cliRecorded != cliHash {
		t.Errorf("cli .sha256 file is stale: recorded=%s, actual=%s", cliRecorded, cliHash)
	}
	if orchRecorded != orchHash {
		t.Errorf("orch .sha256 file is stale: recorded=%s, actual=%s", orchRecorded, orchHash)
	}
}

// repoRoot returns the absolute path to the cli repo root. The test is
// invoked from `test/contract/`, so we walk two levels up.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root, err := filepath.Abs(filepath.Join(wd, "..", ".."))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return root
}

// orchestratorRepo resolves the orchestrator repo path. Honours
// SOPHIA_ORCHESTRATOR_REPO; falls back to ../sophia-orchestator
// relative to the cli repo. Returns "" when neither exists.
func orchestratorRepo(t *testing.T) string {
	t.Helper()
	if env := strings.TrimSpace(os.Getenv(EnvOrchRepo)); env != "" {
		if abs, err := filepath.Abs(env); err == nil {
			if _, err := os.Stat(filepath.Join(abs, "go.mod")); err == nil {
				return abs
			}
		}
	}
	cliRoot := repoRoot(t)
	candidate, err := filepath.Abs(filepath.Join(cliRoot, defaultOrchRepoRel))
	if err != nil {
		return ""
	}
	if _, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil {
		return candidate
	}
	return ""
}

func sha256OfFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// readFirstField returns the first whitespace-delimited token of the
// first non-empty line of the file at path. This matches the
// `shasum -a 256` output format: "<hex>  <path>".
func readFirstField(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		return fields[0]
	}
	t.Fatalf("no checksum line in %s", path)
	return ""
}

// init exists only so go vet sees the package even with all tests
// guarded by build tags or skip conditions.
var _ = fmt.Sprintf
