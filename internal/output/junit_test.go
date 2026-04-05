package output

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"chiperka-cli/internal/model"
)

func TestJUnit_WriteBytes_Empty(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{}

	data, err := w.WriteBytes(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(string(data), "<?xml") {
		t.Errorf("expected XML header, got %q", string(data[:20]))
	}

	var suites JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if suites.Name != "Chiperka" {
		t.Errorf("expected name 'Chiperka', got %q", suites.Name)
	}
	if suites.Tests != 0 {
		t.Errorf("expected 0 tests, got %d", suites.Tests)
	}
	if suites.Failures != 0 {
		t.Errorf("expected 0 failures, got %d", suites.Failures)
	}
	if suites.Errors != 0 {
		t.Errorf("expected 0 errors, got %d", suites.Errors)
	}
	if suites.Skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", suites.Skipped)
	}
}

func TestJUnit_WriteBytes_PassedTests(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{
		SuiteResults: []model.SuiteResult{
			{
				Suite: model.Suite{Name: "auth-suite", FilePath: "tests/auth.chiperka"},
				TestResults: []model.TestResult{
					{
						Test:     model.Test{Name: "login-success"},
						Status:   model.StatusPassed,
						Duration: 843 * time.Millisecond,
						AssertionResults: []model.AssertionResult{
							{Passed: true, Message: "status code is 200"},
							{Passed: true, Message: "body contains token"},
						},
					},
					{
						Test:     model.Test{Name: "login-failure"},
						Status:   model.StatusPassed,
						Duration: 412 * time.Millisecond,
					},
				},
			},
		},
	}

	data, err := w.WriteBytes(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suites JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if suites.Tests != 2 {
		t.Errorf("expected 2 tests, got %d", suites.Tests)
	}
	if suites.Failures != 0 {
		t.Errorf("expected 0 failures, got %d", suites.Failures)
	}
	if suites.Skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", suites.Skipped)
	}
	if len(suites.Suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(suites.Suites))
	}

	suite := suites.Suites[0]
	if suite.Name != "auth-suite" {
		t.Errorf("expected suite name 'auth-suite', got %q", suite.Name)
	}
	if suite.File != "tests/auth.chiperka" {
		t.Errorf("expected file path 'tests/auth.chiperka', got %q", suite.File)
	}
	if suite.Timestamp == "" {
		t.Errorf("expected timestamp on suite")
	}
	if suite.Skipped != 0 {
		t.Errorf("expected 0 skipped in suite, got %d", suite.Skipped)
	}
	if len(suite.TestCases) != 2 {
		t.Fatalf("expected 2 test cases, got %d", len(suite.TestCases))
	}

	tc := suite.TestCases[0]
	if tc.Name != "login-success" {
		t.Errorf("expected test name 'login-success', got %q", tc.Name)
	}
	if tc.ClassName != "auth-suite" {
		t.Errorf("expected class name 'auth-suite', got %q", tc.ClassName)
	}
	if tc.File != "tests/auth.chiperka" {
		t.Errorf("expected file 'tests/auth.chiperka', got %q", tc.File)
	}
	if tc.Assertions != 2 {
		t.Errorf("expected 2 assertions, got %d", tc.Assertions)
	}
	if tc.Failure != nil {
		t.Errorf("expected no failure for passed test")
	}
	if tc.Error != nil {
		t.Errorf("expected no error for passed test")
	}
	if tc.Skipped != nil {
		t.Errorf("expected no skipped for passed test")
	}
}

func TestJUnit_WriteBytes_FailedTests(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{
		SuiteResults: []model.SuiteResult{
			{
				Suite: model.Suite{Name: "suite1"},
				TestResults: []model.TestResult{
					{
						Test:   model.Test{Name: "failed-test"},
						Status: model.StatusFailed,
						AssertionResults: []model.AssertionResult{
							{Passed: true, Message: "header present"},
							{Passed: false, Message: "Expected status code 200, got 404", Expected: "200", Actual: "404"},
							{Passed: false, Message: "Body mismatch", Expected: "ok", Actual: "not found"},
						},
						Duration: 100 * time.Millisecond,
					},
				},
			},
		},
	}

	data, err := w.WriteBytes(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suites JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if suites.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", suites.Failures)
	}

	tc := suites.Suites[0].TestCases[0]
	if tc.Failure == nil {
		t.Fatalf("expected failure element")
	}
	if tc.Failure.Message != "Expected status code 200, got 404" {
		t.Errorf("expected first failure message, got %q", tc.Failure.Message)
	}
	if tc.Failure.Type != "AssertionError" {
		t.Errorf("expected AssertionError type, got %q", tc.Failure.Type)
	}
	if tc.Assertions != 3 {
		t.Errorf("expected 3 assertions, got %d", tc.Assertions)
	}
	// Failure content should include Expected/Actual
	if !strings.Contains(tc.Failure.Content, "Expected: 200") {
		t.Errorf("expected failure content to contain 'Expected: 200', got %q", tc.Failure.Content)
	}
	if !strings.Contains(tc.Failure.Content, "Actual:   404") {
		t.Errorf("expected failure content to contain 'Actual:   404', got %q", tc.Failure.Content)
	}
	// Should contain both failure messages
	if !strings.Contains(tc.Failure.Content, "Body mismatch") {
		t.Errorf("expected failure content to contain second failure message")
	}
}

