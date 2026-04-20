package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"chiperka-cli/internal/config"
	"chiperka-cli/internal/discovery"
	"chiperka-cli/internal/events"
	"chiperka-cli/internal/events/subscribers"
	"chiperka-cli/internal/finder"
	"chiperka-cli/internal/model"
	"chiperka-cli/internal/parser"
	"chiperka-cli/internal/result"
	"chiperka-cli/internal/runner"
	"chiperka-cli/internal/telemetry"
	"time"
)

// --- Tool definitions ---

func contextTool() mcp.Tool {
	return mcp.NewTool("chiperka_context",
		mcp.WithDescription("Get AI-readable Chiperka test runner reference. Call this first to understand the test file format, commands, and workflow."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
}

func listTool() mcp.Tool {
	return mcp.NewTool("chiperka_list",
		mcp.WithDescription("List resources by kind: test, service, or endpoint. Returns a compact summary for each item. Use filter to narrow results by name pattern."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("kind",
			mcp.Description("Resource kind: test, service, or endpoint"),
			mcp.Required(),
		),
		mcp.WithString("filter",
			mcp.Description("Name pattern filter (supports * wildcard). Matches against test name, suite name, or endpoint name."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of items to return (default: all)"),
		),
		mcp.WithNumber("offset",
			mcp.Description("Number of items to skip (default: 0). Use with limit for pagination."),
		),
	)
}

func getTool() mcp.Tool {
	return mcp.NewTool("chiperka_get",
		mcp.WithDescription("Get full detail of a single resource by kind and name. Use after chiperka_list to drill into a specific test, service, or endpoint."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("kind",
			mcp.Description("Resource kind: test, service, or endpoint"),
			mcp.Required(),
		),
		mcp.WithString("name",
			mcp.Description("Resource name"),
			mcp.Required(),
		),
	)
}

func validateTool() mcp.Tool {
	return mcp.NewTool("chiperka_validate",
		mcp.WithDescription("Validate Chiperka test files for structural and semantic errors without executing them. Catches missing images, broken template references, missing execution blocks, etc."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("path",
			mcp.Description("Directory or .chiperka file path"),
			mcp.Required(),
		),
		mcp.WithString("filter",
			mcp.Description("Name pattern filter (supports * wildcard)"),
		),
		mcp.WithString("configuration",
			mcp.Description("Path to chiperka.yaml configuration file"),
		),
	)
}

func runTool() mcp.Tool {
	return mcp.NewTool("chiperka_run",
		mcp.WithDescription("Execute Chiperka tests and return full results including status, assertions, duration, errors, logs, and HTTP exchanges. Failed tests include response bodies and service logs for debugging."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("path",
			mcp.Description("Directory or .chiperka file path"),
			mcp.Required(),
		),
		mcp.WithString("filter",
			mcp.Description("Name pattern filter (supports * wildcard)"),
		),
		mcp.WithString("configuration",
			mcp.Description("Path to chiperka.yaml configuration file"),
		),
		mcp.WithNumber("timeout",
			mcp.Description("Maximum seconds per test (default 300)"),
		),
		mcp.WithBoolean("regenerate_snapshots",
			mcp.Description("Update snapshot files instead of comparing them"),
		),
	)
}

func executeTool() mcp.Tool {
	return mcp.NewTool("chiperka_execute",
		mcp.WithDescription("Execute an inline test definition without a .chiperka file. Pass YAML directly (same format as a .chiperka file). Returns full HTTP exchange — status, headers, response body, service logs — so you can see exactly what the endpoint returns before writing assertions. Use this to probe endpoints and understand their behavior."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("yaml",
			mcp.Description("Inline YAML test definition (same format as .chiperka file content)"),
			mcp.Required(),
		),
		mcp.WithString("configuration",
			mcp.Description("Path to chiperka.yaml configuration file"),
		),
		mcp.WithNumber("timeout",
			mcp.Description("Maximum seconds per test (default 300)"),
		),
	)
}

// --- Handlers ---

func handleContext(contextText string) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(contextText), nil
	}
}

func handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kind, _ := request.GetArguments()["kind"].(string)
	if kind == "" {
		return nil, fmt.Errorf("kind is required")
	}
	filter, _ := request.GetArguments()["filter"].(string)
	limit := -1
	if l, ok := request.GetArguments()["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	offset := 0
	if o, ok := request.GetArguments()["offset"].(float64); ok && o > 0 {
		offset = int(o)
	}

	parsed, err := discovery.AllWithConfig(defaultConfigFile)
	if err != nil {
		return nil, err
	}

	switch kind {
	case "test":
		items := discovery.ListTests(parsed)
		if filter != "" {
			items = filterTests(items, filter)
		}
		return jsonResult(paginate(items, offset, limit))
	case "service":
		items := discovery.ListServices(parsed)
		if filter != "" {
			items = filterServices(items, filter)
		}
		return jsonResult(paginate(items, offset, limit))
	case "endpoint":
		items := discovery.ListEndpoints(parsed)
		if filter != "" {
			items = filterEndpoints(items, filter)
		}
		return jsonResult(paginate(items, offset, limit))
	default:
		return nil, fmt.Errorf("unknown kind %q (expected test, service, or endpoint)", kind)
	}
}

func handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kind, _ := request.GetArguments()["kind"].(string)
	if kind == "" {
		return nil, fmt.Errorf("kind is required")
	}
	name, _ := request.GetArguments()["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	parsed, err := discovery.AllWithConfig(defaultConfigFile)
	if err != nil {
		return nil, err
	}

	switch kind {
	case "test":
		detail, err := discovery.GetTest(parsed, name)
		if err != nil {
			return nil, err
		}
		return jsonResult(detail)
	case "service":
		tmpl, err := discovery.GetService(parsed, name)
		if err != nil {
			return nil, err
		}
		return jsonResult(tmpl)
	case "endpoint":
		ep, err := discovery.GetEndpoint(parsed, name)
		if err != nil {
			return nil, err
		}
		return jsonResult(ep)
	default:
		return nil, fmt.Errorf("unknown kind %q (expected test, service, or endpoint)", kind)
	}
}

func handleValidate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, _ := request.GetArguments()["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	filter, _ := request.GetArguments()["filter"].(string)
	configFile, _ := request.GetArguments()["configuration"].(string)

	if _, err := loadConfig(configFile); err != nil {
		return nil, err
	}

	tests, services, err := discoverTests(path, nil, filter)
	if err != nil {
		return nil, err
	}

	// Also discover endpoints
	parsed, err := discovery.AllWithConfig(defaultConfigFile)
	if err != nil {
		return nil, err
	}

	type issue struct {
		Level    string `json:"level"`
		File     string `json:"file"`
		Suite    string `json:"suite,omitempty"`
		Test     string `json:"test,omitempty"`
		Endpoint string `json:"endpoint,omitempty"`
		Message  string `json:"message"`
	}
	type summary struct {
		Files     int `json:"files"`
		Suites    int `json:"suites"`
		Tests     int `json:"tests"`
		Endpoints int `json:"endpoints"`
		Errors    int `json:"errors"`
		Warnings  int `json:"warnings"`
	}
	type validateResult struct {
		Valid   bool    `json:"valid"`
		Issues  []issue `json:"issues"`
		Summary summary `json:"summary"`
	}

	result := validateResult{
		Valid:  true,
		Issues: []issue{},
	}

	fileSet := make(map[string]bool)
	totalTests := 0

	// Validate endpoints
	for _, ep := range parsed.Endpoints.Endpoints {
		fileSet[ep.FilePath] = true
		if ep.Name == "" {
			result.Issues = append(result.Issues, issue{Level: "error", File: ep.FilePath, Message: "endpoint name is empty"})
		}
		if ep.Service == "" {
			result.Issues = append(result.Issues, issue{Level: "error", File: ep.FilePath, Endpoint: ep.Name, Message: "service is empty"})
		} else if services.GetTemplate(ep.Service) == nil {
			result.Issues = append(result.Issues, issue{Level: "warning", File: ep.FilePath, Endpoint: ep.Name, Message: fmt.Sprintf("service %q not found in service templates", ep.Service)})
		}
		if ep.Method == "" {
			result.Issues = append(result.Issues, issue{Level: "error", File: ep.FilePath, Endpoint: ep.Name, Message: "method is empty"})
		}
		if ep.URL == "" {
			result.Issues = append(result.Issues, issue{Level: "error", File: ep.FilePath, Endpoint: ep.Name, Message: "url is empty"})
		}
	}

	// Validate test suites
	for _, suite := range tests.Suites {
		fileSet[suite.FilePath] = true

		if suite.Name == "" {
			result.Issues = append(result.Issues, issue{
				Level:   "error",
				File:    suite.FilePath,
				Message: "suite name is empty",
			})
		}
		if len(suite.Tests) == 0 {
			result.Issues = append(result.Issues, issue{
				Level:   "error",
				File:    suite.FilePath,
				Suite:   suite.Name,
				Message: "suite has no tests",
			})
		}

		for _, test := range suite.Tests {
			totalTests++
			issues := validateTest(test, suite, services)
			for _, vi := range issues {
				result.Issues = append(result.Issues, issue{
					Level:   vi.Level,
					File:    vi.File,
					Suite:   vi.Suite,
					Test:    vi.Test,
					Message: vi.Message,
				})
			}
		}
	}

	errors := 0
	warnings := 0
	for _, i := range result.Issues {
		if i.Level == "error" {
			errors++
		} else {
			warnings++
		}
	}
	result.Valid = errors == 0
	result.Summary = summary{
		Files:     len(fileSet),
		Suites:    len(tests.Suites),
		Tests:     totalTests,
		Endpoints: len(parsed.Endpoints.Endpoints),
		Errors:    errors,
		Warnings:  warnings,
	}

	return jsonResult(result)
}

func handleRun(version string) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, _ := request.GetArguments()["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		filter, _ := request.GetArguments()["filter"].(string)
		configFile, _ := request.GetArguments()["configuration"].(string)
		timeout := 300
		if t, ok := request.GetArguments()["timeout"].(float64); ok && t > 0 {
			timeout = int(t)
		}
		regenerateSnapshots, _ := request.GetArguments()["regenerate_snapshots"].(bool)
		// Load capacity from machine config
		machineConfig := telemetry.LoadMachineConfig()
		capacity := runtime.NumCPU() * 2
		maxContainers := 0
		if machineConfig != nil {
			if machineConfig.Capacity > 0 {
				capacity = machineConfig.Capacity
			}
			if machineConfig.MaxContainers > 0 {
				maxContainers = machineConfig.MaxContainers
			}
		}

		cfg, err := loadConfig(configFile)
		if err != nil {
			return nil, err
		}

		tests, services, err := discoverTests(path, nil, filter)
		if err != nil {
			return nil, err
		}

		if tests.TotalTests() == 0 {
			if services.HasTemplates() {
				return nil, fmt.Errorf(
					"services are not runnable directly — reference them from a test (found %d service(s) and 0 tests at %s)",
					len(services.Templates), path)
			}
			return jsonResult(map[string]interface{}{
				"status":      "passed",
				"passed":      0,
				"failed":      0,
				"errored":     0,
				"duration_ms": 0,
				"results":     []interface{}{},
			})
		}

		// Set up event bus with collector only (no stdout output)
		bus := events.NewBus()
		collector := subscribers.NewEventCollector()
		collector.Register(bus)

		// Generate run UUID upfront so artifacts are written directly into the result tree
		runUUID := result.NewRunUUID()
		runDir := filepath.Join(".chiperka", "results", "runs", runUUID)
		os.MkdirAll(runDir, 0755)

		r, err := runner.New(bus, capacity, maxContainers, runDir, services, regenerateSnapshots, timeout, version, collector, cfg.ExecutionVariables)
		if err != nil {
			return nil, fmt.Errorf("failed to create test runner: %w", err)
		}

		startTime := time.Now()
		runResult := r.Run(ctx, tests)

		// Record telemetry with MCP source
		runStats := telemetry.CollectRunStats(tests, services)
		telemetry.RecordRun(telemetry.RunParams{
			Version:          version,
			Source:           "mcp",
			DurationMs:       time.Since(startTime).Milliseconds(),
			Capacity:         capacity,
			ExecutorType:     runStats.ExecutorType,
			ServiceCount:     runStats.ServiceCount,
			Snapshots:        runStats.HasSnapshots,
			HasSetup:         runStats.HasSetup,
			HasTeardown:      runStats.HasTeardown,
			HasHooks:         runStats.HasHooks,
			ServiceTemplates: runStats.HasServiceTemplates,
		}, runResult.TotalTests(), runResult.TotalPassed(), runResult.TotalFailed(), runResult.TotalSkipped(), len(tests.Suites))
		telemetry.Wait(2 * time.Second)

		// Persist results — artifacts are already in runDir
		rw := result.NewWriter(".chiperka/results/runs")
		if err := rw.Persist(runUUID, runResult, startTime); err != nil {
			return nil, fmt.Errorf("failed to persist results: %w", err)
		}

		// Return the persisted run summary — same as run.json
		store := result.DefaultLocalStore()
		run, err := store.GetRun(runUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to read persisted run: %w", err)
		}

		return jsonResult(run)
	}
}

func handleExecute(version string) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		yamlStr, _ := request.GetArguments()["yaml"].(string)
		if yamlStr == "" {
			return nil, fmt.Errorf("yaml is required")
		}
		configFile, _ := request.GetArguments()["configuration"].(string)
		timeout := 300
		if t, ok := request.GetArguments()["timeout"].(float64); ok && t > 0 {
			timeout = int(t)
		}

		cfg, err := loadConfig(configFile)
		if err != nil {
			return nil, err
		}
		services, err := discoverServices(".")
		if err != nil {
			return nil, err
		}

		// Parse inline YAML
		p := parser.New()
		suite, err := p.ParseBytes([]byte(yamlStr))
		if err != nil {
			return nil, fmt.Errorf("invalid YAML: %w", err)
		}

		tests := model.NewTestCollection()
		tests.AddSuite(*suite)

		if tests.TotalTests() == 0 {
			return nil, fmt.Errorf("YAML contains no tests")
		}

		// Run with collector only (no stdout)
		bus := events.NewBus()
		collector := subscribers.NewEventCollector()
		collector.Register(bus)

		// Load capacity from machine config
		execMachineConfig := telemetry.LoadMachineConfig()
		execCapacity := runtime.NumCPU() * 2
		execMaxContainers := 0
		if execMachineConfig != nil {
			if execMachineConfig.Capacity > 0 {
				execCapacity = execMachineConfig.Capacity
			}
			if execMachineConfig.MaxContainers > 0 {
				execMaxContainers = execMachineConfig.MaxContainers
			}
		}

		r, err := runner.New(bus, execCapacity, execMaxContainers, os.TempDir(), services, false, timeout, version, collector, cfg.ExecutionVariables)
		if err != nil {
			return nil, fmt.Errorf("failed to create test runner: %w", err)
		}

		execStartTime := time.Now()
		runResult := r.Run(ctx, tests)

		// Record telemetry with MCP source
		execRunStats := telemetry.CollectRunStats(tests, services)
		telemetry.RecordRun(telemetry.RunParams{
			Version:          version,
			Source:           "mcp",
			DurationMs:       time.Since(execStartTime).Milliseconds(),
			Capacity:         execCapacity,
			ExecutorType:     execRunStats.ExecutorType,
			ServiceCount:     execRunStats.ServiceCount,
			Snapshots:        execRunStats.HasSnapshots,
			HasSetup:         execRunStats.HasSetup,
			HasTeardown:      execRunStats.HasTeardown,
			HasHooks:         execRunStats.HasHooks,
			ServiceTemplates: execRunStats.HasServiceTemplates,
		}, runResult.TotalTests(), runResult.TotalPassed(), runResult.TotalFailed(), runResult.TotalSkipped(), len(tests.Suites))
		telemetry.Wait(2 * time.Second)

		// Always return full exchange details (the point is to see what comes back)
		type httpExchangeJSON struct {
			Phase          string            `json:"phase"`
			Method         string            `json:"method"`
			URL            string            `json:"url"`
			RequestHeaders map[string]string `json:"request_headers,omitempty"`
			RequestBody    string            `json:"request_body,omitempty"`
			StatusCode     int               `json:"status_code"`
			ResponseHeaders map[string][]string `json:"response_headers,omitempty"`
			ResponseBody   string            `json:"response_body,omitempty"`
			DurationMs     int64             `json:"duration_ms"`
			Error          string            `json:"error,omitempty"`
		}
		type cliExecJSON struct {
			Phase      string `json:"phase"`
			Service    string `json:"service"`
			Command    string `json:"command"`
			ExitCode   int    `json:"exit_code"`
			Stdout     string `json:"stdout,omitempty"`
			Stderr     string `json:"stderr,omitempty"`
			DurationMs int64  `json:"duration_ms"`
			Error      string `json:"error,omitempty"`
		}
		type assertionJSON struct {
			Assertion string `json:"assertion"`
			Status    string `json:"status"`
			Expected  string `json:"expected,omitempty"`
			Actual    string `json:"actual,omitempty"`
		}
		type testResultJSON struct {
			Test          string             `json:"test"`
			Status        string             `json:"status"`
			DurationMs    int64              `json:"duration_ms"`
			Assertions    []assertionJSON    `json:"assertions,omitempty"`
			Error         string             `json:"error,omitempty"`
			HTTPExchanges []httpExchangeJSON `json:"http_exchanges,omitempty"`
			CLIExecutions []cliExecJSON      `json:"cli_executions,omitempty"`
			Logs          []string           `json:"logs,omitempty"`
		}
		type executeResultJSON struct {
			Status     string           `json:"status"`
			DurationMs int64            `json:"duration_ms"`
			Results    []testResultJSON `json:"results"`
		}

		result := executeResultJSON{
			Status: "passed",
		}
		if runResult.HasFailures() {
			result.Status = "failed"
		}
		if runResult.TotalErrors() > 0 {
			result.Status = "error"
		}

		var totalDuration int64
		for _, sr := range runResult.SuiteResults {
			for _, tr := range sr.TestResults {
				durationMs := tr.Duration.Milliseconds()
				totalDuration += durationMs
				trJSON := testResultJSON{
					Test:       tr.Test.Name,
					Status:     string(tr.Status),
					DurationMs: durationMs,
				}

				if tr.Error != nil {
					trJSON.Error = tr.Error.Error()
				}

				// Assertions (if any were defined)
				for _, ar := range tr.AssertionResults {
					status := "pass"
					if !ar.Passed {
						status = "fail"
					}
					a := assertionJSON{
						Assertion: ar.Message,
						Status:    status,
					}
					if !ar.Passed {
						a.Expected = ar.Expected
						a.Actual = ar.Actual
					}
					trJSON.Assertions = append(trJSON.Assertions, a)
				}

				// Always include full HTTP exchanges (this is the point of execute)
				for _, ex := range tr.HTTPExchanges {
					he := httpExchangeJSON{
						Phase:           ex.Phase,
						Method:          ex.RequestMethod,
						URL:             ex.RequestURL,
						RequestHeaders:  ex.RequestHeaders,
						RequestBody:     ex.RequestBody,
						StatusCode:      ex.ResponseStatusCode,
						ResponseHeaders: ex.ResponseHeaders,
						DurationMs:      ex.Duration.Milliseconds(),
					}
					body := ex.ResponseBody
					if len(body) > 8192 {
						body = body[:8192] + "\n... (truncated)"
					}
					if body != "" {
						he.ResponseBody = body
					}
					if ex.Error != nil {
						he.Error = ex.Error.Error()
					}
					trJSON.HTTPExchanges = append(trJSON.HTTPExchanges, he)
				}

				// CLI executions
				for _, ce := range tr.CLIExecutions {
					cl := cliExecJSON{
						Phase:      ce.Phase,
						Service:    ce.Service,
						Command:    ce.Command,
						ExitCode:   ce.ExitCode,
						DurationMs: ce.Duration.Milliseconds(),
					}
					stdout := ce.Stdout
					if len(stdout) > 8192 {
						stdout = stdout[:8192] + "\n... (truncated)"
					}
					if stdout != "" {
						cl.Stdout = stdout
					}
					stderr := ce.Stderr
					if len(stderr) > 8192 {
						stderr = stderr[:8192] + "\n... (truncated)"
					}
					if stderr != "" {
						cl.Stderr = stderr
					}
					if ce.Error != nil {
						cl.Error = ce.Error.Error()
					}
					trJSON.CLIExecutions = append(trJSON.CLIExecutions, cl)
				}

				// Logs
				for _, log := range tr.LogEntries {
					entry := fmt.Sprintf("[%s] %s", log.Level, log.Message)
					if log.Service != "" {
						entry = fmt.Sprintf("[%s] %s: %s", log.Level, log.Service, log.Message)
					}
					trJSON.Logs = append(trJSON.Logs, entry)
				}

				result.Results = append(result.Results, trJSON)
			}
		}
		result.DurationMs = totalDuration

		return jsonResult(result)
	}
}

// --- Helpers ---

// discoveryPaths returns the configured discovery paths from config, falling
// back to the given default path if none are configured.
func discoveryPaths(fallback string) []string {
	cfg, _ := loadConfig("")
	if cfg != nil && len(cfg.Discovery) > 0 {
		return cfg.Discovery
	}
	return []string{fallback}
}

// discoverTests finds and parses .chiperka files using discovery paths from
// config, applying filters to the test collection. If path points to a
// specific file or directory and discovery is configured, the full spec is
// loaded but tests are filtered to those under path.
func discoverTests(path string, tags []string, filter string) (*model.TestCollection, *model.ServiceTemplateCollection, error) {
	result, err := discovery.AllWithConfig(defaultConfigFile)
	if err != nil {
		return nil, nil, err
	}

	tests := result.Tests

	// If discovery is configured and a specific path was given, filter tests to that path
	cfg, _ := loadConfig("")
	if len(cfg.Discovery) > 0 && path != "." && path != "" {
		absPath, _ := filepath.Abs(path)
		info, statErr := os.Stat(path)
		isFile := statErr == nil && !info.IsDir()

		filtered := model.NewTestCollection()
		for _, suite := range tests.Suites {
			absSuite, _ := filepath.Abs(suite.FilePath)
			if isFile {
				if absSuite == absPath {
					filtered.Suites = append(filtered.Suites, suite)
				}
			} else {
				if strings.HasPrefix(absSuite, absPath+string(filepath.Separator)) || absSuite == absPath {
					filtered.Suites = append(filtered.Suites, suite)
				}
			}
		}
		tests = filtered
	}

	if len(tags) > 0 {
		tests = tests.FilterByTags(tags)
	}
	if filter != "" {
		tests = tests.FilterByName(filter)
	}

	return tests, result.Services, nil
}

// discoverServices finds .chiperka files using discovery paths from config
// and returns the collected service templates only.
func discoverServices(dir string) (*model.ServiceTemplateCollection, error) {
	paths := discoveryPaths(dir)
	files, err := finder.FindAll(paths)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return model.NewServiceTemplateCollection(), nil
	}
	p := parser.New()
	return p.ParseAll(files).Services, nil
}

// loadConfig loads configuration. Priority: per-tool override > server default > auto-discover.
func loadConfig(configFile string) (*config.Config, error) {
	if configFile != "" {
		return config.Load(configFile)
	}
	if defaultConfigFile != "" {
		return config.Load(defaultConfigFile)
	}
	cfg, _ := config.Discover()
	return cfg, nil
}

// validateTest checks a single test for validation issues.
type validationIssue struct {
	Level   string
	File    string
	Suite   string
	Test    string
	Message string
}

func validateTest(test model.Test, suite model.Suite, services *model.ServiceTemplateCollection) []validationIssue {
	var issues []validationIssue

	if test.Name == "" {
		issues = append(issues, validationIssue{
			Level:   "error",
			File:    suite.FilePath,
			Suite:   suite.Name,
			Message: "test name is empty",
		})
	}

	if len(test.Services) == 0 {
		issues = append(issues, validationIssue{
			Level:   "error",
			File:    suite.FilePath,
			Suite:   suite.Name,
			Test:    test.Name,
			Message: "no services defined",
		})
	}

	for _, svc := range test.Services {
		displayName := svc.Name
		if displayName == "" {
			displayName = svc.Ref
		}
		if displayName == "" {
			displayName = "(unnamed)"
		}

		if svc.Ref != "" {
			resolved, err := services.ResolveService(svc)
			if err != nil {
				issues = append(issues, validationIssue{
					Level:   "error",
					File:    suite.FilePath,
					Suite:   suite.Name,
					Test:    test.Name,
					Message: fmt.Sprintf("service %q: template %q not found", displayName, svc.Ref),
				})
				continue
			}
			if resolved.Image == "" {
				issues = append(issues, validationIssue{
					Level:   "error",
					File:    suite.FilePath,
					Suite:   suite.Name,
					Test:    test.Name,
					Message: fmt.Sprintf("service %q: image is empty after resolving template %q", displayName, svc.Ref),
				})
			}
		} else if svc.Image == "" {
			issues = append(issues, validationIssue{
				Level:   "error",
				File:    suite.FilePath,
				Suite:   suite.Name,
				Test:    test.Name,
				Message: fmt.Sprintf("service %q: image is empty", displayName),
			})
		}
	}

	exec := test.Execution
	switch exec.Executor {
	case model.ExecutorHTTP, "":
		if exec.Target == "" {
			issues = append(issues, validationIssue{
				Level:   "error",
				File:    suite.FilePath,
				Suite:   suite.Name,
				Test:    test.Name,
				Message: "execution target is empty",
			})
		}
		if exec.Request.Method == "" {
			issues = append(issues, validationIssue{
				Level:   "error",
				File:    suite.FilePath,
				Suite:   suite.Name,
				Test:    test.Name,
				Message: "execution request method is empty",
			})
		}
	case model.ExecutorCLI:
		if exec.CLI == nil {
			issues = append(issues, validationIssue{
				Level:   "error",
				File:    suite.FilePath,
				Suite:   suite.Name,
				Test:    test.Name,
				Message: "cli executor requires cli configuration",
			})
		} else {
			if exec.CLI.Service == "" {
				issues = append(issues, validationIssue{
					Level:   "error",
					File:    suite.FilePath,
					Suite:   suite.Name,
					Test:    test.Name,
					Message: "cli.service is empty",
				})
			}
			if exec.CLI.Command == "" {
				issues = append(issues, validationIssue{
					Level:   "error",
					File:    suite.FilePath,
					Suite:   suite.Name,
					Test:    test.Name,
					Message: "cli.command is empty",
				})
			}
		}
	default:
		issues = append(issues, validationIssue{
			Level:   "error",
			File:    suite.FilePath,
			Suite:   suite.Name,
			Test:    test.Name,
			Message: fmt.Sprintf("unknown executor type %q (must be \"http\" or \"cli\")", exec.Executor),
		})
	}

	if len(test.Assertions) == 0 {
		issues = append(issues, validationIssue{
			Level:   "warning",
			File:    suite.FilePath,
			Suite:   suite.Name,
			Test:    test.Name,
			Message: "no assertions defined",
		})
	}

	return issues
}

// parseTags splits a comma-separated tags string into a slice.
func parseTags(s string) []string {
	if s == "" {
		return nil
	}
	var tags []string
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// paginate applies offset and limit to a slice. Returns a wrapper with total count.
func paginate[T any](items []T, offset, limit int) map[string]interface{} {
	total := len(items)
	if offset > 0 {
		if offset >= len(items) {
			items = nil
		} else {
			items = items[offset:]
		}
	}
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	if items == nil {
		items = []T{}
	}
	return map[string]interface{}{
		"total": total,
		"items": items,
	}
}

// wildcardMatch does case-insensitive glob matching with * wildcards.
func wildcardMatch(name, pattern string) bool {
	pattern = strings.ToLower(pattern)
	name = strings.ToLower(name)
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return strings.Contains(name, pattern)
	}
	parts := strings.Split(pattern, "*")
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(name[pos:], part)
		if idx == -1 {
			return false
		}
		if i == 0 && !strings.HasPrefix(pattern, "*") && idx != 0 {
			return false
		}
		pos += idx + len(part)
	}
	if !strings.HasSuffix(pattern, "*") && len(parts) > 0 && parts[len(parts)-1] != "" {
		return strings.HasSuffix(name, parts[len(parts)-1])
	}
	return true
}

func filterTests(items []discovery.ListTest, pattern string) []discovery.ListTest {
	var filtered []discovery.ListTest
	for _, item := range items {
		if wildcardMatch(item.Name, pattern) || wildcardMatch(item.Suite, pattern) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterServices(items []discovery.ListService, pattern string) []discovery.ListService {
	var filtered []discovery.ListService
	for _, item := range items {
		if wildcardMatch(item.Name, pattern) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterEndpoints(items []discovery.ListEndpoint, pattern string) []discovery.ListEndpoint {
	var filtered []discovery.ListEndpoint
	for _, item := range items {
		if wildcardMatch(item.Name, pattern) || wildcardMatch(item.URL, pattern) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// jsonResult marshals v to JSON and returns it as a text result.
func jsonResult(v interface{}) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	return mcp.NewToolResultText(string(b)), nil
}
