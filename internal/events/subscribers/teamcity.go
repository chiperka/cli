package subscribers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"chiperka-cli/internal/events"
)

// TeamCityReporter outputs TeamCity service messages for IntelliJ SMTestRunner.
//
// Uses id-based test tree (nodeId/parentNodeId) to support parallel test execution.
// With id-based tree, suites don't need to be nested on a stack — multiple suites
// can be open concurrently and tests are placed under their parent suite via
// parentNodeId, regardless of message ordering.
//
// Protocol messages used:
//   - testCount, testSuiteStarted/Finished, testStarted/Finished, testFailed, testIgnored
//   - Single flowId for all messages (process PID)
//   - testStdOut/testStdErr for per-test output routing
//   - comparisonFailure on testFailed for IDE diff viewer
//   - locationHint for source file navigation
//   - nodeId/parentNodeId for explicit tree structure
type TeamCityReporter struct {
	output io.Writer
	mu     sync.Mutex

	// Single flowId for all messages (matches PHPUnit behavior)
	flowId string

	// Suite lifecycle tracking for id-based tree.
	// Multiple suites can be open concurrently — tree structure is explicit
	// via nodeId/parentNodeId. A suite is closed (testSuiteFinished) when
	// all its tests have completed, not before.
	openSuites      map[string]bool // suites that have been started
	suiteFilePaths  map[string]string
	suiteTestsTotal map[string]int // expected number of tests per suite (from RunStarted)
	suiteTestsDone  map[string]int // number of finished tests per suite

	// Per-test state for assertion collection and phase tracking
	testStates map[string]*testState

	// Path mapping for container -> host path translation
	pathMappingFrom string
	pathMappingTo   string

	// HTML output directory for per-test report links
	htmlOutputDir string
}

// testState tracks per-test state during execution.
type testState struct {
	currentPhase string
	failures     []assertFailure
}

// assertFailure holds a single assertion failure for consolidated reporting.
type assertFailure struct {
	assertion string
	expected  string
	actual    string
}

// NewTeamCityReporter creates a new TeamCity service message reporter.
func NewTeamCityReporter(output io.Writer, artifactsDir string, pathMapping string, htmlOutputDir string) *TeamCityReporter {
	tc := &TeamCityReporter{
		output:          output,
		flowId:          fmt.Sprintf("%d", os.Getpid()),
		openSuites:      make(map[string]bool),
		suiteFilePaths:  make(map[string]string),
		suiteTestsTotal: make(map[string]int),
		suiteTestsDone:  make(map[string]int),
		testStates:      make(map[string]*testState),
		htmlOutputDir:   htmlOutputDir,
	}
	if from, to, ok := strings.Cut(pathMapping, "="); ok {
		tc.pathMappingFrom = from
		tc.pathMappingTo = to
	}
	return tc
}

// mapPath applies the configured path mapping (container -> host) to a path.
func (tc *TeamCityReporter) mapPath(path string) string {
	if tc.pathMappingFrom == "" || tc.pathMappingTo == "" {
		return path
	}
	if strings.HasPrefix(path, tc.pathMappingFrom) {
		return tc.pathMappingTo + path[len(tc.pathMappingFrom):]
	}
	return path
}

// Register subscribes this reporter to all relevant events.
func (tc *TeamCityReporter) Register(bus *events.Bus) {
	// Run lifecycle
	bus.On(events.RunStarted, tc.onRunStarted)
	bus.On(events.RunCompleted, tc.onRunCompleted)

	// Test lifecycle
	bus.On(events.TestStarted, tc.onTestStarted)
	bus.On(events.TestCompleted, tc.onTestCompleted)
	bus.On(events.TestFailed, tc.onTestFailed)
	bus.On(events.TestSkipped, tc.onTestSkipped)
	bus.On(events.TestError, tc.onTestError)
	bus.On(events.TestCleanup, tc.onTestCleanup)

	// Typed events (not currently emitted by runner, but ready for future)
	bus.On(events.TestAssertResult, tc.onTestAssertResult)
	bus.On(events.TestAssertStarted, tc.onTestAssertStarted)
	bus.On(events.TestServiceStarted, tc.onTestServiceStarted)
	bus.On(events.TestServiceReady, tc.onTestServiceReady)
	bus.On(events.TestHealthCheck, tc.onTestHealthCheck)
	bus.On(events.TestSetupStarted, tc.onTestSetupStarted)
	bus.On(events.TestSetupCompleted, tc.onTestSetupCompleted)
	bus.On(events.TestExecStarted, tc.onTestExecStarted)
	bus.On(events.TestExecCompleted, tc.onTestExecCompleted)

	// Log events -> testStdOut/testStdErr with action-based filtering
	bus.On(events.LogInfo, tc.onLogEvent)
	bus.On(events.LogPass, tc.onLogEvent)
	bus.On(events.LogFail, tc.onLogEvent)
	bus.On(events.LogWarn, tc.onLogEvent)

	// Artifacts
	bus.On(events.ArtifactSaved, tc.onArtifactSaved)
}

