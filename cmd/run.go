package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"chiperka-cli/internal/cloud"
	"chiperka-cli/internal/config"
	"chiperka-cli/internal/events"
	"chiperka-cli/internal/events/subscribers"
	"chiperka-cli/internal/finder"
	"chiperka-cli/internal/model"
	"chiperka-cli/internal/output"
	"chiperka-cli/internal/parser"
	"chiperka-cli/internal/runner"
	"chiperka-cli/internal/telemetry"
)

var junitOutput string
var htmlOutput string
var artifactsDir string
var regenerateSnapshots bool
var filterTags []string
var filterName string
var verboseOutput bool
var debugOutput bool
var testTimeout int
var workerCount int
var cloudMode bool
var cpuThreshold float64
var teamcityOutput bool
var jsonOutput bool
var pathMapping string
var configFile string
var cloudProject string

var runCmd = &cobra.Command{
	Use:   "run [path]",
	Short: "Run tests from chiperka.yaml files",
	Long: `Run discovers and executes tests defined in *.chiperka files.

The command walks through the specified directory (or current directory if not specified)
and finds all files matching the *.chiperka pattern. You can also specify a single
.chiperka file directly.

Use --tags to run only tests with specific tags. Multiple tags can be specified
(comma-separated or multiple flags). Tests matching ANY of the specified tags will run.

Use --filter to run only tests whose name matches the pattern. Supports glob patterns
with * wildcard. Without wildcards, performs substring match.

Use --configuration to specify a chiperka.yaml configuration file with shared service
definitions. If not specified, chiperka.yaml/chiperka.yml is auto-discovered in the
current working directory.

Output modes:
  --verbose    Show detailed logs (all events in logfmt format)
  --debug      Show docker commands being executed (implies --verbose)

Example:
  chiperka run ./tests
  chiperka run ./tests/auth.chiperka
  chiperka run .
  chiperka run
  chiperka run ./tests --tags smoke
  chiperka run ./tests --tags smoke,api
  chiperka run ./tests --filter "login*"
  chiperka run ./tests --filter "*authentication*"
  chiperka run ./tests --tags smoke --filter "user*"
  chiperka run ./tests --verbose
  chiperka run ./tests --debug
  chiperka run ./tests --configuration chiperka.yaml
  chiperka run ./tests --env-file .env
  chiperka run ./tests --env-file .env --env-file .env.local
  chiperka run ./tests --cloud                    # uses cloud.url from chiperka.yaml
  CHIPERKA_CLOUD_URL=http://ci.example.com chiperka run ./tests --cloud  # env override for CI/CD`,
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true, // Don't print usage on error - it's confusing in CI logs
	SilenceErrors: true,
	RunE:          runTests,
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringVar(&junitOutput, "junit", "", "Write JUnit XML report to file")
	runCmd.Flags().StringVar(&htmlOutput, "html", "", "Write HTML reports to directory")
	runCmd.Flags().StringVar(&artifactsDir, "artifacts", "./artifacts", "Directory for test artifacts")
	runCmd.Flags().BoolVar(&regenerateSnapshots, "regenerate-snapshots", false, "Update snapshot files instead of comparing them")
	runCmd.Flags().StringSliceVar(&filterTags, "tags", nil, "Run only tests with specified tags (comma-separated or multiple flags)")
	runCmd.Flags().StringVar(&filterName, "filter", "", "Run only tests whose name matches the pattern (supports * wildcard)")
	runCmd.Flags().BoolVar(&verboseOutput, "verbose", false, "Show detailed logs (all events)")
	runCmd.Flags().BoolVar(&debugOutput, "debug", false, "Show docker commands (implies --verbose)")
	runCmd.Flags().IntVar(&testTimeout, "timeout", 300, "Maximum time in seconds for each test execution")
	runCmd.Flags().IntVar(&workerCount, "workers", 0, "Number of parallel test workers (0 = auto-detect from CPU count)")
	runCmd.Flags().BoolVar(&cloudMode, "cloud", false, "Run tests on remote cloud server (configured via chiperka.yaml cloud.url or CHIPERKA_CLOUD_URL env)")
	runCmd.Flags().Float64Var(&cpuThreshold, "cpu-threshold", 0, "CPU load threshold (0.0-1.0) - pause test execution when exceeded (0 = disabled)")
	runCmd.Flags().BoolVar(&teamcityOutput, "teamcity", false, "Output TeamCity service messages for IDE integration")
	runCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output NDJSON for machine consumption")
	runCmd.Flags().StringVar(&pathMapping, "path-mapping", "", "Path prefix mapping for artifact paths (container=host, e.g. /srv/chiperka=/Users/me/project)")
	runCmd.Flags().StringVar(&configFile, "configuration", "", "Path to chiperka.yaml configuration file (auto-discovered if not set)")
	runCmd.Flags().StringVar(&cloudProject, "project", "", "Project slug for cloud runs (env: CHIPERKA_PROJECT, config: cloud.project)")
}

