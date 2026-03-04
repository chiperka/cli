package assertion

import (
	"os"
	"path/filepath"
	"testing"

	"spark-cli/internal/executor"
	"spark-cli/internal/model"
)

// --- EvaluateHTTP ---

func TestAssertion_EvaluateHTTP_StatusCodePass(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
	}
	resp := &executor.HTTPResponse{StatusCode: 200}
	results, allPassed := e.EvaluateHTTP(assertions, resp)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("expected passed")
	}
	if results[0].Type != "statusCode" {
		t.Errorf("expected type statusCode, got %q", results[0].Type)
	}
}

func TestAssertion_EvaluateHTTP_StatusCodeFail(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
	}
	resp := &executor.HTTPResponse{StatusCode: 404}
	results, allPassed := e.EvaluateHTTP(assertions, resp)
	if allPassed {
		t.Errorf("expected not all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Errorf("expected failed")
	}
	if results[0].Expected != "200" {
		t.Errorf("expected Expected=200, got %q", results[0].Expected)
	}
	if results[0].Actual != "404" {
		t.Errorf("expected Actual=404, got %q", results[0].Actual)
	}
}

func TestAssertion_EvaluateHTTP_MultipleAssertions(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
		{StatusCode: &model.StatusCodeAssertion{Equals: 201}},
	}
	resp := &executor.HTTPResponse{StatusCode: 200}
	results, allPassed := e.EvaluateHTTP(assertions, resp)
	if allPassed {
		t.Errorf("expected not all passed (second should fail)")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("expected first assertion to pass")
	}
	if results[1].Passed {
		t.Errorf("expected second assertion to fail")
	}
}

func TestAssertion_EvaluateHTTP_NoAssertion(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{{}}
	resp := &executor.HTTPResponse{StatusCode: 200}
	results, allPassed := e.EvaluateHTTP(assertions, resp)
	if !allPassed {
		t.Errorf("expected all passed for empty assertion")
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty assertion, got %d", len(results))
	}
}

func TestAssertion_EvaluateHTTP_SkipsSnapshotAssertions(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
		{Snapshot: &model.SnapshotAssertion{Artifact: "body", File: "snapshot.txt"}},
	}
	resp := &executor.HTTPResponse{StatusCode: 200}
	results, allPassed := e.EvaluateHTTP(assertions, resp)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (snapshot skipped), got %d", len(results))
	}
	if results[0].Type != "statusCode" {
		t.Errorf("expected type statusCode, got %q", results[0].Type)
	}
}

func TestAssertion_EvaluateHTTP_SkipsCLIAssertions(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
		{ExitCode: &model.ExitCodeAssertion{Equals: 0}},
		{Stdout: &model.StdoutAssertion{Contains: "hello"}},
	}
	resp := &executor.HTTPResponse{StatusCode: 200}
	results, allPassed := e.EvaluateHTTP(assertions, resp)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (CLI assertions skipped), got %d", len(results))
	}
}

func TestAssertion_EvaluateCLI_SkipsSnapshotAssertions(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{ExitCode: &model.ExitCodeAssertion{Equals: 0}},
		{Snapshot: &model.SnapshotAssertion{Artifact: "body", File: "snapshot.txt"}},
	}
	resp := &executor.CLIResponse{ExitCode: 0}
	results, allPassed := e.EvaluateCLI(assertions, resp)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (snapshot skipped), got %d", len(results))
	}
	if results[0].Type != "exitCode" {
		t.Errorf("expected type exitCode, got %q", results[0].Type)
	}
}

func TestAssertion_EvaluateCLI_SkipsHTTPAssertions(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{ExitCode: &model.ExitCodeAssertion{Equals: 0}},
		{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
	}
	resp := &executor.CLIResponse{ExitCode: 0}
	results, allPassed := e.EvaluateCLI(assertions, resp)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (HTTP assertion skipped), got %d", len(results))
	}
}

