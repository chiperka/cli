// Package runner executes test suites and outputs results.
package runner

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"spark-cli/internal/artifact"
	"spark-cli/internal/assertion"
	"spark-cli/internal/docker"
	"spark-cli/internal/events"
	"spark-cli/internal/events/subscribers"
	"spark-cli/internal/executor"
	"spark-cli/internal/model"
)

// generateUUID generates a random UUID v4.
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// testJob represents a single test to be executed by a worker.
type testJob struct {
	suiteIndex    int
	testIndex     int
	suiteName     string
	suiteFilePath string
	test          model.Test
}

// testResultWithIndex holds result with position info for ordering.
type testResultWithIndex struct {
	suiteIndex int
	testIndex  int
	result     model.TestResult
}

// Runner executes tests and outputs results.
type Runner struct {
	events              *events.Emitter
	eventCollector      *subscribers.EventCollector
	evaluator           *assertion.Evaluator
	workerCount         int
	collector           *artifact.Collector
	regenerateSnapshots bool
	serviceTemplates    *model.ServiceTemplateCollection
	testTimeoutSec      int
	version             string
	networkPool         *docker.NetworkPool
	cpuThreshold        float64
	executionVars       []string
	onTestComplete      func(result *model.TestResult, suiteName, suiteFilePath string)
}

// SetOnTestComplete registers a callback invoked immediately after each test
// finishes (with log entries populated) but before the TestCompleted/TestFailed
// event is emitted.  This allows writing per-test HTML reports before the
// TeamCity reporter outputs the testFinished service message.
func (r *Runner) SetOnTestComplete(fn func(result *model.TestResult, suiteName, suiteFilePath string)) {
	r.onTestComplete = fn
}

// New creates a new Runner with the given event bus.
func New(bus *events.Bus, workerCount int, artifactsDir string, services *model.ServiceTemplateCollection, regenerateSnapshots bool, testTimeoutSec int, version string, eventCollector *subscribers.EventCollector, cpuThreshold float64, executionVariables map[string]string) (*Runner, error) {
	collector, err := artifact.NewCollector(artifactsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create artifact collector: %w", err)
	}

	emitter := events.NewEmitter(bus)

	// Convert execution variables map to env slice ("KEY=VALUE")
	var execVars []string
	for k, v := range executionVariables {
		execVars = append(execVars, fmt.Sprintf("%s=%s", k, v))
	}

	return &Runner{
		events:              emitter,
		eventCollector:      eventCollector,
		evaluator:           assertion.NewEvaluator(),
		workerCount:         workerCount,
		collector:           collector,
		regenerateSnapshots: regenerateSnapshots,
		serviceTemplates:    services,
		testTimeoutSec:      testTimeoutSec,
		version:             version,
		cpuThreshold:        cpuThreshold,
		executionVars:       execVars,
	}, nil
}

// emit returns the test-scoped emitter from ctx if available, otherwise the global emitter.
func (r *Runner) emit(ctx context.Context) *events.Emitter {
	if e := events.EmitterFromContext(ctx); e != nil {
		return e
	}
	return r.events
}

// Run executes all tests in the collection and returns the result.
// The context is used for graceful shutdown — cancelling it stops claiming new
// tests and aborts in-flight ones, while container cleanup still runs.
func (r *Runner) Run(ctx context.Context, collection *model.TestCollection) *model.RunResult {
	startTime := time.Now()

	// Clean up stale networks from previous runs before starting
	docker.CleanupStaleNetworks()

	// Pre-warm all Docker images in parallel (pull if not present)
	images := r.collectAllImages(collection)
	if pulled := docker.PrewarmImages(ctx, images); pulled > 0 {
		r.events.Info(events.Fields{
			"action": "images_prewarm",
			"pulled": fmt.Sprintf("%d", pulled),
			"msg":    fmt.Sprintf("Pre-warmed %d Docker images", pulled),
		})
	}

	// Create network pool for test execution
	// Pool size equals worker count to ensure each worker can have a network
	r.networkPool = docker.NewNetworkPool(r.workerCount)
	defer r.networkPool.Close()

	// Limit concurrent container creation to avoid overwhelming the Docker daemon
	docker.SetMaxConcurrentContainers(r.workerCount)

	// Set per-service concurrency limits from service templates
	if r.serviceTemplates != nil {
		limits := make(map[string]int)
		for _, tmpl := range r.serviceTemplates.Templates {
			if tmpl.MaxInstances > 0 {
				limits[tmpl.Name] = tmpl.MaxInstances
			}
		}
		docker.SetServiceLimits(limits)
	}

	// Build job list and suite counts for IDE lifecycle tracking
	var jobs []testJob
	suiteCounts := make(map[string]int)
	for suiteIdx, suite := range collection.Suites {
		suiteCounts[suite.Name] = len(suite.Tests)
		for testIdx, test := range suite.Tests {
			jobs = append(jobs, testJob{
				suiteIndex:    suiteIdx,
				testIndex:     testIdx,
				suiteName:     suite.Name,
				suiteFilePath: suite.FilePath,
				test:          test,
			})
		}
	}

	// Emit run started event (includes per-suite test counts for IDE)
	r.events.RunStarted(collection.TotalTests(), len(collection.Suites), r.workerCount, r.version, suiteCounts)

	r.events.Info(events.Fields{
		"action": "run_start",
		"msg":    fmt.Sprintf("Starting test run with %d workers", r.workerCount),
	})
	results := r.executeParallel(ctx, jobs)

	// Organize results by suite
	runResult := r.organizeResults(collection, results)

	// Emit run completed event
	r.events.RunCompleted(
		time.Since(startTime),
		runResult.TotalPassed(),
		runResult.TotalFailed()+runResult.TotalErrors(),
		runResult.TotalSkipped(),
	)



	return runResult
}

