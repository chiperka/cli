// Package assertion provides assertion evaluation for test responses.
package assertion

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"spark-cli/internal/executor"
	"spark-cli/internal/model"
)

// Evaluator checks assertions against test responses.
type Evaluator struct{}

// NewEvaluator creates a new assertion evaluator.
func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// EvaluateHTTP checks all assertions against the given HTTP response.
// Returns a slice of assertion results and whether all assertions passed.
func (e *Evaluator) EvaluateHTTP(assertions []model.Assertion, response *executor.HTTPResponse) ([]model.AssertionResult, bool) {
	results := make([]model.AssertionResult, 0, len(assertions))
	allPassed := true

	for _, assertion := range assertions {
		if assertion.StatusCode == nil {
			continue // Skip non-HTTP assertions (handled by other evaluators)
		}
		start := time.Now()
		result := e.evaluateHTTPAssertion(assertion, response)
		result.Duration = time.Since(start)
		results = append(results, result)

		if !result.Passed {
			allPassed = false
		}
	}

	return results, allPassed
}

// EvaluateCLI checks all assertions against the given CLI response.
// Returns a slice of assertion results and whether all assertions passed.
func (e *Evaluator) EvaluateCLI(assertions []model.Assertion, response *executor.CLIResponse) ([]model.AssertionResult, bool) {
	results := make([]model.AssertionResult, 0, len(assertions))
	allPassed := true

	for _, assertion := range assertions {
		if assertion.ExitCode == nil && assertion.Stdout == nil && assertion.Stderr == nil {
			continue // Skip non-CLI assertions (handled by other evaluators)
		}
		start := time.Now()
		result := e.evaluateCLIAssertion(assertion, response)
		result.Duration = time.Since(start)
		results = append(results, result)

		if !result.Passed {
			allPassed = false
		}
	}

	return results, allPassed
}

// evaluateHTTPAssertion checks a single assertion against the HTTP response.
func (e *Evaluator) evaluateHTTPAssertion(assertion model.Assertion, response *executor.HTTPResponse) model.AssertionResult {
	// Check status code assertion
	if assertion.StatusCode != nil {
		return e.evaluateStatusCode(assertion.StatusCode, response.StatusCode)
	}

	// No assertion defined - consider it passed
	return model.AssertionResult{
		Passed:  true,
		Message: "No assertion defined",
	}
}

// evaluateCLIAssertion checks a single assertion against the CLI response.
func (e *Evaluator) evaluateCLIAssertion(assertion model.Assertion, response *executor.CLIResponse) model.AssertionResult {
	// Check exit code assertion
	if assertion.ExitCode != nil {
		return e.evaluateExitCode(assertion.ExitCode, response.ExitCode)
	}

	// Check stdout assertion
	if assertion.Stdout != nil {
		return e.evaluateStdout(assertion.Stdout, response.Stdout)
	}

	// Check stderr assertion
	if assertion.Stderr != nil {
		return e.evaluateStderr(assertion.Stderr, response.Stderr)
	}

	// No assertion defined - consider it passed
	return model.AssertionResult{
		Passed:  true,
		Message: "No assertion defined",
	}
}

// evaluateStatusCode checks if the response status code matches expected.
func (e *Evaluator) evaluateStatusCode(assertion *model.StatusCodeAssertion, actualCode int) model.AssertionResult {
	passed := actualCode == assertion.Equals

	result := model.AssertionResult{
		Passed:   passed,
		Type:     "statusCode",
		Expected: fmt.Sprintf("%d", assertion.Equals),
		Actual:   fmt.Sprintf("%d", actualCode),
	}

	if passed {
		result.Message = fmt.Sprintf("Status code is %d", actualCode)
	} else {
		result.Message = fmt.Sprintf("Expected status code %d, got %d", assertion.Equals, actualCode)
	}

	return result
}