func TestAssertion_EvaluateHTTP_EmptySlice(t *testing.T) {
	e := NewEvaluator()
	resp := &executor.HTTPResponse{StatusCode: 200}
	results, allPassed := e.EvaluateHTTP(nil, resp)
	if !allPassed {
		t.Errorf("expected all passed for nil assertions")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// --- EvaluateCLI ---

func TestAssertion_EvaluateCLI_ExitCodePass(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{ExitCode: &model.ExitCodeAssertion{Equals: 0}},
	}
	resp := &executor.CLIResponse{ExitCode: 0}
	results, allPassed := e.EvaluateCLI(assertions, resp)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("expected passed")
	}
	if results[0].Type != "exitCode" {
		t.Errorf("expected type exitCode, got %q", results[0].Type)
	}
}

func TestAssertion_EvaluateCLI_ExitCodeFail(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{ExitCode: &model.ExitCodeAssertion{Equals: 0}},
	}
	resp := &executor.CLIResponse{ExitCode: 1}
	results, allPassed := e.EvaluateCLI(assertions, resp)
	if allPassed {
		t.Errorf("expected not all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Errorf("expected failed")
	}
}

func TestAssertion_EvaluateCLI_StdoutContains(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Stdout: &model.StdoutAssertion{Contains: "hello"}},
	}
	resp := &executor.CLIResponse{Stdout: []byte("hello world")}
	results, allPassed := e.EvaluateCLI(assertions, resp)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("expected passed")
	}
}

func TestAssertion_EvaluateCLI_StdoutContainsFail(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Stdout: &model.StdoutAssertion{Contains: "missing"}},
	}
	resp := &executor.CLIResponse{Stdout: []byte("hello world")}
	results, allPassed := e.EvaluateCLI(assertions, resp)
	if allPassed {
		t.Errorf("expected not all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Errorf("expected failed")
	}
}

func TestAssertion_EvaluateCLI_StdoutEquals(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Stdout: &model.StdoutAssertion{Equals: "exact"}},
	}
	resp := &executor.CLIResponse{Stdout: []byte("exact")}
	results, allPassed := e.EvaluateCLI(assertions, resp)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestAssertion_EvaluateCLI_StdoutEqualsFail(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Stdout: &model.StdoutAssertion{Equals: "exact"}},
	}
	resp := &executor.CLIResponse{Stdout: []byte("not exact")}
	_, allPassed := e.EvaluateCLI(assertions, resp)
	if allPassed {
		t.Errorf("expected not all passed")
	}
}

func TestAssertion_EvaluateCLI_StderrContains(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Stderr: &model.StderrAssertion{Contains: "error"}},
	}
	resp := &executor.CLIResponse{Stderr: []byte("fatal error occurred")}
	results, allPassed := e.EvaluateCLI(assertions, resp)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Type != "stderr" {
		t.Errorf("expected type stderr, got %q", results[0].Type)
	}
}

func TestAssertion_EvaluateCLI_StderrEquals(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Stderr: &model.StderrAssertion{Equals: "error msg"}},
	}
	resp := &executor.CLIResponse{Stderr: []byte("error msg")}
	results, allPassed := e.EvaluateCLI(assertions, resp)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

// --- EvaluateSnapshots ---

func TestAssertion_EvaluateSnapshots_Match(t *testing.T) {
	e := NewEvaluator()
	dir := t.TempDir()

	// Create artifact file
	artifactPath := filepath.Join(dir, "actual.txt")
	if err := os.WriteFile(artifactPath, []byte("expected content"), 0644); err != nil {
		t.Fatalf("failed to create artifact: %v", err)
	}

	// Create snapshot file
	snapshotPath := filepath.Join(dir, "snapshot.txt")
	if err := os.WriteFile(snapshotPath, []byte("expected content"), 0644); err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	assertions := []model.Assertion{
		{Snapshot: &model.SnapshotAssertion{Artifact: "body", File: "snapshot.txt"}},
	}
	ctx := SnapshotContext{
		SuiteFilePath: filepath.Join(dir, "suite.spark"),
		Artifacts:     map[string]string{"body": artifactPath},
	}
	results, allPassed := e.EvaluateSnapshots(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("expected passed: %s", results[0].Message)
	}
}

