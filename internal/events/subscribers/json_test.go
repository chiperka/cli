package subscribers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"chiperka-cli/internal/events"
)

// newJSONTest creates a JSONReporter with a buffer for testing.
func newJSONTest(t *testing.T) (*events.Bus, *JSONReporter, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	bus := events.NewBus()
	jr := NewJSONReporter(&buf)
	jr.Register(bus)
	return bus, jr, &buf
}

// parseJSONLine parses a single NDJSON line into a map.
func parseJSONLine(t *testing.T, line string) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(line), &result); err != nil {
		t.Fatalf("failed to parse JSON line: %v\nline: %s", err, line)
	}
	return result
}

// getLines returns non-empty lines from the output.
func getLines(output string) []string {
	trimmed := strings.TrimRight(output, "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func TestJSONRunStarted(t *testing.T) {
	bus, _, buf := newJSONTest(t)

	e := events.NewEvent(events.RunStarted).
		WithDetail("tests", 24).
		WithDetail("suites", 3).
		WithDetail("workers", 4).
		WithDetail("version", "1.2.3")
	bus.Emit(e)

	lines := getLines(buf.String())
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	obj := parseJSONLine(t, lines[0])
	if obj["event"] != "run.started" {
		t.Errorf("expected event=run.started, got %v", obj["event"])
	}
	if obj["timestamp"] == nil {
		t.Error("missing timestamp")
	}

	data := obj["data"].(map[string]any)
	if data["tests"] != float64(24) {
		t.Errorf("expected tests=24, got %v", data["tests"])
	}
	if data["suites"] != float64(3) {
		t.Errorf("expected suites=3, got %v", data["suites"])
	}
	if data["workers"] != float64(4) {
		t.Errorf("expected workers=4, got %v", data["workers"])
	}
	if data["version"] != "1.2.3" {
		t.Errorf("expected version=1.2.3, got %v", data["version"])
	}
}

func TestJSONTestCompleted(t *testing.T) {
	bus, _, buf := newJSONTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "auth", "login", "/tests/auth.chiperka"))

	// Assertion pass via log event
	passLog := events.NewTestEvent(events.LogPass, "auth", "login")
	passLog.Data.Message = "statusCode == 200"
	passLog.Data.Details["action"] = "assertion_pass"
	bus.Emit(passLog)

	bus.Emit(testEvent(events.TestCompleted, "auth", "login", "/tests/auth.chiperka").
		WithDuration(843 * time.Millisecond))

	lines := getLines(buf.String())
	// run.started + test.started + test.completed = 3 lines
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), buf.String())
	}

	obj := parseJSONLine(t, lines[2])
	if obj["event"] != "test.completed" {
		t.Errorf("expected event=test.completed, got %v", obj["event"])
	}
	if obj["suite"] != "auth" {
		t.Errorf("expected suite=auth, got %v", obj["suite"])
	}
	if obj["test"] != "login" {
		t.Errorf("expected test=login, got %v", obj["test"])
	}

	data := obj["data"].(map[string]any)
	if data["status"] != "passed" {
		t.Errorf("expected status=passed, got %v", data["status"])
	}
	if data["duration_ms"] != float64(843) {
		t.Errorf("expected duration_ms=843, got %v", data["duration_ms"])
	}

	assertions := data["assertions"].([]any)
	if len(assertions) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(assertions))
	}
	a := assertions[0].(map[string]any)
	if a["assertion"] != "statusCode == 200" {
		t.Errorf("expected assertion name, got %v", a["assertion"])
	}
	if a["status"] != "pass" {
		t.Errorf("expected status=pass, got %v", a["status"])
	}
}