// --- Helpers ---

func (tc *TeamCityReporter) testName(e *events.Event) string {
	return e.TestName
}

func (tc *TeamCityReporter) getTestState(e *events.Event) *testState {
	key := e.TestKey()
	if st, ok := tc.testStates[key]; ok {
		return st
	}
	st := &testState{}
	tc.testStates[key] = st
	return st
}

func (tc *TeamCityReporter) cleanTestState(e *events.Event) {
	delete(tc.testStates, e.TestKey())
}

// emitHtmlLink outputs an HTML report link as testStdOut if htmlOutputDir is set.
// The per-test HTML file is guaranteed to exist because the runner's onTestComplete
// callback writes it before emitting the completion event.
func (tc *TeamCityReporter) emitHtmlLink(e *events.Event) {
	if tc.htmlOutputDir == "" || e.TestUUID == "" {
		return
	}
	path := filepath.Join(tc.htmlOutputDir, e.TestUUID, "index.html")
	tc.writeStdOut(e, fmt.Sprintf("\nHTML Report: file://%s", tc.mapPath(path)))
}

// emitPhaseHeader outputs a visual separator when the execution phase changes.
func (tc *TeamCityReporter) emitPhaseHeader(e *events.Event, phase string) {
	st := tc.getTestState(e)
	if st.currentPhase == phase {
		return
	}
	st.currentPhase = phase
	tc.writeStdOut(e, fmt.Sprintf("\n--- %s ---", phase))
}

// locationHint returns a locationHint for IDE source file navigation.
func (tc *TeamCityReporter) locationHint(e *events.Event) string {
	if e.FilePath == "" {
		return ""
	}
	mappedPath := tc.mapPath(e.FilePath)
	if e.TestName != "" {
		return fmt.Sprintf("chiperka://%s::%s", mappedPath, e.TestName)
	}
	return fmt.Sprintf("chiperka://%s", mappedPath)
}

func (tc *TeamCityReporter) suiteLocationHint(suite string) string {
	filePath := tc.suiteFilePaths[suite]
	if filePath == "" {
		return ""
	}
	return fmt.Sprintf("chiperka://%s", tc.mapPath(filePath))
}

// ensureSuiteOpened emits testSuiteStarted for the suite if not already open.
// With id-based tree, multiple suites can be open concurrently — there is no
// stack to manage. Each suite gets nodeId=suiteName, parentNodeId="0" (root).
func (tc *TeamCityReporter) ensureSuiteOpened(e *events.Event) {
	suite := e.SuiteName
	if suite == "" {
		return
	}

	if e.FilePath != "" {
		tc.suiteFilePaths[suite] = e.FilePath
	}

	if tc.openSuites[suite] {
		return
	}

	tc.openSuites[suite] = true
	tc.writeServiceMessage("testSuiteStarted", [][2]string{
		{"name", suite},
		{"nodeId", suite},
		{"parentNodeId", "0"},
		{"locationHint", tc.suiteLocationHint(suite)},
	})
}

// suiteTestFinished increments the finished count for a suite and emits
// testSuiteFinished when all tests in the suite are done.
func (tc *TeamCityReporter) suiteTestFinished(suite string) {
	if suite == "" {
		return
	}
	tc.suiteTestsDone[suite]++

	total, known := tc.suiteTestsTotal[suite]
	if known && tc.suiteTestsDone[suite] >= total {
		tc.closeSuite(suite)
	}
}

// closeSuite emits testSuiteFinished for a single suite.
func (tc *TeamCityReporter) closeSuite(suite string) {
	if !tc.openSuites[suite] {
		return
	}
	tc.writeServiceMessage("testSuiteFinished", [][2]string{
		{"name", suite},
		{"nodeId", suite},
	})
	delete(tc.openSuites, suite)
}

