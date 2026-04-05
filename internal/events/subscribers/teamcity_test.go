package subscribers

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"chiperka-cli/internal/events"
)

// newTeamCityTest creates a TeamCityReporter with a buffer for testing.
// Uses a fixed flowId for deterministic output.
func newTeamCityTest(t *testing.T, opts ...func(*TeamCityReporter)) (*events.Bus, *TeamCityReporter, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	bus := events.NewBus()
	tc := NewTeamCityReporter(&buf, "", "", "")
	tc.flowId = "12345" // fixed for predictable output
	for _, opt := range opts {
		opt(tc)
	}
	tc.Register(bus)
	return bus, tc, &buf
}

// testEvent creates an event with suite/test context and file path.
func testEvent(eventType events.Type, suite, test, filePath string) *events.Event {
	e := events.NewTestEvent(eventType, suite, test)
	e.FilePath = filePath
	return e
}

// assertContains checks that the output contains the expected substring.
func assertContains(t *testing.T, output, expected string) {
	t.Helper()
	if !strings.Contains(output, expected) {
		t.Errorf("output does not contain expected string\nexpected substring: %s\nfull output:\n%s", expected, output)
	}
}

// assertNotContains checks that the output does NOT contain the substring.
func assertNotContains(t *testing.T, output, unexpected string) {
	t.Helper()
	if strings.Contains(output, unexpected) {
		t.Errorf("output should not contain: %s\nfull output:\n%s", unexpected, output)
	}
}

// assertLine checks that a specific line exists in the output.
func assertLine(t *testing.T, output string, lineNum int, expected string) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if lineNum >= len(lines) {
		t.Errorf("expected at least %d lines, got %d\nfull output:\n%s", lineNum+1, len(lines), output)
		return
	}
	if lines[lineNum] != expected {
		t.Errorf("line %d mismatch\nexpected: %s\n  actual: %s", lineNum, expected, lines[lineNum])
	}
}

// lineCount returns the number of non-empty lines in the output.
func lineCount(output string) int {
	trimmed := strings.TrimRight(output, "\n")
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

// --- Escaping ---

func TestTcEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"pipe|char", "pipe||char"},
		{"it's", "it|'s"},
		{"line1\nline2", "line1|nline2"},
		{"cr\rhere", "cr|rhere"},
		{"open[bracket", "open|[bracket"},
		{"close]bracket", "close|]bracket"},
		// pipe must be escaped FIRST to avoid double-escaping
		{"pipe|and'quote", "pipe||and|'quote"},
		{"all|special'\n\r[]chars", "all||special|'|n|r|[|]chars"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("input=%q", tt.input), func(t *testing.T) {
			got := tcEscape(tt.input)
			if got != tt.expected {
				t.Errorf("tcEscape(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// --- testCount ---

func TestRunStarted_EmitsTestCount(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	e := events.NewEvent(events.RunStarted).
		WithDetail("tests", 24).
		WithDetail("suites", 3).
		WithDetail("suiteCounts", map[string]int{"auth": 5, "api": 10, "db": 9})
	bus.Emit(e)

	assertLine(t, buf.String(), 0, "##teamcity[testCount count='24' flowId='12345']")
}

// --- Passing test lifecycle ---

func TestPassingTest_FullLifecycle(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	// Register suite counts
	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"auth-suite": 1}))
	buf.Reset()

	// Test started
	bus.Emit(testEvent(events.TestStarted, "auth-suite", "login-test", "/tests/auth.chiperka"))

	// Test completed
	bus.Emit(testEvent(events.TestCompleted, "auth-suite", "login-test", "/tests/auth.chiperka").
		WithDuration(1234 * time.Millisecond))

	output := buf.String()

	// Line 0: testSuiteStarted
	assertLine(t, output, 0, "##teamcity[testSuiteStarted name='auth-suite' nodeId='auth-suite' parentNodeId='0' locationHint='chiperka:///tests/auth.chiperka' flowId='12345']")

	// Line 1: testStarted
	assertLine(t, output, 1, "##teamcity[testStarted name='login-test' nodeId='auth-suite/login-test' parentNodeId='auth-suite' locationHint='chiperka:///tests/auth.chiperka::login-test' flowId='12345']")

	// Line 2: testFinished
	assertLine(t, output, 2, "##teamcity[testFinished name='login-test' nodeId='auth-suite/login-test' duration='1234' flowId='12345']")

	// Line 3: testSuiteFinished (auto-closed because 1/1 tests done)
	assertLine(t, output, 3, "##teamcity[testSuiteFinished name='auth-suite' nodeId='auth-suite' flowId='12345']")

	if lineCount(output) != 4 {
		t.Errorf("expected 4 lines, got %d\n%s", lineCount(output), output)
	}
}

// --- Failing test with single assertion ---

func TestFailingTest_SingleAssertion(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"api": 1}))
	buf.Reset()

	// Test started
	bus.Emit(testEvent(events.TestStarted, "api", "status-check", "/tests/api.chiperka"))

	// Assertion fail via log event (this is the primary path used by the runner)
	logEvent := events.NewTestEvent(events.LogFail, "api", "status-check")
	logEvent.Data.Message = "statusCode == 200"
	logEvent.Data.Details["action"] = "assertion_fail"
	logEvent.Data.Details["expected"] = "200"
	logEvent.Data.Details["actual"] = "404"
	bus.Emit(logEvent)

	// Test failed
	bus.Emit(testEvent(events.TestFailed, "api", "status-check", "/tests/api.chiperka").
		WithDuration(500 * time.Millisecond))

	output := buf.String()

	// testSuiteStarted
	assertContains(t, output, "##teamcity[testSuiteStarted name='api'")

	// testStarted
	assertContains(t, output, "##teamcity[testStarted name='status-check' nodeId='api/status-check' parentNodeId='api'")

	// Assertion stderr output with nodeId
	assertContains(t, output, "##teamcity[testStdErr name='status-check' nodeId='api/status-check'")

	// testFailed with comparisonFailure
	assertContains(t, output, "##teamcity[testFailed name='status-check' nodeId='api/status-check' message='statusCode == 200' type='comparisonFailure' expected='200' actual='404' flowId='12345']")

	// testFinished with duration (duration only on testFinished, NOT on testFailed)
	assertContains(t, output, "##teamcity[testFinished name='status-check' nodeId='api/status-check' duration='500' flowId='12345']")

	// No duration on testFailed
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "testFailed") {
			assertNotContains(t, line, "duration=")
		}
	}
}

