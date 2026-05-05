package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/infrastructure/logging"
)

func TestJSONLHandlerEmitsValidLines(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewJSONLHandler(&buf, slog.LevelInfo)
	logger := slog.New(h)

	logger.Info("hello", slog.String("cmd", "doctor"))
	logger.Warn("careful", slog.Int("retries", 2))

	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid JSONL line %q: %v", line, err)
		}
		if _, ok := m["ts"]; !ok {
			t.Errorf("line missing ts: %q", line)
		}
		if _, ok := m["level"]; !ok {
			t.Errorf("line missing level: %q", line)
		}
		if _, ok := m["msg"]; !ok {
			t.Errorf("line missing msg: %q", line)
		}
	}
}

func TestJSONLHandlerRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewJSONLHandler(&buf, slog.LevelWarn)
	logger := slog.New(h)
	logger.Info("hidden")
	logger.Warn("visible")
	if !strings.Contains(buf.String(), "visible") {
		t.Error("warn line missing")
	}
	if strings.Contains(buf.String(), "hidden") {
		t.Error("info line should be filtered")
	}
}

func TestJSONLHandlerWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewJSONLHandler(&buf, slog.LevelInfo).WithAttrs([]slog.Attr{slog.String("scope", "doctor")})
	logger := slog.New(h)
	logger.Info("ok")
	if !strings.Contains(buf.String(), `"scope":"doctor"`) {
		t.Errorf("scope attr lost: %s", buf.String())
	}
}

func TestJSONLHandlerHandleContextDoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewJSONLHandler(&buf, slog.LevelInfo)
	r := slog.NewRecord(timeNowForTest(), slog.LevelInfo, "x", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
}

func timeNowForTest() time.Time {
	return time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
}
