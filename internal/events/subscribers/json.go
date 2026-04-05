package subscribers

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"chiperka-cli/internal/events"
)

// JSONReporter outputs NDJSON (one JSON object per line) for machine consumption.
// Designed for AI agents and CI pipelines that need structured, real-time output.
type JSONReporter struct {
	output io.Writer
	mu     sync.Mutex

	// Per-test state for collecting assertions and errors
	testStates map[string]*jsonTestState
}

// jsonTestState tracks assertions and errors collected during test execution.
type jsonTestState struct {
	assertions []jsonAssertion
	errors     []string
}

// jsonAssertion represents a single assertion result.
type jsonAssertion struct {
	Assertion string `json:"assertion"`
	Status    string `json:"status"`
	Expected  string `json:"expected,omitempty"`
	Actual    string `json:"actual,omitempty"`
}

// jsonLine is the top-level structure for each NDJSON line.
type jsonLine struct {
	Event     string         `json:"event"`
	Timestamp string         `json:"timestamp"`
	Suite     string         `json:"suite,omitempty"`
	Test      string         `json:"test,omitempty"`
	File      string         `json:"file,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// NewJSONReporter creates a new NDJSON reporter.
func NewJSONReporter(output io.Writer) *JSONReporter {
	return &JSONReporter{
		output:     output,
		testStates: make(map[string]*jsonTestState),
	}
}

// Register subscribes this reporter to key lifecycle events.
func (jr *JSONReporter) Register(bus *events.Bus) {
	bus.On(events.RunStarted, jr.onRunStarted)
	bus.On(events.RunCompleted, jr.onRunCompleted)

	bus.On(events.TestStarted, jr.onTestStarted)
	bus.On(events.TestCompleted, jr.onTestCompleted)
	bus.On(events.TestFailed, jr.onTestFailed)
	bus.On(events.TestSkipped, jr.onTestSkipped)
	bus.On(events.TestError, jr.onTestError)

	// Typed assertion events
	bus.On(events.TestAssertResult, jr.onTestAssertResult)

	// Log-based assertion events (primary path used by runner)
	bus.On(events.LogPass, jr.onLogEvent)
	bus.On(events.LogFail, jr.onLogEvent)
}

// --- Helpers ---

func (jr *JSONReporter) getTestState(e *events.Event) *jsonTestState {
	key := e.TestKey()
	if st, ok := jr.testStates[key]; ok {
		return st
	}
	st := &jsonTestState{}
	jr.testStates[key] = st
	return st
}

func (jr *JSONReporter) cleanTestState(e *events.Event) {
	delete(jr.testStates, e.TestKey())
}

func (jr *JSONReporter) emit(line jsonLine) {
	data, err := json.Marshal(line)
	if err != nil {
		return
	}
	fmt.Fprintf(jr.output, "%s\n", data)
}

func formatTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// --- Run lifecycle ---

func (jr *JSONReporter) onRunStarted(e *events.Event) {
	jr.mu.Lock()
	defer jr.mu.Unlock()

	data := map[string]any{}
	if tests, ok := e.Data.Details["tests"].(int); ok {
		data["tests"] = tests
	}
	if suites, ok := e.Data.Details["suites"].(int); ok {
		data["suites"] = suites
	}
	if workers, ok := e.Data.Details["workers"].(int); ok {
		data["workers"] = workers
	}
	if version, ok := e.Data.Details["version"].(string); ok {
		data["version"] = version
	}

	jr.emit(jsonLine{
		Event:     "run.started",
		Timestamp: formatTimestamp(e.Timestamp),
		Data:      data,
	})
}

func (jr *JSONReporter) onRunCompleted(e *events.Event) {
	jr.mu.Lock()
	defer jr.mu.Unlock()

	data := map[string]any{}
	if passed, ok := e.Data.Details["passed"].(int); ok {
		data["passed"] = passed
	}
	if failed, ok := e.Data.Details["failed"].(int); ok {
		data["failed"] = failed
	}
	if skipped, ok := e.Data.Details["skipped"].(int); ok {
		data["skipped"] = skipped
	}
	if e.Data.Duration > 0 {
		data["duration_ms"] = e.Data.Duration.Milliseconds()
	}

	jr.emit(jsonLine{
		Event:     "run.completed",
		Timestamp: formatTimestamp(e.Timestamp),
		Data:      data,
	})
}

// --- Test lifecycle ---

func (jr *JSONReporter) onTestStarted(e *events.Event) {
	jr.mu.Lock()
	defer jr.mu.Unlock()

	jr.getTestState(e) // initialize state

	jr.emit(jsonLine{
		Event:     "test.started",
		Timestamp: formatTimestamp(e.Timestamp),
		Suite:     e.SuiteName,
		Test:      e.TestName,
		File:      e.FilePath,
	})
}

func (jr *JSONReporter) onTestCompleted(e *events.Event) {
	jr.mu.Lock()
	defer jr.mu.Unlock()

	st := jr.getTestState(e)

	data := map[string]any{
		"status":      "passed",
		"duration_ms": e.Data.Duration.Milliseconds(),
	}
	if len(st.assertions) > 0 {
		data["assertions"] = st.assertions
	}

	jr.emit(jsonLine{
		Event:     "test.completed",
		Timestamp: formatTimestamp(e.Timestamp),
		Suite:     e.SuiteName,
		Test:      e.TestName,
		Data:      data,
	})
	jr.cleanTestState(e)
}

func (jr *JSONReporter) onTestFailed(e *events.Event) {
	jr.mu.Lock()
	defer jr.mu.Unlock()

	st := jr.getTestState(e)

	data := map[string]any{
		"status":      "failed",
		"duration_ms": e.Data.Duration.Milliseconds(),
	}

	if len(st.assertions) > 0 {
		data["assertions"] = st.assertions
	}
	if len(st.errors) > 0 {
		data["errors"] = st.errors
	}
	if e.Data.Message != "" {
		data["error"] = e.Data.Message
	}

	jr.emit(jsonLine{
		Event:     "test.failed",
		Timestamp: formatTimestamp(e.Timestamp),
		Suite:     e.SuiteName,
		Test:      e.TestName,
		Data:      data,
	})
	jr.cleanTestState(e)
}

func (jr *JSONReporter) onTestSkipped(e *events.Event) {
	jr.mu.Lock()
	defer jr.mu.Unlock()

	data := map[string]any{}
	if e.Data.Message != "" {
		data["reason"] = e.Data.Message
	}

	jr.emit(jsonLine{
		Event:     "test.skipped",
		Timestamp: formatTimestamp(e.Timestamp),
		Suite:     e.SuiteName,
		Test:      e.TestName,
		Data:      data,
	})
	jr.cleanTestState(e)
}

func (jr *JSONReporter) onTestError(e *events.Event) {
	jr.mu.Lock()
	defer jr.mu.Unlock()

	msg := e.Data.Message
	if e.Data.Error != nil {
		msg = e.Data.Error.Error()
	}

	st := jr.getTestState(e)
	st.errors = append(st.errors, msg)
}

// --- Typed assertion events ---

func (jr *JSONReporter) onTestAssertResult(e *events.Event) {
	jr.mu.Lock()
	defer jr.mu.Unlock()

	assertion, _ := e.Data.Details["assertion"].(string)
	st := jr.getTestState(e)

	a := jsonAssertion{
		Assertion: assertion,
		Status:    e.Data.Status,
	}
	if e.Data.Status == "fail" {
		a.Expected = fmt.Sprintf("%v", e.Data.Details["expected"])
		a.Actual = fmt.Sprintf("%v", e.Data.Details["actual"])
	}

	st.assertions = append(st.assertions, a)
}

// --- Log-based assertion events ---

func (jr *JSONReporter) onLogEvent(e *events.Event) {
	jr.mu.Lock()
	defer jr.mu.Unlock()

	if e.TestName == "" || e.Data.Message == "" {
		return
	}

	action, _ := e.Data.Details["action"].(string)

	if action == "assertion_pass" {
		st := jr.getTestState(e)
		st.assertions = append(st.assertions, jsonAssertion{
			Assertion: e.Data.Message,
			Status:    "pass",
		})
		return
	}

	if action == "assertion_fail" {
		st := jr.getTestState(e)
		st.assertions = append(st.assertions, jsonAssertion{
			Assertion: e.Data.Message,
			Status:    "fail",
			Expected:  fmt.Sprintf("%v", e.Data.Details["expected"]),
			Actual:    fmt.Sprintf("%v", e.Data.Details["actual"]),
		})
		return
	}
}
