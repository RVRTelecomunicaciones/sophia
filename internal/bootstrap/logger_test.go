package bootstrap_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/bootstrap"
)

func TestNewLoggerWritesJSONL(t *testing.T) {
	var buf bytes.Buffer
	logger := bootstrap.NewLogger(&buf, slog.LevelInfo)
	logger.Info("hello", slog.String("k", "v"))
	if !strings.Contains(buf.String(), `"msg":"hello"`) {
		t.Errorf("expected JSONL output, got %q", buf.String())
	}
}