// --- Failing test with multiple assertions ---

func TestFailingTest_MultipleAssertions(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"api": 1}))
	buf.Reset()

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

	output := buf.String()

	// testFailed with consolidated comparison
	assertContains(t, output, "message='2 assertion(s) failed: statusCode == 200, body contains |'ok|''")
	assertContains(t, output, "type='comparisonFailure'")
	assertContains(t, output, "expected='statusCode == 200: 200|nbody contains |'ok|': ok'")
	assertContains(t, output, "actual='statusCode == 200: 500|nbody contains |'ok|': error'")
}

// --- Failing test with message only (no collected assertions) ---

func TestFailingTest_MessageOnly(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"suite": 1}))
	buf.Reset()

	bus.Emit(testEvent(events.TestStarted, "suite", "error-test", ""))
	bus.Emit(testEvent(events.TestFailed, "suite", "error-test", "").
		WithDuration(100 * time.Millisecond).
		WithMessage("Connection refused"))

	output := buf.String()
	assertContains(t, output, "##teamcity[testFailed name='error-test' nodeId='suite/error-test' message='Connection refused' flowId='12345']")
}

// --- Failing test with no message and no assertions (default message) ---

func TestFailingTest_DefaultMessage(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"suite": 1}))
	buf.Reset()

	bus.Emit(testEvent(events.TestStarted, "suite", "empty-test", ""))
	bus.Emit(testEvent(events.TestFailed, "suite", "empty-test", "").
		WithDuration(50 * time.Millisecond))

	output := buf.String()
	assertContains(t, output, "message='Test failed'")
}

// --- Skipped test ---

func TestSkippedTest(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"suite": 1}))
	buf.Reset()

	e := testEvent(events.TestSkipped, "suite", "skip-test", "/tests/skip.chiperka")
	e.Data.Message = "requires postgres"
	bus.Emit(e)

	output := buf.String()

	// testSuiteStarted
	assertContains(t, output, "##teamcity[testSuiteStarted name='suite'")

	// testStarted (skipped tests still get started/finished)
	assertContains(t, output, "##teamcity[testStarted name='skip-test' nodeId='suite/skip-test' parentNodeId='suite'")

	// testIgnored with message
	assertContains(t, output, "##teamcity[testIgnored name='skip-test' nodeId='suite/skip-test' message='requires postgres' flowId='12345']")

	// testFinished with duration 0
	assertContains(t, output, "##teamcity[testFinished name='skip-test' nodeId='suite/skip-test' duration='0' flowId='12345']")
}

