package assertion

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"chiperka-cli/internal/executor"
	"chiperka-cli/internal/model"
)

func intPtr(v int) *int       { return &v }
func int64Ptr(v int64) *int64 { return &v }
func boolPtr(v bool) *bool    { return &v }

// --- EvaluateAll: Response assertions ---

func TestAssertion_Response_StatusCodePass(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{StatusCode: intPtr(200)}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{StatusCode: 200},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("expected passed")
	}
	if results[0].Type != "response.statusCode" {
		t.Errorf("expected type response.statusCode, got %q", results[0].Type)
	}
}

func TestAssertion_Response_StatusCodeFail(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{StatusCode: intPtr(200)}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{StatusCode: 404},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
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

func TestAssertion_Response_MultipleAssertions(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{StatusCode: intPtr(200)}},
		{Response: &model.ResponseAssertion{StatusCode: intPtr(201)}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{StatusCode: 200},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
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

func TestAssertion_Response_NoAssertion(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{{}}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{StatusCode: 200},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed for empty assertion")
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty assertion, got %d", len(results))
	}
}

func TestAssertion_Response_EmptySlice(t *testing.T) {
	e := NewEvaluator()
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{StatusCode: 200},
	}
	results, allPassed := e.EvaluateAll(nil, ctx)
	if !allPassed {
		t.Errorf("expected all passed for nil assertions")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestAssertion_Response_HeaderEquals(t *testing.T) {
	e := NewEvaluator()
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Headers: map[string]model.HeaderMatcher{
				"Content-Type": {Equals: "application/json"},
			},
		}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{StatusCode: 200, Headers: h},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
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

func TestAssertion_Response_HeaderExists(t *testing.T) {
	e := NewEvaluator()
	h := http.Header{}
	h.Set("X-Request-Id", "abc123")
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Headers: map[string]model.HeaderMatcher{
				"X-Request-Id": {Exists: boolPtr(true)},
			},
		}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{StatusCode: 200, Headers: h},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestAssertion_Response_BodyContains(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Body: &model.ResponseBodyAssertion{Contains: "success"},
		}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{StatusCode: 200, Body: []byte(`{"status":"success"}`)},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestAssertion_Response_BodyEquals(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Body: &model.ResponseBodyAssertion{Equals: "exact body"},
		}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{StatusCode: 200, Body: []byte("exact body")},
	}
	_, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
}

func TestAssertion_Response_BodyMinSize(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Body: &model.ResponseBodyAssertion{MinSize: int64Ptr(5)},
		}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{StatusCode: 200, Body: []byte("hello world")},
	}
	_, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
}

func TestAssertion_Response_BodyMinSizeFail(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Body: &model.ResponseBodyAssertion{MinSize: int64Ptr(100)},
		}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{StatusCode: 200, Body: []byte("short")},
	}
	_, allPassed := e.EvaluateAll(assertions, ctx)
	if allPassed {
		t.Errorf("expected not all passed")
	}
}

func TestAssertion_Response_Time(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Time: &model.ResponseTimeAssertion{MaxMs: 500},
		}},
	}
	ctx := EvalContext{
		HTTPResponse:      &executor.HTTPResponse{StatusCode: 200},
		ExecutionDuration: 100 * time.Millisecond,
	}
	_, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
}

func TestAssertion_Response_TimeFail(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Time: &model.ResponseTimeAssertion{MaxMs: 100},
		}},
	}
	ctx := EvalContext{
		HTTPResponse:      &executor.HTTPResponse{StatusCode: 200},
		ExecutionDuration: 200 * time.Millisecond,
	}
	_, allPassed := e.EvaluateAll(assertions, ctx)
	if allPassed {
		t.Errorf("expected not all passed")
	}
}

// --- EvaluateAll: CLI assertions ---

func TestAssertion_CLI_ExitCodePass(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{CLI: &model.CLIAssertion{ExitCode: intPtr(0)}},
	}
	ctx := EvalContext{
		CLIResponse: &executor.CLIResponse{ExitCode: 0},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("expected passed")
	}
	if results[0].Type != "cli.exitCode" {
		t.Errorf("expected type cli.exitCode, got %q", results[0].Type)
	}
}