func TestJSONTestFailed_SingleAssertion(t *testing.T) {
	bus, _, buf := newJSONTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "api", "status-check", "/tests/api.chiperka"))

	// Assertion fail via log event
	failLog := events.NewTestEvent(events.LogFail, "api", "status-check")
	failLog.Data.Message = "statusCode == 200"
	failLog.Data.Details["action"] = "assertion_fail"
	failLog.Data.Details["expected"] = "200"
	failLog.Data.Details["actual"] = "404"
	bus.Emit(failLog)

	bus.Emit(testEvent(events.TestFailed, "api", "status-check", "/tests/api.chiperka").
		WithDuration(500 * time.Millisecond))

	lines := getLines(buf.String())
	// Find test.failed line
	var failedLine map[string]any
	for _, line := range lines {
		obj := parseJSONLine(t, line)
		if obj["event"] == "test.failed" {
			failedLine = obj
			break
		}
	}
	if failedLine == nil {
		t.Fatal("no test.failed line found")
	}

	data := failedLine["data"].(map[string]any)
	if data["status"] != "failed" {
		t.Errorf("expected status=failed, got %v", data["status"])
	}
	if data["duration_ms"] != float64(500) {
		t.Errorf("expected duration_ms=500, got %v", data["duration_ms"])
	}

	assertions := data["assertions"].([]any)
	if len(assertions) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(assertions))
	}
	a := assertions[0].(map[string]any)
	if a["assertion"] != "statusCode == 200" {
		t.Errorf("expected assertion=statusCode == 200, got %v", a["assertion"])
	}
	if a["status"] != "fail" {
		t.Errorf("expected status=fail, got %v", a["status"])
	}
	if a["expected"] != "200" {
		t.Errorf("expected expected=200, got %v", a["expected"])
	}
	if a["actual"] != "404" {
		t.Errorf("expected actual=404, got %v", a["actual"])
	}
}

func TestJSONTestFailed_MultipleAssertions(t *testing.T) {
	bus, _, buf := newJSONTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "api", "multi-check", ""))

	// First assertion fail
	e1 := events.NewTestEvent(events.LogFail, "api", "multi-check")
	e1.Data.Message = "statusCode == 200"
	e1.Data.Details["action"] = "assertion_fail"
	e1.Data.Details["expected"] = "200"
	e1.Data.Details["actual"] = "500"
	bus.Emit(e1)

	// Second assertion fail
	e2 := events.NewTestEvent(events.LogFail, "api", "multi-check")
	e2.Data.Message = "body contains 'ok'"
	e2.Data.Details["action"] = "assertion_fail"
	e2.Data.Details["expected"] = "ok"
	e2.Data.Details["actual"] = "error"
	bus.Emit(e2)

	bus.Emit(testEvent(events.TestFailed, "api", "multi-check", "").
		WithDuration(300 * time.Millisecond))

	lines := getLines(buf.String())
	var failedLine map[string]any
	for _, line := range lines {
		obj := parseJSONLine(t, line)
		if obj["event"] == "test.failed" {
			failedLine = obj
			break
		}
	}
	if failedLine == nil {
		t.Fatal("no test.failed line found")
	}

	data := failedLine["data"].(map[string]any)
	assertions := data["assertions"].([]any)
	if len(assertions) != 2 {
		t.Fatalf("expected 2 assertions, got %d", len(assertions))
	}

	a1 := assertions[0].(map[string]any)
	if a1["assertion"] != "statusCode == 200" {
		t.Errorf("first assertion name wrong: %v", a1["assertion"])
	}

	a2 := assertions[1].(map[string]any)
	if a2["assertion"] != "body contains 'ok'" {
		t.Errorf("second assertion name wrong: %v", a2["assertion"])
	}
}