func TestSkippedTest_DefaultMessage(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"suite": 1}))
	buf.Reset()

	bus.Emit(testEvent(events.TestSkipped, "suite", "skip-test", ""))

	assertContains(t, buf.String(), "message='skipped'")
}

// --- testStdOut and testStdErr include nodeId ---

func TestStdOut_IncludesNodeId(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))

	// Emit a log event that produces testStdOut
	logEvent := events.NewTestEvent(events.LogInfo, "suite", "test1")
	logEvent.Data.Message = "some output"
	logEvent.Data.Details["action"] = "http_request"
	bus.Emit(logEvent)

	output := buf.String()

	// testStdOut MUST include nodeId for id-based tree output routing
	assertContains(t, output, "##teamcity[testStdOut name='test1' nodeId='suite/test1' out='")
}

func TestStdErr_IncludesNodeId(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))

	// Emit a log fail event that produces testStdErr
	logEvent := events.NewTestEvent(events.LogFail, "suite", "test1")
	logEvent.Data.Message = "error occurred"
	logEvent.Data.Details["action"] = "some_action"
	bus.Emit(logEvent)

	output := buf.String()

	// testStdErr MUST include nodeId for id-based tree output routing
	assertContains(t, output, "##teamcity[testStdErr name='test1' nodeId='suite/test1' out='")
}

// --- Suite lifecycle ---

func TestSuiteAutoClose_WhenAllTestsDone(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 2).
		WithDetail("suiteCounts", map[string]int{"suite": 2}))
	buf.Reset()

	// First test
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", "/f.chiperka"))
	bus.Emit(testEvent(events.TestCompleted, "suite", "test1", "/f.chiperka").
		WithDuration(100 * time.Millisecond))

	// Suite should NOT be closed yet (1 of 2 done)
	assertNotContains(t, buf.String(), "testSuiteFinished")

	// Second test
	bus.Emit(testEvent(events.TestStarted, "suite", "test2", "/f.chiperka"))
	bus.Emit(testEvent(events.TestCompleted, "suite", "test2", "/f.chiperka").
		WithDuration(200 * time.Millisecond))

	// Now suite should be closed
	assertContains(t, buf.String(), "##teamcity[testSuiteFinished name='suite' nodeId='suite' flowId='12345']")
}

func TestRunCompleted_ClosesRemainingSuites(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	// No suite counts (unknown total) — suite stays open
	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	bus.Emit(testEvent(events.TestCompleted, "suite", "test1", "").WithDuration(100 * time.Millisecond))

	// Suite still open because no suiteCounts
	assertNotContains(t, buf.String(), "testSuiteFinished")

	// RunCompleted closes all open suites
	bus.Emit(events.NewEvent(events.RunCompleted))

	assertContains(t, buf.String(), "##teamcity[testSuiteFinished name='suite' nodeId='suite' flowId='12345']")
}

func TestMultipleSuites_IndependentLifecycles(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 3).
		WithDetail("suiteCounts", map[string]int{"auth": 1, "api": 2}))
	buf.Reset()

	// Auth suite — single test
	bus.Emit(testEvent(events.TestStarted, "auth", "login", "/auth.chiperka"))
	bus.Emit(testEvent(events.TestCompleted, "auth", "login", "/auth.chiperka").WithDuration(100 * time.Millisecond))

	// Auth suite should be closed
	assertContains(t, buf.String(), "##teamcity[testSuiteFinished name='auth'")

	// API suite — 2 tests, only 1 done so far
	bus.Emit(testEvent(events.TestStarted, "api", "get-users", "/api.chiperka"))
	bus.Emit(testEvent(events.TestCompleted, "api", "get-users", "/api.chiperka").WithDuration(200 * time.Millisecond))

	// Count suiteFinished for api — should be 0
	apiFinishCount := strings.Count(buf.String(), "testSuiteFinished name='api'")
	if apiFinishCount != 0 {
		t.Errorf("api suite should not be closed yet, got %d closes", apiFinishCount)
	}

	// Second API test
	bus.Emit(testEvent(events.TestStarted, "api", "create-user", "/api.chiperka"))
	bus.Emit(testEvent(events.TestCompleted, "api", "create-user", "/api.chiperka").WithDuration(300 * time.Millisecond))

	assertContains(t, buf.String(), "##teamcity[testSuiteFinished name='api'")
}