// closeAllSuites closes any remaining open suites. Safety net for suites
// that weren't closed by reference counting (e.g., interrupted runs).
func (tc *TeamCityReporter) closeAllSuites() {
	for suite := range tc.openSuites {
		tc.writeServiceMessage("testSuiteFinished", [][2]string{
			{"name", suite},
			{"nodeId", suite},
		})
	}
	tc.openSuites = make(map[string]bool)
}

// --- Run lifecycle ---

func (tc *TeamCityReporter) onRunStarted(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tests, ok := e.Data.Details["tests"].(int); ok {
		tc.writeServiceMessage("testCount", [][2]string{
			{"count", fmt.Sprintf("%d", tests)},
		})
	}

	// Store per-suite test counts for suite lifecycle tracking
	if counts, ok := e.Data.Details["suiteCounts"].(map[string]int); ok {
		for suite, count := range counts {
			tc.suiteTestsTotal[suite] = count
		}
	}
}

func (tc *TeamCityReporter) onRunCompleted(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Close all open suites now that every test has finished
	tc.closeAllSuites()
}

// --- Test lifecycle ---

func (tc *TeamCityReporter) onTestStarted(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.ensureSuiteOpened(e)
	tc.getTestState(e)

	parentNodeId := "0"
	if e.SuiteName != "" {
		parentNodeId = e.SuiteName
	}

	tc.writeServiceMessage("testStarted", [][2]string{
		{"name", tc.testName(e)},
		{"nodeId", e.TestKey()},
		{"parentNodeId", parentNodeId},
		{"locationHint", tc.locationHint(e)},
	})
}

func (tc *TeamCityReporter) onTestCompleted(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.emitHtmlLink(e)

	tc.writeServiceMessage("testFinished", [][2]string{
		{"name", tc.testName(e)},
		{"nodeId", e.TestKey()},
		{"duration", fmt.Sprintf("%d", e.Data.Duration.Milliseconds())},
	})
	tc.cleanTestState(e)
	tc.suiteTestFinished(e.SuiteName)
}

func (tc *TeamCityReporter) onTestFailed(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.emitHtmlLink(e)

	st := tc.getTestState(e)

	// Build testFailed attributes
	attrs := [][2]string{
		{"name", tc.testName(e)},
		{"nodeId", e.TestKey()},
	}

	if len(st.failures) > 0 {
		if len(st.failures) == 1 {
			f := st.failures[0]
			attrs = append(attrs,
				[2]string{"message", f.assertion},
				[2]string{"type", "comparisonFailure"},
				[2]string{"expected", f.expected},
				[2]string{"actual", f.actual},
			)
		} else {
			var expectedLines, actualLines, names []string
			for _, f := range st.failures {
				names = append(names, f.assertion)
				expectedLines = append(expectedLines, fmt.Sprintf("%s: %s", f.assertion, f.expected))
				actualLines = append(actualLines, fmt.Sprintf("%s: %s", f.assertion, f.actual))
			}
			attrs = append(attrs,
				[2]string{"message", fmt.Sprintf("%d assertion(s) failed: %s", len(st.failures), strings.Join(names, ", "))},
				[2]string{"type", "comparisonFailure"},
				[2]string{"expected", strings.Join(expectedLines, "\n")},
				[2]string{"actual", strings.Join(actualLines, "\n")},
			)
		}
	} else if e.Data.Message != "" {
		attrs = append(attrs, [2]string{"message", e.Data.Message})
	} else {
		attrs = append(attrs, [2]string{"message", "Test failed"})
	}

	tc.writeServiceMessage("testFailed", attrs)

	tc.writeServiceMessage("testFinished", [][2]string{
		{"name", tc.testName(e)},
		{"nodeId", e.TestKey()},
		{"duration", fmt.Sprintf("%d", e.Data.Duration.Milliseconds())},
	})
	tc.cleanTestState(e)
	tc.suiteTestFinished(e.SuiteName)
}

