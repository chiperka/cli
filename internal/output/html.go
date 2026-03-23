// Package output provides test result formatters.
package output

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"spark-cli/internal/model"
)

// HTMLWriter writes test results as a static HTML report.
type HTMLWriter struct {
	startTime     time.Time
	testTemplate  *template.Template
	dashTemplate  *template.Template
}

// NewHTMLWriter creates a new HTML report writer.
func NewHTMLWriter() *HTMLWriter {
	w := &HTMLWriter{
		startTime: time.Now(),
	}
	// Pre-parse templates so they are safe for concurrent Execute() calls.
	w.testTemplate = template.Must(template.New("testPage").Parse(testPageTemplate))
	w.dashTemplate = template.Must(template.New("dashboard").Parse(dashboardTemplate))
	return w
}

// HTMLTestData holds test data for the HTML template.
type HTMLTestData struct {
	ID                string
	UUID              string
	Name              string
	Description       string
	Tags              []string
	Status            string
	StatusClass       string
	Duration          string
	ExecutionDuration string
	Error             string
	SuiteName         string
	SuiteFile         string
	Assertions        []HTMLAssertionData
	Services          []HTMLServiceData
	SetupSteps        []HTMLSetupStepData
	TeardownSteps     []HTMLSetupStepData
	Artifacts         []HTMLArtifactData
	Execution         HTMLExecutionData
	Result            HTMLResultData
	CLIResult         HTMLCLIResultData
	// Phase durations
	NetworkDuration   string
	ServicesDuration  string
	SetupDuration     string
	TeardownDuration  string
	AssertionDuration string
	CleanupDuration   string
	// Phase breakdown data (for timeline bar)
	Phases []HTMLPhaseData
	// Log entries
	LogEntries []HTMLLogEntry
	// Generated timestamp
	GeneratedAt string
	// Full HTTP exchanges (inline request/response)
	HTTPExchanges []HTMLHTTPExchangeData
	// Full CLI executions (inline command/output)
	CLIExecs []HTMLCLIExecData
}

// HTMLPhaseData holds data for a single phase in the timeline bar.
type HTMLPhaseData struct {
	Name       string
	Duration   string
	Percent    float64
	ColorClass string
}

// HTMLLogEntry holds a log entry for the HTML template.
type HTMLLogEntry struct {
	Time    string
	Level   string
	Action  string
	Service string
	Message string
}

// HTMLAssertionData holds assertion data for the HTML template.
type HTMLAssertionData struct {
	Type        string // "statusCode", "exitCode", "snapshot", etc.
	Message     string
	Passed      bool
	StatusClass string
	Expected    string
	Actual      string
	Duration    string
}

// HTMLServiceData holds service data for the HTML template.
type HTMLServiceData struct {
	Name     string
	Image    string
	Duration string
	// Phase breakdown
	ImageResolveDuration   string
	ContainerStartDuration string
	HealthCheckDuration    string
	HasPhaseBreakdown      bool
	// Service definition details
	Command    string
	WorkingDir string
	Environment    []HTMLEnvVarData
	HasEnvironment bool
	HealthCheckTest     string
	HealthCheckInterval string
	HealthCheckTimeout  string
	HealthCheckRetries  int
	HasHealthCheck      bool
}

// HTMLSetupStepData holds setup step data for the HTML template.
type HTMLSetupStepData struct {
	Type           string // "cli" or "http"
	Duration       string
	Success        bool
	StatusClass    string
	Error          string
	// CLI-specific
	CLIService    string
	CLICommand    string
	CLIWorkingDir string
	CLIExitCode   int
	// HTTP-specific
	HTTPTarget     string
	HTTPMethod     string
	HTTPMethodClass string
	HTTPURL        string
	HTTPStatusCode int
	// Inline exchange/exec data
	HTTPExchange *HTMLHTTPExchangeData
	CLIExec      *HTMLCLIExecData
}

// HTMLArtifactData holds artifact data for the HTML template.
type HTMLArtifactData struct {
	Name string
	Path string
	Size string
}

// HTMLExecutionData holds execution data for the HTML template.
type HTMLExecutionData struct {
	Executor    string
	Target      string
	Method      string
	MethodClass string
	URL         string
	Headers     []HTMLHeaderData
	Body        string
	// CLI-specific fields
	CLIService    string
	CLICommand    string
	CLIWorkingDir string
}

// HTMLHeaderData holds a single HTTP header for the HTML template.
type HTMLHeaderData struct {
	Name   string
	Values string
}

// HTMLResultData holds HTTP response result data for the HTML template.
type HTMLResultData struct {
	HasResponse  bool
	StatusCode   int
	StatusClass  string
	Headers      []HTMLHeaderData
	BodyArtifact *HTMLArtifactData
}

// HTMLCLIResultData holds CLI command result data for the HTML template.
type HTMLCLIResultData struct {
	HasResponse    bool
	ExitCode       int
	ExitCodeClass  string
	StdoutArtifact *HTMLArtifactData
	StderrArtifact *HTMLArtifactData
}

// HTMLHTTPExchangeData holds a full HTTP request/response exchange for inline display.
type HTMLHTTPExchangeData struct {
	Phase              string
	PhaseSeq           int
	RequestMethod      string
	RequestMethodClass string
	RequestURL         string
	RequestHeaders     []HTMLHeaderData
	RequestBody        string
	ResponseStatusCode int
	ResponseStatusClass string
	ResponseHeaders    []HTMLHeaderData
	ResponseBody       string
	Duration           string
	Error              string
}

// HTMLCLIExecData holds a full CLI execution for inline display.
type HTMLCLIExecData struct {
	Phase         string
	PhaseSeq      int
	Service       string
	Command       string
	WorkingDir    string
	ExitCode      int
	ExitCodeClass string
	Stdout        string
	Stderr        string
	Duration      string
	Error         string
}

// HTMLEnvVarData holds an environment variable for service detail display.
type HTMLEnvVarData struct {
	Key   string
	Value string
}

// formatFileSize returns a human-readable size string.
func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// methodClassFor returns the CSS class for an HTTP method badge.
func methodClassFor(method string) string {
	switch strings.ToUpper(method) {
	case "GET":
		return "method-get"
	case "POST":
		return "method-post"
	case "PUT":
		return "method-put"
	case "PATCH":
		return "method-patch"
	case "DELETE":
		return "method-delete"
	default:
		return "method-other"
	}
}

// statusClassFor returns the CSS class for an HTTP status code badge.
func statusClassFor(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "success"
	case code >= 300 && code < 400:
		return "redirect"
	case code >= 400 && code < 500:
		return "client-error"
	default:
		return "server-error"
	}
}

// transformArtifactPath converts an artifact path to a relative path from the HTML report location.
func transformArtifactPath(artifactPath, htmlFilePath string) string {
	if artifactPath == "" || htmlFilePath == "" {
		return artifactPath
	}

	// Get absolute paths
	absArtifact, err := filepath.Abs(artifactPath)
	if err != nil {
		return artifactPath
	}
	absHTML, err := filepath.Abs(htmlFilePath)
	if err != nil {
		return artifactPath
	}

	// Get the directory containing the HTML file
	htmlDir := filepath.Dir(absHTML)

	// Calculate relative path from HTML directory to artifact
	relPath, err := filepath.Rel(htmlDir, absArtifact)
	if err != nil {
		return artifactPath
	}

	return relPath
}


