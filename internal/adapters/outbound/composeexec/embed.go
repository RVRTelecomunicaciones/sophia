package composeexec

import (
	_ "embed"
)

// EmbeddedComposeYAML is the bytes of the V1 dev compose stub.
//
//go:embed embedded/compose.yaml
var EmbeddedComposeYAML []byte