func (tc *TeamCityReporter) onTestSkipped(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.ensureSuiteOpened(e)

	parentNodeId := "0"
	if e.SuiteName != "" {
		parentNodeId = e.SuiteName
	}

	tc.writeServiceMessage("testStarted", [][2]string{
		{"name", tc.testName(e)},
		{"nodeId", e.TestKey()},
		{"parentNodeId", parentNodeId},
		{"locationHint", tc.locationHint(e)},
	})

	msg := "skipped"
	if e.Data.Message != "" {
		msg = e.Data.Message
	}
	tc.writeServiceMessage("testIgnored", [][2]string{
		{"name", tc.testName(e)},
		{"nodeId", e.TestKey()},
		{"message", msg},
	})

	tc.writeServiceMessage("testFinished", [][2]string{
		{"name", tc.testName(e)},
		{"nodeId", e.TestKey()},
		{"duration", "0"},
	})
	tc.cleanTestState(e)
	tc.suiteTestFinished(e.SuiteName)
}

// --- Phase: Services (typed events) ---

func (tc *TeamCityReporter) onTestServiceStarted(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.emitPhaseHeader(e, "Services")
	service, _ := e.Data.Details["service"].(string)
	image, _ := e.Data.Details["image"].(string)
	tc.writeStdOut(e, fmt.Sprintf("  Starting service: %s (%s)", service, image))
}

func (tc *TeamCityReporter) onTestServiceReady(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	service, _ := e.Data.Details["service"].(string)
	tc.writeStdOut(e, fmt.Sprintf("  Service ready: %s (%s)", service, e.Data.Duration))
}

func (tc *TeamCityReporter) onTestHealthCheck(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	service, _ := e.Data.Details["service"].(string)
	attempt, _ := e.Data.Details["attempt"].(int)
	msg := fmt.Sprintf("  Healthcheck %s: %s (attempt %d)", service, e.Data.Status, attempt)

	if e.Data.Status == "fail" {
		tc.writeStdErr(e, msg)
	} else {
		tc.writeStdOut(e, msg)
	}
}

// --- Phase: Setup (typed events) ---

func (tc *TeamCityReporter) onTestSetupStarted(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.emitPhaseHeader(e, "Setup")
	tc.writeStdOut(e, fmt.Sprintf("  Step %d/%d", e.Data.Current, e.Data.Total))
}

func (tc *TeamCityReporter) onTestSetupCompleted(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.writeStdOut(e, fmt.Sprintf("  Completed (%s)", e.Data.Duration))
}

// --- Phase: Execution (typed events) ---

func (tc *TeamCityReporter) onTestExecStarted(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.emitPhaseHeader(e, "Execution")
	execType, _ := e.Data.Details["type"].(string)
	target, _ := e.Data.Details["target"].(string)
	tc.writeStdOut(e, fmt.Sprintf("  %s %s", execType, target))
}

func (tc *TeamCityReporter) onTestExecCompleted(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.writeStdOut(e, fmt.Sprintf("  Completed (%s)", e.Data.Duration))
}

// --- Phase: Assertions (typed events) ---

func (tc *TeamCityReporter) onTestAssertStarted(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.emitPhaseHeader(e, "Assertions")
}

func (tc *TeamCityReporter) onTestAssertResult(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	assertion, _ := e.Data.Details["assertion"].(string)
	expected := e.Data.Details["expected"]
	actual := e.Data.Details["actual"]

	if e.Data.Status == "pass" {
		tc.writeStdOut(e, fmt.Sprintf("  \u2713 %s", assertion))
		return
	}

	tc.writeStdOut(e, fmt.Sprintf("  \u2717 %s (expected: %v, got: %v)", assertion, expected, actual))

	st := tc.getTestState(e)
	st.failures = append(st.failures, assertFailure{
		assertion: assertion,
		expected:  fmt.Sprintf("%v", expected),
		actual:    fmt.Sprintf("%v", actual),
	})
}

// --- Error and Cleanup ---

func (tc *TeamCityReporter) onTestError(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	msg := e.Data.Message
	if e.Data.Error != nil {
		msg = e.Data.Error.Error()
	}
	tc.writeStdErr(e, fmt.Sprintf("Error: %s", msg))
}

func (tc *TeamCityReporter) onTestCleanup(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.emitPhaseHeader(e, "Cleanup")
	tc.writeStdOut(e, fmt.Sprintf("  Completed (%s)", e.Data.Duration))
}

// --- Log events ---