// --- Suite opened only once ---

func TestSuiteStarted_OnlyOnce(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 2).
		WithDetail("suiteCounts", map[string]int{"suite": 2}))
	buf.Reset()

	bus.Emit(testEvent(events.TestStarted, "suite", "test1", "/f.chiperka"))
	bus.Emit(testEvent(events.TestStarted, "suite", "test2", "/f.chiperka"))

	count := strings.Count(buf.String(), "testSuiteStarted")
	if count != 1 {
		t.Errorf("expected 1 testSuiteStarted, got %d", count)
	}
}

// --- Test without suite ---

func TestTestWithoutSuite(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "", "standalone-test", "/test.chiperka"))
	bus.Emit(testEvent(events.TestCompleted, "", "standalone-test", "/test.chiperka").
		WithDuration(100 * time.Millisecond))

	output := buf.String()

	// No suite messages
	assertNotContains(t, output, "testSuiteStarted")
	assertNotContains(t, output, "testSuiteFinished")

	// Test has parentNodeId='0' (root)
	assertContains(t, output, "##teamcity[testStarted name='standalone-test' nodeId='/standalone-test' parentNodeId='0'")
}

// --- Location hints ---

func TestLocationHint_WithFilePath(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", "/path/to/test.chiperka"))

	output := buf.String()

	// Suite location hint (file path only)
	assertContains(t, output, "locationHint='chiperka:///path/to/test.chiperka'")

	// Test location hint (file path + test name)
	assertContains(t, output, "locationHint='chiperka:///path/to/test.chiperka::test1'")
}

func TestLocationHint_WithoutFilePath(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))

	output := buf.String()

	// No locationHint when no file path (empty attrs are skipped)
	assertNotContains(t, output, "locationHint=")
}

// --- Path mapping ---

func TestPathMapping(t *testing.T) {
	bus, _, buf := newTeamCityTest(t, func(tc *TeamCityReporter) {
		tc.pathMappingFrom = "/container/path"
		tc.pathMappingTo = "/host/path"
	})

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", "/container/path/tests/auth.chiperka"))

	output := buf.String()
	assertContains(t, output, "locationHint='chiperka:///host/path/tests/auth.chiperka::test1'")
	assertContains(t, output, "locationHint='chiperka:///host/path/tests/auth.chiperka'")
}

func TestPathMapping_NoMatch(t *testing.T) {
	bus, _, buf := newTeamCityTest(t, func(tc *TeamCityReporter) {
		tc.pathMappingFrom = "/container/path"
		tc.pathMappingTo = "/host/path"
	})

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", "/other/path/test.chiperka"))

	output := buf.String()
	assertContains(t, output, "locationHint='chiperka:///other/path/test.chiperka::test1'")
}

// --- Duration format ---

func TestDuration_InMilliseconds(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	bus.Emit(testEvent(events.TestCompleted, "suite", "test1", "").
		WithDuration(2*time.Second + 345*time.Millisecond))

	assertContains(t, buf.String(), "duration='2345'")
}

func TestDuration_ZeroForSkipped(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestSkipped, "suite", "test1", ""))

	assertContains(t, buf.String(), "duration='0'")
}

// --- flowId ---

func TestFlowId_PresentOnAllMessages(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"suite": 1}))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", "/f.chiperka"))
	bus.Emit(testEvent(events.TestCompleted, "suite", "test1", "/f.chiperka").
		WithDuration(100 * time.Millisecond))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for i, line := range lines {
		if !strings.Contains(line, "flowId='12345'") {
			t.Errorf("line %d missing flowId: %s", i, line)
		}
	}
}

// --- Log events: phase headers ---