func TestJSONTestFailed_WithError(t *testing.T) {
	bus, _, buf := newJSONTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "api", "timeout-test", ""))

	// TestError event (collected, not emitted as separate line)
	errEvent := events.NewTestEvent(events.TestError, "api", "timeout-test")
	errEvent.Data.Message = "timeout connecting to service"
	bus.Emit(errEvent)

	// Assertion fail
	failLog := events.NewTestEvent(events.LogFail, "api", "timeout-test")
	failLog.Data.Message = "statusCode == 200"
	failLog.Data.Details["action"] = "assertion_fail"
	failLog.Data.Details["expected"] = "200"
	failLog.Data.Details["actual"] = "0"
	bus.Emit(failLog)

	bus.Emit(testEvent(events.TestFailed, "api", "timeout-test", "").
		WithDuration(5000 * time.Millisecond))

	lines := getLines(buf.String())
	var failedLine map[string]any
	for _, line := range lines {
		obj := parseJSONLine(t, line)
		if obj["event"] == "test.failed" {
			failedLine = obj
			break
		}
	}
	if failedLine == nil {
		t.Fatal("no test.failed line found")
	}

	data := failedLine["data"].(map[string]any)

	// Should have errors array
	errors := data["errors"].([]any)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	if errors[0] != "timeout connecting to service" {
		t.Errorf("expected error message, got %v", errors[0])
	}

	// Should also have assertions
	assertions := data["assertions"].([]any)
	if len(assertions) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(assertions))
	}
}

func TestJSONTestFailed_MessageOnly(t *testing.T) {
	bus, _, buf := newJSONTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "error-test", ""))
	bus.Emit(testEvent(events.TestFailed, "suite", "error-test", "").
		WithDuration(100 * time.Millisecond).
		WithMessage("Connection refused"))

	lines := getLines(buf.String())
	var failedLine map[string]any
	for _, line := range lines {
		obj := parseJSONLine(t, line)
		if obj["event"] == "test.failed" {
			failedLine = obj
			break
		}
	}
	if failedLine == nil {
		t.Fatal("no test.failed line found")
	}

	data := failedLine["data"].(map[string]any)
	if data["error"] != "Connection refused" {
		t.Errorf("expected error=Connection refused, got %v", data["error"])
	}
	// No assertions key when empty
	if data["assertions"] != nil {
		t.Error("expected no assertions key when no assertions collected")
	}
}

func TestJSONTestSkipped(t *testing.T) {
	bus, _, buf := newJSONTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))

	e := testEvent(events.TestSkipped, "api", "admin-test", "/tests/api.chiperka")
	e.Data.Message = "requires admin"
	bus.Emit(e)

	lines := getLines(buf.String())
	var skippedLine map[string]any
	for _, line := range lines {
		obj := parseJSONLine(t, line)
		if obj["event"] == "test.skipped" {
			skippedLine = obj
			break
		}
	}
	if skippedLine == nil {
		t.Fatal("no test.skipped line found")
	}

	if skippedLine["suite"] != "api" {
		t.Errorf("expected suite=api, got %v", skippedLine["suite"])
	}
	if skippedLine["test"] != "admin-test" {
		t.Errorf("expected test=admin-test, got %v", skippedLine["test"])
	}

	data := skippedLine["data"].(map[string]any)
	if data["reason"] != "requires admin" {
		t.Errorf("expected reason=requires admin, got %v", data["reason"])
	}
}

func TestJSONRunCompleted(t *testing.T) {
	bus, _, buf := newJSONTest(t)

	e := events.NewEvent(events.RunCompleted).
		WithDetail("passed", 23).
		WithDetail("failed", 1).
		WithDetail("skipped", 0).
		WithDuration(12400 * time.Millisecond)
	bus.Emit(e)

	lines := getLines(buf.String())
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	obj := parseJSONLine(t, lines[0])
	if obj["event"] != "run.completed" {
		t.Errorf("expected event=run.completed, got %v", obj["event"])
	}

	data := obj["data"].(map[string]any)
	if data["passed"] != float64(23) {
		t.Errorf("expected passed=23, got %v", data["passed"])
	}
	if data["failed"] != float64(1) {
		t.Errorf("expected failed=1, got %v", data["failed"])
	}
	if data["duration_ms"] != float64(12400) {
		t.Errorf("expected duration_ms=12400, got %v", data["duration_ms"])
	}
}