// evaluateExitCode checks if the CLI exit code matches expected.
func (e *Evaluator) evaluateExitCode(assertion *model.ExitCodeAssertion, actualCode int) model.AssertionResult {
	passed := actualCode == assertion.Equals

	result := model.AssertionResult{
		Passed:   passed,
		Type:     "exitCode",
		Expected: fmt.Sprintf("%d", assertion.Equals),
		Actual:   fmt.Sprintf("%d", actualCode),
	}

	if passed {
		result.Message = fmt.Sprintf("Exit code is %d", actualCode)
	} else {
		result.Message = fmt.Sprintf("Expected exit code %d, got %d", assertion.Equals, actualCode)
	}

	return result
}

// evaluateStdout checks if the CLI stdout contains or equals expected content.
func (e *Evaluator) evaluateStdout(assertion *model.StdoutAssertion, stdout []byte) model.AssertionResult {
	actual := string(stdout)

	if assertion.Contains != "" {
		passed := bytes.Contains(stdout, []byte(assertion.Contains))
		result := model.AssertionResult{
			Passed:   passed,
			Type:     "stdout",
			Expected: fmt.Sprintf("contains: %q", assertion.Contains),
			Actual:   truncateString(actual, 200),
		}
		if passed {
			result.Message = fmt.Sprintf("Stdout contains %q", assertion.Contains)
		} else {
			result.Message = fmt.Sprintf("Stdout does not contain %q", assertion.Contains)
		}
		return result
	}

	if assertion.Equals != "" {
		passed := string(stdout) == assertion.Equals
		result := model.AssertionResult{
			Passed:   passed,
			Type:     "stdout",
			Expected: truncateString(assertion.Equals, 200),
			Actual:   truncateString(actual, 200),
		}
		if passed {
			result.Message = "Stdout matches expected value"
		} else {
			result.Message = "Stdout does not match expected value"
		}
		return result
	}

	return model.AssertionResult{
		Passed:  true,
		Type:    "stdout",
		Message: "No stdout assertion criteria specified",
	}
}

// evaluateStderr checks if the CLI stderr contains or equals expected content.
func (e *Evaluator) evaluateStderr(assertion *model.StderrAssertion, stderr []byte) model.AssertionResult {
	actual := string(stderr)

	if assertion.Contains != "" {
		passed := bytes.Contains(stderr, []byte(assertion.Contains))
		result := model.AssertionResult{
			Passed:   passed,
			Type:     "stderr",
			Expected: fmt.Sprintf("contains: %q", assertion.Contains),
			Actual:   truncateString(actual, 200),
		}
		if passed {
			result.Message = fmt.Sprintf("Stderr contains %q", assertion.Contains)
		} else {
			result.Message = fmt.Sprintf("Stderr does not contain %q", assertion.Contains)
		}
		return result
	}

	if assertion.Equals != "" {
		passed := string(stderr) == assertion.Equals
		result := model.AssertionResult{
			Passed:   passed,
			Type:     "stderr",
			Expected: truncateString(assertion.Equals, 200),
			Actual:   truncateString(actual, 200),
		}
		if passed {
			result.Message = "Stderr matches expected value"
		} else {
			result.Message = "Stderr does not match expected value"
		}
		return result
	}

	return model.AssertionResult{
		Passed:  true,
		Type:    "stderr",
		Message: "No stderr assertion criteria specified",
	}
}

// truncateString truncates a string to the specified max length with ellipsis.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// SnapshotContext provides context needed for snapshot assertions.
type SnapshotContext struct {
	// SuiteFilePath is the path to the test suite file (for resolving relative paths)
	SuiteFilePath string
	// Artifacts maps artifact names to their file paths
	Artifacts map[string]string
	// Regenerate indicates whether to update snapshots instead of comparing
	Regenerate bool
}

// EvaluateSnapshots checks all snapshot assertions.
// Returns a slice of assertion results and whether all assertions passed.
func (e *Evaluator) EvaluateSnapshots(assertions []model.Assertion, ctx SnapshotContext) ([]model.AssertionResult, bool) {
	var results []model.AssertionResult
	allPassed := true

	for _, assertion := range assertions {
		if assertion.Snapshot == nil {
			continue
		}

		start := time.Now()
		result := e.evaluateSnapshot(assertion.Snapshot, ctx)
		result.Duration = time.Since(start)
		results = append(results, result)

		if !result.Passed {
			allPassed = false
		}
	}

	return results, allPassed
}

