package domain

import "time"

type Event struct {
	Type      string
	Timestamp time.Time
	Payload   map[string]any
	TraceID   string
	EventID   string
}