func TestJUnit_WriteBytes_ErrorTests(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{
		SuiteResults: []model.SuiteResult{
			{
				Suite: model.Suite{Name: "suite1"},
				TestResults: []model.TestResult{
					{
						Test:     model.Test{Name: "error-test"},
						Status:   model.StatusError,
						Error:    fmt.Errorf("container timeout"),
						Duration: 5 * time.Second,
					},
				},
			},
		},
	}

	data, err := w.WriteBytes(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suites JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if suites.Errors != 1 {
		t.Errorf("expected 1 error, got %d", suites.Errors)
	}

	tc := suites.Suites[0].TestCases[0]
	if tc.Error == nil {
		t.Fatalf("expected error element")
	}
	if tc.Error.Message != "container timeout" {
		t.Errorf("expected error message, got %q", tc.Error.Message)
	}
	// system-err should contain the error
	if tc.SystemErr != "container timeout" {
		t.Errorf("expected system-err 'container timeout', got %q", tc.SystemErr)
	}
}

func TestJUnit_WriteBytes_SkippedTests(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{
		SuiteResults: []model.SuiteResult{
			{
				Suite: model.Suite{Name: "suite1"},
				TestResults: []model.TestResult{
					{Test: model.Test{Name: "skipped-test"}, Status: model.StatusSkipped},
					{Test: model.Test{Name: "passed-test"}, Status: model.StatusPassed, Duration: 100 * time.Millisecond},
				},
			},
		},
	}

	data, err := w.WriteBytes(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suites JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}

	// Root level
	if suites.Skipped != 1 {
		t.Errorf("expected 1 skipped at root, got %d", suites.Skipped)
	}
	if suites.Tests != 2 {
		t.Errorf("expected 2 tests, got %d", suites.Tests)
	}

	// Suite level
	suite := suites.Suites[0]
	if suite.Skipped != 1 {
		t.Errorf("expected 1 skipped in suite, got %d", suite.Skipped)
	}

	// Test case level
	tc := suite.TestCases[0]
	if tc.Skipped == nil {
		t.Fatalf("expected <skipped> element on skipped test")
	}
	if tc.Failure != nil {
		t.Errorf("expected no failure on skipped test")
	}
	if tc.Error != nil {
		t.Errorf("expected no error on skipped test")
	}

	// Passed test should not be skipped
	tc2 := suite.TestCases[1]
	if tc2.Skipped != nil {
		t.Errorf("expected no <skipped> on passed test")
	}
}

func TestJUnit_WriteBytes_SystemOut(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{
		SuiteResults: []model.SuiteResult{
			{
				Suite: model.Suite{Name: "suite1"},
				TestResults: []model.TestResult{
					{
						Test:     model.Test{Name: "test-with-logs"},
						Status:   model.StatusPassed,
						Duration: 100 * time.Millisecond,
						LogEntries: []model.LogEntry{
							{RelativeTime: "0.001s", Level: "info", Action: "image_pull", Message: "Pulling Docker image"},
							{RelativeTime: "0.500s", Level: "info", Action: "healthcheck_pass", Service: "api", Message: "Service is healthy"},
						},
					},
				},
			},
		},
	}

	data, err := w.WriteBytes(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suites JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}

	tc := suites.Suites[0].TestCases[0]
	if tc.SystemOut == "" {
		t.Fatalf("expected system-out content")
	}
	if !strings.Contains(tc.SystemOut, "[0.001s] Pulling Docker image") {
		t.Errorf("expected log entry without service, got %q", tc.SystemOut)
	}
	if !strings.Contains(tc.SystemOut, "[0.500s] [api] Service is healthy") {
		t.Errorf("expected log entry with service, got %q", tc.SystemOut)
	}
}

func TestJUnit_WriteBytes_NoSystemOutWhenNoLogs(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{
		SuiteResults: []model.SuiteResult{
			{
				Suite: model.Suite{Name: "suite1"},
				TestResults: []model.TestResult{
					{Test: model.Test{Name: "no-logs"}, Status: model.StatusPassed, Duration: 50 * time.Millisecond},
				},
			},
		},
	}

	data, err := w.WriteBytes(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// system-out should not appear in XML at all
	if strings.Contains(string(data), "system-out") {
		t.Errorf("expected no system-out element when no logs")
	}
}

func TestJUnit_WriteBytes_MixedStatuses(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{
		SuiteResults: []model.SuiteResult{
			{
				Suite: model.Suite{Name: "suite1"},
				TestResults: []model.TestResult{
					{Test: model.Test{Name: "passed"}, Status: model.StatusPassed, Duration: 100 * time.Millisecond},
					{Test: model.Test{Name: "failed"}, Status: model.StatusFailed, Duration: 200 * time.Millisecond},
					{Test: model.Test{Name: "error"}, Status: model.StatusError, Error: fmt.Errorf("timeout"), Duration: 300 * time.Millisecond},
					{Test: model.Test{Name: "skipped"}, Status: model.StatusSkipped},
				},
			},
		},
	}

	data, err := w.WriteBytes(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suites JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if suites.Tests != 4 {
		t.Errorf("expected 4 tests, got %d", suites.Tests)
	}
	// TotalFailed counts both StatusFailed + StatusError
	if suites.Failures != 2 {
		t.Errorf("expected 2 failures (failed+error), got %d", suites.Failures)
	}
	if suites.Errors != 1 {
		t.Errorf("expected 1 error, got %d", suites.Errors)
	}
	if suites.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", suites.Skipped)
	}

	// Verify time is sum of all durations
	expectedTime := 0.1 + 0.2 + 0.3
	if suites.Time < expectedTime-0.01 || suites.Time > expectedTime+0.01 {
		t.Errorf("expected time ~%.1f, got %.3f", expectedTime, suites.Time)
	}

	// Suite level counts
	suite := suites.Suites[0]
	if suite.Failures != 1 {
		t.Errorf("expected 1 failure in suite, got %d", suite.Failures)
	}
	if suite.Errors != 1 {
		t.Errorf("expected 1 error in suite, got %d", suite.Errors)
	}
	if suite.Skipped != 1 {
		t.Errorf("expected 1 skipped in suite, got %d", suite.Skipped)
	}
}

func TestJUnit_WriteBytes_MultipleSuites(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{
		SuiteResults: []model.SuiteResult{
			{
				Suite:       model.Suite{Name: "suite1"},
				TestResults: []model.TestResult{{Test: model.Test{Name: "t1"}, Status: model.StatusPassed}},
			},
			{
				Suite:       model.Suite{Name: "suite2"},
				TestResults: []model.TestResult{{Test: model.Test{Name: "t2"}, Status: model.StatusPassed}},
			},
		},
	}

	data, err := w.WriteBytes(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suites JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if len(suites.Suites) != 2 {
		t.Errorf("expected 2 suites, got %d", len(suites.Suites))
	}
}

func TestJUnit_WriteBytes_ValidXML(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{
		SuiteResults: []model.SuiteResult{
			{
				Suite:       model.Suite{Name: "suite1"},
				TestResults: []model.TestResult{{Test: model.Test{Name: "t1"}, Status: model.StatusPassed}},
			},
		},
	}

	data, err := w.WriteBytes(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var check interface{}
	if err := xml.Unmarshal(data, &check); err != nil {
		t.Errorf("produced invalid XML: %v", err)
	}
}

func TestJUnit_Write_CreatesFile(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{
		SuiteResults: []model.SuiteResult{
			{
				Suite:       model.Suite{Name: "suite1"},
				TestResults: []model.TestResult{{Test: model.Test{Name: "t1"}, Status: model.StatusPassed}},
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "report.xml")
	if err := w.Write(result, path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if !strings.HasPrefix(string(data), "<?xml") {
		t.Errorf("expected XML header")
	}
}

func TestJUnit_SuiteAttributes(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{
		SuiteResults: []model.SuiteResult{
			{
				Suite: model.Suite{Name: "suite1", FilePath: "tests/suite1.chiperka"},
				TestResults: []model.TestResult{
					{Test: model.Test{Name: "t1"}, Status: model.StatusPassed, Duration: 100 * time.Millisecond},
					{Test: model.Test{Name: "t2"}, Status: model.StatusFailed, Duration: 200 * time.Millisecond},
					{Test: model.Test{Name: "t3"}, Status: model.StatusError, Error: fmt.Errorf("err"), Duration: 50 * time.Millisecond},
					{Test: model.Test{Name: "t4"}, Status: model.StatusSkipped},
				},
			},
		},
	}

	data, err := w.WriteBytes(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suites JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}

	suite := suites.Suites[0]
	if suite.Tests != 4 {
		t.Errorf("expected 4 tests, got %d", suite.Tests)
	}
	if suite.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", suite.Failures)
	}
	if suite.Errors != 1 {
		t.Errorf("expected 1 error, got %d", suite.Errors)
	}
	if suite.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", suite.Skipped)
	}
	if suite.File != "tests/suite1.chiperka" {
		t.Errorf("expected file 'tests/suite1.chiperka', got %q", suite.File)
	}
	if suite.Timestamp == "" {
		t.Errorf("expected timestamp on suite")
	}

	// Suite time should be sum of test durations
	expectedTime := 0.1 + 0.2 + 0.05
	if suite.Time < expectedTime-0.01 || suite.Time > expectedTime+0.01 {
		t.Errorf("expected suite time ~%.2f, got %.3f", expectedTime, suite.Time)
	}
}

func TestJUnit_TestCaseFile(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{
		SuiteResults: []model.SuiteResult{
			{
				Suite: model.Suite{Name: "auth", FilePath: "tests/auth.chiperka"},
				TestResults: []model.TestResult{
					{Test: model.Test{Name: "login"}, Status: model.StatusPassed, Duration: 50 * time.Millisecond},
				},
			},
		},
	}

	data, err := w.WriteBytes(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suites JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}

	tc := suites.Suites[0].TestCases[0]
	if tc.File != "tests/auth.chiperka" {
		t.Errorf("expected file 'tests/auth.chiperka' on testcase, got %q", tc.File)
	}
}

func TestJUnit_FailureContentExpectedActual(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{
		SuiteResults: []model.SuiteResult{
			{
				Suite: model.Suite{Name: "suite1"},
				TestResults: []model.TestResult{
					{
						Test:   model.Test{Name: "assertion-test"},
						Status: model.StatusFailed,
						AssertionResults: []model.AssertionResult{
							{Passed: false, Message: "status code mismatch", Expected: "200", Actual: "500"},
							{Passed: false, Message: "body check", Expected: "ok", Actual: "error"},
						},
						Duration: 100 * time.Millisecond,
					},
				},
			},
		},
	}

	data, err := w.WriteBytes(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var suites JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}

	tc := suites.Suites[0].TestCases[0]
	if tc.Failure == nil {
		t.Fatalf("expected failure")
	}

	// First failure message used as message attr
	if tc.Failure.Message != "status code mismatch" {
		t.Errorf("expected first failure as message, got %q", tc.Failure.Message)
	}

	// Content should have expected/actual for both assertions
	content := tc.Failure.Content
	if !strings.Contains(content, "Expected: 200") || !strings.Contains(content, "Actual:   500") {
		t.Errorf("first assertion expected/actual missing from content: %q", content)
	}
	if !strings.Contains(content, "Expected: ok") || !strings.Contains(content, "Actual:   error") {
		t.Errorf("second assertion expected/actual missing from content: %q", content)
	}
}

func TestJUnit_FormatLogEntries(t *testing.T) {
	entries := []model.LogEntry{
		{RelativeTime: "0.001s", Message: "Starting"},
		{RelativeTime: "0.500s", Service: "db", Message: "Healthy"},
		{RelativeTime: "1.000s", Service: "api", Message: "Ready"},
	}

	out := formatLogEntries(entries)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "[0.001s] Starting" {
		t.Errorf("expected '[0.001s] Starting', got %q", lines[0])
	}
	if lines[1] != "[0.500s] [db] Healthy" {
		t.Errorf("expected '[0.500s] [db] Healthy', got %q", lines[1])
	}
	if lines[2] != "[1.000s] [api] Ready" {
		t.Errorf("expected '[1.000s] [api] Ready', got %q", lines[2])
	}
}

func TestJUnit_FormatFailureContent_PassedSkipped(t *testing.T) {
	assertions := []model.AssertionResult{
		{Passed: true, Message: "this passes"},
		{Passed: false, Message: "this fails", Expected: "a", Actual: "b"},
	}

	content := formatFailureContent(assertions)
	if strings.Contains(content, "this passes") {
		t.Errorf("passed assertions should not appear in failure content")
	}
	if !strings.Contains(content, "this fails") {
		t.Errorf("failed assertion should appear in content")
	}
}

// --- JUnit snapshot tests ---

// normalizeJUnitXML replaces dynamic content in JUnit XML with fixed placeholders.
func normalizeJUnitXML(xmlStr string) string {
	// Replace RFC3339 timestamps in timestamp="..." attributes
	tsRe := regexp.MustCompile(`timestamp="[^"]*"`)
	xmlStr = tsRe.ReplaceAllString(xmlStr, `timestamp="__TIMESTAMP__"`)
	return xmlStr
}

func TestJUnit_WriteBytes_Snapshots(t *testing.T) {
	tests := []struct {
		name   string
		result *model.RunResult
	}{
		{
			name:   "junit_empty",
			result: &model.RunResult{},
		},
		{
			name: "junit_passed",
			result: &model.RunResult{
				SuiteResults: []model.SuiteResult{
					{
						Suite: model.Suite{Name: "auth-suite", FilePath: "tests/auth.chiperka"},
						TestResults: []model.TestResult{
							{
								Test:     model.Test{Name: "login-success"},
								Status:   model.StatusPassed,
								Duration: 843 * time.Millisecond,
								AssertionResults: []model.AssertionResult{
									{Passed: true, Message: "status code is 200"},
									{Passed: true, Message: "body contains token"},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "junit_failed",
			result: &model.RunResult{
				SuiteResults: []model.SuiteResult{
					{
						Suite: model.Suite{Name: "api-suite", FilePath: "tests/api.chiperka"},
						TestResults: []model.TestResult{
							{
								Test:   model.Test{Name: "validate-response"},
								Status: model.StatusFailed,
								AssertionResults: []model.AssertionResult{
									{Passed: true, Message: "header present"},
									{Passed: false, Message: "Expected status code 200, got 404", Expected: "200", Actual: "404"},
									{Passed: false, Message: "Body mismatch", Expected: "ok", Actual: "not found"},
								},
								Duration: 412 * time.Millisecond,
							},
						},
					},
				},
			},
		},
		{
			name: "junit_error",
			result: &model.RunResult{
				SuiteResults: []model.SuiteResult{
					{
						Suite: model.Suite{Name: "timeout-suite", FilePath: "tests/timeout.chiperka"},
						TestResults: []model.TestResult{
							{
								Test:     model.Test{Name: "timeout-test"},
								Status:   model.StatusError,
								Error:    fmt.Errorf("execution timeout after 5s"),
								Duration: 5 * time.Second,
							},
						},
					},
				},
			},
		},
		{
			name: "junit_skipped",
			result: &model.RunResult{
				SuiteResults: []model.SuiteResult{
					{
						Suite: model.Suite{Name: "misc-suite", FilePath: "tests/misc.chiperka"},
						TestResults: []model.TestResult{
							{Test: model.Test{Name: "disabled-test"}, Status: model.StatusSkipped},
							{Test: model.Test{Name: "active-test"}, Status: model.StatusPassed, Duration: 100 * time.Millisecond},
						},
					},
				},
			},
		},
		{
			name: "junit_with_logs",
			result: &model.RunResult{
				SuiteResults: []model.SuiteResult{
					{
						Suite: model.Suite{Name: "log-suite", FilePath: "tests/logs.chiperka"},
						TestResults: []model.TestResult{
							{
								Test:     model.Test{Name: "test-with-logs"},
								Status:   model.StatusPassed,
								Duration: 500 * time.Millisecond,
								LogEntries: []model.LogEntry{
									{RelativeTime: "0.001s", Level: "info", Action: "image_pull", Message: "Pulling Docker image"},
									{RelativeTime: "0.500s", Level: "info", Action: "healthcheck_pass", Service: "api", Message: "Service is healthy"},
									{RelativeTime: "0.800s", Level: "pass", Action: "assertion.pass", Message: "Status code equals 200"},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "junit_multiple_suites",
			result: &model.RunResult{
				SuiteResults: []model.SuiteResult{
					{
						Suite: model.Suite{Name: "auth-suite", FilePath: "tests/auth.chiperka"},
						TestResults: []model.TestResult{
							{Test: model.Test{Name: "login"}, Status: model.StatusPassed, Duration: 300 * time.Millisecond},
							{Test: model.Test{Name: "register"}, Status: model.StatusPassed, Duration: 500 * time.Millisecond},
						},
					},
					{
						Suite: model.Suite{Name: "api-suite", FilePath: "tests/api.chiperka"},
						TestResults: []model.TestResult{
							{Test: model.Test{Name: "get-users"}, Status: model.StatusPassed, Duration: 200 * time.Millisecond},
							{
								Test:   model.Test{Name: "create-user"},
								Status: model.StatusFailed,
								AssertionResults: []model.AssertionResult{
									{Passed: false, Message: "status 201 expected", Expected: "201", Actual: "500"},
								},
								Duration: 150 * time.Millisecond,
							},
						},
					},
				},
			},
		},
		{
			name: "junit_full",
			result: &model.RunResult{
				SuiteResults: []model.SuiteResult{
					{
						Suite: model.Suite{Name: "full-suite", FilePath: "tests/full.chiperka"},
						TestResults: []model.TestResult{
							{
								Test:     model.Test{Name: "passing-test"},
								Status:   model.StatusPassed,
								Duration: 843 * time.Millisecond,
								AssertionResults: []model.AssertionResult{
									{Passed: true, Message: "Status code equals 200"},
									{Passed: true, Message: "$.token exists"},
								},
								LogEntries: []model.LogEntry{
									{RelativeTime: "0.000s", Level: "info", Action: "network.create", Message: "Creating test network"},
									{RelativeTime: "0.650s", Level: "pass", Action: "service.healthy", Service: "api", Message: "Service is healthy"},
									{RelativeTime: "0.840s", Level: "pass", Action: "assertion.pass", Message: "Status code equals 200"},
								},
							},
							{
								Test:   model.Test{Name: "failing-test"},
								Status: model.StatusFailed,
								AssertionResults: []model.AssertionResult{
									{Passed: true, Message: "Status code equals 200"},
									{Passed: false, Message: "$.name equals John", Expected: "John", Actual: "Jane"},
								},
								Duration: 412 * time.Millisecond,
								LogEntries: []model.LogEntry{
									{RelativeTime: "0.000s", Level: "info", Action: "test.execute", Message: "GET /api/users/1"},
									{RelativeTime: "0.410s", Level: "fail", Action: "assertion.fail", Message: "$.name equals John"},
								},
							},
							{
								Test:     model.Test{Name: "error-test"},
								Status:   model.StatusError,
								Error:    fmt.Errorf("container startup failed: image not found"),
								Duration: 2 * time.Second,
							},
							{
								Test:   model.Test{Name: "skipped-test"},
								Status: model.StatusSkipped,
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewJUnitWriter()
			data, err := w.WriteBytes(tt.result)
			if err != nil {
				t.Fatalf("WriteBytes: %v", err)
			}

			compareSnapshot(t, normalizeJUnitXML(string(data)), tt.name, ".xml")
		})
	}
}

func TestJUnit_Write_Snapshot(t *testing.T) {
	w := NewJUnitWriter()
	result := &model.RunResult{
		SuiteResults: []model.SuiteResult{
			{
				Suite: model.Suite{Name: "auth-suite", FilePath: "tests/auth.chiperka"},
				TestResults: []model.TestResult{
					{Test: model.Test{Name: "login"}, Status: model.StatusPassed, Duration: 500 * time.Millisecond},
					{Test: model.Test{Name: "register"}, Status: model.StatusFailed, Duration: 300 * time.Millisecond,
						AssertionResults: []model.AssertionResult{
							{Passed: false, Message: "status 201 expected", Expected: "201", Actual: "400"},
						},
					},
				},
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "report.xml")
	if err := w.Write(result, path); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}

	// Write() uses time.Since for total time, so normalize that too
	normalized := normalizeJUnitXML(string(data))
	// The root <testsuites> time attr is dynamic from time.Since
	timeRe := regexp.MustCompile(`<testsuites name="Chiperka" tests="\d+" failures="\d+" errors="\d+" skipped="\d+" time="[^"]*"`)
	normalized = timeRe.ReplaceAllStringFunc(normalized, func(s string) string {
		re := regexp.MustCompile(`time="[^"]*"`)
		return re.ReplaceAllString(s, `time="__TOTAL_TIME__"`)
	})

	compareSnapshot(t, normalized, "junit_write_file", ".xml")
}
