package tui

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
)

// DefaultBufferCapacity is the cap-256 buffer mandated by spec §4.5.
const DefaultBufferCapacity = 256

// Sender abstracts the subset of *bubbletea.Program that the bridge needs.
// Production code wraps tea.Program so its Send(tea.Msg) satisfies this.
// Tests inject a fake to avoid booting a real terminal program.
type Sender interface {
	Send(m any)
}

// BridgeConfig configures the Bridge.
type BridgeConfig struct {
	Sender         Sender
	BufferCapacity int // 0 ⇒ DefaultBufferCapacity
}

// DropCategory tags drop counters per spec §4.5 priority bucket.
type DropCategory string

const (
	DropCategoryHeartbeat DropCategory = "heartbeat"
	DropCategoryAgentTask DropCategory = "agent_task"
	DropCategoryOther     DropCategory = "other"
)

// SnapshotMsg is dispatched on the Bubble Tea event loop when the SSE feed
// delivers a new domain.Change snapshot.
type SnapshotMsg struct {
	Change *domain.Change
}

// EventMsg is dispatched on the Bubble Tea event loop for every non-heartbeat
// domain.Event that survives the drop policy.
type EventMsg struct {
	Event domain.Event
}

// ApprovalGateMsg is dispatched when the runner translates an
// approval.required event into a structured ApprovalGate.
type ApprovalGateMsg struct {
	Gate domain.ApprovalGate
}

// ErrorMsg is dispatched for non-fatal errors the runner reports via OnError.
type ErrorMsg struct {
	Err error
}

// CompleteMsg is dispatched when the runner reaches a terminal status.
type CompleteMsg struct {
	Status domain.ChangeStatus
}

// queued is the internal envelope held in the bridge's in-memory queue.
type queued struct {
	category DropCategory
	priority bool
	msg      any
}

// Bridge is the cap-256 buffered EventSink that forwards events into a
// Bubble Tea program. Implements inbound.EventSink.
//
// Drop policy (spec §4.5):
//
//   - Buffer cap 256.
//   - At cap, new heartbeat is dropped wholesale.
//   - At cap, new phase.*/approval.* event (priority=true) is queued by
//     EVICTING the oldest non-priority entry from the buffer.
//   - At cap where every slot is already priority, grow past cap rather than
//     drop the incoming priority event.
//   - At cap, new agent.*/task.*/other non-priority event is dropped.
//
// Close() signals the worker to stop and drains any remaining queue entries.
// If the sender is wedged (blocking in Send), Close() does not deadlock —
// it marks the bridge closed (stopping new enqueues) and returns. The worker
// goroutine will exit on its own once Send unblocks or the process exits.
type Bridge struct {
	sender Sender
	cap    int

	mu     sync.Mutex
	queue  []queued
	closed bool
	cond   *sync.Cond

	totalDrops atomic.Uint64
	dropHB     atomic.Uint64
	dropAT     atomic.Uint64
	dropOther  atomic.Uint64

	// done is closed by Close() to signal the worker. Unlike wg.Wait(), reading
	// from done never blocks even when the worker is stuck in sender.Send.
	done chan struct{}
}

// NewBridge constructs a Bridge and starts its forwarding worker.
func NewBridge(cfg BridgeConfig) *Bridge {
	if cfg.BufferCapacity <= 0 {
		cfg.BufferCapacity = DefaultBufferCapacity
	}
	b := &Bridge{
		sender: cfg.Sender,
		cap:    cfg.BufferCapacity,
		queue:  make([]queued, 0, cfg.BufferCapacity),
		done:   make(chan struct{}),
	}
	b.cond = sync.NewCond(&b.mu)
	go b.worker()
	return b
}

func (b *Bridge) OnSnapshot(_ context.Context, c *domain.Change) error {
	cp := *c
	if len(c.Phases) > 0 {
		cp.Phases = make([]domain.Phase, len(c.Phases))
		copy(cp.Phases, c.Phases)
	}
	b.enqueue(queued{category: DropCategoryOther, priority: true, msg: SnapshotMsg{Change: &cp}})
	return nil
}

func (b *Bridge) OnEvent(_ context.Context, ev domain.Event) error {
	cat, prio := classify(ev.Type)
	b.enqueue(queued{category: cat, priority: prio, msg: EventMsg{Event: ev}})
	return nil
}

func (b *Bridge) OnApprovalGate(_ context.Context, g domain.ApprovalGate) error {
	b.enqueue(queued{category: DropCategoryOther, priority: true, msg: ApprovalGateMsg{Gate: g}})
	return nil
}

func (b *Bridge) OnError(_ context.Context, err error) error {
	b.enqueue(queued{category: DropCategoryOther, priority: true, msg: ErrorMsg{Err: err}})
	return nil
}