// executeParallel runs tests using a worker pool.
func (r *Runner) executeParallel(ctx context.Context, jobs []testJob) []testResultWithIndex {
	if len(jobs) == 0 {
		return nil
	}

	jobChan := make(chan testJob, len(jobs))
	resultChan := make(chan testResultWithIndex, len(jobs))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < r.workerCount; i++ {
		wg.Add(1)
		go r.worker(ctx, &wg, jobChan, resultChan)
	}

	// Send jobs
	for _, job := range jobs {
		jobChan <- job
	}
	close(jobChan)

	// Wait for workers to finish
	wg.Wait()
	close(resultChan)

	// Collect results
	var results []testResultWithIndex
	for result := range resultChan {
		results = append(results, result)
	}

	return results
}

// getSystemLoadAverage returns the 1-minute load average normalized by CPU count.
// Returns a value between 0.0 and 1.0+ where 1.0 means 100% CPU utilization.
func getSystemLoadAverage() (float64, error) {
	numCPU := float64(runtime.NumCPU())

	// Try to read from /proc/loadavg (Linux)
	file, err := os.Open("/proc/loadavg")
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		if scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) >= 1 {
				load1, err := strconv.ParseFloat(fields[0], 64)
				if err == nil {
					return load1 / numCPU, nil
				}
			}
		}
	}

	// Fallback for macOS/BSD: /proc/loadavg doesn't exist
	// Return 0 (allow execution) - CPU backpressure only works on Linux
	return 0, nil
}

// isCPUOverloaded checks if the system CPU is above the threshold.
func (r *Runner) isCPUOverloaded() bool {
	if r.cpuThreshold <= 0 {
		return false // disabled
	}
	load, err := getSystemLoadAverage()
	if err != nil {
		return false // on error, allow work
	}
	return load > r.cpuThreshold
}

// waitForCPU blocks until CPU load drops below the threshold.
func (r *Runner) waitForCPU() {
	logged := false
	for r.isCPUOverloaded() {
		if !logged {
			load, _ := getSystemLoadAverage()
			r.events.Info(events.Fields{
				"action": "cpu_backpressure",
				"load":   fmt.Sprintf("%.0f%%", load*100),
				"msg":    fmt.Sprintf("CPU overloaded (%.0f%%), waiting...", load*100),
			})
			logged = true
		}
		time.Sleep(2 * time.Second)
	}
}

// worker processes test jobs from the channel.
func (r *Runner) worker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan testJob, results chan<- testResultWithIndex) {
	defer wg.Done()

	for job := range jobs {
		// Skip remaining jobs if context was cancelled (Ctrl+C)
		if ctx.Err() != nil {
			skipEvents := r.events.ForTest(job.suiteName, job.test.Name)
			skipEvents.SetFilePath(job.suiteFilePath)
			skipEvents.TestSkipped("Interrupted")

			results <- testResultWithIndex{
				suiteIndex: job.suiteIndex,
				testIndex:  job.testIndex,
				result: model.TestResult{
					Test:   job.test,
					Status: model.StatusSkipped,
					UUID:   generateUUID(),
				},
			}
			continue
		}

		// Wait if CPU is overloaded
		r.waitForCPU()
		// Generate unique UUID for this test run
		testUUID := generateUUID()

		// Create test-scoped event emitter with UUID and file path
		testEvents := r.events.ForTest(job.suiteName, job.test.Name)
		testEvents.SetUUID(testUUID)
		testEvents.SetFilePath(job.suiteFilePath)

		// Handle skipped tests
		if job.test.Skipped {
			testEvents.TestSkipped("Test marked as skipped")

			r.events.Skip(events.Fields{
				"test":   job.test.Name,
				"suite":  job.suiteName,
				"action": "test_skipped",
				"uuid":   testUUID,
				"msg":    "Test skipped",
			})

			results <- testResultWithIndex{
				suiteIndex: job.suiteIndex,
				testIndex:  job.testIndex,
				result: model.TestResult{
					Test:   job.test,
					Status: model.StatusSkipped,
					UUID:   testUUID,
				},
			}
			continue
		}

		// Emit test started event
		testEvents.TestStarted()

		testEvents.Info(events.Fields{
			"test":   job.test.Name,
			"suite":  job.suiteName,
			"action": "test_start",
			"uuid":   testUUID,
			"msg":    "Starting test execution",
		})

		startTime := time.Now()
		result := r.runTestWithUUID(ctx, job.test, testUUID, testEvents, job.suiteFilePath)
		result.Duration = time.Since(startTime)
		result.UUID = testUUID

		// Collect log entries BEFORE emitting completion events so that the
		// onTestComplete callback (which writes per-test HTML) has access to
		// the full log.  The TestCompleted/TestFailed event hasn't been
		// emitted yet, so we add a synthetic completion entry manually.
		if r.eventCollector != nil {
			collectedEvents := r.eventCollector.EventsForTest(job.suiteName, job.test.Name)
			for _, e := range collectedEvents {
				level := "info"
				action := ""
				service := ""
				message := e.Data.Message

				switch e.Type {
				case events.LogPass:
					level = "pass"
				case events.LogFail:
					level = "fail"
				case events.LogWarn:
					level = "warn"
				case events.TestStarted:
					action = "test_started"
					message = "Test started"
				case events.TestCompleted:
					level = "pass"
					action = "test_completed"
					message = "Test completed"
				case events.TestFailed, events.TestError:
					level = "fail"
					action = "test_failed"
					if message == "" {
						message = "Test failed"
					}
				case events.TestSkipped:
					level = "warn"
					action = "test_skipped"
					if message == "" {
						message = "Test skipped"
					}
				case events.TestCleanup:
					action = "test_cleanup"
					if message == "" {
						message = "Cleanup completed"
					}
				case events.TestServiceStarted:
					action = "service_started"
				case events.TestServiceReady:
					action = "service_ready"
				case events.TestHealthCheck:
					action = "healthcheck"
				}

				if a, ok := e.Data.Details["action"].(string); ok && a != "" {
					action = a
				}
				if s, ok := e.Data.Details["service"].(string); ok {
					service = s
				}

				// Skip events with no useful information
				if action == "" && message == "" {
					continue
				}

				result.LogEntries = append(result.LogEntries, model.LogEntry{
					RelativeTime: fmt.Sprintf("%.3fs", e.Timestamp.Sub(startTime).Seconds()),
					Level:        level,
					Action:       action,
					Service:      service,
					Message:      message,
				})
			}

			// Synthetic completion log entry (the real event hasn't fired yet)
			completionLevel := "pass"
			completionAction := "test_completed"
			completionMsg := "Test completed"
			if result.Status == model.StatusFailed {
				completionLevel = "fail"
				completionAction = "test_failed"
				completionMsg = "Test failed"
			} else if result.Status == model.StatusError {
				completionLevel = "fail"
				completionAction = "test_failed"
				completionMsg = "Test error"
				if result.Error != nil {
					completionMsg = result.Error.Error()
				}
			}
			result.LogEntries = append(result.LogEntries, model.LogEntry{
				RelativeTime: fmt.Sprintf("%.3fs", result.Duration.Seconds()),
				Level:        completionLevel,
				Action:       completionAction,
				Message:      completionMsg,
			})
		}

		// Call per-test callback (e.g. write per-test HTML report) BEFORE
		// emitting the completion event so the file exists when the TeamCity
		// reporter outputs the testFinished message.
		if r.onTestComplete != nil {
			r.onTestComplete(&result, job.suiteName, job.suiteFilePath)
		}

		// Emit test completion event and log result
		if result.Status == model.StatusPassed {
			testEvents.TestCompleted(result.Duration)

			testEvents.Pass(events.Fields{
				"test":     job.test.Name,
				"suite":    job.suiteName,
				"action":   "test_complete",
				"uuid":     testUUID,
				"duration": fmt.Sprintf("%.3fs", result.Duration.Seconds()),
				"msg":      "Test passed",
			})
		} else if result.Status == model.StatusFailed {
			msg := "Test failed"
			for _, ar := range result.AssertionResults {
				if !ar.Passed {
					msg = ar.Message
					break
				}
			}
			testEvents.TestFailed(result.Duration, msg)

			testEvents.Fail(events.Fields{
				"test":     job.test.Name,
				"suite":    job.suiteName,
				"action":   "test_complete",
				"uuid":     testUUID,
				"duration": fmt.Sprintf("%.3fs", result.Duration.Seconds()),
				"msg":      msg,
			})
		} else {
			errMsg := "Unknown error"
			if result.Error != nil {
				errMsg = result.Error.Error()
			}
			testEvents.TestFailed(result.Duration, errMsg)

			testEvents.Fail(events.Fields{
				"test":     job.test.Name,
				"suite":    job.suiteName,
				"action":   "test_error",
				"uuid":     testUUID,
				"duration": fmt.Sprintf("%.3fs", result.Duration.Seconds()),
				"msg":      errMsg,
			})
		}

		results <- testResultWithIndex{
			suiteIndex: job.suiteIndex,
			testIndex:  job.testIndex,
			result:     result,
		}
	}
}

