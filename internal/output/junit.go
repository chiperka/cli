// Package output provides test result formatters.
package output

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"time"

	"spark-cli/internal/model"
)

// JUnitTestSuites represents the root element of JUnit XML.
type JUnitTestSuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Name     string           `xml:"name,attr"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Errors   int              `xml:"errors,attr"`
	Skipped  int              `xml:"skipped,attr"`
	Time     float64          `xml:"time,attr"`
	Suites   []JUnitTestSuite `xml:"testsuite"`
}

// JUnitTestSuite represents a single test suite.
type JUnitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Skipped   int             `xml:"skipped,attr"`
	Time      float64         `xml:"time,attr"`
	Timestamp string          `xml:"timestamp,attr,omitempty"`
	File      string          `xml:"file,attr,omitempty"`
	TestCases []JUnitTestCase `xml:"testcase"`
}

// JUnitTestCase represents a single test case.
type JUnitTestCase struct {
	XMLName    xml.Name      `xml:"testcase"`
	Name       string        `xml:"name,attr"`
	ClassName  string        `xml:"classname,attr"`
	Time       float64       `xml:"time,attr"`
	File       string        `xml:"file,attr,omitempty"`
	Assertions int           `xml:"assertions,attr,omitempty"`
	Failure    *JUnitFailure `xml:"failure,omitempty"`
	Error      *JUnitError   `xml:"error,omitempty"`
	Skipped    *JUnitSkipped `xml:"skipped,omitempty"`
	SystemOut  string        `xml:"system-out,omitempty"`
	SystemErr  string        `xml:"system-err,omitempty"`
}

// JUnitFailure represents a test failure.
type JUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// JUnitError represents a test error.
type JUnitError struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// JUnitSkipped represents a skipped test.
type JUnitSkipped struct {
	Message string `xml:"message,attr,omitempty"`
}

// JUnitWriter writes test results in JUnit XML format.
type JUnitWriter struct {
	startTime time.Time
}

// NewJUnitWriter creates a new JUnit XML writer.
func NewJUnitWriter() *JUnitWriter {
	return &JUnitWriter{
		startTime: time.Now(),
	}
}

// Write generates JUnit XML from test results and writes to file.
func (w *JUnitWriter) Write(result *model.RunResult, filePath string) error {
	totalTime := time.Since(w.startTime).Seconds()

	suites := w.buildSuites(result, totalTime)

	output, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JUnit XML: %w", err)
	}

	xmlContent := []byte(xml.Header + string(output))

	if err := os.WriteFile(filePath, xmlContent, 0644); err != nil {
		return fmt.Errorf("failed to write JUnit XML file: %w", err)
	}

	return nil
}

// WriteBytes generates JUnit XML from test results and returns as bytes.
func (w *JUnitWriter) WriteBytes(result *model.RunResult) ([]byte, error) {
	var totalTime float64
	for _, sr := range result.SuiteResults {
		for _, tr := range sr.TestResults {
			totalTime += tr.Duration.Seconds()
		}
	}

	suites := w.buildSuites(result, totalTime)

	output, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JUnit XML: %w", err)
	}

	return []byte(xml.Header + string(output)), nil
}

// buildSuites creates the JUnit XML structure from run results.
func (w *JUnitWriter) buildSuites(result *model.RunResult, totalTime float64) JUnitTestSuites {
	suites := JUnitTestSuites{
		Name:     "Spark",
		Tests:    result.TotalTests(),
		Failures: result.TotalFailed(),
		Errors:   result.TotalErrors(),
		Skipped:  result.TotalSkipped(),
		Time:     totalTime,
	}

	for _, suiteResult := range result.SuiteResults {
		junitSuite := w.convertSuite(suiteResult)
		suites.Suites = append(suites.Suites, junitSuite)
	}

	return suites
}

// convertSuite converts a model.SuiteResult to JUnitTestSuite.
func (w *JUnitWriter) convertSuite(suiteResult model.SuiteResult) JUnitTestSuite {
	suite := JUnitTestSuite{
		Name:      suiteResult.Suite.Name,
		Tests:     len(suiteResult.TestResults),
		Timestamp: w.startTime.Format(time.RFC3339),
		File:      suiteResult.Suite.FilePath,
	}

	for _, testResult := range suiteResult.TestResults {
		testCase := w.convertTestCase(testResult, suiteResult.Suite)
		suite.TestCases = append(suite.TestCases, testCase)

		switch testResult.Status {
		case model.StatusFailed:
			suite.Failures++
		case model.StatusError:
			suite.Errors++
		case model.StatusSkipped:
			suite.Skipped++
		}
	}

	// Calculate suite time from test durations
	for _, tc := range suite.TestCases {
		suite.Time += tc.Time
	}

	return suite
}

// convertTestCase converts a model.TestResult to JUnitTestCase.
func (w *JUnitWriter) convertTestCase(testResult model.TestResult, suite model.Suite) JUnitTestCase {
	testCase := JUnitTestCase{
		Name:       testResult.Test.Name,
		ClassName:  suite.Name,
		Time:       testResult.Duration.Seconds(),
		File:       suite.FilePath,
		Assertions: len(testResult.AssertionResults),
	}

	switch testResult.Status {
	case model.StatusFailed:
		var failureMessages []string
		for _, ar := range testResult.AssertionResults {
			if !ar.Passed {
				failureMessages = append(failureMessages, ar.Message)
			}
		}

		message := "Assertion failed"
		if len(failureMessages) > 0 {
			message = failureMessages[0]
		}

		testCase.Failure = &JUnitFailure{
			Message: message,
			Type:    "AssertionError",
			Content: formatFailureContent(testResult.AssertionResults),
		}

	case model.StatusError:
		errorMessage := "Unknown error"
		if testResult.Error != nil {
			errorMessage = testResult.Error.Error()
		}

		testCase.Error = &JUnitError{
			Message: errorMessage,
			Type:    "Error",
			Content: errorMessage,
		}

	case model.StatusSkipped:
		testCase.Skipped = &JUnitSkipped{}
	}

	// system-out: log entries
	if len(testResult.LogEntries) > 0 {
		testCase.SystemOut = formatLogEntries(testResult.LogEntries)
	}

	// system-err: error details
	if testResult.Error != nil {
		testCase.SystemErr = testResult.Error.Error()
	}

	return testCase
}

// formatFailureContent formats assertion results into detailed failure content.
func formatFailureContent(assertions []model.AssertionResult) string {
	var lines []string
	for _, ar := range assertions {
		if ar.Passed {
			continue
		}
		line := ar.Message
		if ar.Expected != "" || ar.Actual != "" {
			line += fmt.Sprintf("\n  Expected: %s\n  Actual:   %s", ar.Expected, ar.Actual)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n\n")
}

// formatLogEntries formats log entries for system-out.
func formatLogEntries(entries []model.LogEntry) string {
	var lines []string
	for _, entry := range entries {
		line := fmt.Sprintf("[%s] %s", entry.RelativeTime, entry.Message)
		if entry.Service != "" {
			line = fmt.Sprintf("[%s] [%s] %s", entry.RelativeTime, entry.Service, entry.Message)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
