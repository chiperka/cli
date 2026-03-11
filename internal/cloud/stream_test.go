package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"spark-cli/internal/events"
)

// eventRecorder captures events emitted on a bus.
type eventRecorder struct {
	mu     sync.Mutex
	events []*events.Event
}

func (r *eventRecorder) handler(e *events.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *eventRecorder) ofType(t events.Type) []*events.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*events.Event
	for _, e := range r.events {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

// --- SSEAdapter ---

func TestAdapter_Snapshot(t *testing.T) {
	bus := events.NewBus()
	rec := &eventRecorder{}
	bus.OnAll(rec.handler)
	adapter := NewSSEAdapter(bus)

	snap := snapshotRun{
		ID: "run-1",
		Suites: []snapshotSuite{
			{Name: "suite-a", Tests: []snapshotTest{
				{ID: 1, Name: "test-one", Status: "pending"},
				{ID: 2, Name: "test-two", Status: "pending"},
			}},
		},
	}
	data, _ := json.Marshal(snap)
	adapter.HandleEvent(SSEEvent{Event: "snapshot", Data: data})

	if adapter.testNames[1] != "test-one" {
		t.Errorf("expected test-one, got %q", adapter.testNames[1])
	}
	if adapter.testNames[2] != "test-two" {
		t.Errorf("expected test-two, got %q", adapter.testNames[2])
	}

	started := rec.ofType(events.RunStarted)
	if len(started) != 1 {
		t.Fatalf("expected 1 RunStarted event, got %d", len(started))
	}
	if tests, ok := started[0].Data.Details["tests"].(int); !ok || tests != 2 {
		t.Errorf("expected 2 tests in RunStarted, got %v", started[0].Data.Details["tests"])
	}
}

func TestAdapter_SnapshotWithCompleted(t *testing.T) {
	bus := events.NewBus()
	rec := &eventRecorder{}
	bus.OnAll(rec.handler)
	adapter := NewSSEAdapter(bus)

	snap := snapshotRun{
		Suites: []snapshotSuite{
			{Name: "s", Tests: []snapshotTest{
				{ID: 1, Name: "t1", Status: "passed"},
				{ID: 2, Name: "t2", Status: "pending"},
			}},
		},
	}
	data, _ := json.Marshal(snap)
	adapter.HandleEvent(SSEEvent{Event: "snapshot", Data: data})

	completed := rec.ofType(events.TestCompleted)
	if len(completed) != 1 {
		t.Errorf("expected 1 already-completed test event, got %d", len(completed))
	}
}

func TestAdapter_TestUpdatePassed(t *testing.T) {
	bus := events.NewBus()
	rec := &eventRecorder{}
	bus.OnAll(rec.handler)
	adapter := NewSSEAdapter(bus)
	adapter.testNames[1] = "login-test"
	adapter.suiteName[1] = "auth"

	update := testUpdate{TestID: 1, Status: "passed", DurationMs: 843}
	data, _ := json.Marshal(update)
	result, done := adapter.HandleEvent(SSEEvent{Event: "test_update", Data: data})

	if done {
		t.Errorf("expected not done after test_update")
	}
	if result != nil {
		t.Errorf("expected nil result")
	}

	completed := rec.ofType(events.TestCompleted)
	if len(completed) != 1 {
		t.Fatalf("expected 1 TestCompleted, got %d", len(completed))
	}
	if completed[0].TestName != "login-test" {
		t.Errorf("expected login-test, got %q", completed[0].TestName)
	}
	if completed[0].SuiteName != "auth" {
		t.Errorf("expected auth suite, got %q", completed[0].SuiteName)
	}
}

func TestAdapter_TestUpdateFailed(t *testing.T) {
	bus := events.NewBus()
	rec := &eventRecorder{}
	bus.OnAll(rec.handler)
	adapter := NewSSEAdapter(bus)
	adapter.testNames[1] = "fail-test"
	adapter.suiteName[1] = "suite"

	update := testUpdate{TestID: 1, Status: "failed", DurationMs: 500}
	data, _ := json.Marshal(update)
	adapter.HandleEvent(SSEEvent{Event: "test_update", Data: data})

	failed := rec.ofType(events.TestFailed)
	if len(failed) != 1 {
		t.Fatalf("expected 1 TestFailed, got %d", len(failed))
	}
}

func TestAdapter_TestUpdateSkipped(t *testing.T) {
	bus := events.NewBus()
	rec := &eventRecorder{}
	bus.OnAll(rec.handler)
	adapter := NewSSEAdapter(bus)
	adapter.testNames[1] = "skip-test"
	adapter.suiteName[1] = "suite"

	update := testUpdate{TestID: 1, Status: "skipped", DurationMs: 0}
	data, _ := json.Marshal(update)
	adapter.HandleEvent(SSEEvent{Event: "test_update", Data: data})

	skipped := rec.ofType(events.TestSkipped)
	if len(skipped) != 1 {
		t.Fatalf("expected 1 TestSkipped, got %d", len(skipped))
	}
}

func TestAdapter_TestUpdateRunning(t *testing.T) {
	bus := events.NewBus()
	rec := &eventRecorder{}
	bus.OnAll(rec.handler)
	adapter := NewSSEAdapter(bus)
	adapter.testNames[1] = "running-test"
	adapter.suiteName[1] = "suite"

	update := testUpdate{TestID: 1, Status: "running", DurationMs: 0}
	data, _ := json.Marshal(update)
	adapter.HandleEvent(SSEEvent{Event: "test_update", Data: data})

	started := rec.ofType(events.TestStarted)
	if len(started) != 1 {
		t.Fatalf("expected 1 TestStarted for running status, got %d", len(started))
	}
}

func TestAdapter_RunCompleted(t *testing.T) {
	bus := events.NewBus()
	rec := &eventRecorder{}
	bus.OnAll(rec.handler)
	adapter := NewSSEAdapter(bus)

	rc := runCompleted{Passed: 2, Failed: 1, Skipped: 0}
	data, _ := json.Marshal(rc)
	result, done := adapter.HandleEvent(SSEEvent{Event: "run_completed", Data: data})

	if !done {
		t.Errorf("expected done=true for run_completed")
	}
	if result == nil {
		t.Fatalf("expected non-nil result")
	}
	if result.Passed != 2 {
		t.Errorf("expected 2 passed, got %d", result.Passed)
	}
	if result.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", result.Failed)
	}

	completed := rec.ofType(events.RunCompleted)
	if len(completed) != 1 {
		t.Fatalf("expected 1 RunCompleted event, got %d", len(completed))
	}
}