func TestJSONAssertResult_TypedEvent(t *testing.T) {
	bus, _, buf := newJSONTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))

	// Typed assertion result (pass)
	passEvent := events.NewTestEvent(events.TestAssertResult, "suite", "test1")
	passEvent.Data.Status = "pass"
	passEvent.Data.Details["assertion"] = "response.statusCode"
	passEvent.Data.Details["expected"] = 200
	passEvent.Data.Details["actual"] = 200
	bus.Emit(passEvent)

	// Typed assertion result (fail)
	failEvent := events.NewTestEvent(events.TestAssertResult, "suite", "test1")
	failEvent.Data.Status = "fail"
	failEvent.Data.Details["assertion"] = "body"
	failEvent.Data.Details["expected"] = "ok"
	failEvent.Data.Details["actual"] = "error"
	bus.Emit(failEvent)

	bus.Emit(testEvent(events.TestFailed, "suite", "test1", "").
		WithDuration(100 * time.Millisecond))

	lines := getLines(buf.String())
	var failedLine map[string]any
	for _, line := range lines {
		obj := parseJSONLine(t, line)
		if obj["event"] == "test.failed" {
			failedLine = obj
			break
		}
	}
	if failedLine == nil {
		t.Fatal("no test.failed line found")
	}

	data := failedLine["data"].(map[string]any)
	assertions := data["assertions"].([]any)
	if len(assertions) != 2 {
		t.Fatalf("expected 2 assertions, got %d", len(assertions))
	}

	// First: pass (no expected/actual)
	a1 := assertions[0].(map[string]any)
	if a1["assertion"] != "response.statusCode" {
		t.Errorf("expected assertion=response.statusCode, got %v", a1["assertion"])
	}
	if a1["status"] != "pass" {
		t.Errorf("expected status=pass, got %v", a1["status"])
	}
	if a1["expected"] != nil {
		t.Error("pass assertion should not have expected")
	}

	// Second: fail (with expected/actual)
	a2 := assertions[1].(map[string]any)
	if a2["assertion"] != "body" {
		t.Errorf("expected assertion=body, got %v", a2["assertion"])
	}
	if a2["status"] != "fail" {
		t.Errorf("expected status=fail, got %v", a2["status"])
	}
	if a2["expected"] != "ok" {
		t.Errorf("expected expected=ok, got %v", a2["expected"])
	}
	if a2["actual"] != "error" {
		t.Errorf("expected actual=error, got %v", a2["actual"])
	}
}

func TestJSONFullRun_MixedResults(t *testing.T) {
	bus, _, buf := newJSONTest(t)

	// Start run
	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 3).
		WithDetail("suites", 1))

	// Test 1: passes
	bus.Emit(testEvent(events.TestStarted, "api", "get-health", "/api.chiperka"))
	passLog := events.NewTestEvent(events.LogPass, "api", "get-health")
	passLog.Data.Message = "statusCode == 200"
	passLog.Data.Details["action"] = "assertion_pass"
	bus.Emit(passLog)
	bus.Emit(testEvent(events.TestCompleted, "api", "get-health", "/api.chiperka").
		WithDuration(100 * time.Millisecond))

	// Test 2: fails
	bus.Emit(testEvent(events.TestStarted, "api", "post-login", "/api.chiperka"))
	failLog := events.NewTestEvent(events.LogFail, "api", "post-login")
	failLog.Data.Message = "statusCode == 200"
	failLog.Data.Details["action"] = "assertion_fail"
	failLog.Data.Details["expected"] = "200"
	failLog.Data.Details["actual"] = "401"
	bus.Emit(failLog)
	bus.Emit(testEvent(events.TestFailed, "api", "post-login", "/api.chiperka").
		WithDuration(200 * time.Millisecond))

	// Test 3: skipped
	skipEvent := testEvent(events.TestSkipped, "api", "delete-user", "/api.chiperka")
	skipEvent.Data.Message = "requires admin"
	bus.Emit(skipEvent)

	// Run completed
	bus.Emit(events.NewEvent(events.RunCompleted).
		WithDetail("passed", 1).
		WithDetail("failed", 1).
		WithDetail("skipped", 1).
		WithDuration(500 * time.Millisecond))

	lines := getLines(buf.String())

	// Expected: run.started, test.started, test.completed, test.started, test.failed, test.skipped, run.completed = 7
	if len(lines) != 7 {
		t.Fatalf("expected 7 lines, got %d:\n%s", len(lines), buf.String())
	}

	// Verify event sequence
	expectedEvents := []string{
		"run.started", "test.started", "test.completed",
		"test.started", "test.failed", "test.skipped",
		"run.completed",
	}
	for i, line := range lines {
		obj := parseJSONLine(t, line)
		if obj["event"] != expectedEvents[i] {
			t.Errorf("line %d: expected event=%s, got %v", i, expectedEvents[i], obj["event"])
		}
	}
}