// runTestWithUUID executes a single test with a UUID and returns the result.
func (r *Runner) runTestWithUUID(ctx context.Context, test model.Test, uuid string, testEmitter *events.Emitter, suiteFilePath string) model.TestResult {
	// Create timeout context for this test, derived from parent for cancellation
	timeout := time.Duration(r.testTimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Embed test-scoped emitter in context — all downstream methods use r.emit(ctx)
	ctx = events.WithEmitter(ctx, testEmitter)

	// Services are required for all tests
	if len(test.Services) == 0 {
		return model.TestResult{
			Test:   test,
			UUID:   uuid,
			Status: model.StatusError,
			Error:  fmt.Errorf("test %q has no services defined — at least one service is required", test.Name),
		}
	}

	return r.runTestWithServices(ctx, test, uuid, suiteFilePath)
}

// resolveServiceReferences resolves all service references in a test.
// Returns the test with resolved services, or an error if any reference cannot be resolved.
func (r *Runner) resolveServiceReferences(ctx context.Context, test model.Test) (model.Test, error) {
	if r.serviceTemplates == nil || !r.serviceTemplates.HasTemplates() {
		return test, nil
	}

	resolvedServices := make([]model.Service, len(test.Services))
	for i, svc := range test.Services {
		resolved, err := r.serviceTemplates.ResolveService(svc)
		if err != nil {
			return test, err
		}
		if svc.Ref != "" {
			r.emit(ctx).Info(events.Fields{
				"test":     test.Name,
				"action":   "service_resolve",
				"ref":      svc.Ref,
				"resolved": resolved.Name,
				"msg":      fmt.Sprintf("Resolved service reference '%s' to '%s'", svc.Ref, resolved.Name),
			})
		}
		resolvedServices[i] = resolved
	}
	test.Services = resolvedServices
	return test, nil
}

// runTestWithServices executes a test with Docker service lifecycle management.
func (r *Runner) runTestWithServices(ctx context.Context, test model.Test, uuid string, suiteFilePath string) (result model.TestResult) {

	// Resolve service references first
	resolvedTest, err := r.resolveServiceReferences(ctx, test)
	if err != nil {
		return model.TestResult{
			Test:   test,
			Status: model.StatusError,
			Error:  fmt.Errorf("failed to resolve service references: %w", err),
			UUID:   uuid,
		}
	}
	test = resolvedTest

	// Validate services have an image
	for _, svc := range test.Services {
		if svc.Image == "" {
			return model.TestResult{
				Test:   test,
				Status: model.StatusError,
				Error:  fmt.Errorf("service '%s' must have 'image' specified", svc.Name),
				UUID:   uuid,
			}
		}
	}

	// Initialize result
	result = model.TestResult{
		Test:   test,
		Status: model.StatusPassed,
		UUID:   uuid,
	}

	// Create Docker manager with network from pool
	networkStart := time.Now()
	var dockerManager *docker.Manager
	if r.networkPool != nil {
		dockerManager, err = docker.NewManagerWithPool(r.emit(ctx), test.Name, r.networkPool)
	} else {
		dockerManager, err = docker.NewManager(r.emit(ctx), test.Name)
	}
	networkDuration := time.Since(networkStart)
	if err != nil {
		result.Status = model.StatusError
		result.Error = fmt.Errorf("failed to create Docker manager: %w", err)
		return result
	}

	// Ensure cleanup happens and artifacts are collected (using named return).
	// Use a dedicated context so cleanup succeeds even when the parent is cancelled (Ctrl+C).
	defer func() {
		cleanupStart := time.Now()

		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		dockerManager.CleanupWithArtifacts(cleanupCtx, r.collector, uuid)
		result.Artifacts = r.collectArtifactsList(uuid)

		result.CleanupDuration = time.Since(cleanupStart)
		result.NetworkDuration = networkDuration
	}()

	// --- Services phase ---
	servicesStart := time.Now()

	// Start services and capture durations
	serviceResults, err := dockerManager.StartServices(ctx, test.Services)
	if err != nil {
		result.Status = model.StatusError
		result.Error = fmt.Errorf("failed to start services: %w", err)
		return result
	}

	result.ServiceResults = serviceResults
	servicesDuration := time.Since(servicesStart)

	// --- Setup phase ---
	var setupResults []model.SetupResult
	var setupHTTPExchanges []model.HTTPExchangeResult
	var setupCLIExecutions []model.CLIExecutionResult
	var setupDuration time.Duration
	if len(test.Setup) > 0 {
		setupStart := time.Now()
		var err error
		setupResults, setupHTTPExchanges, setupCLIExecutions, err = r.executeSetup(ctx, test, dockerManager, uuid, suiteFilePath)
		setupDuration = time.Since(setupStart)
		if err != nil {
			result.Status = model.StatusError
			result.Error = fmt.Errorf("setup failed: %w", err)
			result.SetupResults = setupResults
			result.HTTPExchanges = setupHTTPExchanges
			result.CLIExecutions = setupCLIExecutions
			result.ServicesDuration = servicesDuration
			result.SetupDuration = setupDuration
			return result
		}
	}

	// Execute the actual test
	result = r.runTestExecution(ctx, test, dockerManager, uuid, suiteFilePath)
	// Preserve service results, setup results, and phase durations after test execution
	result.ServiceResults = serviceResults
	result.SetupResults = setupResults
	result.HTTPExchanges = append(setupHTTPExchanges, result.HTTPExchanges...)
	result.CLIExecutions = append(setupCLIExecutions, result.CLIExecutions...)
	result.ServicesDuration = servicesDuration
	result.SetupDuration = setupDuration

	return result
}

// executeSetup executes all setup instructions for a test.
func (r *Runner) executeSetup(ctx context.Context, test model.Test, dockerManager *docker.Manager, uuid string, suiteFilePath string) ([]model.SetupResult, []model.HTTPExchangeResult, []model.CLIExecutionResult, error) {
	var results []model.SetupResult
	var httpExchanges []model.HTTPExchangeResult
	var cliExecutions []model.CLIExecutionResult

	for i, instruction := range test.Setup {
		r.emit(ctx).Info(events.Fields{
			"test":   test.Name,
			"action": "setup_start",
			"step":   fmt.Sprintf("%d/%d", i+1, len(test.Setup)),
			"msg":    "Executing setup instruction",
		})

		startTime := time.Now()
		var setupResult model.SetupResult

		if instruction.CLI != nil {
			// Execute CLI command
			setupResult.Type = "cli"

			r.emit(ctx).Info(events.Fields{
				"test":    test.Name,
				"action":  "setup_cli",
				"service": instruction.CLI.Service,
				"msg":     fmt.Sprintf("Running CLI: %s", instruction.CLI.Command),
			})

			execResult, err := dockerManager.ExecInContainer(
				ctx,
				instruction.CLI.Service,
				instruction.CLI.Command,
				instruction.CLI.WorkingDir,
				nil,
			)
			setupResult.Duration = time.Since(startTime)

			cliExec := model.CLIExecutionResult{
				Phase:      "setup",
				PhaseSeq:   i,
				Service:    instruction.CLI.Service,
				Command:    instruction.CLI.Command,
				WorkingDir: instruction.CLI.WorkingDir,
				Duration:   setupResult.Duration,
			}

			if err != nil {
				setupResult.Success = false
				setupResult.Error = err
				cliExec.Error = err
				cliExecutions = append(cliExecutions, cliExec)
				results = append(results, setupResult)
				return results, httpExchanges, cliExecutions, fmt.Errorf("setup CLI command failed: %w", err)
			}

			cliExec.ExitCode = execResult.ExitCode
			cliExec.Stdout = string(execResult.Stdout)
			cliExec.Stderr = string(execResult.Stderr)
			cliExecutions = append(cliExecutions, cliExec)

			setupResult.CLIExitCode = execResult.ExitCode
			if execResult.ExitCode != 0 {
				setupResult.Success = false
				setupResult.Error = fmt.Errorf("CLI command exited with code %d: %s", execResult.ExitCode, string(execResult.Stderr))
				results = append(results, setupResult)
				return results, httpExchanges, cliExecutions, setupResult.Error
			}

			setupResult.Success = true
			r.emit(ctx).Info(events.Fields{
				"test":     test.Name,
				"action":   "setup_cli_complete",
				"exitCode": fmt.Sprintf("%d", execResult.ExitCode),
				"duration": fmt.Sprintf("%.3fs", setupResult.Duration.Seconds()),
				"msg":      "CLI setup completed",
			})

		} else if instruction.HTTP != nil {
			// Execute HTTP request
			setupResult.Type = "http"

			// Build execution from setup HTTP config
			execution := model.Execution{
				Executor: model.ExecutorHTTP,
				Target:   instruction.HTTP.Target,
				Request:  instruction.HTTP.Request,
			}

			r.emit(ctx).Info(events.Fields{
				"test":   test.Name,
				"action": "setup_http",
				"target": instruction.HTTP.Target + instruction.HTTP.Request.URL,
				"msg":    fmt.Sprintf("Running HTTP %s request", instruction.HTTP.Request.Method),
			})

			response, err := r.executeHTTPInNetwork(ctx, execution, dockerManager, uuid, suiteFilePath)
			setupResult.Duration = time.Since(startTime)

			requestBody := instruction.HTTP.Request.Body.DisplayString()
			httpExchange := model.HTTPExchangeResult{
				Phase:          "setup",
				PhaseSeq:       i,
				RequestMethod:  instruction.HTTP.Request.Method,
				RequestURL:     instruction.HTTP.Target + instruction.HTTP.Request.URL,
				RequestHeaders: instruction.HTTP.Request.Headers,
				RequestBody:    requestBody,
				Duration:       setupResult.Duration,
			}

			if err != nil {
				setupResult.Success = false
				setupResult.Error = err
				httpExchange.Error = err
				httpExchanges = append(httpExchanges, httpExchange)
				results = append(results, setupResult)
				return results, httpExchanges, cliExecutions, fmt.Errorf("setup HTTP request failed: %w", err)
			}

			httpExchange.ResponseStatusCode = response.StatusCode
			httpExchange.ResponseHeaders = response.Headers
			httpExchange.ResponseBody = string(response.Body)
			httpExchanges = append(httpExchanges, httpExchange)

			setupResult.HTTPStatusCode = response.StatusCode
			// Consider 2xx and 3xx as success for setup
			if response.StatusCode >= 200 && response.StatusCode < 400 {
				setupResult.Success = true
			} else {
				setupResult.Success = false
				setupResult.Error = fmt.Errorf("HTTP request returned status %d", response.StatusCode)
				results = append(results, setupResult)
				return results, httpExchanges, cliExecutions, setupResult.Error
			}

			r.emit(ctx).Info(events.Fields{
				"test":     test.Name,
				"action":   "setup_http_complete",
				"status":   fmt.Sprintf("%d", response.StatusCode),
				"duration": fmt.Sprintf("%.3fs", setupResult.Duration.Seconds()),
				"msg":      "HTTP setup completed",
			})

		} else {
			// Invalid instruction - neither CLI nor HTTP
			setupResult.Type = "unknown"
			setupResult.Success = false
			setupResult.Error = fmt.Errorf("setup instruction has neither CLI nor HTTP configuration")
			results = append(results, setupResult)
			return results, httpExchanges, cliExecutions, setupResult.Error
		}

		results = append(results, setupResult)
	}

	return results, httpExchanges, cliExecutions, nil
}

// collectArtifactsList collects the list of artifacts for a test.
func (r *Runner) collectArtifactsList(uuid string) []model.Artifact {
	artifacts, err := r.collector.ListArtifacts(uuid)
	if err != nil {
		return nil
	}

	var result []model.Artifact
	for _, a := range artifacts {
		result = append(result, model.Artifact{
			Name: a.Name,
			Path: a.Path,
			Size: a.Size,
		})
	}
	return result
}

// runTestExecution performs the actual test execution (HTTP request + assertions).
func (r *Runner) runTestExecution(ctx context.Context, test model.Test, dockerManager *docker.Manager, uuid string, suiteFilePath string) model.TestResult {
	result := model.TestResult{
		Test:   test,
		Status: model.StatusPassed,
		UUID:   uuid,
	}

	switch test.Execution.Executor {
	case model.ExecutorHTTP:
		var response *executor.HTTPResponse
		var err error

		target := test.Execution.Target + test.Execution.Request.URL

		executionStart := time.Now()
		r.emit(ctx).Info(events.Fields{
			"test":   test.Name,
			"action": "http_request",
			"target": target,
			"msg":    fmt.Sprintf("Executing HTTP %s request (via network)", test.Execution.Request.Method),
		})

		// Run HTTP request from inside the isolated network using curl
		response, err = r.executeHTTPInNetwork(ctx, test.Execution, dockerManager, uuid, suiteFilePath)
		result.ExecutionDuration = time.Since(executionStart)

		execRequestBody := test.Execution.Request.Body.DisplayString()
		httpExchange := model.HTTPExchangeResult{
			Phase:          "execution",
			PhaseSeq:       0,
			RequestMethod:  test.Execution.Request.Method,
			RequestURL:     test.Execution.Target + test.Execution.Request.URL,
			RequestHeaders: test.Execution.Request.Headers,
			RequestBody:    execRequestBody,
			Duration:       result.ExecutionDuration,
		}

		if err != nil {
			result.Status = model.StatusError
			result.Error = err
			httpExchange.Error = err
			result.HTTPExchanges = append(result.HTTPExchanges, httpExchange)
			return result
		}

		httpExchange.ResponseStatusCode = response.StatusCode
		httpExchange.ResponseHeaders = response.Headers
		httpExchange.ResponseBody = string(response.Body)
		result.HTTPExchanges = append(result.HTTPExchanges, httpExchange)

		r.emit(ctx).Info(events.Fields{
			"test":   test.Name,
			"action": "http_response",
			"status": fmt.Sprintf("%d", response.StatusCode),
			"msg":    fmt.Sprintf("Received HTTP response (status=%d)", response.StatusCode),
		})

		// Store HTTP response data
		result.HTTPResponse = &model.HTTPResponseData{
			StatusCode: response.StatusCode,
			Headers:    response.Headers,
		}

		// Save response body as artifact if not empty
		if len(response.Body) > 0 {
			bodyArtifact := r.saveResponseBodyArtifact(uuid, response)
			if bodyArtifact != nil {
				result.HTTPResponse.BodyArtifact = bodyArtifact
			}
		}

		// Evaluate assertions
		assertionsStart := time.Now()
		assertionResults, allPassed := r.evaluator.EvaluateHTTP(test.Assertions, response)
		result.AssertionDuration = time.Since(assertionsStart)
		result.AssertionResults = assertionResults

		for _, ar := range assertionResults {
			if ar.Passed {
				r.emit(ctx).Info(events.Fields{
					"test":     test.Name,
					"action":   "assertion_pass",
					"type":     ar.Type,
					"expected": ar.Expected,
					"actual":   ar.Actual,
					"msg":      ar.Message,
				})
			} else {
				r.emit(ctx).Fail(events.Fields{
					"test":     test.Name,
					"action":   "assertion_fail",
					"type":     ar.Type,
					"expected": ar.Expected,
					"actual":   ar.Actual,
					"msg":      ar.Message,
				})
			}
		}

		if !allPassed {
			result.Status = model.StatusFailed
		}

		// Evaluate snapshot assertions
		r.evaluateSnapshotAssertions(ctx, test, &result, suiteFilePath)

	case model.ExecutorCLI:
		if dockerManager == nil {
			result.Status = model.StatusError
			result.Error = fmt.Errorf("CLI executor requires services to be defined")
			return result
		}

		if test.Execution.CLI == nil {
			result.Status = model.StatusError
			result.Error = fmt.Errorf("CLI executor requires cli configuration")
			return result
		}

		cli := test.Execution.CLI

		r.emit(ctx).Info(events.Fields{
			"test":    test.Name,
			"action":  "cli_exec",
			"service": cli.Service,
			"msg":     fmt.Sprintf("Executing CLI command: %s", cli.Command),
		})

		executionStart := time.Now()
		execResult, err := dockerManager.ExecInContainer(ctx, cli.Service, cli.Command, cli.WorkingDir, r.executionVars)
		result.ExecutionDuration = time.Since(executionStart)

		cliExec := model.CLIExecutionResult{
			Phase:      "execution",
			PhaseSeq:   0,
			Service:    cli.Service,
			Command:    cli.Command,
			WorkingDir: cli.WorkingDir,
			Duration:   result.ExecutionDuration,
		}

		if err != nil {
			result.Status = model.StatusError
			result.Error = err
			cliExec.Error = err
			result.CLIExecutions = append(result.CLIExecutions, cliExec)
			return result
		}

		cliExec.ExitCode = execResult.ExitCode
		cliExec.Stdout = string(execResult.Stdout)
		cliExec.Stderr = string(execResult.Stderr)
		result.CLIExecutions = append(result.CLIExecutions, cliExec)

		r.emit(ctx).Info(events.Fields{
			"test":     test.Name,
			"action":   "cli_response",
			"exitCode": fmt.Sprintf("%d", execResult.ExitCode),
			"msg":      fmt.Sprintf("CLI command completed (exit code=%d)", execResult.ExitCode),
		})

		// Store CLI response data
		result.CLIResponse = &model.CLIResponseData{
			ExitCode: execResult.ExitCode,
		}

		// Save stdout as artifact if not empty
		if len(execResult.Stdout) > 0 {
			stdoutArtifact := r.saveCLIOutputArtifact(uuid, "stdout.txt", execResult.Stdout)
			if stdoutArtifact != nil {
				result.CLIResponse.StdoutArtifact = stdoutArtifact
			}
		}

		// Save stderr as artifact if not empty
		if len(execResult.Stderr) > 0 {
			stderrArtifact := r.saveCLIOutputArtifact(uuid, "stderr.txt", execResult.Stderr)
			if stderrArtifact != nil {
				result.CLIResponse.StderrArtifact = stderrArtifact
			}
		}

		// Evaluate assertions
		cliResponse := &executor.CLIResponse{
			ExitCode: execResult.ExitCode,
			Stdout:   execResult.Stdout,
			Stderr:   execResult.Stderr,
		}
		assertionsStart := time.Now()
		assertionResults, allPassed := r.evaluator.EvaluateCLI(test.Assertions, cliResponse)
		result.AssertionDuration = time.Since(assertionsStart)
		result.AssertionResults = assertionResults

		for _, ar := range assertionResults {
			if ar.Passed {
				r.emit(ctx).Info(events.Fields{
					"test":     test.Name,
					"action":   "assertion_pass",
					"type":     ar.Type,
					"expected": ar.Expected,
					"actual":   ar.Actual,
					"msg":      ar.Message,
				})
			} else {
				r.emit(ctx).Fail(events.Fields{
					"test":     test.Name,
					"action":   "assertion_fail",
					"type":     ar.Type,
					"expected": ar.Expected,
					"actual":   ar.Actual,
					"msg":      ar.Message,
				})
			}
		}

		if !allPassed {
			result.Status = model.StatusFailed
		}

		// Evaluate snapshot assertions
		r.evaluateSnapshotAssertions(ctx, test, &result, suiteFilePath)

	default:
		result.Status = model.StatusError
		result.Error = fmt.Errorf("unknown executor type: %s", test.Execution.Executor)
	}

	return result
}

// evaluateSnapshotAssertions evaluates snapshot assertions for a test.
// It builds artifact map from result and calls the snapshot evaluator.
func (r *Runner) evaluateSnapshotAssertions(ctx context.Context, test model.Test, result *model.TestResult, suiteFilePath string) {
	// Check if there are any snapshot assertions
	hasSnapshots := false
	for _, a := range test.Assertions {
		if a.Snapshot != nil {
			hasSnapshots = true
			break
		}
	}
	if !hasSnapshots {
		return
	}

	// Build artifact map from result
	artifacts := make(map[string]string)

	// Add HTTP response body artifact
	if result.HTTPResponse != nil && result.HTTPResponse.BodyArtifact != nil {
		artifacts["responseBody"] = result.HTTPResponse.BodyArtifact.Path
	}

	// Add CLI artifacts
	if result.CLIResponse != nil {
		if result.CLIResponse.StdoutArtifact != nil {
			artifacts["stdout"] = result.CLIResponse.StdoutArtifact.Path
		}
		if result.CLIResponse.StderrArtifact != nil {
			artifacts["stderr"] = result.CLIResponse.StderrArtifact.Path
		}
	}

	// Evaluate snapshot assertions
	snapshotCtx := assertion.SnapshotContext{
		SuiteFilePath: suiteFilePath,
		Artifacts:     artifacts,
		Regenerate:    r.regenerateSnapshots,
	}

	snapshotResults, snapshotsAllPassed := r.evaluator.EvaluateSnapshots(test.Assertions, snapshotCtx)

	// Append snapshot results to assertion results
	result.AssertionResults = append(result.AssertionResults, snapshotResults...)

	// Emit events for snapshot assertions
	for _, ar := range snapshotResults {
		if ar.Passed {
			r.emit(ctx).Info(events.Fields{
				"test":     test.Name,
				"action":   "assertion_pass",
				"type":     ar.Type,
				"expected": ar.Expected,
				"actual":   ar.Actual,
				"msg":      ar.Message,
			})
		} else {
			r.emit(ctx).Fail(events.Fields{
				"test":     test.Name,
				"action":   "assertion_fail",
				"type":     ar.Type,
				"expected": ar.Expected,
				"actual":   ar.Actual,
				"msg":      ar.Message,
			})
		}
	}

	// Update status if snapshot assertions failed
	if !snapshotsAllPassed && result.Status == model.StatusPassed {
		result.Status = model.StatusFailed
	}
}

// saveResponseBodyArtifact saves the HTTP response body as an artifact.
func (r *Runner) saveResponseBodyArtifact(uuid string, response *executor.HTTPResponse) *model.Artifact {
	// Determine file extension based on content type
	contentType := ""
	if response.Headers != nil {
		if ct, ok := response.Headers["Content-Type"]; ok && len(ct) > 0 {
			contentType = ct[0]
		}
	}

	ext := ".txt"
	if strings.Contains(contentType, "application/json") {
		ext = ".json"
	} else if strings.Contains(contentType, "application/xml") || strings.Contains(contentType, "text/xml") {
		ext = ".xml"
	} else if strings.Contains(contentType, "text/html") {
		ext = ".html"
	}

	filename := "responseBody" + ext

	body := response.Body
	if ext == ".json" && json.Valid(body) {
		var buf bytes.Buffer
		if json.Indent(&buf, body, "", "  ") == nil {
			buf.WriteByte('\n')
			body = buf.Bytes()
		}
	}

	// Save artifact using collector
	path, err := r.collector.SaveArtifact(uuid, filename, body)
	if err != nil {
		r.events.Info(events.Fields{
			"action": "artifact_save_error",
			"msg":    fmt.Sprintf("Failed to save response body artifact: %v", err),
		})
		return nil
	}

	size := int64(len(body))
	r.events.ArtifactSave(filename, path, size)

	return &model.Artifact{
		Name: filename,
		Path: path,
		Size: size,
	}
}

// saveCLIOutputArtifact saves CLI output (stdout/stderr) as an artifact.
func (r *Runner) saveCLIOutputArtifact(uuid, filename string, content []byte) *model.Artifact {
	path, err := r.collector.SaveArtifact(uuid, filename, content)
	if err != nil {
		r.events.Info(events.Fields{
			"action": "artifact_save_error",
			"msg":    fmt.Sprintf("Failed to save CLI output artifact: %v", err),
		})
		return nil
	}

	size := int64(len(content))
	r.events.ArtifactSave(filename, path, size)

	return &model.Artifact{
		Name: filename,
		Path: path,
		Size: size,
	}
}

// executeHTTPInNetwork runs an HTTP request from inside the isolated Docker network.
func (r *Runner) executeHTTPInNetwork(ctx context.Context, execution model.Execution, dockerManager *docker.Manager, uuid string, suiteFilePath string) (*executor.HTTPResponse, error) {
	reqBody := execution.Request.Body

	// Read files for file/multipart body modes
	var files map[string][]byte
	if reqBody.IsFile() {
		filePath := filepath.Join(filepath.Dir(suiteFilePath), reqBody.File)
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read body file %q: %w", filePath, err)
		}
		files = map[string][]byte{"tmp/spark-body": content}
	} else if reqBody.IsMultipart() {
		files = make(map[string][]byte)
		for name, field := range reqBody.Multipart {
			if field.File != "" {
				filePath := filepath.Join(filepath.Dir(suiteFilePath), field.File)
				content, err := os.ReadFile(filePath)
				if err != nil {
					return nil, fmt.Errorf("failed to read multipart file %q for field %q: %w", filePath, name, err)
				}
				files[fmt.Sprintf("tmp/spark-mp-%s", name)] = content
			}
		}
	}

	// Build curl command with header output
	curlArgs := []string{
		"-s",                   // Silent mode
		"-i",                   // Include response headers
		"-w", "\n%{http_code}", // Output status code at the end
		"-X", execution.Request.Method,
	}

	// Add test ID header for artifact collection (e.g., code coverage)
	curlArgs = append(curlArgs, "-H", fmt.Sprintf("X-Spark-Test-Id: %s", uuid))

	// Add headers (skip Content-Type for multipart — curl -F sets it with boundary)
	for key, value := range execution.Request.Headers {
		if reqBody.IsMultipart() && strings.EqualFold(key, "Content-Type") {
			continue
		}
		curlArgs = append(curlArgs, "-H", fmt.Sprintf("%s: %s", key, value))
	}

	// Add body arguments
	if reqBody.Raw != "" {
		curlArgs = append(curlArgs, "-d", reqBody.Raw)
	} else if reqBody.IsFile() {
		curlArgs = append(curlArgs, "--data-binary", "@/tmp/spark-body")
	} else if reqBody.IsMultipart() {
		for name, field := range reqBody.Multipart {
			if field.File != "" {
				filename := field.Filename
				if filename == "" {
					filename = filepath.Base(field.File)
				}
				mimeType := mime.TypeByExtension(filepath.Ext(field.File))
				if mimeType == "" {
					mimeType = "application/octet-stream"
				}
				curlArgs = append(curlArgs, "-F", fmt.Sprintf("%s=@/tmp/spark-mp-%s;filename=%s;type=%s", name, name, filename, mimeType))
			} else {
				curlArgs = append(curlArgs, "-F", fmt.Sprintf("%s=%s", name, field.Value))
			}
		}
	}

	// Build full URL
	url := execution.Target + execution.Request.URL
	curlArgs = append(curlArgs, url)

	// Run curl in the network
	var output []byte
	var err error
	if len(files) > 0 {
		output, err = dockerManager.RunInNetworkWithFiles(ctx, docker.CurlImage(), curlArgs, files)
	} else {
		output, err = dockerManager.RunInNetwork(ctx, docker.CurlImage(), curlArgs)
	}
	if err != nil {
		return nil, fmt.Errorf("curl request failed: %w\n%s", err, string(output))
	}

	// Parse output - last line is status code
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 1 {
		return nil, fmt.Errorf("invalid curl output")
	}

	statusCodeStr := lines[len(lines)-1]
	statusCode, err := strconv.Atoi(statusCodeStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse status code: %w", err)
	}

	// Parse headers and body from -i output
	// Format: HTTP/1.1 200 OK\r\nHeader: value\r\n\r\nBody
	// Note: When server sends 100 Continue, output contains multiple HTTP responses.
	// We must parse from the LAST HTTP/ status line to get the actual response.
	headers := make(http.Header)
	body := ""

	contentLines := lines[:len(lines)-1]
	content := strings.Join(contentLines, "\n")

	// Find the last HTTP/ status line to skip intermediate responses (e.g. 100 Continue)
	lastHTTPIdx := strings.LastIndex(content, "HTTP/")
	if lastHTTPIdx > 0 {
		content = content[lastHTTPIdx:]
	}

	// Find the empty line that separates headers from body
	headerBodySplit := strings.Index(content, "\r\n\r\n")
	if headerBodySplit == -1 {
		headerBodySplit = strings.Index(content, "\n\n")
	}

	if headerBodySplit != -1 {
		headerSection := content[:headerBodySplit]
		body = content[headerBodySplit:]
		// Remove leading \r\n\r\n or \n\n
		body = strings.TrimPrefix(body, "\r\n\r\n")
		body = strings.TrimPrefix(body, "\n\n")

		// Parse headers
		headerLines := strings.Split(headerSection, "\n")
		for _, line := range headerLines {
			line = strings.TrimRight(line, "\r")
			// Skip the status line (HTTP/1.1 200 OK)
			if strings.HasPrefix(line, "HTTP/") {
				continue
			}
			// Parse header
			colonIdx := strings.Index(line, ":")
			if colonIdx > 0 {
				name := strings.TrimSpace(line[:colonIdx])
				value := strings.TrimSpace(line[colonIdx+1:])
				headers[name] = append(headers[name], value)
			}
		}
	} else {
		// No headers found, everything is body
		body = content
	}

	return &executor.HTTPResponse{
		StatusCode: statusCode,
		Headers:    headers,
		Body:       []byte(body),
	}, nil
}

