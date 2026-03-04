package events

import (
	"sync"
	"time"
)

// Handler is a function that processes events.
type Handler func(event *Event)

// Bus is a publish/subscribe event bus for lifecycle events.
// It is thread-safe and can be used concurrently.
type Bus struct {
	mu          sync.RWMutex
	handlers    map[Type][]Handler
	allHandlers []Handler
	runID       string
	startTime   time.Time
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		handlers:  make(map[Type][]Handler),
		startTime: time.Now(),
		runID:     generateRunID(),
	}
}

// On subscribes a handler to a specific event type.
func (b *Bus) On(eventType Type, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// OnMany subscribes a handler to multiple event types.
func (b *Bus) OnMany(handler Handler, eventTypes ...Type) {
	for _, t := range eventTypes {
		b.On(t, handler)
	}
}

// OnAll subscribes a handler to receive ALL events.
// Useful for collectors and verbose loggers.
func (b *Bus) OnAll(handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.allHandlers = append(b.allHandlers, handler)
}

// Emit publishes an event to all relevant handlers.
// Handlers are called synchronously in the order they were registered.
func (b *Bus) Emit(event *Event) {
	// Ensure timestamp and run ID
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.RunID == "" {
		event.RunID = b.runID
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	// Notify specific handlers
	for _, h := range b.handlers[event.Type] {
		h(event)
	}

	// Notify catch-all handlers
	for _, h := range b.allHandlers {
		h(event)
	}
}

// RunID returns the unique ID for this run.
func (b *Bus) RunID() string {
	return b.runID
}

// StartTime returns when the bus was created (start of run).
func (b *Bus) StartTime() time.Time {
	return b.startTime
}

// Elapsed returns time since the bus was created.
func (b *Bus) Elapsed() time.Duration {
	return time.Since(b.startTime)
}

// generateRunID creates a short unique ID for a test run.
func generateRunID() string {
	return time.Now().Format("20060102-150405")
}