func TestAssertion_CLI_ExitCodeFail(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{CLI: &model.CLIAssertion{ExitCode: intPtr(0)}},
	}
	ctx := EvalContext{
		CLIResponse: &executor.CLIResponse{ExitCode: 1},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
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

func TestAssertion_CLI_StdoutContains(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{CLI: &model.CLIAssertion{Stdout: &model.CLIOutputAssertion{Contains: "hello"}}},
	}
	ctx := EvalContext{
		CLIResponse: &executor.CLIResponse{Stdout: []byte("hello world")},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
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

func TestAssertion_CLI_StdoutContainsFail(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{CLI: &model.CLIAssertion{Stdout: &model.CLIOutputAssertion{Contains: "missing"}}},
	}
	ctx := EvalContext{
		CLIResponse: &executor.CLIResponse{Stdout: []byte("hello world")},
	}
	_, allPassed := e.EvaluateAll(assertions, ctx)
	if allPassed {
		t.Errorf("expected not all passed")
	}
}

func TestAssertion_CLI_StdoutEquals(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{CLI: &model.CLIAssertion{Stdout: &model.CLIOutputAssertion{Equals: "exact"}}},
	}
	ctx := EvalContext{
		CLIResponse: &executor.CLIResponse{Stdout: []byte("exact")},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestAssertion_CLI_StdoutEqualsFail(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{CLI: &model.CLIAssertion{Stdout: &model.CLIOutputAssertion{Equals: "exact"}}},
	}
	ctx := EvalContext{
		CLIResponse: &executor.CLIResponse{Stdout: []byte("not exact")},
	}
	_, allPassed := e.EvaluateAll(assertions, ctx)
	if allPassed {
		t.Errorf("expected not all passed")
	}
}

func TestAssertion_CLI_StderrContains(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{CLI: &model.CLIAssertion{Stderr: &model.CLIOutputAssertion{Contains: "error"}}},
	}
	ctx := EvalContext{
		CLIResponse: &executor.CLIResponse{Stderr: []byte("fatal error occurred")},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Type != "cli.stderr" {
		t.Errorf("expected type cli.stderr, got %q", results[0].Type)
	}
}

func TestAssertion_CLI_StderrEquals(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{CLI: &model.CLIAssertion{Stderr: &model.CLIOutputAssertion{Equals: "error msg"}}},
	}
	ctx := EvalContext{
		CLIResponse: &executor.CLIResponse{Stderr: []byte("error msg")},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

// --- Body snapshot ---

func TestAssertion_Response_BodySnapshotMatch(t *testing.T) {
	e := NewEvaluator()
	dir := t.TempDir()

	snapshotPath := filepath.Join(dir, "snapshot.txt")
	if err := os.WriteFile(snapshotPath, []byte("expected content"), 0644); err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Body: &model.ResponseBodyAssertion{Snapshot: "snapshot.txt"},
		}},
	}
	ctx := EvalContext{
		HTTPResponse:  &executor.HTTPResponse{StatusCode: 200, Body: []byte("expected content")},
		SuiteFilePath: filepath.Join(dir, "suite.chiperka"),
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
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

func TestAssertion_Response_BodySnapshotMismatch(t *testing.T) {
	e := NewEvaluator()
	dir := t.TempDir()

	snapshotPath := filepath.Join(dir, "snapshot.txt")
	if err := os.WriteFile(snapshotPath, []byte("different content"), 0644); err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Body: &model.ResponseBodyAssertion{Snapshot: "snapshot.txt"},
		}},
	}
	ctx := EvalContext{
		HTTPResponse:  &executor.HTTPResponse{StatusCode: 200, Body: []byte("actual content")},
		SuiteFilePath: filepath.Join(dir, "suite.chiperka"),
	}
	_, allPassed := e.EvaluateAll(assertions, ctx)
	if allPassed {
		t.Errorf("expected not all passed")
	}
}

func TestAssertion_Response_BodySnapshotRegenerate(t *testing.T) {
	e := NewEvaluator()
	dir := t.TempDir()

	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Body: &model.ResponseBodyAssertion{Snapshot: "snapshot.txt"},
		}},
	}
	ctx := EvalContext{
		HTTPResponse:  &executor.HTTPResponse{StatusCode: 200, Body: []byte("new content")},
		SuiteFilePath: filepath.Join(dir, "suite.chiperka"),
		Regenerate:    true,
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed in regenerate mode")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	content, err := os.ReadFile(filepath.Join(dir, "snapshot.txt"))
	if err != nil {
		t.Fatalf("snapshot file not created: %v", err)
	}
	if string(content) != "new content" {
		t.Errorf("expected 'new content', got %q", string(content))
	}
}

// --- CLI stdout snapshot ---

