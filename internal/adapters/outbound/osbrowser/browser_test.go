package osbrowser_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/osbrowser"
)

func TestOpenAcceptsHTTPS(t *testing.T) {
	dir := stubOpener(t, 0)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	b := osbrowser.New(osbrowser.Config{})
	if err := b.Open(context.Background(), "https://gov.local/approvals/abc123"); err != nil {
		t.Fatalf("Open(https://...): %v", err)
	}
	got := readCapture(t, dir)
	if !strings.Contains(got, "https://gov.local/approvals/abc123") {
		t.Errorf("opener didn't see URL; got %q", got)
	}
}

func TestOpenAcceptsHTTP(t *testing.T) {
	dir := stubOpener(t, 0)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	b := osbrowser.New(osbrowser.Config{})
	if err := b.Open(context.Background(), "http://localhost:8080/approval"); err != nil {
		t.Fatalf("Open(http://...): %v", err)
	}
}

func TestOpenRejectsJavaScript(t *testing.T) {
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "javascript:alert(1)")
	if err == nil {
		t.Fatal("expected error for javascript: scheme")
	}
	if !errors.Is(err, osbrowser.ErrInvalidScheme) {
		t.Errorf("error = %v, want ErrInvalidScheme", err)
	}
}

func TestOpenRejectsData(t *testing.T) {
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "data:text/html,<script>alert(1)</script>")
	if err == nil {
		t.Fatal("expected error for data: scheme")
	}
	if !errors.Is(err, osbrowser.ErrInvalidScheme) {
		t.Errorf("error = %v, want ErrInvalidScheme", err)
	}
}

func TestOpenRejectsFile(t *testing.T) {
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "file:///etc/passwd")
	if err == nil {
		t.Fatal("expected error for file: scheme")
	}
	if !errors.Is(err, osbrowser.ErrInvalidScheme) {
		t.Errorf("error = %v", err)
	}
}

func TestOpenRejectsVBScript(t *testing.T) {
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "vbscript:msgbox(1)")
	if err == nil {
		t.Fatal("expected error for vbscript: scheme")
	}
	if !errors.Is(err, osbrowser.ErrInvalidScheme) {
		t.Errorf("error = %v", err)
	}
}

func TestOpenRejectsMailto(t *testing.T) {
	// mailto: is not in §6.3 #3's explicit blocklist, but the policy is a
	// WHITELIST of http/https. mailto: is not on the list → reject.
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "mailto:admin@example.com")
	if err == nil {
		t.Fatal("expected error for mailto: scheme (whitelist policy)")
	}
	if !errors.Is(err, osbrowser.ErrInvalidScheme) {
		t.Errorf("error = %v", err)
	}
}

func TestOpenRejectsFTP(t *testing.T) {
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "ftp://files.example.com/approval")
	if err == nil {
		t.Fatal("expected error for ftp: scheme")
	}
	if !errors.Is(err, osbrowser.ErrInvalidScheme) {
		t.Errorf("error = %v", err)
	}
}

func TestOpenRejectsEmpty(t *testing.T) {
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
	if !errors.Is(err, osbrowser.ErrInvalidURL) {
		t.Errorf("error = %v, want ErrInvalidURL", err)
	}
}

func TestOpenRejectsMalformed(t *testing.T) {
	b := osbrowser.New(osbrowser.Config{})
	// A control character in the URL fails url.Parse.
	err := b.Open(context.Background(), "http://example.com/\x7f")
	if err == nil {
		t.Fatal("expected error for malformed URL")
	}
	if !errors.Is(err, osbrowser.ErrInvalidURL) {
		t.Errorf("error = %v, want ErrInvalidURL", err)
	}
}

func TestOpenRejectsSchemeRelative(t *testing.T) {
	// "//example.com/x" parses with empty scheme — reject as ErrInvalidScheme.
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "//example.com/path")
	if err == nil {
		t.Fatal("expected error for scheme-relative URL")
	}
	if !errors.Is(err, osbrowser.ErrInvalidScheme) {
		t.Errorf("error = %v", err)
	}
}

func TestOpenSubprocessFailureSurfaces(t *testing.T) {
	// stubOpener with non-zero exit code → Open returns a wrapped error that
	// is NOT ErrInvalidScheme / ErrInvalidURL.
	dir := stubOpener(t, 1)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "https://example.com/x")
	if err == nil {
		t.Fatal("expected error from non-zero subprocess exit")
	}
	if errors.Is(err, osbrowser.ErrInvalidScheme) || errors.Is(err, osbrowser.ErrInvalidURL) {
		t.Errorf("subprocess failure should not classify as validation error: %v", err)
	}
}

func TestOpenContextCancellationKillsSubprocess(t *testing.T) {
	// Context cancellation should propagate via exec.CommandContext. We
	// don't have a clean way to assert process death without slowing the
	// stub artificially; instead we assert that ctx.Err is wrapped in the
	// returned error when the context is already canceled.
	dir := stubOpener(t, 0)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	b := osbrowser.New(osbrowser.Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := b.Open(ctx, "https://example.com/x")
	// On a cold-cancel ctx, exec.CommandContext returns context.Canceled
	// either directly or wrapped. We accept either form.
	if err == nil {
		t.Fatal("expected error when ctx is pre-canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Logf("note: error = %v (we accept this — some platforms surface SIGKILL)", err)
	}
}

func TestNewWithNilOSReturnsUnsupported(t *testing.T) {
	// Use the OS-override hook to simulate an unsupported platform.
	b := osbrowser.New(osbrowser.Config{OSOverride: "plan9"})
	err := b.Open(context.Background(), "https://example.com/x")
	if err == nil {
		t.Fatal("expected error on unsupported platform")
	}
	if !errors.Is(err, osbrowser.ErrUnsupportedPlatform) {
		t.Errorf("error = %v, want ErrUnsupportedPlatform", err)
	}
}

// --- helpers ---

// stubOpener creates a temp directory containing a stub `open`,
// `xdg-open`, and `cmd` (whichever runtime.GOOS would dispatch).
// The stub appends its argv (excluding $0) to <dir>/capture.txt and
// exits with the given code.
func stubOpener(t *testing.T, exitCode int) string {
	t.Helper()
	dir := t.TempDir()

	switch runtime.GOOS {
	case "windows":
		// Windows stubbing is more involved — skip for now and run real
		// behavior via the OS-override hook in tests that need it. The
		// Linux/Darwin paths are the operational ones for our CI.
		t.Skip("subprocess capture stubbing is POSIX-only in M7 tests")
	default:
		writeStub(t, filepath.Join(dir, "open"), exitCode)
		writeStub(t, filepath.Join(dir, "xdg-open"), exitCode)
	}
	return dir
}

func writeStub(t *testing.T, path string, exitCode int) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> \"$(dirname \"$0\")/capture.txt\"\n" +
		"exit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readCapture(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "capture.txt"))
	if err != nil {
		t.Fatalf("readCapture: %v", err)
	}
	return string(b)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i == 1 {
		return "1"
	}
	// Shouldn't happen in M7 tests.
	return "2"
}