// transformTestResult converts a model.TestResult into HTMLTestData.
// htmlFilePath is used to compute relative artifact paths.
func transformTestResult(testResult model.TestResult, testID int, suiteName, suiteFilePath, htmlFilePath string) HTMLTestData {
	testData := HTMLTestData{
		ID:                fmt.Sprintf("test-%d", testID),
		UUID:              testResult.UUID,
		Name:              testResult.Test.Name,
		Description:       testResult.Test.Description,
		Tags:              testResult.Test.Tags,
		Duration:          fmt.Sprintf("%.3fs", testResult.Duration.Seconds()),
		ExecutionDuration: fmt.Sprintf("%.3fs", testResult.ExecutionDuration.Seconds()),
		SuiteName:         suiteName,
		SuiteFile:         suiteFilePath,
		NetworkDuration:   fmt.Sprintf("%.3fs", testResult.NetworkDuration.Seconds()),
		ServicesDuration:  fmt.Sprintf("%.3fs", testResult.ServicesDuration.Seconds()),
		SetupDuration:     fmt.Sprintf("%.3fs", testResult.SetupDuration.Seconds()),
		TeardownDuration:  fmt.Sprintf("%.3fs", testResult.TeardownDuration.Seconds()),
		AssertionDuration: fmt.Sprintf("%.3fs", testResult.AssertionDuration.Seconds()),
		CleanupDuration:   fmt.Sprintf("%.3fs", testResult.CleanupDuration.Seconds()),
	}

	// Build phase breakdown for timeline bar
	totalMs := testResult.Duration.Milliseconds()
	if totalMs > 0 {
		type phaseInfo struct {
			name  string
			dur   time.Duration
			color string
		}
		phases := []phaseInfo{
			{"Network", testResult.NetworkDuration, "phase-network"},
			{"Services", testResult.ServicesDuration, "phase-services"},
			{"Setup", testResult.SetupDuration, "phase-setup"},
			{"Execution", testResult.ExecutionDuration, "phase-execution"},
			{"Teardown", testResult.TeardownDuration, "phase-teardown"},
			{"Assertions", testResult.AssertionDuration, "phase-assertions"},
			{"Cleanup", testResult.CleanupDuration, "phase-cleanup"},
		}
		for _, p := range phases {
			if p.dur > 0 {
				pct := float64(p.dur.Milliseconds()) * 100.0 / float64(totalMs)
				if pct < 0.5 {
					pct = 0.5 // minimum visible width
				}
				testData.Phases = append(testData.Phases, HTMLPhaseData{
					Name:       p.name,
					Duration:   fmt.Sprintf("%.3fs", p.dur.Seconds()),
					Percent:    pct,
					ColorClass: p.color,
				})
			}
		}
	}

	// Log entries
	for _, entry := range testResult.LogEntries {
		testData.LogEntries = append(testData.LogEntries, HTMLLogEntry{
			Time:    entry.RelativeTime,
			Level:   entry.Level,
			Action:  entry.Action,
			Service: entry.Service,
			Message: entry.Message,
		})
	}

	// Generated timestamp
	testData.GeneratedAt = time.Now().Format("2006-01-02 15:04:05")

	// Execution info
	method := testResult.Test.Execution.Request.Method
	execBody := testResult.Test.Execution.Request.Body.DisplayString()
	testData.Execution = HTMLExecutionData{
		Executor:    string(testResult.Test.Execution.Executor),
		Target:      testResult.Test.Execution.Target,
		Method:      method,
		MethodClass: methodClassFor(method),
		URL:         testResult.Test.Execution.Request.URL,
		Body:        execBody,
	}

	// CLI execution info
	if testResult.Test.Execution.CLI != nil {
		testData.Execution.CLIService = testResult.Test.Execution.CLI.Service
		testData.Execution.CLICommand = testResult.Test.Execution.CLI.Command
		testData.Execution.CLIWorkingDir = testResult.Test.Execution.CLI.WorkingDir
	}

	// Request headers
	for name, value := range testResult.Test.Execution.Request.Headers {
		testData.Execution.Headers = append(testData.Execution.Headers, HTMLHeaderData{
			Name:   name,
			Values: value,
		})
	}

	// Map HTTP exchanges for inline display
	for _, ex := range testResult.HTTPExchanges {
		hd := HTMLHTTPExchangeData{
			Phase:              ex.Phase,
			PhaseSeq:           ex.PhaseSeq,
			RequestMethod:      ex.RequestMethod,
			RequestMethodClass: methodClassFor(ex.RequestMethod),
			RequestURL:         ex.RequestURL,
			RequestBody:        ex.RequestBody,
			ResponseStatusCode: ex.ResponseStatusCode,
			ResponseStatusClass: statusClassFor(ex.ResponseStatusCode),
			ResponseBody:       ex.ResponseBody,
			Duration:           fmt.Sprintf("%.3fs", ex.Duration.Seconds()),
		}
		if ex.Error != nil {
			hd.Error = ex.Error.Error()
		}
		for k, v := range ex.RequestHeaders {
			hd.RequestHeaders = append(hd.RequestHeaders, HTMLHeaderData{Name: k, Values: v})
		}
		for k, vals := range ex.ResponseHeaders {
			hd.ResponseHeaders = append(hd.ResponseHeaders, HTMLHeaderData{Name: k, Values: strings.Join(vals, ", ")})
		}
		testData.HTTPExchanges = append(testData.HTTPExchanges, hd)
	}

	// Map CLI executions for inline display
	for _, ce := range testResult.CLIExecutions {
		cd := HTMLCLIExecData{
			Phase:      ce.Phase,
			PhaseSeq:   ce.PhaseSeq,
			Service:    ce.Service,
			Command:    ce.Command,
			WorkingDir: ce.WorkingDir,
			ExitCode:   ce.ExitCode,
			Stdout:     ce.Stdout,
			Stderr:     ce.Stderr,
			Duration:   fmt.Sprintf("%.3fs", ce.Duration.Seconds()),
		}
		if ce.ExitCode == 0 {
			cd.ExitCodeClass = "success"
		} else {
			cd.ExitCodeClass = "error"
		}
		if ce.Error != nil {
			cd.Error = ce.Error.Error()
		}
		testData.CLIExecs = append(testData.CLIExecs, cd)
	}

	// Build service name -> definition lookup for enriching service data
	svcDefMap := make(map[string]model.Service)
	for _, svc := range testResult.Test.Services {
		svcDefMap[svc.Name] = svc
	}

	// Services (use ServiceResults if available for duration info)
	if len(testResult.ServiceResults) > 0 {
		for _, svc := range testResult.ServiceResults {
			sd := HTMLServiceData{
				Name:     svc.Name,
				Image:    svc.Image,
				Duration: fmt.Sprintf("%.3fs", svc.Duration.Seconds()),
			}
			if svc.ImageResolveDuration > 0 {
				sd.ImageResolveDuration = fmt.Sprintf("%.3fs", svc.ImageResolveDuration.Seconds())
				sd.HasPhaseBreakdown = true
			}
			if svc.ContainerStartDuration > 0 {
				sd.ContainerStartDuration = fmt.Sprintf("%.3fs", svc.ContainerStartDuration.Seconds())
				sd.HasPhaseBreakdown = true
			}
			if svc.HealthCheckDuration > 0 {
				sd.HealthCheckDuration = fmt.Sprintf("%.3fs", svc.HealthCheckDuration.Seconds())
				sd.HasPhaseBreakdown = true
			}
			// Enrich with definition details
			if def, ok := svcDefMap[svc.Name]; ok {
				if len(def.Command) > 0 {
					sd.Command = strings.Join([]string(def.Command), " ")
				}
				sd.WorkingDir = def.WorkingDir
				if len(def.Environment) > 0 {
					sd.HasEnvironment = true
					keys := make([]string, 0, len(def.Environment))
					for k := range def.Environment {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					for _, k := range keys {
						sd.Environment = append(sd.Environment, HTMLEnvVarData{Key: k, Value: def.Environment[k]})
					}
				}
				if def.HealthCheck != nil {
					sd.HasHealthCheck = true
					sd.HealthCheckTest = string(def.HealthCheck.Test)
					sd.HealthCheckInterval = def.HealthCheck.Interval
					sd.HealthCheckTimeout = def.HealthCheck.Timeout
					sd.HealthCheckRetries = def.HealthCheck.Retries
				}
			}
			testData.Services = append(testData.Services, sd)
		}
	} else {
		for _, svc := range testResult.Test.Services {
			sd := HTMLServiceData{
				Name:  svc.Name,
				Image: svc.Image,
			}
			if len(svc.Command) > 0 {
				sd.Command = strings.Join([]string(svc.Command), " ")
			}
			sd.WorkingDir = svc.WorkingDir
			if len(svc.Environment) > 0 {
				sd.HasEnvironment = true
				keys := make([]string, 0, len(svc.Environment))
				for k := range svc.Environment {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					sd.Environment = append(sd.Environment, HTMLEnvVarData{Key: k, Value: svc.Environment[k]})
				}
			}
			if svc.HealthCheck != nil {
				sd.HasHealthCheck = true
				sd.HealthCheckTest = string(svc.HealthCheck.Test)
				sd.HealthCheckInterval = svc.HealthCheck.Interval
				sd.HealthCheckTimeout = svc.HealthCheck.Timeout
				sd.HealthCheckRetries = svc.HealthCheck.Retries
			}
			testData.Services = append(testData.Services, sd)
		}
	}

	// Setup steps
	for i, setupResult := range testResult.SetupResults {
		setupData := HTMLSetupStepData{
			Type:     setupResult.Type,
			Duration: fmt.Sprintf("%.3fs", setupResult.Duration.Seconds()),
			Success:  setupResult.Success,
		}
		if setupResult.Success {
			setupData.StatusClass = "passed"
		} else {
			setupData.StatusClass = "failed"
			if setupResult.Error != nil {
				setupData.Error = setupResult.Error.Error()
			}
		}
		if i < len(testResult.Test.Setup) {
			setupDef := testResult.Test.Setup[i]
			if setupDef.CLI != nil {
				setupData.CLIService = setupDef.CLI.Service
				setupData.CLICommand = setupDef.CLI.Command
				setupData.CLIWorkingDir = setupDef.CLI.WorkingDir
				setupData.CLIExitCode = setupResult.CLIExitCode
			} else if setupDef.HTTP != nil {
				setupData.HTTPTarget = setupDef.HTTP.Target
				setupData.HTTPMethod = setupDef.HTTP.Request.Method
				setupData.HTTPURL = setupDef.HTTP.Request.URL
				setupData.HTTPStatusCode = setupResult.HTTPStatusCode
				setupData.HTTPMethodClass = methodClassFor(setupDef.HTTP.Request.Method)
			}
		}
		// Cross-reference with exchanges/execs for inline data
		for idx := range testData.HTTPExchanges {
			if testData.HTTPExchanges[idx].Phase == "setup" && testData.HTTPExchanges[idx].PhaseSeq == i {
				setupData.HTTPExchange = &testData.HTTPExchanges[idx]
				break
			}
		}
		for idx := range testData.CLIExecs {
			if testData.CLIExecs[idx].Phase == "setup" && testData.CLIExecs[idx].PhaseSeq == i {
				setupData.CLIExec = &testData.CLIExecs[idx]
				break
			}
		}
		testData.SetupSteps = append(testData.SetupSteps, setupData)
	}

	// Teardown steps
	for i, teardownResult := range testResult.TeardownResults {
		teardownData := HTMLSetupStepData{
			Type:     teardownResult.Type,
			Duration: fmt.Sprintf("%.3fs", teardownResult.Duration.Seconds()),
			Success:  teardownResult.Success,
		}
		if teardownResult.Success {
			teardownData.StatusClass = "passed"
		} else {
			teardownData.StatusClass = "failed"
			if teardownResult.Error != nil {
				teardownData.Error = teardownResult.Error.Error()
			}
		}
		if i < len(testResult.Test.Teardown) {
			teardownDef := testResult.Test.Teardown[i]
			if teardownDef.CLI != nil {
				teardownData.CLIService = teardownDef.CLI.Service
				teardownData.CLICommand = teardownDef.CLI.Command
				teardownData.CLIWorkingDir = teardownDef.CLI.WorkingDir
				teardownData.CLIExitCode = teardownResult.CLIExitCode
			} else if teardownDef.HTTP != nil {
				teardownData.HTTPTarget = teardownDef.HTTP.Target
				teardownData.HTTPMethod = teardownDef.HTTP.Request.Method
				teardownData.HTTPURL = teardownDef.HTTP.Request.URL
				teardownData.HTTPStatusCode = teardownResult.HTTPStatusCode
				teardownData.HTTPMethodClass = methodClassFor(teardownDef.HTTP.Request.Method)
			}
		}
		// Cross-reference with exchanges/execs for inline data
		for idx := range testData.HTTPExchanges {
			if testData.HTTPExchanges[idx].Phase == "teardown" && testData.HTTPExchanges[idx].PhaseSeq == i {
				teardownData.HTTPExchange = &testData.HTTPExchanges[idx]
				break
			}
		}
		for idx := range testData.CLIExecs {
			if testData.CLIExecs[idx].Phase == "teardown" && testData.CLIExecs[idx].PhaseSeq == i {
				teardownData.CLIExec = &testData.CLIExecs[idx]
				break
			}
		}
		testData.TeardownSteps = append(testData.TeardownSteps, teardownData)
	}

	// Artifacts
	for _, art := range testResult.Artifacts {
		testData.Artifacts = append(testData.Artifacts, HTMLArtifactData{
			Name: art.Name,
			Path: transformArtifactPath(art.Path, htmlFilePath),
			Size: formatFileSize(art.Size),
		})
	}

	// HTTP Response Result
	if testResult.HTTPResponse != nil {
		testData.Result.HasResponse = true
		testData.Result.StatusCode = testResult.HTTPResponse.StatusCode
		testData.Result.StatusClass = statusClassFor(testResult.HTTPResponse.StatusCode)
		for name, values := range testResult.HTTPResponse.Headers {
			testData.Result.Headers = append(testData.Result.Headers, HTMLHeaderData{
				Name:   name,
				Values: strings.Join(values, ", "),
			})
		}
		if testResult.HTTPResponse.BodyArtifact != nil {
			testData.Result.BodyArtifact = &HTMLArtifactData{
				Name: testResult.HTTPResponse.BodyArtifact.Name,
				Path: transformArtifactPath(testResult.HTTPResponse.BodyArtifact.Path, htmlFilePath),
				Size: formatFileSize(testResult.HTTPResponse.BodyArtifact.Size),
			}
		}
	}

	// CLI Response Result
	if testResult.CLIResponse != nil {
		testData.CLIResult.HasResponse = true
		testData.CLIResult.ExitCode = testResult.CLIResponse.ExitCode
		if testResult.CLIResponse.ExitCode == 0 {
			testData.CLIResult.ExitCodeClass = "success"
		} else {
			testData.CLIResult.ExitCodeClass = "error"
		}
		if testResult.CLIResponse.StdoutArtifact != nil {
			testData.CLIResult.StdoutArtifact = &HTMLArtifactData{
				Name: testResult.CLIResponse.StdoutArtifact.Name,
				Path: transformArtifactPath(testResult.CLIResponse.StdoutArtifact.Path, htmlFilePath),
				Size: formatFileSize(testResult.CLIResponse.StdoutArtifact.Size),
			}
		}
		if testResult.CLIResponse.StderrArtifact != nil {
			testData.CLIResult.StderrArtifact = &HTMLArtifactData{
				Name: testResult.CLIResponse.StderrArtifact.Name,
				Path: transformArtifactPath(testResult.CLIResponse.StderrArtifact.Path, htmlFilePath),
				Size: formatFileSize(testResult.CLIResponse.StderrArtifact.Size),
			}
		}
	}

	// Status
	switch testResult.Status {
	case model.StatusPassed:
		testData.Status = "PASSED"
		testData.StatusClass = "passed"
	case model.StatusFailed:
		testData.Status = "FAILED"
		testData.StatusClass = "failed"
	case model.StatusError:
		testData.Status = "ERROR"
		testData.StatusClass = "error"
		if testResult.Error != nil {
			testData.Error = testResult.Error.Error()
		}
	case model.StatusSkipped:
		testData.Status = "SKIPPED"
		testData.StatusClass = "skipped"
	}

	// Assertions
	for _, ar := range testResult.AssertionResults {
		assertionData := HTMLAssertionData{
			Type:     ar.Type,
			Message:  ar.Message,
			Passed:   ar.Passed,
			Expected: ar.Expected,
			Actual:   ar.Actual,
			Duration: fmt.Sprintf("%.3fs", ar.Duration.Seconds()),
		}
		if ar.Passed {
			assertionData.StatusClass = "passed"
		} else {
			assertionData.StatusClass = "failed"
		}
		testData.Assertions = append(testData.Assertions, assertionData)
	}

	return testData
}

// WriteTestReport writes a standalone HTML report for a single test result.
// It creates <outputDir>/<uuid>/index.html and returns the path to the file.
func (w *HTMLWriter) WriteTestReport(testResult *model.TestResult, suiteName, suiteFilePath, outputDir string) (string, error) {
	if testResult.UUID == "" {
		return "", fmt.Errorf("test result has no UUID")
	}

	// Create directory for this test
	testDir := filepath.Join(outputDir, testResult.UUID)
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create test report directory: %w", err)
	}

	filePath := filepath.Join(testDir, "index.html")

	// Transform result into template data
	testData := transformTestResult(*testResult, 1, suiteName, suiteFilePath, filePath)

	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create test report file: %w", err)
	}
	defer file.Close()

	if err := w.testTemplate.Execute(file, testData); err != nil {
		return "", fmt.Errorf("failed to render test report: %w", err)
	}

	return filePath, nil
}

