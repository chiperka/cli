package events

import (
	"sync"
	"testing"
	"time"
)

func TestBus_On_ReceivesMatchingEvents(t *testing.T) {
	bus := NewBus()
	var received []*Event
	bus.On(TestStarted, func(e *Event) {
		received = append(received, e)
	})

	bus.Emit(NewEvent(TestStarted))
	bus.Emit(NewEvent(TestCompleted))

	if len(received) != 1 {
		t.Errorf("expected 1 event, got %d", len(received))
	}
	if received[0].Type != TestStarted {
		t.Errorf("expected TestStarted, got %q", received[0].Type)
	}
}

func TestBus_OnMany_ReceivesMultipleTypes(t *testing.T) {
	bus := NewBus()
	var count int
	bus.OnMany(func(e *Event) {
		count++
	}, TestStarted, TestCompleted)

	bus.Emit(NewEvent(TestStarted))
	bus.Emit(NewEvent(TestCompleted))
	bus.Emit(NewEvent(TestFailed))

	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestBus_OnAll_ReceivesAllEvents(t *testing.T) {
	bus := NewBus()
	var count int
	bus.OnAll(func(e *Event) {
		count++
	})

	bus.Emit(NewEvent(TestStarted))
	bus.Emit(NewEvent(TestCompleted))
	bus.Emit(NewEvent(RunStarted))

	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}

func TestBus_Emit_SetsTimestamp(t *testing.T) {
	bus := NewBus()
	var received *Event
	bus.OnAll(func(e *Event) {
		received = e
	})

	event := &Event{Type: TestStarted}
	bus.Emit(event)

	if received.Timestamp.IsZero() {
		t.Errorf("expected timestamp to be set")
	}
}

func TestBus_Emit_SetsRunID(t *testing.T) {
	bus := NewBus()
	var received *Event
	bus.OnAll(func(e *Event) {
		received = e
	})

	event := &Event{Type: TestStarted}
	bus.Emit(event)

	if received.RunID == "" {
		t.Errorf("expected RunID to be set")
	}
	if received.RunID != bus.RunID() {
		t.Errorf("expected RunID=%q, got %q", bus.RunID(), received.RunID)
	}
}

func TestBus_Emit_PreservesExistingTimestamp(t *testing.T) {
	bus := NewBus()
	var received *Event
	bus.OnAll(func(e *Event) {
		received = e
	})

	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	event := &Event{Type: TestStarted, Timestamp: ts}
	bus.Emit(event)

	if !received.Timestamp.Equal(ts) {
		t.Errorf("expected preserved timestamp %v, got %v", ts, received.Timestamp)
	}
}

func TestBus_Emit_PreservesExistingRunID(t *testing.T) {
	bus := NewBus()
	var received *Event
	bus.OnAll(func(e *Event) {
		received = e
	})

	event := &Event{Type: TestStarted, RunID: "custom-run-id"}
	bus.Emit(event)

	if received.RunID != "custom-run-id" {
		t.Errorf("expected preserved RunID 'custom-run-id', got %q", received.RunID)
	}
}

func TestBus_Emit_HandlerOrdering(t *testing.T) {
	bus := NewBus()
	var order []int
	bus.On(TestStarted, func(e *Event) { order = append(order, 1) })
	bus.On(TestStarted, func(e *Event) { order = append(order, 2) })
	bus.On(TestStarted, func(e *Event) { order = append(order, 3) })

	bus.Emit(NewEvent(TestStarted))

	if len(order) != 3 {
		t.Fatalf("expected 3 handlers called, got %d", len(order))
	}
	for i, v := range order {
		if v != i+1 {
			t.Errorf("expected order[%d]=%d, got %d", i, i+1, v)
		}
	}
}

func TestBus_Emit_SpecificBeforeCatchAll(t *testing.T) {
	bus := NewBus()
	var order []string
	bus.On(TestStarted, func(e *Event) { order = append(order, "specific") })
	bus.OnAll(func(e *Event) { order = append(order, "catchall") })

	bus.Emit(NewEvent(TestStarted))

	if len(order) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(order))
	}
	if order[0] != "specific" {
		t.Errorf("expected specific handler first, got %q", order[0])
	}
	if order[1] != "catchall" {
		t.Errorf("expected catchall handler second, got %q", order[1])
	}
}

func TestBus_RunID_Format(t *testing.T) {
	bus := NewBus()
	id := bus.RunID()
	if len(id) == 0 {
		t.Errorf("expected non-empty RunID")
	}
	// Format: "20060102-150405"
	if len(id) != 15 {
		t.Errorf("expected RunID length 15, got %d: %q", len(id), id)
	}
}

func TestBus_StartTime(t *testing.T) {
	before := time.Now()
	bus := NewBus()
	after := time.Now()

	if bus.StartTime().Before(before) || bus.StartTime().After(after) {
		t.Errorf("StartTime should be between creation bounds")
	}
}

func TestBus_Elapsed(t *testing.T) {
	bus := NewBus()
	time.Sleep(5 * time.Millisecond)
	elapsed := bus.Elapsed()
	if elapsed < 5*time.Millisecond {
		t.Errorf("expected elapsed >= 5ms, got %v", elapsed)
	}
}

func TestBus_ConcurrentEmit(t *testing.T) {
	bus := NewBus()
	var mu sync.Mutex
	var count int
	bus.OnAll(func(e *Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Emit(NewEvent(TestStarted))
		}()
	}
	wg.Wait()

	if count != 100 {
		t.Errorf("expected 100, got %d", count)
	}
}
