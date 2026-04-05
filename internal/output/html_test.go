package output

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"chiperka-cli/internal/model"
)

func TestHTML_WriteTestReport_HasBackLink(t *testing.T) {
	w := NewHTMLWriter()
	testResult := &model.TestResult{
		Test:     model.Test{Name: "login-test"},
		Status:   model.StatusPassed,
		Duration: 500 * time.Millisecond,
		UUID:     "test-uuid-123",
	}

	dir := t.TempDir()
	filePath, err := w.WriteTestReport(testResult, "auth-suite", "tests/auth.chiperka", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	html := string(data)

	if !strings.Contains(html, `href="../index.html"`) {
		t.Errorf("expected relative back link to ../index.html in test page")
	}
	if !strings.Contains(html, "Back to Summary") {
		t.Errorf("expected 'Back to Summary' text in test page")
	}
}

// --- Snapshot test infrastructure ---

// compareSnapshot compares normalized content against a stored snapshot file,
// or updates the snapshot when UPDATE_SNAPSHOTS is set. The ext parameter
// determines the file extension (e.g. ".html", ".xml").
func compareSnapshot(t *testing.T, normalized, name, ext string) {
	t.Helper()
	path := filepath.Join("testdata", "snapshots", name+ext)

	if os.Getenv("UPDATE_SNAPSHOTS") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create snapshot dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(normalized), 0o644); err != nil {
			t.Fatalf("write snapshot: %v", err)
		}
		t.Logf("updated snapshot: %s", path)
		return
	}

	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("snapshot not found: %s (run with UPDATE_SNAPSHOTS=1 to create)", path)
	}

	if normalized != string(expected) {
		lines1 := strings.Split(normalized, "\n")
		lines2 := strings.Split(string(expected), "\n")
		for i := 0; i < len(lines1) && i < len(lines2); i++ {
			if lines1[i] != lines2[i] {
				t.Fatalf("snapshot mismatch in %s at line %d:\n  got:  %q\n  want: %q\n\nrun with UPDATE_SNAPSHOTS=1 to update", name, i+1, lines1[i], lines2[i])
			}
		}
		if len(lines1) != len(lines2) {
			t.Fatalf("snapshot mismatch in %s: got %d lines, want %d lines\n\nrun with UPDATE_SNAPSHOTS=1 to update", name, len(lines1), len(lines2))
		}
	}
}

