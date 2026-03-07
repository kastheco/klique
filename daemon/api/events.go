package api

import (
	"sync"
	"time"
)

// Event is a daemon event emitted over the SSE stream.
// It supersedes the stub Event type that was defined in server.go.
type Event struct {
	Kind      string    `json:"kind"`
	Message   string    `json:"message"`
	Repo      string    `json:"repo,omitempty"`
	PlanFile  string    `json:"plan_file,omitempty"`
	AgentType string    `json:"agent_type,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// EventBroadcaster is a fan-out event broadcaster. Callers Subscribe to
// receive a buffered channel of Events and call Emit to send an event to all
// active subscribers. Close shuts down all subscriber channels.
type EventBroadcaster struct {
	mu   sync.Mutex
	subs []chan Event
}

// NewEventBroadcaster returns a new, empty EventBroadcaster.
func NewEventBroadcaster() *EventBroadcaster {
	return &EventBroadcaster{}
}

// Subscribe registers a new subscriber and returns a buffered receive channel.
// The channel has capacity 64 to allow non-blocking fan-out under normal load.
// Callers must call Unsubscribe when they are done to prevent the subscriber
// list from growing without bound over the daemon's lifetime.
func (b *EventBroadcaster) Subscribe() <-chan Event {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subs = append(b.subs, ch)
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes the subscription identified by ch, closes the channel,
// and signals EOF to the caller. It is safe to call concurrently with Emit and
// Close. If ch is not found (e.g. Close already ran), Unsubscribe is a no-op.
func (b *EventBroadcaster) Unsubscribe(ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, sub := range b.subs {
		if (<-chan Event)(sub) == ch {
			close(sub)
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			return
		}
	}
}

// Emit broadcasts ev to all current subscribers in a non-blocking fashion.
// If a subscriber's buffer is full the event is dropped for that subscriber.
// If Timestamp is zero it is set to the current wall time.
func (b *EventBroadcaster) Emit(ev Event) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		select {
		case ch <- ev:
		default:
			// subscriber buffer full — drop event rather than block
		}
	}
}

// Close closes all subscriber channels, signalling EOF to readers.
// It is safe to call Close multiple times.
func (b *EventBroadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		close(ch)
	}
	b.subs = nil
}