func TestLogEvent_PhaseHeaders(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	buf.Reset()

	// Service resolve -> Services phase
	e1 := events.NewTestEvent(events.LogInfo, "suite", "test1")
	e1.Data.Message = "resolving image"
	e1.Data.Details["action"] = "service_resolve"
	bus.Emit(e1)

	assertContains(t, buf.String(), "--- Services ---")

	// HTTP request -> Execution phase
	e2 := events.NewTestEvent(events.LogInfo, "suite", "test1")
	e2.Data.Message = "POST /api/login"
	e2.Data.Details["action"] = "http_request"
	bus.Emit(e2)

	assertContains(t, buf.String(), "--- Execution ---")

	// Assertion pass -> Assertions phase
	e3 := events.NewTestEvent(events.LogPass, "suite", "test1")
	e3.Data.Message = "statusCode == 200"
	e3.Data.Details["action"] = "assertion_pass"
	bus.Emit(e3)

	assertContains(t, buf.String(), "--- Assertions ---")
}

func TestLogEvent_PhaseHeader_NotRepeated(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	buf.Reset()

	// Two events in same phase
	for _, msg := range []string{"first", "second"} {
		e := events.NewTestEvent(events.LogInfo, "suite", "test1")
		e.Data.Message = msg
		e.Data.Details["action"] = "http_request"
		bus.Emit(e)
	}

	count := strings.Count(buf.String(), "--- Execution ---")
	if count != 1 {
		t.Errorf("expected 1 Execution header, got %d", count)
	}
}

// --- Log events: skipped actions ---

func TestLogEvent_SkippedActions(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	buf.Reset()

	skipped := []string{"test_start", "container_start", "container_stop", "network_acquire", "network_release", "image_pull"}
	for _, action := range skipped {
		e := events.NewTestEvent(events.LogInfo, "suite", "test1")
		e.Data.Message = "should be skipped"
		e.Data.Details["action"] = action
		bus.Emit(e)
	}

	// No testStdOut should be emitted for skipped actions
	assertNotContains(t, buf.String(), "testStdOut")
}

// --- Log events: no test context ---

func TestLogEvent_NoTestName_Ignored(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	buf.Reset()

	// Event with no test name
	e := events.NewTestEvent(events.LogInfo, "", "")
	e.Data.Message = "global message"
	bus.Emit(e)

	// Should be empty — log events without test context are ignored
	if buf.Len() > 0 {
		t.Errorf("expected no output for event without test name, got: %s", buf.String())
	}
}

// --- Assertion pass via log event ---

func TestLogEvent_AssertionPass(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	buf.Reset()

	e := events.NewTestEvent(events.LogPass, "suite", "test1")
	e.Data.Message = "statusCode == 200"
	e.Data.Details["action"] = "assertion_pass"
	bus.Emit(e)

	output := buf.String()
	assertContains(t, output, "testStdOut")
	assertContains(t, output, "\u2713 statusCode == 200")
}

// --- Assertion fail via log event collects failure ---

func TestLogEvent_AssertionFail_CollectsForComparisonFailure(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"suite": 1}))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))

	// Assertion fail
	e := events.NewTestEvent(events.LogFail, "suite", "test1")
	e.Data.Message = "statusCode == 200"
	e.Data.Details["action"] = "assertion_fail"
	e.Data.Details["expected"] = "200"
	e.Data.Details["actual"] = "404"
	bus.Emit(e)

	// Test failed (should pick up collected assertion)
	bus.Emit(testEvent(events.TestFailed, "suite", "test1", "").
		WithDuration(100 * time.Millisecond))

	output := buf.String()

	// Assertion output goes to stderr
	assertContains(t, output, "testStdErr")
	assertContains(t, output, "\u2717 statusCode == 200")

	// testFailed has the collected comparison failure
	assertContains(t, output, "message='statusCode == 200'")
	assertContains(t, output, "type='comparisonFailure'")
	assertContains(t, output, "expected='200'")
	assertContains(t, output, "actual='404'")
}

// --- Assertion fail with non-string expected/actual ---

func TestLogEvent_AssertionFail_NonStringValues(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"suite": 1}))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))

	// Assertion fail with integer values (via Details which is map[string]any)
	e := events.NewTestEvent(events.LogFail, "suite", "test1")
	e.Data.Message = "statusCode == 200"
	e.Data.Details["action"] = "assertion_fail"
	e.Data.Details["expected"] = 200  // int, not string
	e.Data.Details["actual"] = 404    // int, not string
	bus.Emit(e)

	bus.Emit(testEvent(events.TestFailed, "suite", "test1", "").
		WithDuration(100 * time.Millisecond))

	output := buf.String()
	// Values should be formatted with %v, not lost due to type assertion
	assertContains(t, output, "expected='200'")
	assertContains(t, output, "actual='404'")
}

// --- TestError emits stderr ---