// HTMLDashboardData holds all data needed for the dashboard template.
type HTMLDashboardData struct {
	GeneratedAt  string
	Duration     string
	Version      string
	TotalTests   int
	PassedTests  int
	FailedTests  int
	ErrorTests   int
	SkippedTests int
	PassRate     float64
	Suites       []HTMLDashboardSuiteData
}

// HTMLDashboardSuiteData holds suite data for the dashboard template.
type HTMLDashboardSuiteData struct {
	Name     string
	FilePath string
	Tests    []HTMLDashboardTestData
	Passed   int
	Failed   int
	Skipped  int
	Total    int
}

// HTMLDashboardTestData holds minimal test data for the dashboard link list.
type HTMLDashboardTestData struct {
	UUID        string
	Name        string
	Tags        []string
	Status      string
	StatusClass string
	Duration    string
	HasLink     bool // false for skipped tests (no per-test HTML)
}

// WriteDashboard writes the dashboard index.html with links to per-test reports.
func (w *HTMLWriter) WriteDashboard(result *model.RunResult, outputDir, version string) error {
	totalTime := time.Since(w.startTime)

	data := HTMLDashboardData{
		GeneratedAt:  time.Now().Format("2006-01-02 15:04:05"),
		Duration:     fmt.Sprintf("%.2fs", totalTime.Seconds()),
		Version:      version,
		TotalTests:   result.TotalTests(),
		PassedTests:  result.TotalPassed(),
		FailedTests:  result.TotalFailed(),
		ErrorTests:   result.TotalErrors(),
		SkippedTests: result.TotalSkipped(),
	}

	runTests := data.TotalTests - data.SkippedTests
	if runTests > 0 {
		data.PassRate = float64(data.PassedTests) / float64(runTests) * 100
	}

	for _, suiteResult := range result.SuiteResults {
		suiteData := HTMLDashboardSuiteData{
			Name:     suiteResult.Suite.Name,
			FilePath: suiteResult.Suite.FilePath,
			Total:    len(suiteResult.TestResults),
		}

		for _, testResult := range suiteResult.TestResults {
			td := HTMLDashboardTestData{
				UUID:     testResult.UUID,
				Name:     testResult.Test.Name,
				Tags:     testResult.Test.Tags,
				Duration: fmt.Sprintf("%.3fs", testResult.Duration.Seconds()),
				HasLink:  testResult.Status != model.StatusSkipped,
			}

			switch testResult.Status {
			case model.StatusPassed:
				td.Status = "PASSED"
				td.StatusClass = "passed"
				suiteData.Passed++
			case model.StatusFailed:
				td.Status = "FAILED"
				td.StatusClass = "failed"
				suiteData.Failed++
			case model.StatusError:
				td.Status = "ERROR"
				td.StatusClass = "error"
				suiteData.Failed++
			case model.StatusSkipped:
				td.Status = "SKIPPED"
				td.StatusClass = "skipped"
				suiteData.Skipped++
			}

			suiteData.Tests = append(suiteData.Tests, td)
		}

		data.Suites = append(data.Suites, suiteData)
	}

	filePath := filepath.Join(outputDir, "index.html")
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create dashboard file: %w", err)
	}
	defer file.Close()

	if err := w.dashTemplate.Execute(file, data); err != nil {
		return fmt.Errorf("failed to render dashboard: %w", err)
	}

	return nil
}


