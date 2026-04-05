package telemetry

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Event is the telemetry payload sent to the server.
type Event struct {
	Event          string `json:"event"`
	Version        string `json:"version"`
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	CI             bool   `json:"ci"`
	TestsTotal     int    `json:"tests_total,omitempty"`
	TestsPassed    int    `json:"tests_passed,omitempty"`
	TestsFailed    int    `json:"tests_failed,omitempty"`
	TestsSkipped   int    `json:"tests_skipped,omitempty"`
	SuiteCount     int    `json:"suite_count,omitempty"`
	DurationMs     int64  `json:"duration_ms,omitempty"`
	WorkerCount    int    `json:"worker_count,omitempty"`
	CloudMode      bool   `json:"cloud_mode,omitempty"`
	HasHTMLReport  bool   `json:"has_html_report,omitempty"`
	HasJUnitReport bool   `json:"has_junit_report,omitempty"`
	HasArtifacts   bool   `json:"has_artifacts,omitempty"`
	HasTagsFilter  bool   `json:"has_tags_filter,omitempty"`
	HasNameFilter  bool   `json:"has_name_filter,omitempty"`
	ErrorType      string `json:"error_type,omitempty"`
}

// RunParams holds the context from cmd/run.go needed to build a telemetry event.
type RunParams struct {
	Version        string
	DurationMs     int64
	WorkerCount    int
	CloudMode      bool
	HasHTMLReport  bool
	HasJUnitReport bool
	HasArtifacts   bool
	HasTagsFilter  bool
	HasNameFilter  bool
}

var wg sync.WaitGroup

// IsDisabled checks whether telemetry is disabled via env vars or config file.
func IsDisabled() bool {
	if os.Getenv("DO_NOT_TRACK") == "1" {
		return true
	}
	cfg := LoadConfig()
	if cfg != nil && !cfg.Enabled {
		return true
	}
	return false
}

// ShowNoticeIfNeeded displays the first-run telemetry notice on stderr.
// Only shown once, then the config is saved. Suppressed in TeamCity mode.
func ShowNoticeIfNeeded(teamcityMode bool) {
	if teamcityMode {
		return
	}

	cfg := LoadConfig()
	if cfg != nil {
		// Config already exists — notice was shown or user made a choice
		return
	}

	// Show notice
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Notice: Chiperka collects anonymous usage stats — thank you for helping us improve this awesome tool! If you don't want to be part of it, you can disable it using `chiperka telemetry disable`.")
	fmt.Fprintln(os.Stderr, "")

	// Save config so notice isn't shown again
	SaveConfig(&TelemetryConfig{
		Enabled:     true,
		NoticeShown: true,
	})
}

// RecordRun records a completed test run. Fire-and-forget via goroutine.
func RecordRun(params RunParams, testsTotal, testsPassed, testsFailed, testsSkipped, suiteCount int) {
	if IsDisabled() {
		return
	}

	event := &Event{
		Event:          "run_completed",
		Version:        params.Version,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		CI:             isCI(),
		TestsTotal:     testsTotal,
		TestsPassed:    testsPassed,
		TestsFailed:    testsFailed,
		TestsSkipped:   testsSkipped,
		SuiteCount:     suiteCount,
		DurationMs:     params.DurationMs,
		WorkerCount:    params.WorkerCount,
		CloudMode:      params.CloudMode,
		HasHTMLReport:  params.HasHTMLReport,
		HasJUnitReport: params.HasJUnitReport,
		HasArtifacts:   params.HasArtifacts,
		HasTagsFilter:  params.HasTagsFilter,
		HasNameFilter:  params.HasNameFilter,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		send(params.Version, event)
	}()
}

// RecordError records an error event. Fire-and-forget via goroutine.
func RecordError(version, errType string) {
	if IsDisabled() {
		return
	}

	event := &Event{
		Event:     "error",
		Version:   version,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		CI:        isCI(),
		ErrorType: errType,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		send(version, event)
	}()
}

// ClassifyError maps an error to a category. Never sends raw error text.
func ClassifyError(err error) string {
	if err == nil {
		return "unknown"
	}

	msg := strings.ToLower(err.Error())

	switch {
	case strings.Contains(msg, "docker"):
		return "docker"
	case strings.Contains(msg, "parse") || strings.Contains(msg, "unmarshal") || strings.Contains(msg, "yaml"):
		return "parse_error"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return "timeout"
	case strings.Contains(msg, "no *.chiperka files") || strings.Contains(msg, "no test"):
		return "no_test_files"
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host"):
		return "network"
	case strings.Contains(msg, "permission denied"):
		return "permission"
	case strings.Contains(msg, "not found") || strings.Contains(msg, "does not exist"):
		return "not_found"
	case strings.Contains(msg, "cloud") || strings.Contains(msg, "api"):
		return "cloud"
	default:
		return "unknown"
	}
}

// Wait blocks until all pending telemetry sends complete or timeout expires.
func Wait(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// isCI detects whether the process is running in a CI environment.
func isCI() bool {
	ciVars := []string{
		"CI",
		"GITHUB_ACTIONS",
		"GITLAB_CI",
		"CIRCLECI",
		"TRAVIS",
		"JENKINS_URL",
		"CODEBUILD_BUILD_ID",
		"TF_BUILD",
		"BITBUCKET_PIPELINE",
		"BUILDKITE",
	}
	for _, v := range ciVars {
		if os.Getenv(v) != "" {
			return true
		}
	}
	return false
}

