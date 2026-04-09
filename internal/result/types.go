package result

import "time"

// RunSummary is the top-level result stored in run.json.
// Contains just enough info for AI to decide whether to drill down.
type RunSummary struct {
	UUID      string    `json:"uuid"`
	Status    string    `json:"status"` // "passed", "failed", "error"
	StartedAt time.Time `json:"started_at"`
	Duration  int64     `json:"duration_ms"`
	Passed    int       `json:"passed"`
	Failed    int       `json:"failed"`
	Errored   int       `json:"errored"`
	Skipped   int       `json:"skipped"`
	Total     int       `json:"total"`
	Tests     []TestRef `json:"tests"`
}

// TestRef is a lightweight reference to a test within a run.
type TestRef struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Suite    string `json:"suite"`
	Status   string `json:"status"`
	Duration int64  `json:"duration_ms"`
}

// TestDetail is the full test result stored in test.json.
// Contains everything except raw artifact content.
type TestDetail struct {
	UUID          string             `json:"uuid"`
	Name          string             `json:"name"`
	Suite         string             `json:"suite"`
	Status        string             `json:"status"`
	Duration      int64              `json:"duration_ms"`
	Error         string             `json:"error,omitempty"`
	Assertions    []AssertionDetail  `json:"assertions"`
	HTTPExchanges []HTTPExchangeJSON `json:"http_exchanges,omitempty"`
	CLIExecutions []CLIExecutionJSON `json:"cli_executions,omitempty"`
	Services      []ServiceDetail    `json:"services,omitempty"`
	Setup         []StepDetail       `json:"setup,omitempty"`
	Teardown      []StepDetail       `json:"teardown,omitempty"`
	Artifacts     []ArtifactRef      `json:"artifacts"`
	Phases        PhaseBreakdown     `json:"phases"`
}

// AssertionDetail is a single assertion result.
type AssertionDetail struct {
	Passed   bool   `json:"passed"`
	Type     string `json:"type"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Message  string `json:"message"`
}

// HTTPExchangeJSON captures a full HTTP request/response.
type HTTPExchangeJSON struct {
	Phase          string            `json:"phase"`
	Sequence       int               `json:"sequence"`
	Method         string            `json:"method"`
	URL            string            `json:"url"`
	RequestHeaders map[string]string `json:"request_headers,omitempty"`
	RequestBody    string            `json:"request_body,omitempty"`
	StatusCode     int               `json:"status_code"`
	ResponseBody   string            `json:"response_body,omitempty"`
	Duration       int64             `json:"duration_ms"`
	Error          string            `json:"error,omitempty"`
}

// CLIExecutionJSON captures a CLI command execution.
type CLIExecutionJSON struct {
	Phase      string `json:"phase"`
	Sequence   int    `json:"sequence"`
	Service    string `json:"service"`
	Command    string `json:"command"`
	WorkingDir string `json:"working_dir,omitempty"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	Duration   int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// ServiceDetail captures service startup information.
type ServiceDetail struct {
	Name     string `json:"name"`
	Image    string `json:"image"`
	Duration int64  `json:"duration_ms"`
}

// StepDetail captures a setup or teardown step result.
type StepDetail struct {
	Type     string `json:"type"` // "http" or "cli"
	Duration int64  `json:"duration_ms"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// ArtifactRef is a reference to an artifact file.
type ArtifactRef struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// PhaseBreakdown shows timing for each test phase.
type PhaseBreakdown struct {
	Network   int64 `json:"network_ms"`
	Services  int64 `json:"services_ms"`
	Setup     int64 `json:"setup_ms"`
	Execution int64 `json:"execution_ms"`
	Assertion int64 `json:"assertion_ms"`
	Teardown  int64 `json:"teardown_ms"`
	Cleanup   int64 `json:"cleanup_ms"`
}
