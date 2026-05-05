package domain

// XDGPaths is the resolved set of XDG-style roots scoped to "sophia".
//
// Defaults (all platforms) when env vars are unset:
//   ConfigRoot = ~/.config/sophia
//   StateRoot  = ~/.local/state/sophia
//   DataRoot   = ~/.local/share/sophia
//   CacheRoot  = ~/.cache/sophia          (reserved for V1.1)
//
// On macOS, when XDG vars are not set, the CLI defaults to the same
// Linux-style paths for cross-platform consistency. This is documented
// in --help and reported by `sophia doctor`.
type XDGPaths struct {
	ConfigRoot string
	StateRoot  string
	DataRoot   string
	CacheRoot  string
}
