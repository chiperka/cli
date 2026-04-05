package subscribers

import (
	"testing"
	"time"

	"chiperka-cli/internal/events"
)

func newTestBus(t *testing.T) (*events.Bus, *EventCollector) {
	t.Helper()
	bus := events.NewBus()
	collector := NewEventCollector()
	collector.Register(bus)
	return bus, collector
}

func TestCollector_CollectsEvents(t *testing.T) {
	bus, collector := newTestBus(t)

	bus.Emit(events.NewEvent(events.TestStarted))
	bus.Emit(events.NewEvent(events.TestCompleted))

	all := collector.AllEvents()
	if len(all) != 2 {
		t.Errorf("expected 2 events, got %d", len(all))
	}
}

func TestCollector_EventsForTest(t *testing.T) {
	bus, collector := newTestBus(t)

	bus.Emit(events.NewTestEvent(events.TestStarted, "suite1", "test1"))
	bus.Emit(events.NewTestEvent(events.TestStarted, "suite1", "test2"))
	bus.Emit(events.NewTestEvent(events.TestCompleted, "suite1", "test1"))

	test1Events := collector.EventsForTest("suite1", "test1")
	if len(test1Events) != 2 {
		t.Errorf("expected 2 events for test1, got %d", len(test1Events))
	}

	test2Events := collector.EventsForTest("suite1", "test2")
	if len(test2Events) != 1 {
		t.Errorf("expected 1 event for test2, got %d", len(test2Events))
	}
}

func TestCollector_EventsForTest_NotFound(t *testing.T) {
	_, collector := newTestBus(t)

	result := collector.EventsForTest("suite1", "nonexistent")
	if len(result) != 0 {
		t.Errorf("expected 0 events, got %d", len(result))
	}
}

func TestCollector_EventsByType(t *testing.T) {
	bus, collector := newTestBus(t)

	bus.Emit(events.NewEvent(events.TestStarted))
	bus.Emit(events.NewEvent(events.TestCompleted))
	bus.Emit(events.NewEvent(events.TestStarted))

	started := collector.EventsByType(events.TestStarted)
	if len(started) != 2 {
		t.Errorf("expected 2 TestStarted events, got %d", len(started))
	}

	completed := collector.EventsByType(events.TestCompleted)
	if len(completed) != 1 {
		t.Errorf("expected 1 TestCompleted event, got %d", len(completed))
	}
}

func TestCollector_EventsByType_NotFound(t *testing.T) {
	_, collector := newTestBus(t)

	result := collector.EventsByType(events.RunStarted)
	if len(result) != 0 {
		t.Errorf("expected 0 events, got %d", len(result))
	}
}

// --- TestTiming ---

func TestCollector_TestTiming_Phases(t *testing.T) {
	bus, collector := newTestBus(t)

	// Simulate test lifecycle
	bus.Emit(events.NewTestEvent(events.TestStarted, "suite1", "test1"))
	bus.Emit(events.NewTestEvent(events.TestServiceReady, "suite1", "test1").WithDuration(100 * time.Millisecond))
	bus.Emit(events.NewTestEvent(events.TestSetupCompleted, "suite1", "test1").WithDuration(50 * time.Millisecond))
	bus.Emit(events.NewTestEvent(events.TestExecCompleted, "suite1", "test1").WithDuration(200 * time.Millisecond))
	bus.Emit(events.NewTestEvent(events.TestAssertResult, "suite1", "test1").WithDuration(5 * time.Millisecond))
	bus.Emit(events.NewTestEvent(events.TestCleanup, "suite1", "test1").WithDuration(30 * time.Millisecond))
	bus.Emit(events.NewTestEvent(events.TestCompleted, "suite1", "test1").WithDuration(400 * time.Millisecond))

	timing := collector.TestTiming("suite1", "test1")
	if timing == nil {
		t.Fatalf("expected timing data")
	}
	if timing.ServiceStartup != 100*time.Millisecond {
		t.Errorf("expected ServiceStartup=100ms, got %v", timing.ServiceStartup)
	}
	if timing.Setup != 50*time.Millisecond {
		t.Errorf("expected Setup=50ms, got %v", timing.Setup)
	}
	if timing.Execution != 200*time.Millisecond {
		t.Errorf("expected Execution=200ms, got %v", timing.Execution)
	}
	if timing.Assertion != 5*time.Millisecond {
		t.Errorf("expected Assertion=5ms, got %v", timing.Assertion)
	}
	if timing.Cleanup != 30*time.Millisecond {
		t.Errorf("expected Cleanup=30ms, got %v", timing.Cleanup)
	}
	if timing.Total != 400*time.Millisecond {
		t.Errorf("expected Total=400ms, got %v", timing.Total)
	}
	if !timing.Passed {
		t.Errorf("expected Passed=true")
	}
}

func TestCollector_TestTiming_Failed(t *testing.T) {
	bus, collector := newTestBus(t)

	bus.Emit(events.NewTestEvent(events.TestStarted, "suite1", "test1"))
	bus.Emit(events.NewTestEvent(events.TestFailed, "suite1", "test1").
		WithDuration(100 * time.Millisecond).
		WithMessage("assertion failed"))

	timing := collector.TestTiming("suite1", "test1")
	if timing == nil {
		t.Fatalf("expected timing data")
	}
	if timing.Passed {
		t.Errorf("expected Passed=false")
	}
	if timing.Error != "assertion failed" {
		t.Errorf("expected error message, got %q", timing.Error)
	}
}

func TestCollector_TestTiming_NotFound(t *testing.T) {
	_, collector := newTestBus(t)

	timing := collector.TestTiming("suite1", "nonexistent")
	if timing != nil {
		t.Errorf("expected nil timing for unknown test")
	}
}

// --- RunTiming ---