func TestAdapter_RunCompletedAllPassed(t *testing.T) {
	bus := events.NewBus()
	rec := &eventRecorder{}
	bus.OnAll(rec.handler)
	adapter := NewSSEAdapter(bus)

	rc := runCompleted{Passed: 2, Failed: 0, Skipped: 0}
	data, _ := json.Marshal(rc)
	result, done := adapter.HandleEvent(SSEEvent{Event: "run_completed", Data: data})

	if !done {
		t.Errorf("expected done=true")
	}
	if result.HasFailures() {
		t.Errorf("expected no failures")
	}
}

func TestAdapter_UnknownEvent(t *testing.T) {
	bus := events.NewBus()
	adapter := NewSSEAdapter(bus)

	result, done := adapter.HandleEvent(SSEEvent{Event: "unknown", Data: json.RawMessage(`{}`)})
	if done {
		t.Errorf("expected not done for unknown event")
	}
	if result != nil {
		t.Errorf("expected nil result for unknown event")
	}
}

// --- RunResult ---

func TestStream_RunResult_HasFailures(t *testing.T) {
	r := RunResult{Passed: 5, Failed: 1}
	if !r.HasFailures() {
		t.Errorf("expected HasFailures=true")
	}

	r2 := RunResult{Passed: 5, Failed: 0}
	if r2.HasFailures() {
		t.Errorf("expected HasFailures=false")
	}
}

// --- StreamRun integration ---

func TestStream_StreamRun_FullLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events/runs" {
			t.Errorf("expected /api/events/runs, got %q", r.URL.Path)
		}
		if r.URL.Query().Get("id") != "run-123" {
			t.Errorf("expected id=run-123, got %q", r.URL.Query().Get("id"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected Flusher")
		}

		// Send snapshot
		snap := snapshotRun{
			ID: "run-123",
			Suites: []snapshotSuite{
				{Name: "suite-1", Tests: []snapshotTest{
					{ID: 1, Name: "test-a", Status: "pending"},
					{ID: 2, Name: "test-b", Status: "pending"},
				}},
			},
		}
		snapData, _ := json.Marshal(snap)
		fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", snapData)
		flusher.Flush()

		// Send test updates
		update1 := testUpdate{TestID: 1, Status: "passed", DurationMs: 100}
		data1, _ := json.Marshal(update1)
		fmt.Fprintf(w, "event: test_update\ndata: %s\n\n", data1)
		flusher.Flush()

		update2 := testUpdate{TestID: 2, Status: "passed", DurationMs: 200}
		data2, _ := json.Marshal(update2)
		fmt.Fprintf(w, "event: test_update\ndata: %s\n\n", data2)
		flusher.Flush()

		// Send run completed
		rc := runCompleted{Passed: 2, Failed: 0, Skipped: 0}
		rcData, _ := json.Marshal(rc)
		fmt.Fprintf(w, "event: run_completed\ndata: %s\n\n", rcData)
		flusher.Flush()
	}))
	defer server.Close()

	bus := events.NewBus()
	rec := &eventRecorder{}
	bus.OnAll(rec.handler)

	client := NewClient(server.URL, "")
	result, err := client.StreamRun(context.Background(), "run-123", bus)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatalf("expected non-nil result")
	}
	if result.Passed != 2 {
		t.Errorf("expected 2 passed, got %d", result.Passed)
	}
	if result.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", result.Failed)
	}

	// Verify events were emitted
	if len(rec.ofType(events.RunStarted)) != 1 {
		t.Errorf("expected 1 RunStarted event")
	}
	if len(rec.ofType(events.TestCompleted)) != 2 {
		t.Errorf("expected 2 TestCompleted events, got %d", len(rec.ofType(events.TestCompleted)))
	}
	if len(rec.ofType(events.RunCompleted)) != 1 {
		t.Errorf("expected 1 RunCompleted event")
	}
}

func TestStream_StreamRun_ContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected Flusher")
		}

		// Send initial snapshot but never complete
		snap := snapshotRun{Suites: []snapshotSuite{{Name: "s", Tests: []snapshotTest{{ID: 1, Name: "t1", Status: "pending"}}}}}
		snapData, _ := json.Marshal(snap)
		fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", snapData)
		flusher.Flush()

		// Wait for context cancellation
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	bus := events.NewBus()
	_, err := client.StreamRun(ctx, "run-123", bus)
	if err == nil {
		t.Errorf("expected error for cancelled context")
	}
}

func TestStream_StreamRun_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	bus := events.NewBus()
	_, err := client.StreamRun(context.Background(), "run-123", bus)
	if err == nil {
		t.Errorf("expected error for server error")
	}
}
