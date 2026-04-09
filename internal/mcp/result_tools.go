package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"chiperka-cli/internal/result"
)

func readRunTool() mcp.Tool {
	return mcp.NewTool("chiperka_read_run",
		mcp.WithDescription("Read a stored test run result. Returns run summary with test list and UUIDs. Use chiperka_read_test to drill into individual test details."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("uuid",
			mcp.Description("Run UUID (e.g. lr-abcd1234-...)"),
			mcp.Required(),
		),
	)
}

func readTestTool() mcp.Tool {
	return mcp.NewTool("chiperka_read_test",
		mcp.WithDescription("Read a stored test result detail. Returns assertions, HTTP exchanges, CLI executions, services, and artifact list with UUIDs. Use chiperka_read_artifact to read artifact content."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("uuid",
			mcp.Description("Test UUID (e.g. lt-abcd1234-...)"),
			mcp.Required(),
		),
	)
}

func readArtifactTool() mcp.Tool {
	return mcp.NewTool("chiperka_read_artifact",
		mcp.WithDescription("Read raw content of a stored test artifact (response body, stdout, stderr, logs, etc.). Get available artifact names from chiperka_read_test."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("test_uuid",
			mcp.Description("Test UUID that owns the artifact"),
			mcp.Required(),
		),
		mcp.WithString("name",
			mcp.Description("Artifact filename (e.g. response_body.json, stdout.txt)"),
			mcp.Required(),
		),
	)
}

func readResultRunsTool() mcp.Tool {
	return mcp.NewTool("chiperka_read_runs",
		mcp.WithDescription("List recent test runs with summary info. Use chiperka_read_run to see details of a specific run."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of runs to return (default 10)"),
		),
	)
}

func handleReadRuns(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := 10
	if l, ok := request.GetArguments()["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	store := result.DefaultLocalStore()
	runs, err := store.ListRuns(limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list runs: %w", err)
	}

	if runs == nil {
		runs = []result.RunSummary{}
	}

	return jsonResult(runs)
}

func handleReadRun(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, _ := request.GetArguments()["uuid"].(string)
	if uuid == "" {
		return nil, fmt.Errorf("uuid is required")
	}

	store := storeForUUID(uuid)
	run, err := store.GetRun(uuid)
	if err != nil {
		return nil, fmt.Errorf("failed to read run: %w", err)
	}

	return jsonResult(run)
}

func handleReadTest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uuid, _ := request.GetArguments()["uuid"].(string)
	if uuid == "" {
		return nil, fmt.Errorf("uuid is required")
	}

	store := storeForUUID(uuid)
	test, err := store.GetTest(uuid)
	if err != nil {
		return nil, fmt.Errorf("failed to read test: %w", err)
	}

	return jsonResult(test)
}

func handleReadArtifact(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	testUUID, _ := request.GetArguments()["test_uuid"].(string)
	if testUUID == "" {
		return nil, fmt.Errorf("test_uuid is required")
	}
	name, _ := request.GetArguments()["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	store := storeForUUID(testUUID)
	content, err := store.GetArtifact(testUUID, name)
	if err != nil {
		return nil, fmt.Errorf("failed to read artifact: %w", err)
	}

	// Truncate large artifacts for MCP responses
	const maxSize = 32 * 1024
	truncated := false
	if len(content) > maxSize {
		content = content[:maxSize]
		truncated = true
	}

	resp := map[string]interface{}{
		"name":    name,
		"content": string(content),
		"size":    len(content),
	}
	if truncated {
		resp["truncated"] = true
		resp["hint"] = fmt.Sprintf("Artifact truncated to 32KB. Use 'chiperka result artifact %s %s' for full content.", testUUID, name)
	}

	return jsonResult(resp)
}

func storeForUUID(uuid string) result.Store {
	if result.IsCloud(uuid) {
		return result.NewCloudStore()
	}
	return result.DefaultLocalStore()
}
