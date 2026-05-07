package osbrowser

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"

	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// Sentinel errors callers can match with errors.Is.
var (
	// ErrInvalidURL is returned when the URL fails to parse OR is empty.
	ErrInvalidURL = errors.New("osbrowser: invalid URL")
	// ErrInvalidScheme is returned when the URL parses but its scheme is
	// not in the http/https whitelist (spec §6.3 #3).
	ErrInvalidScheme = errors.New("osbrowser: scheme not allowed (only http/https)")
	// ErrUnsupportedPlatform is returned when runtime.GOOS has no known
	// handler dispatch.
	ErrUnsupportedPlatform = errors.New("osbrowser: unsupported platform")
)

// Config configures New.
type Config struct {
	// OSOverride lets tests pin runtime.GOOS to a specific value. Empty in
	// production. Accepts the canonical Go values: "darwin", "linux",
	// "windows", "freebsd", etc. Unknown values produce ErrUnsupportedPlatform.
	OSOverride string
}

// Browser implements outbound.Browser via OS subprocess dispatch.
type Browser struct {
	osName string
}

// New constructs a Browser. With cfg.OSOverride empty, runtime.GOOS is used.
func New(cfg Config) *Browser {
	osName := cfg.OSOverride
	if osName == "" {
		osName = runtime.GOOS
	}
	return &Browser{osName: osName}
}

// Compile-time check: Browser must satisfy outbound.Browser.
var _ outbound.Browser = (*Browser)(nil)

// Open validates the URL, then dispatches to the platform-native handler.
func (b *Browser) Open(ctx context.Context, raw string) error {
	if err := validate(raw); err != nil {
		return err
	}
	cmd, err := b.command(ctx, raw)
	if err != nil {
		return err
	}
	if err := cmd.Run(); err != nil {
		// exec.Cmd.Run wraps non-zero exit and ctx errors. We surface them
		// verbatim so callers can errors.Is(err, context.Canceled) etc.
		return fmt.Errorf("osbrowser: subprocess: %w", err)
	}
	return nil
}

// validate parses raw with net/url and enforces the scheme whitelist.
// Returns ErrInvalidURL or ErrInvalidScheme on failure.
//
// Notes:
//
//   - Empty input is ErrInvalidURL.
//   - Anything net/url rejects (control chars, malformed escapes, etc.) is
//     ErrInvalidURL.
//   - Anything that parses but has scheme != "http" / "https" is
//     ErrInvalidScheme. This includes "" (scheme-relative URLs like
//     "//example.com/x") because the policy is a strict whitelist.
func validate(raw string) error {
	if raw == "" {
		return fmt.Errorf("%w: empty", ErrInvalidURL)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	switch u.Scheme {
	case "http", "https":
		// Accept.
	default:
		return fmt.Errorf("%w: got %q", ErrInvalidScheme, u.Scheme)
	}
	if u.Host == "" {
		// "http:" with no host is not a useful target.
		return fmt.Errorf("%w: missing host", ErrInvalidURL)
	}
	return nil
}

// command constructs the exec.Cmd for the configured OS. URL has already
// been validated; we still pass it as a single argv element (no shell
// interpolation) so exec.Cmd's argv-quoting is the only path.
func (b *Browser) command(ctx context.Context, validated string) (*exec.Cmd, error) {
	switch b.osName {
	case "darwin":
		return exec.CommandContext(ctx, "open", validated), nil
	case "windows":
		// `cmd /c start "" <url>` — the empty quoted title prevents `start`
		// from treating the first quoted argument as a window title when the
		// URL contains spaces (unlikely for our gov.local URLs but kept
		// defensive). The URL is still a single argv element.
		return exec.CommandContext(ctx, "cmd", "/c", "start", "", validated), nil
	case "linux", "freebsd", "openbsd", "netbsd", "dragonfly":
		// xdg-open is the de-facto Unix opener — installed by default on
		// every modern desktop distro. Headless servers don't have it; the
		// subprocess will fail with exec: "xdg-open": executable file not
		// found in $PATH and the caller will surface the error.
		return exec.CommandContext(ctx, "xdg-open", validated), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPlatform, b.osName)
	}
}
