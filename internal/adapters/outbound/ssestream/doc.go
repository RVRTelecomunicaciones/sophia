// Package ssestream implements outbound.EventStreamClient by consuming the
// orchestrator's Server-Sent Events feed. It exposes a single Client that
// composes four internal pieces:
//
//   - parser:   tolerant SSE-event → domain.Event translation (§5.3, §5.4)
//   - redactor: strips secrets from event payloads before sinks see them (§6.3)
//   - reconnect/backoff/watchdog: 1s→30s exponential backoff, 5 retries
//     per phase, 60s heartbeat watchdog, Last-Event-ID resumption (§5.7)
//   - client:   wires the above on top of github.com/tmaxmax/go-sse
//
// The package is M5 scope only. Spec §7.2 lists the M5 DoD.
package ssestream
