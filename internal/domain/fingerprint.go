package domain

import (
	"crypto/sha256"
	"encoding/hex"
)

type Fingerprint string

func (f Fingerprint) String() string { return string(f) }

func ComputeFingerprint(projectName, repoRoot, remoteURL string) Fingerprint {
	h := sha256.New()
	h.Write([]byte(projectName))
	h.Write([]byte{0})
	h.Write([]byte(repoRoot))
	h.Write([]byte{0})
	h.Write([]byte(remoteURL))
	return Fingerprint(hex.EncodeToString(h.Sum(nil))[:16])
}
