package telemetry

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Event is the telemetry payload sent to the server.
type Event struct {
	// Identity
	InstallID string `json:"install_id"`
	Version   string `json:"version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`

	// Command
	Command    string `json:"command"`
	Success    bool   `json:"success"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	ErrorType  string `json:"error_type,omitempty"`

	// Environment
	CI         bool   `json:"ci"`
	CIProvider string `json:"ci_provider,omitempty"`
	RuntimeEnv string `json:"runtime_env,omitempty"` // native, docker, docker_compose

	// Run-specific
	TestsTotal   int `json:"tests_total,omitempty"`
	TestsPassed  int `json:"tests_passed,omitempty"`
	TestsFailed  int `json:"tests_failed,omitempty"`
	TestsSkipped int `json:"tests_skipped,omitempty"`
	SuiteCount   int `json:"suite_count,omitempty"`
	ServiceCount int `json:"service_count,omitempty"`
	WorkerCount  int `json:"worker_count,omitempty"`

	// Executor
	ExecutorType string `json:"executor_type,omitempty"` // http, cli, mixed

	// Feature flags
	CloudMode          bool `json:"cloud_mode,omitempty"`
	FlagHTMLReport     bool `json:"flag_html_report,omitempty"`
	FlagJUnitReport    bool `json:"flag_junit_report,omitempty"`
	FlagArtifacts      bool `json:"flag_artifacts,omitempty"`
	FlagTagsFilter     bool `json:"flag_tags_filter,omitempty"`
	FlagNameFilter     bool `json:"flag_name_filter,omitempty"`
	FlagSnapshots      bool `json:"flag_snapshots,omitempty"`
	FlagSetup          bool `json:"flag_setup,omitempty"`
	FlagTeardown       bool `json:"flag_teardown,omitempty"`
	FlagHooks          bool `json:"flag_hooks,omitempty"`
	FlagServiceTempls  bool `json:"flag_service_templates,omitempty"`
	FlagVerbose        bool `json:"flag_verbose,omitempty"`
	FlagDebug          bool `json:"flag_debug,omitempty"`
}

// RunParams holds the context from cmd/run.go needed to build a telemetry event.
type RunParams struct {
	Version      string
	DurationMs   int64
	WorkerCount  int
	CloudMode    bool
	ExecutorType string
	ServiceCount int

	// Feature flags
	HTMLReport       bool
	JUnitReport      bool
	Artifacts        bool
	TagsFilter       bool
	NameFilter       bool
	Snapshots        bool
	HasSetup         bool
	HasTeardown      bool
	HasHooks         bool
	ServiceTemplates bool
	Verbose          bool
	Debug            bool
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
		return
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Chiperka collects anonymous usage data (commands, OS, test counts, feature flags)")
	fmt.Fprintln(os.Stderr, "to help us improve the tool. No personal information is collected.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  Disable:  chiperka telemetry disable")
	fmt.Fprintln(os.Stderr, "  Details:  https://chiperka.com/privacy")
	fmt.Fprintln(os.Stderr, "")

	SaveConfig(&TelemetryConfig{
		Enabled:     true,
		NoticeShown: true,
	})
}

// RecordCommand records any CLI command invocation. Fire-and-forget.
func RecordCommand(version, command string, success bool, durationMs int64) {
	if IsDisabled() {
		return
	}

	event := baseEvent(version)
	event.Command = command
	event.Success = success
	event.DurationMs = durationMs

	wg.Add(1)
	go func() {
		defer wg.Done()
		send(version, event)
	}()
}

// RecordRun records a completed test run. Fire-and-forget.
func RecordRun(params RunParams, testsTotal, testsPassed, testsFailed, testsSkipped, suiteCount int) {
	if IsDisabled() {
		return
	}

	event := baseEvent(params.Version)
	event.Command = "run"
	event.Success = testsFailed == 0
	event.DurationMs = params.DurationMs

	// Run-specific
	event.TestsTotal = testsTotal
	event.TestsPassed = testsPassed
	event.TestsFailed = testsFailed
	event.TestsSkipped = testsSkipped
	event.SuiteCount = suiteCount
	event.ServiceCount = params.ServiceCount
	event.WorkerCount = params.WorkerCount
	event.ExecutorType = params.ExecutorType

	// Feature flags
	event.CloudMode = params.CloudMode
	event.FlagHTMLReport = params.HTMLReport
	event.FlagJUnitReport = params.JUnitReport
	event.FlagArtifacts = params.Artifacts
	event.FlagTagsFilter = params.TagsFilter
	event.FlagNameFilter = params.NameFilter
	event.FlagSnapshots = params.Snapshots
	event.FlagSetup = params.HasSetup
	event.FlagTeardown = params.HasTeardown
	event.FlagHooks = params.HasHooks
	event.FlagServiceTempls = params.ServiceTemplates
	event.FlagVerbose = params.Verbose
	event.FlagDebug = params.Debug

	wg.Add(1)
	go func() {
		defer wg.Done()
		send(params.Version, event)
	}()
}

// RecordError records an error event. Fire-and-forget.
func RecordError(version, command, errType string) {
	if IsDisabled() {
		return
	}

	event := baseEvent(version)
	event.Command = command
	event.Success = false
	event.ErrorType = errType

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

// baseEvent creates an event with common fields pre-filled.
func baseEvent(version string) *Event {
	return &Event{
		InstallID:  GetInstallID(),
		Version:    version,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		CI:         isCI(),
		CIProvider: detectCIProvider(),
		RuntimeEnv: detectRuntimeEnv(),
	}
}

// isCI detects whether the process is running in a CI environment.
func isCI() bool {
	return detectCIProvider() != ""
}

// detectCIProvider returns the CI provider name or empty string.
func detectCIProvider() string {
	providers := []struct {
		env  string
		name string
	}{
		{"GITHUB_ACTIONS", "github_actions"},
		{"GITLAB_CI", "gitlab_ci"},
		{"CIRCLECI", "circleci"},
		{"TRAVIS", "travis"},
		{"JENKINS_URL", "jenkins"},
		{"CODEBUILD_BUILD_ID", "codebuild"},
		{"TF_BUILD", "azure_devops"},
		{"BITBUCKET_PIPELINE", "bitbucket"},
		{"BUILDKITE", "buildkite"},
		{"DRONE", "drone"},
		{"TEAMCITY_VERSION", "teamcity"},
	}
	for _, p := range providers {
		if os.Getenv(p.env) != "" {
			return p.name
		}
	}
	if os.Getenv("CI") != "" {
		return "unknown_ci"
	}
	return ""
}

// detectRuntimeEnv detects if running inside Docker or natively.
func detectRuntimeEnv() string {
	if os.Getenv("COMPOSE_PROJECT_NAME") != "" {
		return "docker_compose"
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "docker"
	}
	// cgroup check for Docker (Linux)
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		if strings.Contains(string(data), "docker") || strings.Contains(string(data), "containerd") {
			return "docker"
		}
	}
	return "native"
}

// GetInstallID returns a persistent anonymous install ID.
// Generated once on first call, stored in ~/.chiperka/config.json.
func GetInstallID() string {
	mcfg := LoadMachineConfig()
	if mcfg != nil && mcfg.InstallID != "" {
		return mcfg.InstallID
	}

	id := generateID()

	// Save to config
	cfg := LoadConfig()
	if cfg == nil {
		cfg = &TelemetryConfig{Enabled: true, NoticeShown: false}
	}
	saveMachineConfigWithInstallID(cfg, id)

	return id
}

// generateID creates a random 16-byte hex string.
func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
