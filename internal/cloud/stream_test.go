package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- progressReporter ---

func TestStream_ProgressReporter_Snapshot(t *testing.T) {
	var buf bytes.Buffer
	reporter := &progressReporter{
		out:        &buf,
		testNames:  make(map[int64]string),
		totalTests: 0,
	}

	snap := snapshotRun{
		ID: "run-1",
		Suites: []snapshotSuite{
			{Tests: []snapshotTest{
				{ID: 1, Name: "test-one", Status: "pending"},
				{ID: 2, Name: "test-two", Status: "pending"},
			}},
		},
	}
	data, _ := json.Marshal(snap)
	reporter.handleEvent(SSEEvent{Event: "snapshot", Data: data})

	if reporter.totalTests != 2 {
		t.Errorf("expected 2 total tests, got %d", reporter.totalTests)
	}
	if reporter.testNames[1] != "test-one" {
		t.Errorf("expected test-one, got %q", reporter.testNames[1])
	}
	if reporter.testNames[2] != "test-two" {
		t.Errorf("expected test-two, got %q", reporter.testNames[2])
	}
}

func TestStream_ProgressReporter_SnapshotWithCompleted(t *testing.T) {
	var buf bytes.Buffer
	reporter := &progressReporter{
		out:        &buf,
		testNames:  make(map[int64]string),
		totalTests: 0,
	}

	snap := snapshotRun{
		Suites: []snapshotSuite{
			{Tests: []snapshotTest{
				{ID: 1, Name: "t1", Status: "passed"},
				{ID: 2, Name: "t2", Status: "pending"},
			}},
		},
	}
	data, _ := json.Marshal(snap)
	reporter.handleEvent(SSEEvent{Event: "snapshot", Data: data})

	if reporter.completed != 1 {
		t.Errorf("expected 1 already completed, got %d", reporter.completed)
	}
}

func TestStream_ProgressReporter_TestUpdate(t *testing.T) {
	var buf bytes.Buffer
	reporter := &progressReporter{
		out:        &buf,
		testNames:  map[int64]string{1: "login-test"},
		totalTests: 2,
	}

	update := testUpdate{TestID: 1, Status: "passed", DurationMs: 843}
	data, _ := json.Marshal(update)
	result, done := reporter.handleEvent(SSEEvent{Event: "test_update", Data: data})

	if done {
		t.Errorf("expected not done after test_update")
	}
	if result != nil {
		t.Errorf("expected nil result")
	}
	if reporter.completed != 1 {
		t.Errorf("expected 1 completed, got %d", reporter.completed)
	}

	output := buf.String()
	if !strings.Contains(output, "login-test") {
		t.Errorf("expected test name in output, got %q", output)
	}
	if !strings.Contains(output, "✓") {
		t.Errorf("expected ✓ for passed test, got %q", output)
	}
}

func TestStream_ProgressReporter_TestUpdateFailed(t *testing.T) {
	var buf bytes.Buffer
	reporter := &progressReporter{
		out:        &buf,
		testNames:  map[int64]string{1: "fail-test"},
		totalTests: 1,
	}

	update := testUpdate{TestID: 1, Status: "failed", DurationMs: 500}
	data, _ := json.Marshal(update)
	reporter.handleEvent(SSEEvent{Event: "test_update", Data: data})

	output := buf.String()
	if !strings.Contains(output, "✗") {
		t.Errorf("expected ✗ for failed test, got %q", output)
	}
}

func TestStream_ProgressReporter_TestUpdateSkipped(t *testing.T) {
	var buf bytes.Buffer
	reporter := &progressReporter{
		out:        &buf,
		testNames:  map[int64]string{1: "skip-test"},
		totalTests: 1,
	}

	update := testUpdate{TestID: 1, Status: "skipped", DurationMs: 0}
	data, _ := json.Marshal(update)
	reporter.handleEvent(SSEEvent{Event: "test_update", Data: data})

	output := buf.String()
	if !strings.Contains(output, "-") {
		t.Errorf("expected - for skipped test, got %q", output)
	}
}

func TestStream_ProgressReporter_TestUpdateRunning(t *testing.T) {
	var buf bytes.Buffer
	reporter := &progressReporter{
		out:        &buf,
		testNames:  map[int64]string{1: "running-test"},
		totalTests: 1,
	}

	update := testUpdate{TestID: 1, Status: "running", DurationMs: 0}
	data, _ := json.Marshal(update)
	reporter.handleEvent(SSEEvent{Event: "test_update", Data: data})

	// Running status should not produce output
	if buf.Len() != 0 {
		t.Errorf("expected no output for running status, got %q", buf.String())
	}
	if reporter.completed != 0 {
		t.Errorf("expected 0 completed for running status, got %d", reporter.completed)
	}
}

func TestStream_ProgressReporter_RunCompleted(t *testing.T) {
	var buf bytes.Buffer
	reporter := &progressReporter{
		out:        &buf,
		testNames:  make(map[int64]string),
		totalTests: 3,
		completed:  3,
	}

	rc := runCompleted{Passed: 2, Failed: 1, Skipped: 0}
	data, _ := json.Marshal(rc)
	result, done := reporter.handleEvent(SSEEvent{Event: "run_completed", Data: data})

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

	output := buf.String()
	if !strings.Contains(output, "FAILED") {
		t.Errorf("expected FAILED in output, got %q", output)
	}
}

func TestStream_ProgressReporter_RunCompletedAllPassed(t *testing.T) {
	var buf bytes.Buffer
	reporter := &progressReporter{
		out:        &buf,
		testNames:  make(map[int64]string),
		totalTests: 2,
		completed:  2,
	}

	rc := runCompleted{Passed: 2, Failed: 0, Skipped: 0}
	data, _ := json.Marshal(rc)
	result, done := reporter.handleEvent(SSEEvent{Event: "run_completed", Data: data})

	if !done {
		t.Errorf("expected done=true")
	}
	if result.HasFailures() {
		t.Errorf("expected no failures")
	}

	output := buf.String()
	if !strings.Contains(output, "PASSED") {
		t.Errorf("expected PASSED in output, got %q", output)
	}
}

func TestStream_ProgressReporter_UnknownEvent(t *testing.T) {
	var buf bytes.Buffer
	reporter := &progressReporter{
		out:       &buf,
		testNames: make(map[int64]string),
	}

	result, done := reporter.handleEvent(SSEEvent{Event: "unknown", Data: json.RawMessage(`{}`)})
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
				{Tests: []snapshotTest{
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

	client := NewClient(server.URL)
	var buf bytes.Buffer
	result, err := client.StreamRun(context.Background(), "run-123", &buf)
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

	output := buf.String()
	if !strings.Contains(output, "test-a") {
		t.Errorf("expected test-a in output")
	}
	if !strings.Contains(output, "PASSED") {
		t.Errorf("expected PASSED in output")
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
		snap := snapshotRun{Suites: []snapshotSuite{{Tests: []snapshotTest{{ID: 1, Name: "t1", Status: "pending"}}}}}
		snapData, _ := json.Marshal(snap)
		fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", snapData)
		flusher.Flush()

		// Wait for context cancellation
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	var buf bytes.Buffer
	_, err := client.StreamRun(ctx, "run-123", &buf)
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

	client := NewClient(server.URL)
	var buf bytes.Buffer
	_, err := client.StreamRun(context.Background(), "run-123", &buf)
	if err == nil {
		t.Errorf("expected error for server error")
	}
}