// collectAllImages collects all Docker images used in tests for pre-warming.
// Only includes service template images that are actually referenced by tests.
func (r *Runner) collectAllImages(collection *model.TestCollection) []string {
	imageSet := make(map[string]bool)
	referencedTemplates := make(map[string]bool)

	// Collect from test services and track referenced templates
	for _, suite := range collection.Suites {
		for _, test := range suite.Tests {
			for _, svc := range test.Services {
				if svc.Image != "" {
					imageSet[svc.Image] = true
				}
				if svc.Ref != "" {
					referencedTemplates[svc.Ref] = true
				}
			}
		}
	}

	// Collect only from referenced service templates
	if r.serviceTemplates != nil {
		for name, tmpl := range r.serviceTemplates.Templates {
			if tmpl.Image != "" && referencedTemplates[name] {
				imageSet[tmpl.Image] = true
			}
		}
	}

	// Convert to slice
	images := make([]string, 0, len(imageSet))
	for img := range imageSet {
		images = append(images, img)
	}
	return images
}

// organizeResults maps results back to their suites.
func (r *Runner) organizeResults(collection *model.TestCollection, results []testResultWithIndex) *model.RunResult {
	runResult := &model.RunResult{
		SuiteResults: make([]model.SuiteResult, len(collection.Suites)),
	}

	// Initialize suite results
	for i, suite := range collection.Suites {
		runResult.SuiteResults[i] = model.SuiteResult{
			Suite:       suite,
			TestResults: make([]model.TestResult, len(suite.Tests)),
		}
	}

	// Place results in correct positions
	for _, r := range results {
		runResult.SuiteResults[r.suiteIndex].TestResults[r.testIndex] = r.result
	}

	return runResult
}