func (b *Bridge) OnComplete(_ context.Context, st domain.ChangeStatus) error {
	b.enqueue(queued{category: DropCategoryOther, priority: true, msg: CompleteMsg{Status: st}})
	return nil
}

// Close stops the forwarding worker. Idempotent.
//
// Close marks the bridge as closed (preventing new enqueues) and signals the
// worker to exit. If the underlying Sender is currently blocking in Send,
// Close returns without waiting — the worker will exit on its own once Send
// unblocks. This avoids a deadlock when the caller holds a wedged sender.
func (b *Bridge) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.cond.Broadcast()
	b.mu.Unlock()
	close(b.done)
	return nil
}

// Drops returns the total number of dropped events across all categories.
func (b *Bridge) Drops() uint64 { return b.totalDrops.Load() }

// DropsByCategory returns a snapshot of per-category drop counters.
func (b *Bridge) DropsByCategory() map[DropCategory]uint64 {
	return map[DropCategory]uint64{
		DropCategoryHeartbeat: b.dropHB.Load(),
		DropCategoryAgentTask: b.dropAT.Load(),
		DropCategoryOther:     b.dropOther.Load(),
	}
}

// Pending reports the queue depth — used by tests to wait for a drain.
func (b *Bridge) Pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.queue)
}

// classify maps an event Type to its DropCategory and priority flag per §4.5.
func classify(eventType string) (DropCategory, bool) {
	switch {
	case eventType == "heartbeat":
		return DropCategoryHeartbeat, false
	case strings.HasPrefix(eventType, "phase."), strings.HasPrefix(eventType, "approval."):
		return DropCategoryOther, true
	case strings.HasPrefix(eventType, "agent."), strings.HasPrefix(eventType, "task."):
		return DropCategoryAgentTask, false
	default:
		return DropCategoryOther, false
	}
}

// enqueue applies the drop policy and either appends to the queue or drops.
// Must be called without b.mu held.
func (b *Bridge) enqueue(q queued) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	if len(b.queue) < b.cap {
		b.queue = append(b.queue, q)
		b.cond.Signal()
		return
	}
	// Buffer is at or above cap.
	if !q.priority {
		// Non-priority newcomer: drop it.
		b.recordDrop(q.category)
		return
	}
	// Newcomer is priority. Try to evict the oldest non-priority entry.
	idx := -1
	for i, existing := range b.queue {
		if !existing.priority {
			idx = i
			break
		}
	}
	if idx == -1 {
		// Buffer is 100% priority entries — grow rather than drop the newcomer.
		// In practice bounded by the SSE retry budget; priority events are rare.
		b.queue = append(b.queue, q)
		b.cond.Signal()
		return
	}
	evicted := b.queue[idx]
	// Splice out the evicted entry and append the newcomer.
	b.queue = append(b.queue[:idx], b.queue[idx+1:]...)
	b.recordDrop(evicted.category)
	b.queue = append(b.queue, q)
	b.cond.Signal()
}

func (b *Bridge) recordDrop(cat DropCategory) {
	b.totalDrops.Add(1)
	switch cat {
	case DropCategoryHeartbeat:
		b.dropHB.Add(1)
	case DropCategoryAgentTask:
		b.dropAT.Add(1)
	default:
		b.dropOther.Add(1)
	}
}

// worker drains the queue and calls Sender.Send for each message.
// It holds no lock while calling Send so a slow/wedged sender does not block
// enqueuers — they continue filling the queue and the drop policy kicks in.
//
// Shutdown behaviour: after each Send, if Close has been called the worker
// prunes all non-priority items from the queue (counting them as drops) and
// exits as soon as no priority items remain. This guarantees that in-flight
// CompleteMsg/ErrorMsg/SnapshotMsg (all priority=true) are delivered even when
// Close is called immediately after enqueueing them.
func (b *Bridge) worker() {
	for {
		b.mu.Lock()
		for len(b.queue) == 0 && !b.closed {
			b.cond.Wait()
		}
		if b.closed && len(b.queue) == 0 {
			b.mu.Unlock()
			return
		}
		q := b.queue[0]
		b.queue = b.queue[1:]
		b.mu.Unlock()

		b.sender.Send(q.msg)

		// After each Send, check for shutdown. If Close was called, prune
		// non-priority items from the queue and exit once none remain.
		b.mu.Lock()
		if b.closed {
			pruned := b.queue[:0]
			for _, item := range b.queue {
				if item.priority {
					pruned = append(pruned, item)
				} else {
					b.recordDrop(item.category)
				}
			}
			b.queue = pruned
			if len(b.queue) == 0 {
				b.mu.Unlock()
				return
			}
			// Priority items still pending — keep draining.
		}
		b.mu.Unlock()
	}
}

// Compile-time check: Bridge must satisfy inbound.EventSink.
var _ inbound.EventSink = (*Bridge)(nil)