func TestAssertion_EvaluateSnapshots_Mismatch(t *testing.T) {
	e := NewEvaluator()
	dir := t.TempDir()

	artifactPath := filepath.Join(dir, "actual.txt")
	if err := os.WriteFile(artifactPath, []byte("actual content"), 0644); err != nil {
		t.Fatalf("failed to create artifact: %v", err)
	}
	snapshotPath := filepath.Join(dir, "snapshot.txt")
	if err := os.WriteFile(snapshotPath, []byte("different content"), 0644); err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	assertions := []model.Assertion{
		{Snapshot: &model.SnapshotAssertion{Artifact: "body", File: "snapshot.txt"}},
	}
	ctx := SnapshotContext{
		SuiteFilePath: filepath.Join(dir, "suite.spark"),
		Artifacts:     map[string]string{"body": artifactPath},
	}
	results, allPassed := e.EvaluateSnapshots(assertions, ctx)
	if allPassed {
		t.Errorf("expected not all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Errorf("expected failed")
	}
}

func TestAssertion_EvaluateSnapshots_Regenerate(t *testing.T) {
	e := NewEvaluator()
	dir := t.TempDir()

	artifactPath := filepath.Join(dir, "actual.txt")
	if err := os.WriteFile(artifactPath, []byte("new content"), 0644); err != nil {
		t.Fatalf("failed to create artifact: %v", err)
	}

	assertions := []model.Assertion{
		{Snapshot: &model.SnapshotAssertion{Artifact: "body", File: "snapshot.txt"}},
	}
	ctx := SnapshotContext{
		SuiteFilePath: filepath.Join(dir, "suite.spark"),
		Artifacts:     map[string]string{"body": artifactPath},
		Regenerate:    true,
	}
	results, allPassed := e.EvaluateSnapshots(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed in regenerate mode")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Verify snapshot was created
	content, err := os.ReadFile(filepath.Join(dir, "snapshot.txt"))
	if err != nil {
		t.Fatalf("snapshot file not created: %v", err)
	}
	if string(content) != "new content" {
		t.Errorf("expected 'new content', got %q", string(content))
	}
}

func TestAssertion_EvaluateSnapshots_MissingSnapshotFile(t *testing.T) {
	e := NewEvaluator()
	dir := t.TempDir()

	artifactPath := filepath.Join(dir, "actual.txt")
	if err := os.WriteFile(artifactPath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create artifact: %v", err)
	}

	assertions := []model.Assertion{
		{Snapshot: &model.SnapshotAssertion{Artifact: "body", File: "nonexistent.txt"}},
	}
	ctx := SnapshotContext{
		SuiteFilePath: filepath.Join(dir, "suite.spark"),
		Artifacts:     map[string]string{"body": artifactPath},
	}
	results, allPassed := e.EvaluateSnapshots(assertions, ctx)
	if allPassed {
		t.Errorf("expected not all passed for missing snapshot")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Errorf("expected failed for missing snapshot file")
	}
}

func TestAssertion_EvaluateSnapshots_MissingArtifact(t *testing.T) {
	e := NewEvaluator()
	dir := t.TempDir()

	assertions := []model.Assertion{
		{Snapshot: &model.SnapshotAssertion{Artifact: "nonexistent", File: "snapshot.txt"}},
	}
	ctx := SnapshotContext{
		SuiteFilePath: filepath.Join(dir, "suite.spark"),
		Artifacts:     map[string]string{},
	}
	results, allPassed := e.EvaluateSnapshots(assertions, ctx)
	if allPassed {
		t.Errorf("expected not all passed for missing artifact")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Errorf("expected failed")
	}
}

func TestAssertion_EvaluateSnapshots_SkipsNonSnapshot(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{StatusCode: &model.StatusCodeAssertion{Equals: 200}},
	}
	ctx := SnapshotContext{
		SuiteFilePath: "/fake/suite.spark",
		Artifacts:     map[string]string{},
	}
	results, allPassed := e.EvaluateSnapshots(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed when no snapshot assertions")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// --- truncateString ---

func TestAssertion_TruncateString_Short(t *testing.T) {
	result := truncateString("hello", 10)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestAssertion_TruncateString_Exact(t *testing.T) {
	result := truncateString("hello", 5)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestAssertion_TruncateString_Long(t *testing.T) {
	result := truncateString("hello world", 8)
	if result != "hello..." {
		t.Errorf("expected 'hello...', got %q", result)
	}
}