// evaluateSnapshot compares an artifact with an expected snapshot file.
func (e *Evaluator) evaluateSnapshot(assertion *model.SnapshotAssertion, ctx SnapshotContext) model.AssertionResult {
	// Resolve snapshot file path relative to test suite file
	suiteDir := filepath.Dir(ctx.SuiteFilePath)
	snapshotPath := filepath.Join(suiteDir, assertion.File)

	// Find the artifact
	artifactPath, ok := ctx.Artifacts[assertion.Artifact]
	if !ok {
		return model.AssertionResult{
			Passed:   false,
			Type:     "snapshot",
			Expected: assertion.File,
			Actual:   "(artifact not found)",
			Message:  fmt.Sprintf("Artifact '%s' not found. Available: %v", assertion.Artifact, mapKeys(ctx.Artifacts)),
		}
	}

	// Read the actual artifact content
	actualContent, err := os.ReadFile(artifactPath)
	if err != nil {
		return model.AssertionResult{
			Passed:   false,
			Type:     "snapshot",
			Expected: assertion.File,
			Actual:   "(read error)",
			Message:  fmt.Sprintf("Failed to read artifact '%s': %v", assertion.Artifact, err),
		}
	}

	// Regenerate mode: write the snapshot file
	if ctx.Regenerate {
		// Create directories if needed
		if err := os.MkdirAll(filepath.Dir(snapshotPath), 0755); err != nil {
			return model.AssertionResult{
				Passed:   false,
				Type:     "snapshot",
				Expected: assertion.File,
				Actual:   "(write error)",
				Message:  fmt.Sprintf("Failed to create snapshot directory: %v", err),
			}
		}

		if err := os.WriteFile(snapshotPath, actualContent, 0644); err != nil {
			return model.AssertionResult{
				Passed:   false,
				Type:     "snapshot",
				Expected: assertion.File,
				Actual:   "(write error)",
				Message:  fmt.Sprintf("Failed to write snapshot file: %v", err),
			}
		}

		return model.AssertionResult{
			Passed:   true,
			Type:     "snapshot",
			Expected: assertion.File,
			Actual:   assertion.File,
			Message:  fmt.Sprintf("Snapshot '%s' updated from artifact '%s'", assertion.File, assertion.Artifact),
		}
	}

	// Compare mode: read expected and compare
	expectedContent, err := os.ReadFile(snapshotPath)
	if err != nil {
		if os.IsNotExist(err) {
			return model.AssertionResult{
				Passed:   false,
				Type:     "snapshot",
				Expected: assertion.File,
				Actual:   "(file missing)",
				Message:  fmt.Sprintf("Snapshot file '%s' not found. Run with --regenerate-snapshots to create it.", assertion.File),
			}
		}
		return model.AssertionResult{
			Passed:   false,
			Type:     "snapshot",
			Expected: assertion.File,
			Actual:   "(read error)",
			Message:  fmt.Sprintf("Failed to read snapshot file '%s': %v", assertion.File, err),
		}
	}

	// Compare content
	if bytes.Equal(actualContent, expectedContent) {
		return model.AssertionResult{
			Passed:   true,
			Type:     "snapshot",
			Expected: assertion.File,
			Actual:   assertion.File,
			Message:  fmt.Sprintf("Snapshot '%s' matches artifact '%s'", assertion.File, assertion.Artifact),
		}
	}

	// Content differs
	return model.AssertionResult{
		Passed:   false,
		Type:     "snapshot",
		Expected: assertion.File,
		Actual:   fmt.Sprintf("%s (differs)", assertion.Artifact),
		Message:  fmt.Sprintf("Snapshot '%s' does not match artifact '%s'. Run with --regenerate-snapshots to update.", assertion.File, assertion.Artifact),
	}
}

// mapKeys returns the keys of a map as a slice.
func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