// runTests is the main entry point for the run command.
func runTests(cmd *cobra.Command, args []string) error {
	defer telemetry.Wait(4 * time.Second)

	// Determine the search path
	searchPath := "."
	if len(args) > 0 {
		searchPath = args[0]
	}

	// Verify path exists
	info, err := os.Stat(searchPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", searchPath)
	}

	// Find test files - either single file or directory search
	var files []string
	if !info.IsDir() && strings.HasSuffix(searchPath, ".chiperka") {
		// Single file specified
		files = []string{searchPath}
	} else {
		// Directory search
		f := finder.New(searchPath)
		files, err = f.FindTestFiles()
		if err != nil {
			return fmt.Errorf("failed to find test files: %w", err)
		}
	}

	if len(files) == 0 {
		fmt.Printf("No *.chiperka files found in %s\n", searchPath)
		return nil
	}

	// Parse all found files
	p := parser.New()
	parseResult := p.ParseAll(files)

	// Report any parse errors
	for _, err := range parseResult.Errors {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	// Load configuration file
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	services := cfg.ServiceTemplates()

	// Show telemetry notice on first run
	telemetry.ShowNoticeIfNeeded(teamcityOutput || jsonOutput)
	startTime := time.Now()

	// Create event bus and subscribers
	bus := events.NewBus()
	collector := subscribers.NewEventCollector()
	collector.Register(bus)

	// debug implies verbose
	if debugOutput {
		verboseOutput = true
	}

	// Set up output based on flags
	if teamcityOutput {
		// TeamCity mode: output service messages for IDE test runner
		tc := subscribers.NewTeamCityReporter(os.Stdout, artifactsDir, pathMapping, htmlOutput)
		tc.Register(bus)
	} else if jsonOutput {
		// JSON mode: NDJSON output for machine consumption
		jr := subscribers.NewJSONReporter(os.Stdout)
		jr.Register(bus)
	} else if verboseOutput {
		// Verbose mode: show all events in logfmt format
		verbose := subscribers.NewVerboseLogger(os.Stdout)
		verbose.Register(bus)

		if debugOutput {
			// Debug mode: also show docker commands
			debug := subscribers.NewDebugLogger(os.Stdout)
			debug.Register(bus)
		}
	} else {
		// Default mode: user-friendly progress output
		cli := subscribers.NewCLIReporter(os.Stdout)
		if cloudMode {
			cli.SetCloudMode(true)
		}
		cli.Register(bus)
	}

	// Create emitter for local logging
	emitter := events.NewEmitter(bus)

	// Log service template discovery
	if services.HasTemplates() {
		emitter.Info(events.Fields{
			"action": "service_discover",
			"count":  fmt.Sprintf("%d", len(services.Templates)),
			"msg":    fmt.Sprintf("Found %d service template(s)", len(services.Templates)),
		})
	}

	// Filter tests by tags if specified
	tests := parseResult.Tests
	if len(filterTags) > 0 {
		tests = tests.FilterByTags(filterTags)
		emitter.Info(events.Fields{
			"action": "tag_filter",
			"tags":   strings.Join(filterTags, ","),
			"count":  fmt.Sprintf("%d", tests.TotalTests()),
			"msg":    fmt.Sprintf("Filtered to %d test(s) matching tags: %s", tests.TotalTests(), strings.Join(filterTags, ", ")),
		})
		if tests.TotalTests() == 0 {
			fmt.Println("No tests match the specified tags")
			return nil
		}
	}

	// Filter tests by name pattern if specified
	if filterName != "" {
		tests = tests.FilterByName(filterName)
		emitter.Info(events.Fields{
			"action":  "name_filter",
			"pattern": filterName,
			"count":   fmt.Sprintf("%d", tests.TotalTests()),
			"msg":     fmt.Sprintf("Filtered to %d test(s) matching pattern: %s", tests.TotalTests(), filterName),
		})
		if tests.TotalTests() == 0 {
			fmt.Println("No tests match the specified filter pattern")
			return nil
		}
	}

	// Warn about flags that have no effect in cloud mode
	if cloudMode {
		ignoredInCloud := []string{
			"regenerate-snapshots",
			"timeout", "workers",
		}
		for _, flag := range ignoredInCloud {
			if cmd.Flags().Changed(flag) {
				fmt.Fprintf(os.Stderr, "Warning: --%s has no effect in cloud mode\n", flag)
			}
		}
	}

	// Cloud mode: upload tests to remote API and stream results
	if cloudMode {
		// Resolve cloud URL: chiperka.yaml > CHIPERKA_CLOUD_URL env
		cloudURL := ""
		if cfg != nil && cfg.Cloud.URL != "" {
			cloudURL = cfg.Cloud.URL
		}
		if envURL := os.Getenv("CHIPERKA_CLOUD_URL"); envURL != "" {
			cloudURL = envURL // env overrides config
		}
		if cloudURL == "" {
			fmt.Fprintln(os.Stderr, "Error: cloud URL not configured. Set 'cloud.url' in chiperka.yaml or CHIPERKA_CLOUD_URL environment variable.")
			os.Exit(1)
		}
		// Only download artifacts if --artifacts was explicitly set
		cloudArtifactsDir := ""
		if cmd.Flags().Changed("artifacts") {
			cloudArtifactsDir = artifactsDir
		}
		// Resolve project slug: --project flag > CHIPERKA_PROJECT env > chiperka.yaml cloud.project
		projectSlug := cloudProject
		if projectSlug == "" {
			projectSlug = os.Getenv("CHIPERKA_PROJECT")
		}
		if projectSlug == "" {
			projectSlug = cfg.Cloud.Project
		}
		err := runTestsCloud(cloudURL, tests, services, startTime, bus, emitter, cloudArtifactsDir, projectSlug)
		return err // runTestsCloud already wraps with ExitError
	}

	// Create report writers if output requested (needs to track time from start)
	var junitWriter *output.JUnitWriter
	if junitOutput != "" {
		junitWriter = output.NewJUnitWriter()
	}

	var htmlWriter *output.HTMLWriter
	if htmlOutput != "" {
		if err := os.RemoveAll(htmlOutput); err != nil {
			return fmt.Errorf("failed to clean HTML output directory: %w", err)
		}
		if err := os.MkdirAll(htmlOutput, 0o755); err != nil {
			return fmt.Errorf("failed to create HTML output directory: %w", err)
		}
		htmlWriter = output.NewHTMLWriter()
	}

	// Auto-detect worker count from CPU count if not specified
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}
	if workerCount < 1 {
		workerCount = 1
	}

	// Set up context with Ctrl+C handler for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\nInterrupted, cleaning up containers...\n")
		cancel()
		<-sigCh
		fmt.Fprintf(os.Stderr, "\nForce quit\n")
		os.Exit(1)
	}()
	defer signal.Stop(sigCh)

	// Run tests and output results
	r, err := runner.New(bus, workerCount, artifactsDir, services, regenerateSnapshots, testTimeout, Version, collector, cpuThreshold, cfg.ExecutionVariables)
	if err != nil {
		telemetry.RecordError(Version, "run", "", telemetry.ClassifyError(err))
		return fmt.Errorf("failed to create test runner: %w", err)
	}

	// Set per-test HTML callback so each test gets its own HTML file
	// immediately after completion (before the TeamCity event fires).
	if htmlWriter != nil {
		htmlDir := htmlOutput
		hw := htmlWriter
		r.SetOnTestComplete(func(result *model.TestResult, suiteName, suiteFilePath string) {
			if _, err := hw.WriteTestReport(result, suiteName, suiteFilePath, htmlDir); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to write test report for %s: %v\n", result.Test.Name, err)
			}
		})
	}

	result := r.Run(ctx, tests)

	// Record telemetry
	runStats := telemetry.CollectRunStats(tests, services)
	telemetry.RecordRun(telemetry.RunParams{
		Version:          Version,
		DurationMs:       time.Since(startTime).Milliseconds(),
		WorkerCount:      workerCount,
		ExecutorType:     runStats.ExecutorType,
		ServiceCount:     runStats.ServiceCount,
		HTMLReport:       htmlOutput != "",
		JUnitReport:      junitOutput != "",
		Artifacts:        artifactsDir != "./artifacts",
		TagsFilter:       len(filterTags) > 0,
		NameFilter:       filterName != "",
		Snapshots:        runStats.HasSnapshots,
		HasSetup:         runStats.HasSetup,
		HasTeardown:      runStats.HasTeardown,
		HasHooks:         runStats.HasHooks,
		ServiceTemplates: runStats.HasServiceTemplates,
		Verbose:          verboseOutput,
		Debug:            debugOutput,
	}, result.TotalTests(), result.TotalPassed(), result.TotalFailed(), result.TotalSkipped(), len(tests.Suites))

	// Get weblink prefix for CLI output (e.g., "http://localhost:8080/reports")
	weblink := os.Getenv("CHIPERKA_WEBLINK")
	if weblink != "" {
		weblink = strings.TrimSuffix(weblink, "/")
	}

	// Write JUnit report if requested
	if junitWriter != nil {
		if err := junitWriter.Write(result, junitOutput); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write JUnit report: %v\n", err)
		} else {
			fields := events.Fields{
				"action": "junit_write",
				"target": junitOutput,
				"msg":    "JUnit report written",
			}
			if weblink != "" {
				fields["url"] = weblink + "/" + filepath.Base(junitOutput)
			}
			emitter.Info(fields)
		}
	}

	// Write HTML dashboard (index.html with links to per-test reports)
	if htmlWriter != nil {
		if err := htmlWriter.WriteDashboard(result, htmlOutput, Version); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write HTML dashboard: %v\n", err)
		} else {
			target := filepath.Join(htmlOutput, "index.html")
			fields := events.Fields{
				"action": "html_write",
				"target": target,
				"msg":    "HTML report written",
			}
			if weblink != "" {
				fields["url"] = weblink + "/" + filepath.Base(htmlOutput) + "/index.html"
			}
			emitter.Info(fields)
		}
	}

	// Ensure all TeamCity service messages are flushed before process exits.
	// Docker relay may buffer stdout; sync forces kernel flush so IDE receives
	// testFinished/testSuiteFinished before processTerminated fires.
	if teamcityOutput || jsonOutput {
		os.Stdout.Sync()
	}



	// Determine exit code based on test results:
	//   exit 2 = infrastructure errors (service startup, healthcheck, setup, Docker failures)
	//   exit 1 = assertion failures (tests ran but assertions didn't pass)
	if result.TotalErrors() > 0 {
		return exitErrorf(ExitInfraError, "test run failed: %d test(s) errored, %d failed",
			result.TotalErrors(), result.TotalFailed()-result.TotalErrors())
	}
	if result.HasFailures() {
		return exitErrorf(ExitTestFailure, "test run failed: %d test(s) failed", result.TotalFailed())
	}

	return nil
}

