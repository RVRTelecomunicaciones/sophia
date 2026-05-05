package outbound

import "context"

// SSEProber probes whether an SSE endpoint is reachable and accepts
// `text/event-stream`. It does NOT consume events; the real consumer
// (M5) lives behind EventStreamClient.
type SSEProber interface {
	Probe(ctx context.Context) error
}
