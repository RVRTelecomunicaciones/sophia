package application

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// IsLoopbackURL reports whether rawURL points at a loopback host:
// "localhost", anything in 127.0.0.0/8, or "::1" (D-M10-09). Used by
// bootstrap/auth to decide whether SOPHIA_API_KEY is required.
//
// Returns (false, nil) for a syntactically-valid non-loopback URL.
// Returns (false, error) for an unparseable URL — callers MUST treat
// this as "not loopback, auth required" to fail-safe.
func IsLoopbackURL(rawURL string) (bool, error) {
	if strings.TrimSpace(rawURL) == "" {
		return false, errors.New("networktrust: empty url")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false, fmt.Errorf("networktrust: parse url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false, fmt.Errorf("networktrust: unsupported scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return false, errors.New("networktrust: empty host")
	}
	if host == "localhost" {
		return true, nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback(), nil
	}
	return false, nil
}