func TestCollector_RunTiming_Counts(t *testing.T) {
	bus, collector := newTestBus(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 4))

	bus.Emit(events.NewTestEvent(events.TestStarted, "s", "t1"))
	bus.Emit(events.NewTestEvent(events.TestCompleted, "s", "t1").WithDuration(100 * time.Millisecond))

	bus.Emit(events.NewTestEvent(events.TestStarted, "s", "t2"))
	bus.Emit(events.NewTestEvent(events.TestCompleted, "s", "t2").WithDuration(100 * time.Millisecond))

	bus.Emit(events.NewTestEvent(events.TestStarted, "s", "t3"))
	bus.Emit(events.NewTestEvent(events.TestFailed, "s", "t3").WithDuration(100 * time.Millisecond))

	bus.Emit(events.NewTestEvent(events.TestSkipped, "s", "t4"))

	bus.Emit(events.NewEvent(events.RunCompleted).WithDuration(500 * time.Millisecond))

	rt := collector.RunTiming()
	if rt.TotalTests != 4 {
		t.Errorf("expected 4 total tests, got %d", rt.TotalTests)
	}
	if rt.PassedTests != 2 {
		t.Errorf("expected 2 passed, got %d", rt.PassedTests)
	}
	if rt.FailedTests != 1 {
		t.Errorf("expected 1 failed, got %d", rt.FailedTests)
	}
	if rt.SkippedTests != 1 {
		t.Errorf("expected 1 skipped, got %d", rt.SkippedTests)
	}
	if rt.Total != 500*time.Millisecond {
		t.Errorf("expected Total=500ms, got %v", rt.Total)
	}
}

// --- SlowestTests ---

func TestCollector_SlowestTests(t *testing.T) {
	bus, collector := newTestBus(t)

	bus.Emit(events.NewTestEvent(events.TestStarted, "s", "fast"))
	bus.Emit(events.NewTestEvent(events.TestCompleted, "s", "fast").WithDuration(100 * time.Millisecond))

	bus.Emit(events.NewTestEvent(events.TestStarted, "s", "slow"))
	bus.Emit(events.NewTestEvent(events.TestCompleted, "s", "slow").WithDuration(500 * time.Millisecond))

	bus.Emit(events.NewTestEvent(events.TestStarted, "s", "medium"))
	bus.Emit(events.NewTestEvent(events.TestCompleted, "s", "medium").WithDuration(300 * time.Millisecond))

	slowest := collector.SlowestTests(2)
	if len(slowest) != 2 {
		t.Fatalf("expected 2, got %d", len(slowest))
	}
	if slowest[0].TestName != "slow" {
		t.Errorf("expected slowest to be 'slow', got %q", slowest[0].TestName)
	}
	if slowest[1].TestName != "medium" {
		t.Errorf("expected second to be 'medium', got %q", slowest[1].TestName)
	}
}

func TestCollector_SlowestTests_LessThanN(t *testing.T) {
	bus, collector := newTestBus(t)

	bus.Emit(events.NewTestEvent(events.TestStarted, "s", "only"))
	bus.Emit(events.NewTestEvent(events.TestCompleted, "s", "only").WithDuration(100 * time.Millisecond))

	slowest := collector.SlowestTests(5)
	if len(slowest) != 1 {
		t.Errorf("expected 1, got %d", len(slowest))
	}
}

// --- AverageTestDuration ---

func TestCollector_AverageTestDuration(t *testing.T) {
	bus, collector := newTestBus(t)

	bus.Emit(events.NewTestEvent(events.TestStarted, "s", "t1"))
	bus.Emit(events.NewTestEvent(events.TestCompleted, "s", "t1").WithDuration(100 * time.Millisecond))

	bus.Emit(events.NewTestEvent(events.TestStarted, "s", "t2"))
	bus.Emit(events.NewTestEvent(events.TestCompleted, "s", "t2").WithDuration(300 * time.Millisecond))

	avg := collector.AverageTestDuration()
	if avg != 200*time.Millisecond {
		t.Errorf("expected 200ms, got %v", avg)
	}
}

func TestCollector_AverageTestDuration_Empty(t *testing.T) {
	_, collector := newTestBus(t)

	avg := collector.AverageTestDuration()
	if avg != 0 {
		t.Errorf("expected 0, got %v", avg)
	}
}

// --- PhaseBreakdown ---

func TestCollector_PhaseBreakdown(t *testing.T) {
	bus, collector := newTestBus(t)

	bus.Emit(events.NewTestEvent(events.TestStarted, "s", "t1"))
	bus.Emit(events.NewTestEvent(events.TestServiceReady, "s", "t1").WithDuration(100 * time.Millisecond))
	bus.Emit(events.NewTestEvent(events.TestExecCompleted, "s", "t1").WithDuration(200 * time.Millisecond))
	bus.Emit(events.NewTestEvent(events.TestCompleted, "s", "t1").WithDuration(300 * time.Millisecond))

	breakdown := collector.PhaseBreakdown()
	if breakdown[events.PhaseServiceStartup] != 100*time.Millisecond {
		t.Errorf("expected ServiceStartup=100ms, got %v", breakdown[events.PhaseServiceStartup])
	}
	if breakdown[events.PhaseExecution] != 200*time.Millisecond {
		t.Errorf("expected Execution=200ms, got %v", breakdown[events.PhaseExecution])
	}
}

// --- Immutability ---

func TestCollector_ReturnsCopies(t *testing.T) {
	bus, collector := newTestBus(t)

	bus.Emit(events.NewEvent(events.TestStarted))

	all1 := collector.AllEvents()
	all2 := collector.AllEvents()

	if &all1[0] == &all2[0] {
		t.Errorf("expected different slice backing arrays (copies)")
	}
}
