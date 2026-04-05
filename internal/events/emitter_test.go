package events

import (
	"context"
	"testing"
)

func TestEmitter_ForTest(t *testing.T) {
	bus := NewBus()
	emitter := NewEmitter(bus)
	scoped := emitter.ForTest("suite1", "test1")

	var received *Event
	bus.OnAll(func(e *Event) {
		received = e
	})

	scoped.Emit(NewEvent(TestStarted))

	if received == nil {
		t.Fatalf("expected event to be received")
	}
	if received.SuiteName != "suite1" {
		t.Errorf("expected suite1, got %q", received.SuiteName)
	}
	if received.TestName != "test1" {
		t.Errorf("expected test1, got %q", received.TestName)
	}
}

func TestEmitter_SetUUID(t *testing.T) {
	bus := NewBus()
	emitter := NewEmitter(bus)
	scoped := emitter.ForTest("suite1", "test1")
	scoped.SetUUID("uuid-123")

	var received *Event
	bus.OnAll(func(e *Event) {
		received = e
	})

	scoped.Emit(NewEvent(TestStarted))

	if received.TestUUID != "uuid-123" {
		t.Errorf("expected TestUUID=uuid-123, got %q", received.TestUUID)
	}
}

func TestEmitter_SetFilePath(t *testing.T) {
	bus := NewBus()
	emitter := NewEmitter(bus)
	scoped := emitter.ForTest("suite1", "test1")
	scoped.SetFilePath("/path/to/test.chiperka")

	var received *Event
	bus.OnAll(func(e *Event) {
		received = e
	})

	scoped.Emit(NewEvent(TestStarted))

	if received.FilePath != "/path/to/test.chiperka" {
		t.Errorf("expected FilePath, got %q", received.FilePath)
	}
}

func TestEmitter_DoesNotOverrideExistingContext(t *testing.T) {
	bus := NewBus()
	emitter := NewEmitter(bus)
	scoped := emitter.ForTest("suite1", "test1")

	var received *Event
	bus.OnAll(func(e *Event) {
		received = e
	})

	// Event with pre-set context should not be overridden
	event := NewTestEvent(TestStarted, "other-suite", "other-test")
	scoped.Emit(event)

	if received.SuiteName != "other-suite" {
		t.Errorf("expected pre-set suite name preserved, got %q", received.SuiteName)
	}
	if received.TestName != "other-test" {
		t.Errorf("expected pre-set test name preserved, got %q", received.TestName)
	}
}

func TestEmitter_NilSafety_NewEmitter(t *testing.T) {
	emitter := NewEmitter(nil)
	if emitter != nil {
		t.Errorf("expected nil emitter for nil bus")
	}
}

func TestEmitter_NilSafety_ForTest(t *testing.T) {
	var emitter *Emitter
	scoped := emitter.ForTest("suite1", "test1")
	if scoped != nil {
		t.Errorf("expected nil for nil emitter")
	}
}

func TestEmitter_NilSafety_SetUUID(t *testing.T) {
	var emitter *Emitter
	// Should not panic
	emitter.SetUUID("uuid")
}

func TestEmitter_NilSafety_SetFilePath(t *testing.T) {
	var emitter *Emitter
	// Should not panic
	emitter.SetFilePath("/path")
}

func TestEmitter_NilSafety_Emit(t *testing.T) {
	var emitter *Emitter
	// Should not panic
	emitter.Emit(NewEvent(TestStarted))
}

// --- Context round-trip ---

func TestEmitter_ContextRoundTrip(t *testing.T) {
	bus := NewBus()
	emitter := NewEmitter(bus)
	scoped := emitter.ForTest("suite1", "test1")

	ctx := context.Background()
	ctx = WithEmitter(ctx, scoped)

	retrieved := EmitterFromContext(ctx)
	if retrieved == nil {
		t.Fatalf("expected emitter from context")
	}

	var received *Event
	bus.OnAll(func(e *Event) {
		received = e
	})

	retrieved.Emit(NewEvent(TestStarted))

	if received.SuiteName != "suite1" {
		t.Errorf("expected suite1, got %q", received.SuiteName)
	}
}

func TestEmitter_EmitterFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	emitter := EmitterFromContext(ctx)
	if emitter != nil {
		t.Errorf("expected nil for context without emitter")
	}
}

// --- Convenience methods ---

func TestEmitter_RunStarted(t *testing.T) {
	bus := NewBus()
	emitter := NewEmitter(bus)
	var received *Event
	bus.On(RunStarted, func(e *Event) {
		received = e
	})

	emitter.RunStarted(10, 2, 4, "1.0.0", map[string]int{"s1": 5, "s2": 5})

	if received == nil {
		t.Fatalf("expected RunStarted event")
	}
	if received.Data.Details["tests"] != 10 {
		t.Errorf("expected tests=10, got %v", received.Data.Details["tests"])
	}
	if received.Data.Details["version"] != "1.0.0" {
		t.Errorf("expected version=1.0.0, got %v", received.Data.Details["version"])
	}
}

func TestEmitter_TestStarted(t *testing.T) {
	bus := NewBus()
	emitter := NewEmitter(bus).ForTest("suite1", "test1")
	var received *Event
	bus.On(TestStarted, func(e *Event) {
		received = e
	})

	emitter.TestStarted()

	if received == nil {
		t.Fatalf("expected TestStarted event")
	}
	if received.SuiteName != "suite1" {
		t.Errorf("expected suite1, got %q", received.SuiteName)
	}
}

func TestEmitter_Info(t *testing.T) {
	bus := NewBus()
	emitter := NewEmitter(bus).ForTest("suite1", "test1")
	var received *Event
	bus.On(LogInfo, func(e *Event) {
		received = e
	})

	emitter.Info(Fields{"msg": "hello", "action": "setup"})

	if received == nil {
		t.Fatalf("expected LogInfo event")
	}
	if received.Data.Message != "hello" {
		t.Errorf("expected message 'hello', got %q", received.Data.Message)
	}
	if received.Data.Details["action"] != "setup" {
		t.Errorf("expected action=setup, got %v", received.Data.Details["action"])
	}
}
