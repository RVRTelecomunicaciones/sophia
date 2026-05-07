package composeexec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrUserEdited indicates the materialized compose.yaml diverges from the
// last embedded version. Caller should retry with reset=true.
var ErrUserEdited = errors.New("composeexec: materialized compose.yaml has been user-edited; pass --reset-compose to overwrite")

// MaterializeResult reports what Materialize did.
type MaterializeResult struct {
	Path  string
	Wrote bool
}

type composeMeta struct {
	LastEmbeddedHash string `json:"last_embedded_hash"`
}

// Materialize copies embedded bytes into <dataRoot>/compose/ following spec §3.6:
//
//   - file absent → write
//   - file == embedded → no-op
//   - file != embedded but file == last_embedded_hash → automatic upgrade
//   - file != embedded and file != last_embedded_hash → user-edited
//     · if reset=false → return ErrUserEdited
//     · if reset=true  → save current as compose.yaml.previous, write new
func Materialize(dataRoot string, embed []byte, reset bool) (MaterializeResult, error) {
	dir := filepath.Join(dataRoot, "compose")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return MaterializeResult{}, fmt.Errorf("mkdir %q: %w", dir, err)
	}
	target := filepath.Join(dir, "compose.yaml")
	metaPath := filepath.Join(dir, "compose.meta.json")
	prevPath := filepath.Join(dir, "compose.yaml.previous")

	embedHash := hashBytes(embed)

	current, err := os.ReadFile(target)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := writeFile0644(target, embed); err != nil {
			return MaterializeResult{}, err
		}
		if err := writeMeta(metaPath, composeMeta{LastEmbeddedHash: embedHash}); err != nil {
			return MaterializeResult{}, err
		}
		return MaterializeResult{Path: target, Wrote: true}, nil
	case err != nil:
		return MaterializeResult{}, fmt.Errorf("read target: %w", err)
	}

	currentHash := hashBytes(current)
	if currentHash == embedHash {
		return MaterializeResult{Path: target, Wrote: false}, nil
	}

	meta, _ := readMeta(metaPath)
	if meta.LastEmbeddedHash == currentHash {
		if err := writeFile0644(target, embed); err != nil {
			return MaterializeResult{}, err
		}
		if err := writeMeta(metaPath, composeMeta{LastEmbeddedHash: embedHash}); err != nil {
			return MaterializeResult{}, err
		}
		return MaterializeResult{Path: target, Wrote: true}, nil
	}

	if !reset {
		return MaterializeResult{Path: target}, ErrUserEdited
	}
	if err := writeFile0644(prevPath, current); err != nil {
		return MaterializeResult{}, fmt.Errorf("backup .previous: %w", err)
	}
	if err := writeFile0644(target, embed); err != nil {
		return MaterializeResult{}, err
	}
	if err := writeMeta(metaPath, composeMeta{LastEmbeddedHash: embedHash}); err != nil {
		return MaterializeResult{}, err
	}
	return MaterializeResult{Path: target, Wrote: true}, nil
}

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func writeFile0644(p string, b []byte) error {
	// #nosec G304,G306 -- path is constructed by Materialize from a caller-
	// supplied dataRoot joined with fixed filenames (compose.yaml,
	// compose.meta.json, compose.yaml.previous). No path component comes
	// from end-user input; dataRoot is the orchestrator-resolved XDG data
	// directory. 0o644 is intentional because docker compose may run as a
	// different uid in some daemon setups and must read the file.
	return os.WriteFile(p, b, 0o644) //nolint:gosec // duplicate suppression for golangci-lint v1
}

func readMeta(p string) (composeMeta, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return composeMeta{}, err
	}
	var m composeMeta
	if err := json.Unmarshal(b, &m); err != nil {
		return composeMeta{}, err
	}
	return m, nil
}

func writeMeta(p string, m composeMeta) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFile0644(p, b)
}