func TestTestError_EmitsStdErr(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	buf.Reset()

	e := events.NewTestEvent(events.TestError, "suite", "test1")
	e.Data.Error = fmt.Errorf("docker: connection refused")
	bus.Emit(e)

	output := buf.String()
	assertContains(t, output, "testStdErr")
	assertContains(t, output, "Error: docker: connection refused")
	assertContains(t, output, "nodeId='suite/test1'")
}

// --- HTML report link ---

func TestHtmlReportLink(t *testing.T) {
	bus, _, buf := newTeamCityTest(t, func(tc *TeamCityReporter) {
		tc.htmlOutputDir = "/tmp/reports"
	})

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"suite": 1}))

	e := testEvent(events.TestStarted, "suite", "test1", "")
	e.TestUUID = "abc-123"
	bus.Emit(e)

	e2 := testEvent(events.TestCompleted, "suite", "test1", "")
	e2.TestUUID = "abc-123"
	e2.Data.Duration = 100 * time.Millisecond
	bus.Emit(e2)

	output := buf.String()
	assertContains(t, output, "HTML Report: file:///tmp/reports/abc-123/index.html")
}

func TestHtmlReportLink_WithPathMapping(t *testing.T) {
	bus, _, buf := newTeamCityTest(t, func(tc *TeamCityReporter) {
		tc.htmlOutputDir = "/container/reports"
		tc.pathMappingFrom = "/container"
		tc.pathMappingTo = "/host"
	})

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"suite": 1}))

	e := testEvent(events.TestStarted, "suite", "test1", "")
	e.TestUUID = "abc-123"
	bus.Emit(e)

	e2 := testEvent(events.TestCompleted, "suite", "test1", "")
	e2.TestUUID = "abc-123"
	e2.Data.Duration = 100 * time.Millisecond
	bus.Emit(e2)

	assertContains(t, buf.String(), "HTML Report: file:///host/reports/abc-123/index.html")
}

func TestHtmlReportLink_NotEmittedWithoutDir(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"suite": 1}))

	e := testEvent(events.TestStarted, "suite", "test1", "")
	e.TestUUID = "abc-123"
	bus.Emit(e)

	e2 := testEvent(events.TestCompleted, "suite", "test1", "")
	e2.TestUUID = "abc-123"
	e2.Data.Duration = 100 * time.Millisecond
	bus.Emit(e2)

	assertNotContains(t, buf.String(), "HTML Report")
}

// --- Typed events: Services phase ---

func TestTypedEvent_ServiceStarted(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	buf.Reset()

	e := events.NewTestEvent(events.TestServiceStarted, "suite", "test1")
	e.Data.Details["service"] = "postgres"
	e.Data.Details["image"] = "postgres:15"
	bus.Emit(e)

	output := buf.String()
	assertContains(t, output, "--- Services ---")
	assertContains(t, output, "Starting service: postgres (postgres:15)")
}

func TestTypedEvent_ServiceReady(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	buf.Reset()

	e := events.NewTestEvent(events.TestServiceReady, "suite", "test1")
	e.Data.Details["service"] = "postgres"
	e.Data.Duration = 2500 * time.Millisecond
	bus.Emit(e)

	assertContains(t, buf.String(), "Service ready: postgres (2.5s)")
}

// --- Typed events: Healthcheck ---

func TestTypedEvent_HealthCheck_Pass(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	buf.Reset()

	e := events.NewTestEvent(events.TestHealthCheck, "suite", "test1")
	e.Data.Details["service"] = "api"
	e.Data.Details["attempt"] = 3
	e.Data.Status = "pass"
	bus.Emit(e)

	output := buf.String()
	assertContains(t, output, "testStdOut")
	assertContains(t, output, "Healthcheck api: pass (attempt 3)")
}

func TestTypedEvent_HealthCheck_Fail(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	buf.Reset()

	e := events.NewTestEvent(events.TestHealthCheck, "suite", "test1")
	e.Data.Details["service"] = "api"
	e.Data.Details["attempt"] = 1
	e.Data.Status = "fail"
	bus.Emit(e)

	output := buf.String()
	// Failed healthcheck goes to stderr
	assertContains(t, output, "testStdErr")
	assertContains(t, output, "Healthcheck api: fail (attempt 1)")
}

// --- Typed events: Assertions ---