// actionPhase maps log event actions to execution phases.
var actionPhase = map[string]string{
	"service_resolve":  "Services",
	"healthcheck_pass": "Services",
	"healthcheck_fail": "Services",
	"setup_start":      "Setup",
	"setup_cli":        "Setup",
	"setup_http":       "Setup",
	"http_request":     "Execution",
	"cli_exec":         "Execution",
	"assertion_pass":   "Assertions",
	"assertion_fail":   "Assertions",
}

// skippedActions are log actions too verbose for IDE output.
var skippedActions = map[string]bool{
	"test_start":         true,
	"container_start":    true,
	"container_stop":     true,
	"network_acquire":    true,
	"network_release":    true,
	"network_remove":     true,
	"logs_collect":       true,
	"healthcheck_start":  true,
	"exec_start":         true,
	"exec_complete":      true,
	"setup_cli_complete": true,
	"setup_http_complete": true,
	"images_prewarm":     true,
	"run_start":          true,
	"service_discover":   true,
	"image_pull":         true,
	"image_found":        true,
}

func (tc *TeamCityReporter) onLogEvent(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	msg := e.Data.Message
	if msg == "" || e.TestName == "" {
		return
	}

	action, _ := e.Data.Details["action"].(string)

	if skippedActions[action] {
		return
	}

	if phase, ok := actionPhase[action]; ok {
		tc.emitPhaseHeader(e, phase)
	}

	// Assertion results: collect failures for testFailed comparisonFailure
	if action == "assertion_fail" {
		expected := fmt.Sprintf("%v", e.Data.Details["expected"])
		actual := fmt.Sprintf("%v", e.Data.Details["actual"])
		tc.writeStdErr(e, fmt.Sprintf("  \u2717 %s", msg))
		st := tc.getTestState(e)
		st.failures = append(st.failures, assertFailure{
			assertion: msg,
			expected:  expected,
			actual:    actual,
		})
		return
	}

	if action == "assertion_pass" {
		tc.writeStdOut(e, fmt.Sprintf("  \u2713 %s", msg))
		return
	}

	if e.Type == events.LogFail {
		tc.writeStdErr(e, msg)
	} else {
		tc.writeStdOut(e, msg)
	}
}

// --- Artifacts ---

func (tc *TeamCityReporter) onArtifactSaved(e *events.Event) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if e.TestName == "" {
		return
	}

	name, _ := e.Data.Details["name"].(string)
	if name == "" {
		return
	}

	tc.writeStdOut(e, fmt.Sprintf("  Artifact: %s", name))
}

// --- Output ---

func (tc *TeamCityReporter) writeStdOut(e *events.Event, text string) {
	tc.writeServiceMessage("testStdOut", [][2]string{
		{"name", tc.testName(e)},
		{"nodeId", e.TestKey()},
		{"out", text + "\n"},
	})
}

func (tc *TeamCityReporter) writeStdErr(e *events.Event, text string) {
	tc.writeServiceMessage("testStdErr", [][2]string{
		{"name", tc.testName(e)},
		{"nodeId", e.TestKey()},
		{"out", text + "\n"},
	})
}

// writeServiceMessage outputs a single TeamCity service message with deterministic
// attribute order and a single flowId (matching PHPUnit's format).
func (tc *TeamCityReporter) writeServiceMessage(messageName string, attrs [][2]string) {
	var sb strings.Builder
	sb.WriteString("##teamcity[")
	sb.WriteString(messageName)
	for _, attr := range attrs {
		if attr[1] == "" {
			continue // skip empty attributes
		}
		sb.WriteString(" ")
		sb.WriteString(attr[0])
		sb.WriteString("='")
		sb.WriteString(tcEscape(attr[1]))
		sb.WriteString("'")
	}
	// Always append flowId last (like PHPUnit)
	sb.WriteString(" flowId='")
	sb.WriteString(tc.flowId)
	sb.WriteString("'")
	sb.WriteString("]\n")
	fmt.Fprint(tc.output, sb.String())
}

// tcEscape escapes a string for TeamCity service message values.
func tcEscape(s string) string {
	s = strings.ReplaceAll(s, "|", "||")
	s = strings.ReplaceAll(s, "'", "|'")
	s = strings.ReplaceAll(s, "\n", "|n")
	s = strings.ReplaceAll(s, "\r", "|r")
	s = strings.ReplaceAll(s, "[", "|[")
	s = strings.ReplaceAll(s, "]", "|]")
	return s
}