// loadConfig loads the configuration file from --configuration flag or auto-discovers it.
func loadConfig() (*config.Config, error) {
	if configFile != "" {
		cfg, err := config.Load(configFile)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	}

	cfg, found := config.Discover()
	if !found {
		fmt.Fprintln(os.Stderr, "Starting without configuration file")
	}
	return cfg, nil
}

// resolveCloudToken resolves the auth token for cloud mode.
// Priority: CHIPERKA_TOKEN env var > ~/.chiperka/auth.json > empty string.
func resolveCloudToken(apiURL string) string {
	// 1. Environment variable takes priority
	if token := os.Getenv("CHIPERKA_TOKEN"); token != "" {
		return token
	}

	// 2. Try ./auth.json in current directory
	data, err := os.ReadFile("auth.json")
	if err != nil {
		return ""
	}

	var authFile struct {
		ChiperkaToken map[string]string `json:"chiperka-token"`
	}
	if err := json.Unmarshal(data, &authFile); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: auth.json is not valid JSON, ignoring\n")
		return ""
	}

	if authFile.ChiperkaToken == nil {
		return ""
	}

	// Extract host from apiURL
	parsed, err := url.Parse(apiURL)
	if err != nil {
		return ""
	}

	if token, ok := authFile.ChiperkaToken[parsed.Host]; ok {
		return token
	}

	return ""
}