func TestTypedEvent_AssertResult_Pass(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	buf.Reset()

	e := events.NewTestEvent(events.TestAssertResult, "suite", "test1")
	e.Data.Status = "pass"
	e.Data.Details["assertion"] = "response.statusCode"
	e.Data.Details["expected"] = 200
	e.Data.Details["actual"] = 200
	bus.Emit(e)

	output := buf.String()
	assertContains(t, output, "testStdOut")
	assertContains(t, output, "\u2713 response.statusCode")
}

func TestTypedEvent_AssertResult_Fail(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"suite": 1}))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))

	e := events.NewTestEvent(events.TestAssertResult, "suite", "test1")
	e.Data.Status = "fail"
	e.Data.Details["assertion"] = "response.statusCode"
	e.Data.Details["expected"] = 200
	e.Data.Details["actual"] = 500
	bus.Emit(e)

	// Now fail the test
	bus.Emit(testEvent(events.TestFailed, "suite", "test1", "").
		WithDuration(100 * time.Millisecond))

	output := buf.String()
	assertContains(t, output, "\u2717 response.statusCode (expected: 200, got: 500)")
	assertContains(t, output, "message='response.statusCode'")
	assertContains(t, output, "expected='200'")
	assertContains(t, output, "actual='500'")
}

// --- Artifacts ---

func TestArtifactSaved(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	buf.Reset()

	e := events.NewTestEvent(events.ArtifactSaved, "suite", "test1")
	e.Data.Details["name"] = "error.log"
	bus.Emit(e)

	assertContains(t, buf.String(), "Artifact: error.log")
}

func TestArtifactSaved_NoTestName_Ignored(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	buf.Reset()

	e := events.NewEvent(events.ArtifactSaved)
	e.Data.Details["name"] = "error.log"
	bus.Emit(e)

	if buf.Len() > 0 {
		t.Errorf("expected no output for artifact without test name, got: %s", buf.String())
	}
}

// --- Escaping in attribute values ---

func TestEscaping_InTestName(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test with 'quotes'", ""))

	assertContains(t, buf.String(), "name='test with |'quotes|''")
}

func TestEscaping_InMessage(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"suite": 1}))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	bus.Emit(testEvent(events.TestFailed, "suite", "test1", "").
		WithDuration(100 * time.Millisecond).
		WithMessage("expected [200] but got\n404"))

	output := buf.String()
	assertContains(t, output, "message='expected |[200|] but got|n404'")
}

// --- Empty attribute values are skipped ---

func TestEmptyAttributes_Skipped(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	// No file path — locationHint should be empty and skipped
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))

	// The output should NOT contain "locationHint=''" — it should be omitted entirely
	assertNotContains(t, buf.String(), "locationHint=''")
}

// --- Cleanup phase ---

func TestTestCleanup(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "suite", "test1", ""))
	buf.Reset()

	bus.Emit(testEvent(events.TestCleanup, "suite", "test1", "").
		WithDuration(50 * time.Millisecond))

	output := buf.String()
	assertContains(t, output, "--- Cleanup ---")
	assertContains(t, output, "Completed (50ms)")
}

// --- Message format: single line, proper structure ---

func TestMessageFormat(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "##teamcity[") {
			t.Errorf("line %d: missing ##teamcity[ prefix: %s", i, line)
		}
		if !strings.HasSuffix(line, "]") {
			t.Errorf("line %d: missing ] suffix: %s", i, line)
		}
	}
}

// --- Full integration test: complete run with mixed results ---