func TestAssertion_CLI_StdoutSnapshot(t *testing.T) {
	e := NewEvaluator()
	dir := t.TempDir()

	snapshotPath := filepath.Join(dir, "expected-stdout.txt")
	if err := os.WriteFile(snapshotPath, []byte("migrated ok"), 0644); err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	assertions := []model.Assertion{
		{CLI: &model.CLIAssertion{
			Stdout: &model.CLIOutputAssertion{Snapshot: "expected-stdout.txt"},
		}},
	}
	ctx := EvalContext{
		CLIResponse:   &executor.CLIResponse{Stdout: []byte("migrated ok")},
		SuiteFilePath: filepath.Join(dir, "suite.chiperka"),
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

// --- JsonPath ---

func TestAssertion_Response_JsonPathExists(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Body: &model.ResponseBodyAssertion{
				JsonPath: []model.JsonPathCheck{
					{Path: "$.token", Expected: "exists"},
				},
			},
		}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{
			StatusCode: 200,
			Body:       []byte(`{"token":"abc123"}`),
		},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
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

func TestAssertion_Response_JsonPathExistsFail(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Body: &model.ResponseBodyAssertion{
				JsonPath: []model.JsonPathCheck{
					{Path: "$.missing", Expected: "exists"},
				},
			},
		}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{
			StatusCode: 200,
			Body:       []byte(`{"token":"abc123"}`),
		},
	}
	_, allPassed := e.EvaluateAll(assertions, ctx)
	if allPassed {
		t.Errorf("expected not all passed")
	}
}

func TestAssertion_Response_JsonPathValue(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Body: &model.ResponseBodyAssertion{
				JsonPath: []model.JsonPathCheck{
					{Path: "$.user.email", Expected: "test@example.com"},
				},
			},
		}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{
			StatusCode: 200,
			Body:       []byte(`{"user":{"email":"test@example.com"}}`),
		},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestAssertion_Response_JsonPathArrayIndex(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Body: &model.ResponseBodyAssertion{
				JsonPath: []model.JsonPathCheck{
					{Path: "$.items[0].name", Expected: "first"},
				},
			},
		}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{
			StatusCode: 200,
			Body:       []byte(`{"items":[{"name":"first"},{"name":"second"}]}`),
		},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestAssertion_Response_JsonPathWildcard(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{
			Body: &model.ResponseBodyAssertion{
				JsonPath: []model.JsonPathCheck{
					{Path: "$.items[*].name", Expected: `["first","second"]`},
				},
			},
		}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{
			StatusCode: 200,
			Body:       []byte(`{"items":[{"name":"first"},{"name":"second"}]}`),
		},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed: %v", results)
	}
}

// --- Mixed assertions ---

func TestAssertion_MixedResponseAndCLI(t *testing.T) {
	e := NewEvaluator()
	assertions := []model.Assertion{
		{Response: &model.ResponseAssertion{StatusCode: intPtr(200)}},
		{CLI: &model.CLIAssertion{ExitCode: intPtr(0)}},
	}
	ctx := EvalContext{
		HTTPResponse: &executor.HTTPResponse{StatusCode: 200},
		CLIResponse:  &executor.CLIResponse{ExitCode: 0},
	}
	results, allPassed := e.EvaluateAll(assertions, ctx)
	if !allPassed {
		t.Errorf("expected all passed")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
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

// --- walkJsonPath ---

func TestWalkJsonPath_TopLevel(t *testing.T) {
	data := map[string]interface{}{"name": "test"}
	result, found := walkJsonPath(data, "$.name")
	if !found {
		t.Fatalf("expected found")
	}
	if result != "test" {
		t.Errorf("expected 'test', got %v", result)
	}
}

func TestWalkJsonPath_Nested(t *testing.T) {
	data := map[string]interface{}{
		"user": map[string]interface{}{"email": "a@b.c"},
	}
	result, found := walkJsonPath(data, "$.user.email")
	if !found {
		t.Fatalf("expected found")
	}
	if result != "a@b.c" {
		t.Errorf("expected 'a@b.c', got %v", result)
	}
}

func TestWalkJsonPath_ArrayIndex(t *testing.T) {
	data := map[string]interface{}{
		"items": []interface{}{"a", "b", "c"},
	}
	result, found := walkJsonPath(data, "$.items[1]")
	if !found {
		t.Fatalf("expected found")
	}
	if result != "b" {
		t.Errorf("expected 'b', got %v", result)
	}
}

func TestWalkJsonPath_NotFound(t *testing.T) {
	data := map[string]interface{}{"name": "test"}
	_, found := walkJsonPath(data, "$.missing")
	if found {
		t.Errorf("expected not found")
	}
}