// testPageTemplate is a standalone HTML page for a single test result.
// Used by WriteTestReport to create per-test HTML files.
const testPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}} - Spark Test Report</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=DM+Sans:wght@400;500;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-base: #030712;
            --bg-card: #111827;
            --bg-elevated: #1f2937;
            --bg-code: #0a0a0a;
            --border: #1f2937;
            --border-subtle: #374151;
            --text-heading: #ffffff;
            --text-body: #d1d5db;
            --text-secondary: #9ca3af;
            --text-muted: #6b7280;
            --accent: #f97316;
            --accent-light: #fb923c;
            --accent-bg: rgba(249, 115, 22, 0.1);
            --accent-border: rgba(249, 115, 22, 0.25);
            --pass: #4ade80;
            --pass-bg: rgba(74, 222, 128, 0.1);
            --fail: #f87171;
            --fail-bg: rgba(248, 113, 113, 0.1);
            --error-color: #fbbf24;
            --error-bg: rgba(251, 191, 36, 0.1);
            --skip: #64748b;
            --skip-bg: rgba(100, 116, 139, 0.1);
            --font-sans: 'DM Sans', ui-sans-serif, system-ui, sans-serif;
            --font-mono: 'JetBrains Mono', ui-monospace, monospace;
        }
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: var(--font-sans); background: var(--bg-base); color: var(--text-body); line-height: 1.6; min-height: 100vh; -webkit-font-smoothing: antialiased; }
        .container { max-width: 960px; margin: 0 auto; padding: 3rem 2rem; }
        h2 { font-size: 1.5rem; margin-bottom: 1rem; color: var(--text-heading); font-weight: 700; }
        h3 { font-size: 1.125rem; margin-bottom: 0.75rem; color: var(--text-heading); font-weight: 700; }
        code, pre { font-family: var(--font-mono); }
        a { color: var(--accent-light); text-decoration: none; }
        a:hover { text-decoration: underline; }
        /* Header */
        .header { border-bottom: 1px solid var(--border); padding-bottom: 2rem; margin-bottom: 2rem; }
        .brand { display: flex; align-items: center; gap: 0.5rem; margin-bottom: 1.5rem; }
        .brand-icon { width: 28px; height: 28px; background: var(--accent); border-radius: 6px; display: flex; align-items: center; justify-content: center; }
        .brand-icon svg { width: 16px; height: 16px; fill: white; }
        .brand-text { font-size: 0.875rem; font-weight: 700; color: var(--text-secondary); text-transform: uppercase; letter-spacing: 0.08em; }
        .back-link { margin-left: auto; font-size: 0.8rem; font-weight: 500; color: var(--text-muted); }
        .back-link:hover { color: var(--accent-light); }
        .test-title { display: flex; align-items: center; gap: 1rem; flex-wrap: wrap; margin-bottom: 0.75rem; }
        .test-title h1 { font-size: 1.75rem; font-weight: 700; color: var(--text-heading); line-height: 1.3; }
        .status-badge { display: inline-block; padding: 0.2rem 0.75rem; border-radius: 9999px; font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; }
        .status-badge.passed { background: var(--pass-bg); color: var(--pass); border: 1px solid rgba(74,222,128,0.2); }
        .status-badge.failed { background: var(--fail-bg); color: var(--fail); border: 1px solid rgba(248,113,113,0.2); }
        .status-badge.error { background: var(--error-bg); color: var(--error-color); border: 1px solid rgba(251,191,36,0.2); }
        .status-badge.skipped { background: var(--skip-bg); color: var(--skip); border: 1px solid rgba(100,116,139,0.2); }
        .description { color: var(--text-secondary); margin-bottom: 1rem; line-height: 1.7; }
        .tags { display: flex; flex-wrap: wrap; gap: 0.4rem; margin-bottom: 1rem; }
        .tag { display: inline-block; padding: 0.2rem 0.6rem; border-radius: 9999px; font-size: 0.7rem; font-weight: 500; background: var(--accent-bg); color: var(--accent-light); border: 1px solid var(--accent-border); }
        .meta-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 0.75rem; }
        .meta-item { display: flex; flex-direction: column; gap: 0.15rem; }
        .meta-label { font-size: 0.7rem; font-weight: 500; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.06em; }
        .meta-value { font-size: 0.875rem; color: var(--text-body); font-family: var(--font-mono); }
        .error-box { background: var(--fail-bg); border: 1px solid rgba(248,113,113,0.25); border-radius: 8px; padding: 1rem 1.25rem; font-family: var(--font-mono); font-size: 0.8rem; color: var(--fail); white-space: pre-wrap; word-break: break-word; margin-bottom: 1.5rem; }
        .section { background: var(--bg-card); border: 1px solid var(--border); border-radius: 12px; margin-bottom: 1.25rem; overflow: hidden; }
        .section > summary { display: flex; align-items: center; gap: 0.75rem; padding: 1rem 1.25rem; cursor: pointer; user-select: none; list-style: none; font-weight: 700; font-size: 1rem; color: var(--text-heading); }
        .section > summary::-webkit-details-marker { display: none; }
        .section > summary::before { content: ''; display: inline-block; width: 0; height: 0; border-left: 5px solid var(--text-muted); border-top: 4px solid transparent; border-bottom: 4px solid transparent; transition: transform 0.2s; flex-shrink: 0; }
        .section[open] > summary::before { transform: rotate(90deg); }
        .section > summary .section-dur { margin-left: auto; font-weight: 400; font-size: 0.8rem; color: var(--text-muted); font-family: var(--font-mono); }
        .section > summary .section-count { font-weight: 400; font-size: 0.8rem; color: var(--text-muted); }
        .section-body { padding: 0 1.25rem 1.25rem; }
        .http-method { display: inline-block; padding: 0.15rem 0.5rem; border-radius: 4px; font-weight: 600; font-family: var(--font-mono); font-size: 0.75rem; }
        .http-method.method-get { background: rgba(96,165,250,0.15); color: #60a5fa; }
        .http-method.method-post { background: rgba(74,222,128,0.15); color: #4ade80; }
        .http-method.method-put, .http-method.method-patch { background: rgba(167,139,250,0.15); color: #a78bfa; }
        .http-method.method-delete { background: rgba(248,113,113,0.15); color: #f87171; }
        .http-method.method-other { background: rgba(148,163,184,0.15); color: #94a3b8; }
        .status-code { display: inline-block; padding: 0.15rem 0.5rem; border-radius: 4px; font-weight: 600; font-family: var(--font-mono); font-size: 0.8rem; }
        .status-code.success { background: rgba(74,222,128,0.15); color: #4ade80; }
        .status-code.redirect { background: rgba(251,191,36,0.15); color: #fbbf24; }
        .status-code.client-error { background: rgba(248,113,113,0.15); color: #f87171; }
        .status-code.server-error { background: rgba(220,38,38,0.2); color: #ef4444; }
        .exit-code { display: inline-block; padding: 0.15rem 0.5rem; border-radius: 4px; font-weight: 600; font-family: var(--font-mono); font-size: 0.8rem; }
        .exit-code.success { background: rgba(74,222,128,0.15); color: #4ade80; }
        .exit-code.error { background: rgba(248,113,113,0.15); color: #f87171; }
        .code-block { background: var(--bg-code); border: 1px solid var(--border); border-radius: 8px; padding: 0.75rem 1rem; font-family: var(--font-mono); font-size: 0.8rem; color: var(--text-body); overflow-x: auto; white-space: pre-wrap; word-break: break-word; max-height: 400px; overflow-y: auto; }
        .code-block.compact { padding: 0.5rem 0.75rem; font-size: 0.75rem; }
        .svc-card { background: var(--bg-code); border: 1px solid var(--border); border-radius: 8px; margin-bottom: 0.75rem; overflow: hidden; }
        .svc-card:last-child { margin-bottom: 0; }
        .svc-header { display: flex; align-items: center; gap: 0.75rem; padding: 0.75rem 1rem; }
        .svc-icon { width: 32px; height: 32px; background: var(--bg-elevated); border-radius: 6px; display: flex; align-items: center; justify-content: center; font-size: 0.875rem; flex-shrink: 0; }
        .svc-info { flex: 1; min-width: 0; }
        .svc-name { font-weight: 600; color: var(--text-heading); font-size: 0.9rem; }
        .svc-image { color: var(--text-muted); font-size: 0.75rem; font-family: var(--font-mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
        .svc-dur { font-family: var(--font-mono); font-size: 0.8rem; color: var(--text-muted); flex-shrink: 0; }
        .svc-details { padding: 0 1rem 0.75rem; }
        .svc-detail-grid { display: grid; grid-template-columns: auto 1fr; gap: 0.25rem 0.75rem; font-size: 0.8rem; }
        .svc-detail-label { color: var(--text-muted); font-size: 0.75rem; padding: 0.2rem 0; }
        .svc-detail-value { color: var(--text-body); font-family: var(--font-mono); font-size: 0.75rem; padding: 0.2rem 0; word-break: break-all; }
        .svc-section-title { font-size: 0.7rem; font-weight: 600; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.05em; padding: 0.5rem 0 0.25rem; grid-column: 1 / -1; border-top: 1px solid var(--border); margin-top: 0.25rem; }
        .svc-section-title:first-child { border-top: none; margin-top: 0; }
        .env-table { width: 100%; font-size: 0.75rem; font-family: var(--font-mono); border-collapse: collapse; }
        .env-table td { padding: 0.2rem 0.5rem; border-bottom: 1px solid var(--border); }
        .env-table td:first-child { color: var(--accent-light); white-space: nowrap; }
        .env-table td:last-child { color: var(--text-body); word-break: break-all; }
        .env-table tr:last-child td { border-bottom: none; }
        .phase-row { display: flex; justify-content: space-between; padding: 0.2rem 0; font-size: 0.75rem; color: var(--text-secondary); }
        .phase-row-label { display: flex; align-items: center; gap: 0.4rem; }
        .phase-dot { width: 5px; height: 5px; border-radius: 50%; background: var(--text-muted); }
        .phase-row-dur { font-family: var(--font-mono); color: var(--text-muted); }
        .setup-step { background: var(--bg-code); border: 1px solid var(--border); border-radius: 8px; padding: 0.75rem 1rem; margin-bottom: 0.75rem; }
        .setup-step:last-child { margin-bottom: 0; }
        .setup-header { display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.5rem; }
        .setup-icon { width: 22px; height: 22px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 0.7rem; flex-shrink: 0; }
        .setup-icon.passed { background: var(--pass-bg); color: var(--pass); }
        .setup-icon.failed { background: var(--fail-bg); color: var(--fail); }
        .setup-type { font-weight: 600; font-size: 0.85rem; color: var(--text-heading); }
        .setup-dur { margin-left: auto; font-family: var(--font-mono); font-size: 0.8rem; color: var(--text-muted); }
        .setup-body { padding-left: 2.25rem; }
        .exchange { margin-top: 0.5rem; }
        .exchange-label { font-size: 0.7rem; font-weight: 600; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 0.25rem; margin-top: 0.5rem; }
        .exchange-line { font-family: var(--font-mono); font-size: 0.8rem; color: var(--text-body); margin-bottom: 0.25rem; }
        .headers-list { font-family: var(--font-mono); font-size: 0.75rem; max-height: 150px; overflow-y: auto; }
        .header-line { padding: 0.1rem 0; }
        .header-name { color: var(--accent-light); }
        .header-val { color: var(--text-secondary); }
        .exec-grid { display: grid; grid-template-columns: auto 1fr; gap: 0.4rem 1rem; font-size: 0.85rem; align-items: baseline; }
        .exec-label { color: var(--text-muted); font-size: 0.8rem; white-space: nowrap; }
        .exec-value { color: var(--text-body); font-family: var(--font-mono); font-size: 0.8rem; word-break: break-all; }
        .exec-divider { grid-column: 1 / -1; border-top: 1px solid var(--border); margin: 0.4rem 0; }
        .assert-item { display: flex; align-items: flex-start; gap: 0.75rem; padding: 0.75rem 1rem; background: var(--bg-code); border: 1px solid var(--border); border-radius: 8px; margin-bottom: 0.5rem; }
        .assert-item:last-child { margin-bottom: 0; }
        .assert-icon { width: 22px; height: 22px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 0.7rem; flex-shrink: 0; margin-top: 0.1rem; }
        .assert-icon.passed { background: var(--pass-bg); color: var(--pass); }
        .assert-icon.failed { background: var(--fail-bg); color: var(--fail); }
        .assert-body { flex: 1; min-width: 0; }
        .assert-msg { font-weight: 500; font-size: 0.85rem; color: var(--text-heading); margin-bottom: 0.15rem; }
        .assert-detail { font-size: 0.75rem; color: var(--text-secondary); font-family: var(--font-mono); }
        .assert-dur { font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-muted); flex-shrink: 0; }
        .artifact-row { display: flex; align-items: center; gap: 0.75rem; padding: 0.5rem 0; border-bottom: 1px solid var(--border); font-size: 0.85rem; }
        .artifact-row:last-child { border-bottom: none; }
        .artifact-row a { color: var(--accent-light); font-family: var(--font-mono); font-size: 0.8rem; }
        .artifact-size { color: var(--text-muted); font-family: var(--font-mono); font-size: 0.75rem; margin-left: auto; }
        .phase-timeline { padding: 1rem 0; }
        .phase-bar { display: flex; height: 20px; border-radius: 4px; overflow: hidden; background: var(--bg-elevated); margin-bottom: 0.75rem; }
        .phase-segment { height: 100%; min-width: 2px; transition: opacity 0.15s; }
        .phase-segment:hover { opacity: 0.8; }
        .phase-network { background: #6366f1; }
        .phase-services { background: #3b82f6; }
        .phase-setup { background: #8b5cf6; }
        .phase-execution { background: #f59e0b; }
        .phase-assertions { background: #10b981; }
        .phase-teardown { background: #ec4899; }
        .phase-cleanup { background: #64748b; }
        .phase-legend { display: flex; flex-wrap: wrap; gap: 0.75rem; }
        .phase-legend-item { display: flex; align-items: center; gap: 0.35rem; font-size: 0.75rem; color: var(--text-secondary); }
        .phase-legend-dot { width: 8px; height: 8px; border-radius: 2px; flex-shrink: 0; }
        .phase-legend-dur { font-family: var(--font-mono); color: var(--text-body); }
        .log-table { width: 100%; border-collapse: collapse; font-size: 0.75rem; font-family: var(--font-mono); }
        .log-table th { text-align: left; padding: 0.4rem 0.5rem; color: var(--text-muted); font-weight: 600; border-bottom: 1px solid var(--border-subtle); font-size: 0.7rem; text-transform: uppercase; letter-spacing: 0.05em; }
        .log-table td { padding: 0.3rem 0.5rem; border-bottom: 1px solid var(--border); vertical-align: top; }
        .log-time { color: var(--text-muted); white-space: nowrap; }
        .log-level { font-weight: 600; font-size: 0.65rem; padding: 0.1rem 0.35rem; border-radius: 3px; text-transform: uppercase; }
        .log-level.info { background: rgba(96,165,250,0.12); color: #60a5fa; }
        .log-level.pass { background: rgba(74,222,128,0.12); color: #4ade80; }
        .log-level.fail { background: rgba(248,113,113,0.12); color: #f87171; }
        .log-level.warn { background: rgba(251,191,36,0.12); color: #fbbf24; }
        .log-action { color: #a78bfa; }
        .log-service { color: var(--text-secondary); }
        .log-message { color: var(--text-body); word-break: break-word; }
        .footer { text-align: center; padding: 2.5rem 0 1rem; color: var(--text-muted); font-size: 0.8rem; border-top: 1px solid var(--border); margin-top: 1rem; }
        .empty { text-align: center; padding: 1.5rem; color: var(--text-muted); font-size: 0.85rem; }
        @media print {
            body { background: white; color: #1a1a1a; }
            .section { border-color: #e5e7eb; background: white; }
            .section > summary::before { border-left-color: #6b7280; }
            .code-block { background: #f9fafb; border-color: #e5e7eb; }
            .footer { display: none; }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div class="brand">
                <div class="brand-icon"><svg viewBox="0 0 24 24"><path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/></svg></div>
                <span class="brand-text">Spark Test Report</span>
                <a href="../index.html" class="back-link">&#8592; Back to Summary</a>
            </div>
            <div class="test-title">
                <h1>{{.Name}}</h1>
                <span class="status-badge {{.StatusClass}}">{{.Status}}</span>
            </div>
            {{if .Description}}<p class="description">{{.Description}}</p>{{end}}
            {{if .Tags}}<div class="tags">{{range .Tags}}<span class="tag">{{.}}</span>{{end}}</div>{{end}}
            <div class="meta-grid">
                <div class="meta-item"><span class="meta-label">Suite</span><span class="meta-value">{{.SuiteName}}</span></div>
                <div class="meta-item"><span class="meta-label">File</span><span class="meta-value">{{.SuiteFile}}</span></div>
                <div class="meta-item"><span class="meta-label">Duration</span><span class="meta-value">{{.Duration}}</span></div>
                <div class="meta-item"><span class="meta-label">Run at</span><span class="meta-value">{{.GeneratedAt}}</span></div>
            </div>
        </div>

        {{if .Error}}
        <div class="error-box">{{.Error}}</div>
        {{end}}

        <details class="section" open>
            <summary>Services <span class="section-count">({{len .Services}})</span><span class="section-dur">{{.ServicesDuration}}</span></summary>
            <div class="section-body">
            {{if .Services}}
                {{range .Services}}
                <div class="svc-card">
                    <div class="svc-header">
                        <div class="svc-icon">&#9881;</div>
                        <div class="svc-info">
                            <div class="svc-name">{{.Name}}</div>
                            <div class="svc-image">{{.Image}}</div>
                        </div>
                        {{if .Duration}}<div class="svc-dur">{{.Duration}}</div>{{end}}
                    </div>
                    {{if or .Command .WorkingDir .HasEnvironment .HasHealthCheck .HasPhaseBreakdown}}
                    <div class="svc-details">
                        <div class="svc-detail-grid">
                            {{if .Command}}<div class="svc-detail-label">Command</div><div class="svc-detail-value">{{.Command}}</div>{{end}}
                            {{if .WorkingDir}}<div class="svc-detail-label">Working Dir</div><div class="svc-detail-value">{{.WorkingDir}}</div>{{end}}
                        </div>
                        {{if .HasEnvironment}}
                        <div class="svc-section-title">Environment</div>
                        <table class="env-table">{{range .Environment}}<tr><td>{{.Key}}</td><td>{{.Value}}</td></tr>{{end}}</table>
                        {{end}}
                        {{if .HasHealthCheck}}
                        <div class="svc-section-title">Health Check</div>
                        <div class="svc-detail-grid">
                            {{if .HealthCheckTest}}<div class="svc-detail-label">Test</div><div class="svc-detail-value">{{.HealthCheckTest}}</div>{{end}}
                            {{if .HealthCheckInterval}}<div class="svc-detail-label">Interval</div><div class="svc-detail-value">{{.HealthCheckInterval}}</div>{{end}}
                            {{if .HealthCheckTimeout}}<div class="svc-detail-label">Timeout</div><div class="svc-detail-value">{{.HealthCheckTimeout}}</div>{{end}}
                            {{if .HealthCheckRetries}}<div class="svc-detail-label">Retries</div><div class="svc-detail-value">{{.HealthCheckRetries}}</div>{{end}}
                        </div>
                        {{end}}
                        {{if .HasPhaseBreakdown}}
                        <div class="svc-section-title">Phase Breakdown</div>
                        {{if .ImageResolveDuration}}<div class="phase-row"><div class="phase-row-label"><span class="phase-dot"></span> Image resolve</div><div class="phase-row-dur">{{.ImageResolveDuration}}</div></div>{{end}}
                        {{if .ContainerStartDuration}}<div class="phase-row"><div class="phase-row-label"><span class="phase-dot"></span> Container start</div><div class="phase-row-dur">{{.ContainerStartDuration}}</div></div>{{end}}
                        {{if .HealthCheckDuration}}<div class="phase-row"><div class="phase-row-label"><span class="phase-dot"></span> Healthcheck wait</div><div class="phase-row-dur">{{.HealthCheckDuration}}</div></div>{{end}}
                        {{end}}
                    </div>
                    {{end}}
                </div>
                {{end}}
            {{else}}
                <div class="empty">No services defined</div>
            {{end}}
            </div>
        </details>

        <details class="section" open>
            <summary>Setup <span class="section-count">({{len .SetupSteps}})</span><span class="section-dur">{{.SetupDuration}}</span></summary>
            <div class="section-body">
            {{if .SetupSteps}}
                {{range .SetupSteps}}
                <div class="setup-step">
                    <div class="setup-header">
                        <div class="setup-icon {{.StatusClass}}">{{if .Success}}&#10003;{{else}}&#10007;{{end}}</div>
                        <span class="setup-type">{{if eq .Type "cli"}}CLI{{else}}HTTP{{end}}</span>
                        {{if eq .Type "http"}}<span class="http-method {{.HTTPMethodClass}}">{{.HTTPMethod}}</span>{{end}}
                        <span class="setup-dur">{{.Duration}}</span>
                    </div>
                    <div class="setup-body">
                        {{if eq .Type "cli"}}
                            <div style="font-size:0.8rem; color:var(--text-secondary); margin-bottom:0.25rem;">{{.CLIService}}</div>
                            <div class="code-block compact">{{.CLICommand}}</div>
                            {{if .CLIWorkingDir}}<div style="font-size:0.75rem; color:var(--text-muted); margin-top:0.25rem;">Working dir: {{.CLIWorkingDir}}</div>{{end}}
                            {{if .CLIExec}}
                            <div class="exchange">
                                <div class="exchange-label">Exit Code</div>
                                <span class="exit-code {{.CLIExec.ExitCodeClass}}">{{.CLIExec.ExitCode}}</span>
                                {{if .CLIExec.Stdout}}<div class="exchange-label">Stdout</div><div class="code-block compact">{{.CLIExec.Stdout}}</div>{{end}}
                                {{if .CLIExec.Stderr}}<div class="exchange-label">Stderr</div><div class="code-block compact">{{.CLIExec.Stderr}}</div>{{end}}
                            </div>
                            {{end}}
                        {{else}}
                            <div class="exchange-line"><span class="http-method {{.HTTPMethodClass}}">{{.HTTPMethod}}</span> {{.HTTPTarget}}{{.HTTPURL}}</div>
                            {{if .HTTPExchange}}
                            <div class="exchange">
                                {{if .HTTPExchange.RequestHeaders}}<div class="exchange-label">Request Headers</div><div class="headers-list">{{range .HTTPExchange.RequestHeaders}}<div class="header-line"><span class="header-name">{{.Name}}:</span> <span class="header-val">{{.Values}}</span></div>{{end}}</div>{{end}}
                                {{if .HTTPExchange.RequestBody}}<div class="exchange-label">Request Body</div><div class="code-block compact">{{.HTTPExchange.RequestBody}}</div>{{end}}
                                <div class="exchange-label">Response</div>
                                <div class="exchange-line"><span class="status-code {{.HTTPExchange.ResponseStatusClass}}">{{.HTTPExchange.ResponseStatusCode}}</span></div>
                                {{if .HTTPExchange.ResponseHeaders}}<div class="headers-list" style="margin-top:0.25rem;">{{range .HTTPExchange.ResponseHeaders}}<div class="header-line"><span class="header-name">{{.Name}}:</span> <span class="header-val">{{.Values}}</span></div>{{end}}</div>{{end}}
                                {{if .HTTPExchange.ResponseBody}}<div class="exchange-label">Response Body</div><div class="code-block compact">{{.HTTPExchange.ResponseBody}}</div>{{end}}
                            </div>
                            {{end}}
                        {{end}}
                        {{if .Error}}<div class="error-box" style="margin-top:0.5rem; font-size:0.75rem;">{{.Error}}</div>{{end}}
                    </div>
                </div>
                {{end}}
            {{else}}
                <div class="empty">No setup steps</div>
            {{end}}
            </div>
        </details>

        <details class="section" open>
            <summary>Execution<span class="section-dur">{{.ExecutionDuration}}</span></summary>
            <div class="section-body">
                {{if .Execution.CLICommand}}
                <div class="exec-grid">
                    <div class="exec-label">Service</div>
                    <div class="exec-value">{{.Execution.CLIService}}</div>
                    <div class="exec-label">Command</div>
                    <div class="exec-value">{{.Execution.CLICommand}}</div>
                    {{if .Execution.CLIWorkingDir}}
                    <div class="exec-label">Working Dir</div>
                    <div class="exec-value">{{.Execution.CLIWorkingDir}}</div>
                    {{end}}
                </div>
                {{if .CLIResult.HasResponse}}
                <div class="exec-divider"></div>
                <div class="exec-grid">
                    <div class="exec-label">Exit Code</div>
                    <div class="exec-value"><span class="exit-code {{.CLIResult.ExitCodeClass}}">{{.CLIResult.ExitCode}}</span></div>
                    {{if .CLIResult.StdoutArtifact}}
                    <div class="exec-label">Stdout</div>
                    <div class="exec-value"><a href="{{.CLIResult.StdoutArtifact.Path}}" target="_blank">{{.CLIResult.StdoutArtifact.Name}}</a> <span style="color:var(--text-muted);font-size:0.75rem;">{{.CLIResult.StdoutArtifact.Size}}</span></div>
                    {{end}}
                    {{if .CLIResult.StderrArtifact}}
                    <div class="exec-label">Stderr</div>
                    <div class="exec-value"><a href="{{.CLIResult.StderrArtifact.Path}}" target="_blank">{{.CLIResult.StderrArtifact.Name}}</a> <span style="color:var(--text-muted);font-size:0.75rem;">{{.CLIResult.StderrArtifact.Size}}</span></div>
                    {{end}}
                </div>
                {{end}}
                {{range .CLIExecs}}{{if eq .Phase "execution"}}
                <div class="exec-divider"></div>
                {{if .Stdout}}<div class="exchange-label">Stdout</div><div class="code-block compact">{{.Stdout}}</div>{{end}}
                {{if .Stderr}}<div class="exchange-label">Stderr</div><div class="code-block compact">{{.Stderr}}</div>{{end}}
                {{end}}{{end}}
                {{else}}
                <div class="exec-grid">
                    <div class="exec-label">Target</div>
                    <div class="exec-value">{{.Execution.Target}}</div>
                    <div class="exec-label">Request</div>
                    <div class="exec-value"><span class="http-method {{.Execution.MethodClass}}">{{.Execution.Method}}</span> {{.Execution.URL}}</div>
                    {{if .Execution.Headers}}
                    <div class="exec-label">Headers</div>
                    <div class="exec-value"><div class="headers-list">{{range .Execution.Headers}}<div class="header-line"><span class="header-name">{{.Name}}:</span> <span class="header-val">{{.Values}}</span></div>{{end}}</div></div>
                    {{end}}
                    {{if .Execution.Body}}
                    <div class="exec-label">Body</div>
                    <div class="exec-value"><div class="code-block compact">{{.Execution.Body}}</div></div>
                    {{end}}
                </div>
                {{if .Result.HasResponse}}
                <div class="exec-divider"></div>
                <div class="exec-grid">
                    <div class="exec-label">Status</div>
                    <div class="exec-value"><span class="status-code {{.Result.StatusClass}}">{{.Result.StatusCode}}</span></div>
                    {{if .Result.Headers}}
                    <div class="exec-label">Headers</div>
                    <div class="exec-value"><div class="headers-list">{{range .Result.Headers}}<div class="header-line"><span class="header-name">{{.Name}}:</span> <span class="header-val">{{.Values}}</span></div>{{end}}</div></div>
                    {{end}}
                    {{if .Result.BodyArtifact}}
                    <div class="exec-label">Body</div>
                    <div class="exec-value"><a href="{{.Result.BodyArtifact.Path}}" target="_blank">{{.Result.BodyArtifact.Name}}</a> <span style="color:var(--text-muted);font-size:0.75rem;">{{.Result.BodyArtifact.Size}}</span></div>
                    {{end}}
                </div>
                {{end}}
                {{range .HTTPExchanges}}{{if eq .Phase "execution"}}
                <div class="exec-divider"></div>
                {{if .ResponseHeaders}}<div class="exchange-label">Response Headers</div><div class="headers-list">{{range .ResponseHeaders}}<div class="header-line"><span class="header-name">{{.Name}}:</span> <span class="header-val">{{.Values}}</span></div>{{end}}</div>{{end}}
                {{if .ResponseBody}}<div class="exchange-label">Response Body</div><div class="code-block">{{.ResponseBody}}</div>{{end}}
                {{end}}{{end}}
                {{end}}
            </div>
        </details>

        <details class="section" open>
            <summary>Teardown <span class="section-count">({{len .TeardownSteps}})</span><span class="section-dur">{{.TeardownDuration}}</span></summary>
            <div class="section-body">
            {{if .TeardownSteps}}
                {{range .TeardownSteps}}
                <div class="setup-step">
                    <div class="setup-header">
                        <div class="setup-icon {{.StatusClass}}">{{if .Success}}&#10003;{{else}}&#10007;{{end}}</div>
                        <span class="setup-type">{{if eq .Type "cli"}}CLI{{else}}HTTP{{end}}</span>
                        {{if eq .Type "http"}}<span class="http-method {{.HTTPMethodClass}}">{{.HTTPMethod}}</span>{{end}}
                        <span class="setup-dur">{{.Duration}}</span>
                    </div>
                    <div class="setup-body">
                        {{if eq .Type "cli"}}
                            <div style="font-size:0.8rem; color:var(--text-secondary); margin-bottom:0.25rem;">{{.CLIService}}</div>
                            <div class="code-block compact">{{.CLICommand}}</div>
                            {{if .CLIWorkingDir}}<div style="font-size:0.75rem; color:var(--text-muted); margin-top:0.25rem;">Working dir: {{.CLIWorkingDir}}</div>{{end}}
                            {{if .CLIExec}}
                            <div class="exchange">
                                <div class="exchange-label">Exit Code</div>
                                <span class="exit-code {{.CLIExec.ExitCodeClass}}">{{.CLIExec.ExitCode}}</span>
                                {{if .CLIExec.Stdout}}<div class="exchange-label">Stdout</div><div class="code-block compact">{{.CLIExec.Stdout}}</div>{{end}}
                                {{if .CLIExec.Stderr}}<div class="exchange-label">Stderr</div><div class="code-block compact">{{.CLIExec.Stderr}}</div>{{end}}
                            </div>
                            {{end}}
                        {{else}}
                            <div class="exchange-line"><span class="http-method {{.HTTPMethodClass}}">{{.HTTPMethod}}</span> {{.HTTPTarget}}{{.HTTPURL}}</div>
                            {{if .HTTPExchange}}
                            <div class="exchange">
                                {{if .HTTPExchange.RequestHeaders}}<div class="exchange-label">Request Headers</div><div class="headers-list">{{range .HTTPExchange.RequestHeaders}}<div class="header-line"><span class="header-name">{{.Name}}:</span> <span class="header-val">{{.Values}}</span></div>{{end}}</div>{{end}}
                                {{if .HTTPExchange.RequestBody}}<div class="exchange-label">Request Body</div><div class="code-block compact">{{.HTTPExchange.RequestBody}}</div>{{end}}
                                <div class="exchange-label">Response</div>
                                <div class="exchange-line"><span class="status-code {{.HTTPExchange.ResponseStatusClass}}">{{.HTTPExchange.ResponseStatusCode}}</span></div>
                                {{if .HTTPExchange.ResponseHeaders}}<div class="headers-list" style="margin-top:0.25rem;">{{range .HTTPExchange.ResponseHeaders}}<div class="header-line"><span class="header-name">{{.Name}}:</span> <span class="header-val">{{.Values}}</span></div>{{end}}</div>{{end}}
                                {{if .HTTPExchange.ResponseBody}}<div class="exchange-label">Response Body</div><div class="code-block compact">{{.HTTPExchange.ResponseBody}}</div>{{end}}
                            </div>
                            {{end}}
                        {{end}}
                        {{if .Error}}<div class="error-box" style="margin-top:0.5rem; font-size:0.75rem;">{{.Error}}</div>{{end}}
                    </div>
                </div>
                {{end}}
            {{else}}
                <div class="empty">No teardown steps</div>
            {{end}}
            </div>
        </details>

        <details class="section" open>
            <summary>Assertions <span class="section-count">({{len .Assertions}})</span><span class="section-dur">{{.AssertionDuration}}</span></summary>
            <div class="section-body">
            {{if .Assertions}}
                {{range .Assertions}}
                <div class="assert-item">
                    <div class="assert-icon {{.StatusClass}}">{{if .Passed}}&#10003;{{else}}&#10007;{{end}}</div>
                    <div class="assert-body">
                        <div class="assert-msg">{{.Message}}</div>
                        {{if or .Expected .Actual}}
                        <div class="assert-detail">
                            {{if .Expected}}Expected: {{.Expected}}{{end}}
                            {{if .Actual}} | Actual: {{.Actual}}{{end}}
                        </div>
                        {{end}}
                    </div>
                    <div class="assert-dur">{{.Duration}}</div>
                </div>
                {{end}}
            {{else}}
                <div class="empty">No assertions defined</div>
            {{end}}
            </div>
        </details>

        <details class="section">
            <summary>Artifacts <span class="section-count">({{len .Artifacts}})</span></summary>
            <div class="section-body">
            {{if .Artifacts}}
                {{range .Artifacts}}
                <div class="artifact-row">
                    <a href="{{.Path}}" target="_blank">{{.Name}}</a>
                    <span class="artifact-size">{{.Size}}</span>
                </div>
                {{end}}
            {{else}}
                <div class="empty">No artifacts collected</div>
            {{end}}
            </div>
        </details>

        <details class="section">
            <summary>Performance</summary>
            <div class="section-body">
            {{if .Phases}}
                <div class="phase-timeline">
                    <div class="phase-bar">
                        {{range .Phases}}<div class="phase-segment {{.ColorClass}}" style="width:{{printf "%.1f" .Percent}}%" title="{{.Name}}: {{.Duration}}"></div>{{end}}
                    </div>
                    <div class="phase-legend">
                        {{range .Phases}}<div class="phase-legend-item"><span class="phase-legend-dot {{.ColorClass}}"></span><span>{{.Name}}</span><span class="phase-legend-dur">{{.Duration}}</span></div>{{end}}
                    </div>
                </div>
            {{end}}
            {{if .LogEntries}}
                <div style="margin-top:1rem;">
                    <div style="font-size:0.8rem; font-weight:600; color:var(--text-heading); margin-bottom:0.5rem;">Event Log ({{len .LogEntries}})</div>
                    <table class="log-table">
                        <thead><tr><th>Time</th><th>Level</th><th>Action</th><th>Service</th><th>Message</th></tr></thead>
                        <tbody>
                            {{range .LogEntries}}
                            <tr>
                                <td class="log-time">{{.Time}}</td>
                                <td><span class="log-level {{.Level}}">{{.Level}}</span></td>
                                <td class="log-action">{{.Action}}</td>
                                <td class="log-service">{{.Service}}</td>
                                <td class="log-message">{{.Message}}</td>
                            </tr>
                            {{end}}
                        </tbody>
                    </table>
                </div>
            {{end}}
            {{if not .Phases}}{{if not .LogEntries}}<div class="empty">No performance data available</div>{{end}}{{end}}
            </div>
        </details>

        <div class="footer">Generated by Spark Test Runner{{if .GeneratedAt}} &middot; {{.GeneratedAt}}{{end}}</div>
    </div>
</body>
</html>
`

// dashboardTemplate is the index.html dashboard with links to per-test reports.
const dashboardTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Spark Test Report</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=DM+Sans:wght@400;500;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-base: #030712;
            --bg-card: #111827;
            --bg-elevated: #1f2937;
            --bg-code: #0a0a0a;
            --border: #1f2937;
            --border-subtle: #374151;
            --text-heading: #ffffff;
            --text-body: #d1d5db;
            --text-secondary: #9ca3af;
            --text-muted: #6b7280;
            --accent: #f97316;
            --accent-light: #fb923c;
            --accent-bg: rgba(249, 115, 22, 0.1);
            --accent-border: rgba(249, 115, 22, 0.25);
            --pass: #4ade80;
            --pass-bg: rgba(74, 222, 128, 0.1);
            --fail: #f87171;
            --fail-bg: rgba(248, 113, 113, 0.1);
            --error-color: #fbbf24;
            --error-bg: rgba(251, 191, 36, 0.1);
            --skip: #64748b;
            --skip-bg: rgba(100, 116, 139, 0.1);
            --font-sans: 'DM Sans', ui-sans-serif, system-ui, sans-serif;
            --font-mono: 'JetBrains Mono', ui-monospace, monospace;
        }
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: var(--font-sans); background: var(--bg-base); color: var(--text-body); line-height: 1.6; min-height: 100vh; -webkit-font-smoothing: antialiased; }
        .container { max-width: 960px; margin: 0 auto; padding: 3rem 2rem; }
        a { color: var(--accent-light); text-decoration: none; }
        a:hover { text-decoration: underline; }

        /* Header */
        .header { border-bottom: 1px solid var(--border); padding-bottom: 2rem; margin-bottom: 2rem; }
        .brand { display: flex; align-items: center; gap: 0.5rem; margin-bottom: 1.5rem; }
        .brand-icon { width: 28px; height: 28px; background: var(--accent); border-radius: 6px; display: flex; align-items: center; justify-content: center; }
        .brand-icon svg { width: 16px; height: 16px; fill: white; }
        .brand-text { font-size: 0.875rem; font-weight: 700; color: var(--text-secondary); text-transform: uppercase; letter-spacing: 0.08em; }
        .header h1 { font-size: 1.75rem; font-weight: 700; color: var(--text-heading); margin-bottom: 0.5rem; }
        .header-meta { color: var(--text-secondary); font-size: 0.85rem; display: flex; flex-wrap: wrap; gap: 0.25rem 1.25rem; }
        .header-meta strong { color: var(--accent-light); }

        /* Stats grid */
        .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 0.75rem; margin-bottom: 2rem; }
        .stat { background: var(--bg-card); border: 1px solid var(--border); border-radius: 10px; padding: 1.25rem; text-align: center; }
        .stat-val { font-size: 2rem; font-weight: 700; font-family: var(--font-mono); margin-bottom: 0.1rem; }
        .stat-label { font-size: 0.7rem; font-weight: 500; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.06em; }
        .stat.total .stat-val { color: var(--text-heading); }
        .stat.passed .stat-val { color: var(--pass); }
        .stat.failed .stat-val { color: var(--fail); }
        .stat.skipped .stat-val { color: var(--skip); }
        .stat.rate .stat-val { color: var(--accent-light); }

        /* Progress bar */
        .progress { width: 100%; height: 6px; background: var(--bg-elevated); border-radius: 3px; margin-bottom: 2rem; overflow: hidden; }
        .progress-fill { height: 100%; background: linear-gradient(90deg, var(--pass), #22c55e); border-radius: 3px; transition: width 0.5s ease; }

        /* Suite card */
        .suite { background: var(--bg-card); border: 1px solid var(--border); border-radius: 12px; margin-bottom: 1.25rem; overflow: hidden; }
        .suite-head { padding: 1rem 1.25rem; display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid var(--border); }
        .suite-name { font-size: 1rem; font-weight: 700; color: var(--text-heading); }
        .suite-path { color: var(--text-muted); font-size: 0.75rem; margin-top: 0.15rem; font-family: var(--font-mono); }
        .suite-badges { display: flex; gap: 0.5rem; align-items: center; }
        .badge { padding: 0.2rem 0.6rem; border-radius: 9999px; font-size: 0.7rem; font-weight: 600; }
        .badge.passed { background: var(--pass-bg); color: var(--pass); }
        .badge.failed { background: var(--fail-bg); color: var(--fail); }
        .badge.skipped { background: var(--skip-bg); color: var(--skip); }

        /* Test row */
        .test-row { display: flex; align-items: center; padding: 0.75rem 1.25rem; border-bottom: 1px solid var(--border); transition: background 0.15s; }
        .test-row:last-child { border-bottom: none; }
        .test-row:hover { background: rgba(255,255,255,0.02); }
        .test-row a { text-decoration: none; color: inherit; display: flex; align-items: center; width: 100%; }
        .test-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; margin-right: 0.75rem; }
        .test-dot.passed { background: var(--pass); }
        .test-dot.failed { background: var(--fail); }
        .test-dot.error { background: var(--error-color); }
        .test-dot.skipped { background: var(--skip); }
        .test-name { font-weight: 500; font-size: 0.9rem; color: var(--text-heading); flex: 1; }
        .test-tags { display: inline-flex; gap: 0.3rem; margin-left: 0.5rem; }
        .tag { display: inline-block; padding: 0.1rem 0.5rem; border-radius: 9999px; font-size: 0.6rem; font-weight: 500; background: var(--accent-bg); color: var(--accent-light); border: 1px solid var(--accent-border); }
        .test-right { display: flex; align-items: center; gap: 0.75rem; flex-shrink: 0; margin-left: 1rem; }
        .test-dur { color: var(--text-muted); font-size: 0.8rem; font-family: var(--font-mono); }
        .test-badge { padding: 0.15rem 0.5rem; border-radius: 9999px; font-size: 0.65rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.03em; }
        .test-badge.passed { background: var(--pass-bg); color: var(--pass); }
        .test-badge.failed { background: var(--fail-bg); color: var(--fail); }
        .test-badge.error { background: var(--error-bg); color: var(--error-color); }
        .test-badge.skipped { background: var(--skip-bg); color: var(--skip); }
        .test-arrow { color: var(--text-muted); font-size: 0.7rem; }

        /* Footer */
        .footer { text-align: center; padding: 2.5rem 0 1rem; color: var(--text-muted); font-size: 0.8rem; border-top: 1px solid var(--border); margin-top: 1rem; }

        @media print {
            body { background: white; color: #1a1a1a; }
            .suite { border-color: #e5e7eb; background: white; }
            .stat { border-color: #e5e7eb; background: white; }
            .footer { display: none; }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div class="brand">
                <div class="brand-icon"><svg viewBox="0 0 24 24"><path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/></svg></div>
                <span class="brand-text">Spark Test Report</span>
            </div>
            <h1>Test Run Summary</h1>
            <div class="header-meta">
                <span>{{.GeneratedAt}}</span>
                <span>Duration: {{.Duration}}</span>
                {{if and .Version (ne .Version "dev")}}<span>v{{.Version}}</span>{{end}}
            </div>
        </div>

        <div class="stats">
            <div class="stat total"><div class="stat-val">{{.TotalTests}}</div><div class="stat-label">Total Tests</div></div>
            <div class="stat passed"><div class="stat-val">{{.PassedTests}}</div><div class="stat-label">Passed</div></div>
            <div class="stat failed"><div class="stat-val">{{.FailedTests}}</div><div class="stat-label">Failed</div></div>
            <div class="stat skipped"><div class="stat-val">{{.SkippedTests}}</div><div class="stat-label">Skipped</div></div>
            <div class="stat rate"><div class="stat-val">{{printf "%.0f" .PassRate}}%</div><div class="stat-label">Pass Rate</div></div>
        </div>

        <div class="progress">
            <div class="progress-fill" style="width: {{printf "%.0f" .PassRate}}%"></div>
        </div>

        {{range .Suites}}
        <div class="suite">
            <div class="suite-head">
                <div>
                    <div class="suite-name">{{.Name}}</div>
                    <div class="suite-path">{{.FilePath}}</div>
                </div>
                <div class="suite-badges">
                    <span class="badge passed">{{.Passed}} passed</span>
                    {{if gt .Failed 0}}<span class="badge failed">{{.Failed}} failed</span>{{end}}
                    {{if gt .Skipped 0}}<span class="badge skipped">{{.Skipped}} skipped</span>{{end}}
                </div>
            </div>
            {{range .Tests}}
            <div class="test-row">
                {{if .HasLink}}<a href="{{.UUID}}/index.html">{{end}}
                <div class="test-dot {{.StatusClass}}"></div>
                <span class="test-name">{{.Name}}{{if .Tags}}<span class="test-tags">{{range .Tags}}<span class="tag">{{.}}</span>{{end}}</span>{{end}}</span>
                <div class="test-right">
                    <span class="test-dur">{{.Duration}}</span>
                    <span class="test-badge {{.StatusClass}}">{{.Status}}</span>
                    {{if .HasLink}}<span class="test-arrow">&#9656;</span>{{end}}
                </div>
                {{if .HasLink}}</a>{{end}}
            </div>
            {{end}}
        </div>
        {{end}}

        <div class="footer">Generated by Spark Test Runner{{if and .Version (ne .Version "dev")}} v{{.Version}}{{end}} &middot; {{.GeneratedAt}}</div>
    </div>
</body>
</html>
`