// runTestsCloud uploads tests to a remote API server and streams results.
func runTestsCloud(apiURL string, tests *model.TestCollection, services *model.ServiceTemplateCollection, startTime time.Time, bus *events.Bus, emitter *events.Emitter, artifactsDir string, projectSlug string) error {
	token := resolveCloudToken(apiURL)
	client := cloud.NewClient(apiURL, token)

	// Health check
	if err := client.HealthCheck(); err != nil {
		telemetry.RecordError(Version, "run", "", telemetry.ClassifyError(err))
		return fmt.Errorf("cloud API not reachable at %s: %w\n\nHint: run without --cloud for local execution", apiURL, err)
	}

	// Resolve project slug to ID if specified
	var projectID *int64
	if projectSlug != "" {
		id, err := client.ResolveProject(projectSlug)
		if err != nil {
			return fmt.Errorf("failed to resolve project %q: %w", projectSlug, err)
		}
		projectID = &id
		emitter.Info(events.Fields{
			"action":  "project_resolve",
			"slug":    projectSlug,
			"project": fmt.Sprintf("%d", id),
			"msg":     fmt.Sprintf("Resolved project %q (ID: %d)", projectSlug, id),
		})
	}

	// Build submission with resolved service templates
	submission, err := cloud.BuildSubmission(tests, services, Version, projectID)
	if err != nil {
		telemetry.RecordError(Version, "run", "", telemetry.ClassifyError(err))
		return fmt.Errorf("failed to build submission: %w", err)
	}

	emitter.Info(events.Fields{
		"action": "cloud_upload",
		"msg":    fmt.Sprintf("Uploading %d tests to %s...", tests.TotalTests(), apiURL),
	})

	// Create run
	resp, err := client.CreateRun(submission)
	if err != nil {
		telemetry.RecordError(Version, "run", "", telemetry.ClassifyError(err))
		return fmt.Errorf("failed to create run: %w", err)
	}

	emitter.Info(events.Fields{
		"action": "cloud_run_created",
		"run_id": resp.ID,
		"msg":    fmt.Sprintf("Run created: %s", resp.ID),
	})

	// Collect and upload snapshot files
	snapshots, err := cloud.CollectSnapshotFiles(submission.Suites)
	if err != nil {
		return fmt.Errorf("failed to collect snapshot files: %w", err)
	}
	if len(snapshots) > 0 {
		var totalSize int64
		for _, content := range snapshots {
			totalSize += int64(len(content))
		}
		fmt.Fprintf(os.Stdout, "\nUploading local snapshots (%d files, %s)\n", len(snapshots), formatBytes(totalSize))
		uploadStart := time.Now()
		if err := client.UploadSnapshots(resp.ID, snapshots); err != nil {
			return fmt.Errorf("failed to upload snapshots: %w", err)
		}
		fmt.Fprintf(os.Stdout, "  Uploaded in %s\n\n", time.Since(uploadStart).Round(time.Millisecond))
	}

	// Set up context with Ctrl+C handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\nInterrupted, stopping run...\n")
		if err := client.StopRun(resp.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to stop run: %v\n", err)
		}
		// Don't cancel context yet — let the SSE stream receive run_cancelled naturally
		// Second signal: force quit immediately
		<-sigCh
		fmt.Fprintf(os.Stderr, "\nForce quit.\n")
		os.Exit(1)
	}()

	// Stream results — events go through the bus to all registered reporters
	result, err := client.StreamRun(ctx, resp.ID, bus)
	if err != nil {
		telemetry.RecordError(Version, "run", "", telemetry.ClassifyError(err))
		if ctx.Err() != nil {
			return exitErrorf(ExitCancelled, "run cancelled")
		}
		return fmt.Errorf("stream error: %w", err)
	}

	// Download HTML report if requested
	if htmlOutput != "" {
		if err := os.RemoveAll(htmlOutput); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to clean HTML output directory: %v\n", err)
		}
		if err := os.MkdirAll(htmlOutput, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create HTML output directory: %v\n", err)
		} else if err := client.DownloadHTMLReportZip(resp.ID, htmlOutput); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to download HTML report: %v\n", err)
		} else {
			emitter.Info(events.Fields{
				"action": "html_download",
				"target": filepath.Join(htmlOutput, "index.html"),
				"msg":    fmt.Sprintf("HTML report written to %s", filepath.Join(htmlOutput, "index.html")),
			})
		}
	}

	// Download JUnit report if requested
	if junitOutput != "" {
		if err := client.DownloadReport(resp.ID, "xml", junitOutput); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to download JUnit report: %v\n", err)
		} else {
			emitter.Info(events.Fields{
				"action": "junit_download",
				"target": junitOutput,
				"msg":    fmt.Sprintf("JUnit report written to %s", junitOutput),
			})
		}
	}

	// Download artifacts if requested
	if artifactsDir != "" {
		if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create artifacts directory: %v\n", err)
		} else if err := client.DownloadArtifactsZip(resp.ID, artifactsDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to download artifacts: %v\n", err)
		} else {
			emitter.Info(events.Fields{
				"action": "artifacts_download",
				"target": artifactsDir,
				"msg":    fmt.Sprintf("Artifacts downloaded to %s", artifactsDir),
			})
		}
	}

	// Record telemetry
	total := result.Passed + result.Failed + result.Skipped
	cloudRunStats := telemetry.CollectRunStats(tests, services)
	telemetry.RecordRun(telemetry.RunParams{
		Version:          Version,
		DurationMs:       time.Since(startTime).Milliseconds(),
		CloudMode:        true,
		WorkerCount:      workerCount,
		ExecutorType:     cloudRunStats.ExecutorType,
		ServiceCount:     cloudRunStats.ServiceCount,
		HTMLReport:       htmlOutput != "",
		JUnitReport:      junitOutput != "",
		Artifacts:        artifactsDir != "",
		TagsFilter:       len(filterTags) > 0,
		NameFilter:       filterName != "",
		Snapshots:        cloudRunStats.HasSnapshots,
		HasSetup:         cloudRunStats.HasSetup,
		HasTeardown:      cloudRunStats.HasTeardown,
		HasHooks:         cloudRunStats.HasHooks,
		ServiceTemplates: cloudRunStats.HasServiceTemplates,
		Verbose:          verboseOutput,
		Debug:            debugOutput,
	}, total, result.Passed, result.Failed, result.Skipped, len(tests.Suites))

	// Print link
	fmt.Printf("\n  %s/runs/%s\n", strings.TrimSuffix(apiURL, "/"), resp.ID)

	if result.Cancelled {
		return exitErrorf(ExitCancelled, "run cancelled")
	}

	if result.HasFailures() {
		return exitErrorf(ExitTestFailure, "test run failed: %d test(s) failed", result.Failed)
	}

	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMG"[exp])
}
