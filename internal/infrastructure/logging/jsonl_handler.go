package logging

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
)

type JSONLHandler struct {
	mu     *sync.Mutex
	w      io.Writer
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

func NewJSONLHandler(w io.Writer, level slog.Level) *JSONLHandler {
	return &JSONLHandler{
		mu:    &sync.Mutex{},
		w:     w,
		level: level,
	}
}

func (h *JSONLHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *JSONLHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &c
}

func (h *JSONLHandler) WithGroup(name string) slog.Handler {
	c := *h
	c.groups = append(append([]string{}, h.groups...), name)
	return &c
}

func (h *JSONLHandler) Handle(_ context.Context, r slog.Record) error {
	m := map[string]any{
		"ts":    r.Time.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		"level": r.Level.String(),
		"msg":   r.Message,
	}
	for _, a := range h.attrs {
		m[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.Any()
		return true
	})
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err = h.w.Write(b)
	return err
}