func TestFullRun_MixedResults(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	// Run started with 3 tests in 1 suite
	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 3).
		WithDetail("suiteCounts", map[string]int{"api": 3}))

	// Test 1: passes
	bus.Emit(testEvent(events.TestStarted, "api", "get-health", "/api.chiperka"))
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
	e := testEvent(events.TestSkipped, "api", "delete-user", "/api.chiperka")
	e.Data.Message = "requires admin"
	bus.Emit(e)

	// Run completed
	bus.Emit(events.NewEvent(events.RunCompleted))

	output := buf.String()

	// Verify all expected messages
	assertContains(t, output, "testCount count='3'")
	assertContains(t, output, "testSuiteStarted name='api'")
	assertContains(t, output, "testStarted name='get-health'")
	assertContains(t, output, "testFinished name='get-health'")
	assertContains(t, output, "testStarted name='post-login'")
	assertContains(t, output, "testFailed name='post-login'")
	assertContains(t, output, "testFinished name='post-login'")
	assertContains(t, output, "testStarted name='delete-user'")
	assertContains(t, output, "testIgnored name='delete-user'")
	assertContains(t, output, "testFinished name='delete-user'")
	assertContains(t, output, "testSuiteFinished name='api'")

	// Verify order: testFailed before testFinished for failed test
	failedIdx := strings.Index(output, "testFailed name='post-login'")
	finishedIdx := strings.Index(output, "testFinished name='post-login'")
	if failedIdx > finishedIdx {
		t.Error("testFailed must appear before testFinished")
	}

	// Verify order: testIgnored before testFinished for skipped test
	ignoredIdx := strings.Index(output, "testIgnored name='delete-user'")
	skipFinishedIdx := strings.LastIndex(output, "testFinished name='delete-user'")
	if ignoredIdx > skipFinishedIdx {
		t.Error("testIgnored must appear before testFinished")
	}

	// Verify suite only started once
	suiteStartCount := strings.Count(output, "testSuiteStarted name='api'")
	if suiteStartCount != 1 {
		t.Errorf("expected 1 testSuiteStarted, got %d", suiteStartCount)
	}

	// Verify suite finished (3/3 tests done)
	suiteFinishCount := strings.Count(output, "testSuiteFinished name='api'")
	if suiteFinishCount != 1 {
		t.Errorf("expected 1 testSuiteFinished, got %d", suiteFinishCount)
	}
}

// --- nodeId format ---

func TestNodeId_Format(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).WithDetail("tests", 1))
	bus.Emit(testEvent(events.TestStarted, "auth-suite", "login-test", ""))

	output := buf.String()

	// Suite nodeId = suite name
	assertContains(t, output, "nodeId='auth-suite'")

	// Test nodeId = suite/test (matches TestKey())
	assertContains(t, output, "nodeId='auth-suite/login-test'")

	// Test parentNodeId = suite name
	assertContains(t, output, "parentNodeId='auth-suite'")

	// Suite parentNodeId = 0 (root)
	assertContains(t, output, "parentNodeId='0'")
}

// --- writeServiceMessage: attribute ordering ---

func TestWriteServiceMessage_DeterministicOrder(t *testing.T) {
	bus, _, buf := newTeamCityTest(t)

	bus.Emit(events.NewEvent(events.RunStarted).
		WithDetail("tests", 1).
		WithDetail("suiteCounts", map[string]int{"suite": 1}))
	buf.Reset()

	bus.Emit(testEvent(events.TestStarted, "suite", "test1", "/test.chiperka"))

	line := strings.TrimRight(buf.String(), "\n")
	lines := strings.Split(line, "\n")

	// testSuiteStarted
	expected := "##teamcity[testSuiteStarted name='suite' nodeId='suite' parentNodeId='0' locationHint='chiperka:///test.chiperka' flowId='12345']"
	if lines[0] != expected {
		t.Errorf("testSuiteStarted mismatch\nexpected: %s\n  actual: %s", expected, lines[0])
	}

	// testStarted - attributes in exact order: name, nodeId, parentNodeId, locationHint, flowId
	expected = "##teamcity[testStarted name='test1' nodeId='suite/test1' parentNodeId='suite' locationHint='chiperka:///test.chiperka::test1' flowId='12345']"
	if lines[1] != expected {
		t.Errorf("testStarted mismatch\nexpected: %s\n  actual: %s", expected, lines[1])
	}
}

// --- mapPath ---

func TestMapPath(t *testing.T) {
	tc := &TeamCityReporter{
		pathMappingFrom: "/srv/chiperka",
		pathMappingTo:   "/Users/dev/chiperka",
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"/srv/chiperka/tests/auth.chiperka", "/Users/dev/chiperka/tests/auth.chiperka"},
		{"/other/path/test.chiperka", "/other/path/test.chiperka"},
		{"/srv/chiperkale/test.chiperka", "/Users/dev/chiperkale/test.chiperka"}, // prefix match (HasPrefix)
		{"", ""},
	}
	for _, tt := range tests {
		got := tc.mapPath(tt.input)
		if got != tt.expected {
			t.Errorf("mapPath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestMapPath_NoMapping(t *testing.T) {
	tc := &TeamCityReporter{}
	if got := tc.mapPath("/any/path"); got != "/any/path" {
		t.Errorf("expected no mapping, got %q", got)
	}
}