func TestJSONOutput_ValidNDJSON(t *testing.T) {
	bus, _, buf := newJSONTest(t)

	// Emit a variety of events
	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 2))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", "/test.chiperka"))
	bus.Emit(testEvent(events.TestCompleted, "suite", "test1", "/test.chiperka").
		WithDuration(100 * time.Millisecond))
	bus.Emit(testEvent(events.TestStarted, "suite", "test2", "/test.chiperka"))
	bus.Emit(testEvent(events.TestFailed, "suite", "test2", "/test.chiperka").
		WithDuration(200 * time.Millisecond).
		WithMessage("failed"))
	bus.Emit(events.NewEvent(events.RunCompleted).WithDuration(300 * time.Millisecond))

	lines := getLines(buf.String())
	if len(lines) == 0 {
		t.Fatal("expected output, got none")
	}

	for i, line := range lines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("line %d is not valid JSON: %v\nline: %s", i, err, line)
		}
		if obj["event"] == nil {
			t.Errorf("line %d missing 'event' field: %s", i, line)
		}
		if obj["timestamp"] == nil {
			t.Errorf("line %d missing 'timestamp' field: %s", i, line)
		}
	}
}

func TestJSONTestError_NotEmittedAsSeparateLine(t *testing.T) {
	bus, _, buf := newJSONTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))

	// TestError should NOT produce a line
	errEvent := events.NewTestEvent(events.TestError, "suite", "test1")
	errEvent.Data.Message = "docker: connection refused"
	bus.Emit(errEvent)

	lines := getLines(buf.String())
	for _, line := range lines {
		obj := parseJSONLine(t, line)
		event := obj["event"].(string)
		if strings.Contains(event, "error") {
			t.Errorf("TestError should not emit a separate line, got event=%s", event)
		}
	}
}

func TestJSONTestError_WithErrorField(t *testing.T) {
	bus, _, buf := newJSONTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))

	// TestError with Error field (not Message)
	errEvent := events.NewTestEvent(events.TestError, "suite", "test1")
	errEvent.Data.Error = fmt.Errorf("docker: connection refused")
	bus.Emit(errEvent)

	bus.Emit(testEvent(events.TestFailed, "suite", "test1", "").
		WithDuration(100 * time.Millisecond))

	lines := getLines(buf.String())
	var failedLine map[string]any
	for _, line := range lines {
		obj := parseJSONLine(t, line)
		if obj["event"] == "test.failed" {
			failedLine = obj
			break
		}
	}
	if failedLine == nil {
		t.Fatal("no test.failed line found")
	}

	data := failedLine["data"].(map[string]any)
	errors := data["errors"].([]any)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	if errors[0] != "docker: connection refused" {
		t.Errorf("expected error message from Error field, got %v", errors[0])
	}
}

func TestJSONOmitsEmptyFields(t *testing.T) {
	bus, _, buf := newJSONTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))

	// Test started without suite or file
	bus.Emit(testEvent(events.TestStarted, "", "standalone", ""))

	lines := getLines(buf.String())
	// Find test.started line
	var startedLine map[string]any
	for _, line := range lines {
		obj := parseJSONLine(t, line)
		if obj["event"] == "test.started" {
			startedLine = obj
			break
		}
	}
	if startedLine == nil {
		t.Fatal("no test.started line found")
	}

	// suite and file should be omitted (empty string with omitempty)
	if startedLine["suite"] != nil {
		t.Errorf("expected suite to be omitted, got %v", startedLine["suite"])
	}
	if startedLine["file"] != nil {
		t.Errorf("expected file to be omitted, got %v", startedLine["file"])
	}
}