// normalizeHTML replaces dynamic content with fixed placeholders for stable snapshot comparison.
func normalizeHTML(html string) string {
	// Replace timestamps like "2026-03-02 14:05:33"
	tsRe := regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`)
	html = tsRe.ReplaceAllString(html, "__GENERATED_AT__")

	// Replace dashboard duration "Duration: 0.00s" (computed from time.Since)
	durRe := regexp.MustCompile(`Duration: \d+\.\d+s`)
	html = durRe.ReplaceAllString(html, "Duration: __DURATION__")

	return html
}

// --- WriteTestReport snapshot tests ---

func TestHTML_WriteTestReport_Snapshots(t *testing.T) {
	tests := []struct {
		name        string
		buildResult func(dir string) *model.TestResult
		suiteName   string
		suiteFile   string
	}{
		{
			name: "test_report_passed_http",
			buildResult: func(dir string) *model.TestResult {
				return &model.TestResult{
					Test: model.Test{
						Name: "login-test",
						Services: []model.Service{
							{
								Name:  "api",
								Image: "myapp:latest",
								HealthCheck: &model.HealthCheck{
									Test:     "curl -f http://localhost:8080/health",
									Retries:  30,
									Interval: "1s",
								},
							},
						},
						Execution: model.Execution{
							Executor: model.ExecutorHTTP,
							Target:   "http://api:8080",
							Request: model.HTTPRequest{
								Method: "POST",
								URL:    "/api/login",
							},
						},
					},
					Status:            model.StatusPassed,
					Duration:          843 * time.Millisecond,
					ExecutionDuration: 120 * time.Millisecond,
					UUID:              "passed-http-uuid",
					NetworkDuration:   50 * time.Millisecond,
					ServicesDuration:  650 * time.Millisecond,
					AssertionDuration: 1 * time.Millisecond,
					CleanupDuration:   22 * time.Millisecond,
					ServiceResults: []model.ServiceResult{
						{
							Name:                   "api",
							Image:                  "myapp:latest",
							Duration:               650 * time.Millisecond,
							ImageResolveDuration:   50 * time.Millisecond,
							ContainerStartDuration: 200 * time.Millisecond,
							HealthCheckDuration:    400 * time.Millisecond,
						},
					},
					AssertionResults: []model.AssertionResult{
						{Passed: true, Type: "response.statusCode", Expected: "200", Actual: "200", Message: "Status code equals 200", Duration: 1 * time.Millisecond},
					},
					HTTPResponse: &model.HTTPResponseData{StatusCode: 200},
				}
			},
			suiteName: "auth-suite",
			suiteFile: "tests/auth.chiperka",
		},
		{
			name: "test_report_failed_assertions",
			buildResult: func(dir string) *model.TestResult {
				return &model.TestResult{
					Test: model.Test{
						Name: "validate-response",
						Tags: []string{"api", "validation"},
						Execution: model.Execution{
							Executor: model.ExecutorHTTP,
							Target:   "http://api:8080",
							Request: model.HTTPRequest{
								Method: "GET",
								URL:    "/api/users/1",
							},
						},
					},
					Status:            model.StatusFailed,
					Duration:          412 * time.Millisecond,
					ExecutionDuration: 95 * time.Millisecond,
					UUID:              "failed-assertions-uuid",
					AssertionResults: []model.AssertionResult{
						{Passed: true, Type: "response.statusCode", Expected: "200", Actual: "200", Message: "Status code equals 200", Duration: 1 * time.Millisecond},
						{Passed: false, Type: "jsonPath", Expected: "John", Actual: "Jane", Message: "$.name equals John", Duration: 2 * time.Millisecond},
					},
					HTTPResponse: &model.HTTPResponseData{
						StatusCode: 200,
						Headers:    map[string][]string{"Content-Type": {"application/json"}},
					},
				}
			},
			suiteName: "users-suite",
			suiteFile: "tests/users.chiperka",
		},
		{
			name: "test_report_error",
			buildResult: func(dir string) *model.TestResult {
				return &model.TestResult{
					Test: model.Test{
						Name: "timeout-test",
						Execution: model.Execution{
							Executor: model.ExecutorHTTP,
							Target:   "http://api:8080",
							Request: model.HTTPRequest{
								Method: "GET",
								URL:    "/api/slow",
							},
						},
					},
					Status:   model.StatusError,
					Duration: 5 * time.Second,
					UUID:     "error-test-uuid",
					Error:    fmt.Errorf("execution timeout after 5s"),
				}
			},
			suiteName: "timeout-suite",
			suiteFile: "tests/timeout.chiperka",
		},
		{
			name: "test_report_skipped",
			buildResult: func(dir string) *model.TestResult {
				return &model.TestResult{
					Test: model.Test{
						Name: "disabled-test",
						Tags: []string{"wip"},
						Execution: model.Execution{
							Executor: model.ExecutorHTTP,
							Target:   "http://api:8080",
							Request:  model.HTTPRequest{Method: "GET", URL: "/"},
						},
					},
					Status: model.StatusSkipped,
					UUID:   "skipped-test-uuid",
				}
			},
			suiteName: "misc-suite",
			suiteFile: "tests/misc.chiperka",
		},
		{
			name: "test_report_with_setup",
			buildResult: func(dir string) *model.TestResult {
				return &model.TestResult{
					Test: model.Test{
						Name: "test-with-setup",
						Services: []model.Service{
							{Name: "api", Image: "myapp:latest"},
						},
						Setup: []model.SetupInstruction{
							{
								HTTP: &model.SetupHTTP{
									Target:  "http://api:8080",
									Request: model.HTTPRequest{Method: "POST", URL: "/setup-data"},
								},
							},
							{
								CLI: &model.CLICommand{
									Service: "api",
									Command: "php artisan migrate",
								},
							},
						},
						Execution: model.Execution{
							Executor: model.ExecutorHTTP,
							Target:   "http://api:8080",
							Request:  model.HTTPRequest{Method: "GET", URL: "/api/health"},
						},
					},
					Status:            model.StatusPassed,
					Duration:          1200 * time.Millisecond,
					ExecutionDuration: 50 * time.Millisecond,
					SetupDuration:     800 * time.Millisecond,
					UUID:              "setup-test-uuid",
					SetupResults: []model.SetupResult{
						{Type: "http", Duration: 300 * time.Millisecond, Success: true, HTTPStatusCode: 200},
						{Type: "cli", Duration: 500 * time.Millisecond, Success: true, CLIExitCode: 0},
					},
					HTTPExchanges: []model.HTTPExchangeResult{
						{
							Phase:              "setup",
							PhaseSeq:           0,
							RequestMethod:      "POST",
							RequestURL:         "http://api:8080/setup-data",
							ResponseStatusCode: 200,
							ResponseBody:       `{"status":"ok"}`,
							Duration:           300 * time.Millisecond,
						},
					},
					CLIExecutions: []model.CLIExecutionResult{
						{
							Phase:    "setup",
							PhaseSeq: 1,
							Service:  "api",
							Command:  "php artisan migrate",
							ExitCode: 0,
							Stdout:   "Migration complete.",
							Duration: 500 * time.Millisecond,
						},
					},
					HTTPResponse: &model.HTTPResponseData{StatusCode: 200},
					AssertionResults: []model.AssertionResult{
						{Passed: true, Type: "response.statusCode", Expected: "200", Actual: "200", Message: "Status code equals 200", Duration: 1 * time.Millisecond},
					},
				}
			},
			suiteName: "setup-suite",
			suiteFile: "tests/setup.chiperka",
		},
		{
			name: "test_report_cli_execution",
			buildResult: func(dir string) *model.TestResult {
				return &model.TestResult{
					Test: model.Test{
						Name: "cli-test",
						Services: []model.Service{
							{Name: "app", Image: "myapp:latest"},
						},
						Execution: model.Execution{
							Executor: model.ExecutorCLI,
							CLI: &model.CLICommand{
								Service: "app",
								Command: "php artisan test --filter=UserTest",
							},
						},
					},
					Status:            model.StatusPassed,
					Duration:          2500 * time.Millisecond,
					ExecutionDuration: 2200 * time.Millisecond,
					UUID:              "cli-exec-uuid",
					AssertionResults: []model.AssertionResult{
						{Passed: true, Type: "cli.exitCode", Expected: "0", Actual: "0", Message: "Exit code equals 0", Duration: 1 * time.Millisecond},
					},
					CLIResponse: &model.CLIResponseData{
						ExitCode: 0,
						StdoutArtifact: &model.Artifact{
							Name: "stdout.txt",
							Path: filepath.Join(dir, "artifacts", "stdout.txt"),
							Size: 256,
						},
						StderrArtifact: &model.Artifact{
							Name: "stderr.txt",
							Path: filepath.Join(dir, "artifacts", "stderr.txt"),
							Size: 0,
						},
					},
					CLIExecutions: []model.CLIExecutionResult{
						{
							Phase:    "execution",
							PhaseSeq: 0,
							Service:  "app",
							Command:  "php artisan test --filter=UserTest",
							ExitCode: 0,
							Stdout:   "OK (5 tests, 12 assertions)",
							Duration: 2200 * time.Millisecond,
						},
					},
				}
			},
			suiteName: "cli-suite",
			suiteFile: "tests/cli.chiperka",
		},
		{
			name: "test_report_full",
			buildResult: func(dir string) *model.TestResult {
				return &model.TestResult{
					Test: model.Test{
						Name:        "full-test",
						Description: "A comprehensive test exercising all features",
						Tags:        []string{"smoke", "api"},
						Services: []model.Service{
							{
								Name:        "db",
								Image:       "postgres:15",
								Environment: map[string]string{"POSTGRES_PASSWORD": "secret"},
								HealthCheck: &model.HealthCheck{
									Test:     "pg_isready",
									Retries:  30,
									Interval: "1s",
									Timeout:  "3s",
								},
							},
							{
								Name:        "api",
								Image:       "myapp:latest",
								Environment: map[string]string{"DATABASE_URL": "postgres://db:5432/test"},
								HealthCheck: &model.HealthCheck{
									Test:    "curl -f http://localhost:8080/health",
									Retries: 30,
								},
							},
						},
						Setup: []model.SetupInstruction{
							{
								HTTP: &model.SetupHTTP{
									Target:  "http://api:8080",
									Request: model.HTTPRequest{Method: "POST", URL: "/seed"},
								},
							},
							{
								CLI: &model.CLICommand{Service: "db", Command: "psql -c 'SELECT 1'"},
							},
						},
						Execution: model.Execution{
							Executor: model.ExecutorHTTP,
							Target:   "http://api:8080",
							Request: model.HTTPRequest{
								Method:  "POST",
								URL:     "/api/login",
								Headers: map[string]string{"Content-Type": "application/json"},
								Body:    model.Body{Raw: `{"email":"test@example.com","password":"secret"}`},
							},
						},
					},
					Status:            model.StatusPassed,
					Duration:          3500 * time.Millisecond,
					ExecutionDuration: 150 * time.Millisecond,
					NetworkDuration:   80 * time.Millisecond,
					ServicesDuration:  2800 * time.Millisecond,
					SetupDuration:     400 * time.Millisecond,
					AssertionDuration: 5 * time.Millisecond,
					CleanupDuration:   65 * time.Millisecond,
					UUID:              "full-test-uuid",
					ServiceResults: []model.ServiceResult{
						{Name: "db", Image: "postgres:15", Duration: 1800 * time.Millisecond, ImageResolveDuration: 100 * time.Millisecond, ContainerStartDuration: 500 * time.Millisecond, HealthCheckDuration: 1200 * time.Millisecond},
						{Name: "api", Image: "myapp:latest", Duration: 1000 * time.Millisecond, ImageResolveDuration: 50 * time.Millisecond, ContainerStartDuration: 300 * time.Millisecond, HealthCheckDuration: 650 * time.Millisecond},
					},
					SetupResults: []model.SetupResult{
						{Type: "http", Duration: 200 * time.Millisecond, Success: true, HTTPStatusCode: 200},
						{Type: "cli", Duration: 200 * time.Millisecond, Success: true, CLIExitCode: 0},
					},
					HTTPExchanges: []model.HTTPExchangeResult{
						{
							Phase:              "setup",
							PhaseSeq:           0,
							RequestMethod:      "POST",
							RequestURL:         "http://api:8080/seed",
							ResponseStatusCode: 200,
							ResponseBody:       `{"seeded":true}`,
							Duration:           200 * time.Millisecond,
						},
						{
							Phase:              "execution",
							PhaseSeq:           0,
							RequestMethod:      "POST",
							RequestURL:         "http://api:8080/api/login",
							RequestHeaders:     map[string]string{"Content-Type": "application/json"},
							RequestBody:        `{"email":"test@example.com","password":"secret"}`,
							ResponseStatusCode: 200,
							ResponseHeaders:    map[string][]string{"Content-Type": {"application/json"}},
							ResponseBody:       `{"token":"eyJhbGciOiJIUzI1NiJ9"}`,
							Duration:           150 * time.Millisecond,
						},
					},
					CLIExecutions: []model.CLIExecutionResult{
						{
							Phase:    "setup",
							PhaseSeq: 1,
							Service:  "db",
							Command:  "psql -c 'SELECT 1'",
							ExitCode: 0,
							Stdout:   " ?column? \n----------\n        1\n(1 row)",
							Duration: 200 * time.Millisecond,
						},
					},
					AssertionResults: []model.AssertionResult{
						{Passed: true, Type: "response.statusCode", Expected: "200", Actual: "200", Message: "Status code equals 200", Duration: 2 * time.Millisecond},
						{Passed: true, Type: "jsonPath", Expected: "exists", Actual: "eyJhbGciOiJIUzI1NiJ9", Message: "$.token exists", Duration: 3 * time.Millisecond},
					},
					HTTPResponse: &model.HTTPResponseData{
						StatusCode: 200,
						Headers:    map[string][]string{"Content-Type": {"application/json"}},
						BodyArtifact: &model.Artifact{
							Name: "response-body.json",
							Path: filepath.Join(dir, "artifacts", "response-body.json"),
							Size: 42,
						},
					},
					Artifacts: []model.Artifact{
						{Name: "error.log", Path: filepath.Join(dir, "artifacts", "error.log"), Size: 1234},
					},
					LogEntries: []model.LogEntry{
						{RelativeTime: "0.000s", Level: "info", Action: "network.create", Message: "Creating test network"},
						{RelativeTime: "0.080s", Level: "info", Action: "service.start", Service: "db", Message: "Starting postgres:15"},
						{RelativeTime: "1.880s", Level: "pass", Action: "service.healthy", Service: "db", Message: "Service is healthy"},
						{RelativeTime: "2.880s", Level: "pass", Action: "service.healthy", Service: "api", Message: "Service is healthy"},
						{RelativeTime: "3.280s", Level: "info", Action: "test.execute", Message: "POST /api/login"},
						{RelativeTime: "3.430s", Level: "pass", Action: "assertion.pass", Message: "Status code equals 200"},
					},
				}
			},
			suiteName: "full-suite",
			suiteFile: "tests/full.chiperka",
		},
		{
			name: "test_report_snapshot_assertion",
			buildResult: func(dir string) *model.TestResult {
				return &model.TestResult{
					Test: model.Test{
						Name: "snapshot-test",
						Execution: model.Execution{
							Executor: model.ExecutorHTTP,
							Target:   "http://api:8080",
							Request:  model.HTTPRequest{Method: "GET", URL: "/api/config"},
						},
					},
					Status:            model.StatusFailed,
					Duration:          300 * time.Millisecond,
					ExecutionDuration: 100 * time.Millisecond,
					UUID:              "snapshot-test-uuid",
					AssertionResults: []model.AssertionResult{
						{Passed: true, Type: "response.body.snapshot", Expected: "snapshots/config.json", Actual: "snapshots/config.json", Message: "Snapshot matches snapshots/config.json", Duration: 5 * time.Millisecond},
						{Passed: false, Type: "response.body.snapshot", Expected: `{"version":"1.0"}`, Actual: `{"version":"2.0"}`, Message: "Snapshot matches snapshots/version.json", Duration: 3 * time.Millisecond},
					},
					HTTPResponse: &model.HTTPResponseData{StatusCode: 200},
				}
			},
			suiteName: "snapshot-suite",
			suiteFile: "tests/snapshot.chiperka",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewHTMLWriter()
			dir := t.TempDir()
			result := tt.buildResult(dir)

			filePath, err := w.WriteTestReport(result, tt.suiteName, tt.suiteFile, dir)
			if err != nil {
				t.Fatalf("WriteTestReport: %v", err)
			}

			data, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("read report: %v", err)
			}

			compareSnapshot(t, normalizeHTML(string(data)), tt.name, ".html")
		})
	}
}

// --- WriteDashboard snapshot tests ---

func TestHTML_WriteDashboard_Snapshots(t *testing.T) {
	tests := []struct {
		name    string
		result  *model.RunResult
		version string
	}{
		{
			name: "dashboard_all_passed",
			result: &model.RunResult{
				SuiteResults: []model.SuiteResult{
					{
						Suite: model.Suite{Name: "auth-suite", FilePath: "tests/auth.chiperka"},
						TestResults: []model.TestResult{
							{Test: model.Test{Name: "login"}, Status: model.StatusPassed, Duration: 500 * time.Millisecond, UUID: "uuid-1"},
							{Test: model.Test{Name: "register"}, Status: model.StatusPassed, Duration: 800 * time.Millisecond, UUID: "uuid-2"},
							{Test: model.Test{Name: "logout"}, Status: model.StatusPassed, Duration: 200 * time.Millisecond, UUID: "uuid-3"},
						},
					},
				},
			},
			version: "1.0.0",
		},
		{
			name: "dashboard_mixed_statuses",
			result: &model.RunResult{
				SuiteResults: []model.SuiteResult{
					{
						Suite: model.Suite{Name: "api-suite", FilePath: "tests/api.chiperka"},
						TestResults: []model.TestResult{
							{Test: model.Test{Name: "get-users", Tags: []string{"api"}}, Status: model.StatusPassed, Duration: 300 * time.Millisecond, UUID: "uuid-a1"},
							{Test: model.Test{Name: "create-user"}, Status: model.StatusFailed, Duration: 500 * time.Millisecond, UUID: "uuid-a2"},
							{Test: model.Test{Name: "delete-user"}, Status: model.StatusError, Duration: 5 * time.Second, UUID: "uuid-a3"},
						},
					},
					{
						Suite: model.Suite{Name: "misc-suite", FilePath: "tests/misc.chiperka"},
						TestResults: []model.TestResult{
							{Test: model.Test{Name: "health-check"}, Status: model.StatusPassed, Duration: 100 * time.Millisecond, UUID: "uuid-b1"},
							{Test: model.Test{Name: "disabled-test"}, Status: model.StatusSkipped, Duration: 0, UUID: "uuid-b2"},
						},
					},
				},
			},
			version: "2.1.0",
		},
		{
			name:    "dashboard_empty",
			result:  &model.RunResult{},
			version: "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewHTMLWriter()
			dir := t.TempDir()

			err := w.WriteDashboard(tt.result, dir, tt.version)
			if err != nil {
				t.Fatalf("WriteDashboard: %v", err)
			}

			data, err := os.ReadFile(filepath.Join(dir, "index.html"))
			if err != nil {
				t.Fatalf("read dashboard: %v", err)
			}

			compareSnapshot(t, normalizeHTML(string(data)), tt.name, ".html")
		})
	}
}
