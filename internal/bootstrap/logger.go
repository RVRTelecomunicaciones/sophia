package bootstrap

import (
	"io"
	"log/slog"

	"github.com/RVRTelecomunicaciones/sophia/internal/infrastructure/logging"
)

func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(logging.NewJSONLHandler(w, level))
}
