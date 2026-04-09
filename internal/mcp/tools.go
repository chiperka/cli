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
	"chiperka-cli/internal/events"
	"chiperka-cli/internal/result"
	"chiperka-cli/internal/events/subscribers"
	"chiperka-cli/internal/finder"
	"chiperka-cli/internal/model"
	"chiperka-cli/internal/parser"
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
		mcp.WithDescription("Discover Chiperka tests and available service templates. Returns suites, tests, tags, and reusable service templates (ref: values) from chiperka.yaml config."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("path",
			mcp.Description("Directory or .chiperka file path"),
			mcp.Required(),
		),
		mcp.WithString("filter",
			mcp.Description("Name pattern filter (supports * wildcard)"),
		),
		mcp.WithString("tags",
			mcp.Description("Comma-separated tags to filter by (e.g. \"smoke,api\")"),
		),
		mcp.WithString("configuration",
			mcp.Description("Path to chiperka.yaml configuration file (auto-discovered if not set)"),
		),
	)
}

func readTool() mcp.Tool {
	return mcp.NewTool("chiperka_read",
		mcp.WithDescription("Read .chiperka test files and return parsed structured JSON. Use this to see how existing tests are written — services, setup, execution, assertions — so you can match project conventions when writing new tests."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("path",
			mcp.Description("Directory or .chiperka file path"),
			mcp.Required(),
		),
		mcp.WithString("filter",
			mcp.Description("Name pattern filter (supports * wildcard)"),
		),
		mcp.WithString("tags",
			mcp.Description("Comma-separated tags to filter by (e.g. \"smoke,api\")"),
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
		mcp.WithNumber("workers",
			mcp.Description("Number of parallel test workers (0 or omit = auto-detect from CPU count)"),
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
	path, _ := request.GetArguments()["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	filter, _ := request.GetArguments()["filter"].(string)
	tagsStr, _ := request.GetArguments()["tags"].(string)
	configFile, _ := request.GetArguments()["configuration"].(string)

	tests, err := discoverTests(path, parseTags(tagsStr), filter)
	if err != nil {
		return nil, err
	}

	cfg, err := loadConfig(configFile)
	if err != nil {
		return nil, err
	}
	services := cfg.ServiceTemplates()

	type listTest struct {
		Name     string   `json:"name"`
		Tags     []string `json:"tags,omitempty"`
		Services []string `json:"services,omitempty"`
		Executor string   `json:"executor,omitempty"`
	}
	type listSuite struct {
		Name  string     `json:"name"`
		File  string     `json:"file"`
		Tests []listTest `json:"tests"`
	}
	type templateJSON struct {
		Image        string            `json:"image"`
		HealthCheck  string            `json:"healthcheck,omitempty"`
		Environment  map[string]string `json:"environment,omitempty"`
		MaxInstances int               `json:"max_instances,omitempty"`
	}
	type listResult struct {
		Suites     []listSuite            `json:"suites"`
		TotalTests int                    `json:"total_tests"`
		TotalSuits int                    `json:"total_suites"`
		Templates  map[string]templateJSON `json:"templates,omitempty"`
	}

	result := listResult{
		TotalTests: tests.TotalTests(),
		TotalSuits: len(tests.Suites),
	}

	for _, suite := range tests.Suites {
		ls := listSuite{
			Name: suite.Name,
			File: suite.FilePath,
		}
		for _, test := range suite.Tests {
			lt := listTest{
				Name: test.Name,
				Tags: test.Tags,
			}
			for _, svc := range test.Services {
				name := svc.Name
				if name == "" {
					name = svc.Ref
				}
				if name != "" {
					lt.Services = append(lt.Services, name)
				}
			}
			executor := string(test.Execution.Executor)
			if executor == "" {
				executor = "http"
			}
			lt.Executor = executor
			ls.Tests = append(ls.Tests, lt)
		}
		result.Suites = append(result.Suites, ls)
	}

	// Include service templates so Claude knows what ref: values are available
	if services.HasTemplates() {
		result.Templates = make(map[string]templateJSON)
		for name, tmpl := range services.Templates {
			tj := templateJSON{
				Image:        tmpl.Image,
				Environment:  tmpl.Environment,
				MaxInstances: tmpl.MaxInstances,
			}
			if tmpl.HealthCheck != nil && tmpl.HealthCheck.Test != "" {
				tj.HealthCheck = string(tmpl.HealthCheck.Test)
			}
			result.Templates[name] = tj
		}
	}

	return jsonResult(result)
}

func handleRead(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, _ := request.GetArguments()["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	filter, _ := request.GetArguments()["filter"].(string)
	tagsStr, _ := request.GetArguments()["tags"].(string)

	tests, err := discoverTests(path, parseTags(tagsStr), filter)
	if err != nil {
		return nil, err
	}

	// Return the full parsed structure — suites with all test details
	type readAssertion struct {
		Response *model.ResponseAssertion `json:"response,omitempty"`
		CLI      *model.CLIAssertion      `json:"cli,omitempty"`
		Artifact *model.ArtifactAssertion `json:"artifact,omitempty"`
	}
	type readService struct {
		Name        string            `json:"name,omitempty"`
		Ref         string            `json:"ref,omitempty"`
		Image       string            `json:"image,omitempty"`
		Environment map[string]string `json:"environment,omitempty"`
		HealthCheck string            `json:"healthcheck,omitempty"`
	}
	type readSetup struct {
		Type    string `json:"type"`
		Target  string `json:"target,omitempty"`
		Method  string `json:"method,omitempty"`
		URL     string `json:"url,omitempty"`
		Service string `json:"service,omitempty"`
		Command string `json:"command,omitempty"`
	}
	type readExecution struct {
		Executor string `json:"executor"`
		Target   string `json:"target,omitempty"`
		Method   string `json:"method,omitempty"`
		URL      string `json:"url,omitempty"`
		Service  string `json:"service,omitempty"`
		Command  string `json:"command,omitempty"`
	}
	type readTest struct {
		Name       string          `json:"name"`
		Tags       []string        `json:"tags,omitempty"`
		Skipped    bool            `json:"skipped,omitempty"`
		Services   []readService   `json:"services,omitempty"`
		Setup      []readSetup     `json:"setup,omitempty"`
		Execution  readExecution   `json:"execution"`
		Assertions []readAssertion `json:"assertions,omitempty"`
		Teardown   []readSetup     `json:"teardown,omitempty"`
	}
	type readSuite struct {
		Name  string     `json:"name"`
		File  string     `json:"file"`
		Tests []readTest `json:"tests"`
	}
	type readResult struct {
		Suites     []readSuite `json:"suites"`
		TotalTests int         `json:"total_tests"`
	}

	result := readResult{
		TotalTests: tests.TotalTests(),
	}

	for _, suite := range tests.Suites {
		rs := readSuite{
			Name: suite.Name,
			File: suite.FilePath,
		}
		for _, test := range suite.Tests {
			rt := readTest{
				Name:    test.Name,
				Tags:    test.Tags,
				Skipped: test.Skipped,
			}

			// Services
			for _, svc := range test.Services {
				s := readService{
					Name:        svc.Name,
					Ref:         svc.Ref,
					Image:       svc.Image,
					Environment: svc.Environment,
				}
				if svc.HealthCheck != nil && svc.HealthCheck.Test != "" {
					s.HealthCheck = string(svc.HealthCheck.Test)
				}
				rt.Services = append(rt.Services, s)
			}

			// Setup
			for _, step := range test.Setup {
				if step.HTTP != nil {
					rt.Setup = append(rt.Setup, readSetup{
						Type:   "http",
						Target: step.HTTP.Target,
						Method: step.HTTP.Request.Method,
						URL:    step.HTTP.Request.URL,
					})
				}
				if step.CLI != nil {
					rt.Setup = append(rt.Setup, readSetup{
						Type:    "cli",
						Service: step.CLI.Service,
						Command: step.CLI.Command,
					})
				}
			}

			// Teardown
			for _, step := range test.Teardown {
				if step.HTTP != nil {
					rt.Teardown = append(rt.Teardown, readSetup{
						Type:   "http",
						Target: step.HTTP.Target,
						Method: step.HTTP.Request.Method,
						URL:    step.HTTP.Request.URL,
					})
				}
				if step.CLI != nil {
					rt.Teardown = append(rt.Teardown, readSetup{
						Type:    "cli",
						Service: step.CLI.Service,
						Command: step.CLI.Command,
					})
				}
			}

			// Execution
			executor := string(test.Execution.Executor)
			if executor == "" {
				executor = "http"
			}
			rt.Execution = readExecution{Executor: executor}
			if executor == "http" {
				rt.Execution.Target = test.Execution.Target
				rt.Execution.Method = test.Execution.Request.Method
				rt.Execution.URL = test.Execution.Request.URL
			} else if test.Execution.CLI != nil {
				rt.Execution.Service = test.Execution.CLI.Service
				rt.Execution.Command = test.Execution.CLI.Command
			}

			// Assertions
			for _, a := range test.Assertions {
				rt.Assertions = append(rt.Assertions, readAssertion{
					Response: a.Response,
					CLI:      a.CLI,
					Artifact: a.Artifact,
				})
			}

			rs.Tests = append(rs.Tests, rt)
		}
		result.Suites = append(result.Suites, rs)
	}

	return jsonResult(result)
}

func handleValidate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, _ := request.GetArguments()["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	filter, _ := request.GetArguments()["filter"].(string)
	configFile, _ := request.GetArguments()["configuration"].(string)

	cfg, err := loadConfig(configFile)
	if err != nil {
		return nil, err
	}
	services := cfg.ServiceTemplates()

	tests, err := discoverTests(path, nil, filter)
	if err != nil {
		return nil, err
	}

	type issue struct {
		Level   string `json:"level"`
		File    string `json:"file"`
		Suite   string `json:"suite,omitempty"`
		Test    string `json:"test,omitempty"`
		Message string `json:"message"`
	}
	type summary struct {
		Files    int `json:"files"`
		Suites   int `json:"suites"`
		Tests    int `json:"tests"`
		Errors   int `json:"errors"`
		Warnings int `json:"warnings"`
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
		Files:    len(fileSet),
		Suites:   len(tests.Suites),
		Tests:    totalTests,
		Errors:   errors,
		Warnings: warnings,
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
		requestedWorkers := 0
		if w, ok := request.GetArguments()["workers"].(float64); ok && w > 0 {
			requestedWorkers = int(w)
		}

		cfg, err := loadConfig(configFile)
		if err != nil {
			return nil, err
		}
		services := cfg.ServiceTemplates()

		tests, err := discoverTests(path, nil, filter)
		if err != nil {
			return nil, err
		}

		if tests.TotalTests() == 0 {
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

		workerCount := requestedWorkers
		if workerCount <= 0 {
			workerCount = runtime.NumCPU()
			if workerCount < 1 {
				workerCount = 1
			}
		}

		// Generate run UUID upfront so artifacts are written directly into the result tree
		runUUID := result.NewRunUUID()
		runDir := filepath.Join(".chiperka", "results", "runs", runUUID)
		os.MkdirAll(runDir, 0755)

		r, err := runner.New(bus, workerCount, runDir, services, regenerateSnapshots, timeout, version, collector, 0, cfg.ExecutionVariables)
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
			WorkerCount:      workerCount,
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
		services := cfg.ServiceTemplates()

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

		workerCount := runtime.NumCPU()
		if workerCount < 1 {
			workerCount = 1
		}

		r, err := runner.New(bus, workerCount, os.TempDir(), services, false, timeout, version, collector, 0, cfg.ExecutionVariables)
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
			WorkerCount:      workerCount,
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

// discoverTests finds and parses test files, applying filters.
func discoverTests(path string, tags []string, filter string) (*model.TestCollection, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("path does not exist: %s", path)
	}

	var files []string
	if !info.IsDir() && strings.HasSuffix(path, ".chiperka") {
		files = []string{path}
	} else {
		f := finder.New(path)
		files, err = f.FindTestFiles()
		if err != nil {
			return nil, fmt.Errorf("failed to find test files: %w", err)
		}
	}

	if len(files) == 0 {
		return model.NewTestCollection(), nil
	}

	p := parser.New()
	parseResult := p.ParseAll(files)

	tests := parseResult.Tests
	if len(tags) > 0 {
		tests = tests.FilterByTags(tags)
	}
	if filter != "" {
		tests = tests.FilterByName(filter)
	}

	return tests, nil
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

// jsonResult marshals v to JSON and returns it as a text result.
func jsonResult(v interface{}) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	return mcp.NewToolResultText(string(b)), nil
}
