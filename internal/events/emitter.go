package events

import (
	"context"
	"time"
)

// contextKey is the key used to store the emitter in a context.
type contextKey struct{}

// WithEmitter returns a new context with the emitter stored in it.
func WithEmitter(ctx context.Context, emitter *Emitter) context.Context {
	return context.WithValue(ctx, contextKey{}, emitter)
}

// EmitterFromContext returns the emitter stored in the context, or nil.
func EmitterFromContext(ctx context.Context) *Emitter {
	if e, ok := ctx.Value(contextKey{}).(*Emitter); ok {
		return e
	}
	return nil
}

// Emitter provides convenience methods for emitting events.
// It wraps a Bus and provides context (suite/test name) for events.
type Emitter struct {
	bus       *Bus
	suiteName string
	testName  string
	testUUID  string
	filePath  string
}

// NewEmitter creates a new emitter wrapping the bus.
func NewEmitter(bus *Bus) *Emitter {
	if bus == nil {
		return nil
	}
	return &Emitter{bus: bus}
}

// ForTest returns a new emitter scoped to a test.
func (e *Emitter) ForTest(suite, test string) *Emitter {
	if e == nil {
		return nil
	}
	return &Emitter{
		bus:       e.bus,
		suiteName: suite,
		testName:  test,
	}
}

// SetUUID sets the test UUID on the emitter so it propagates to all emitted events.
func (e *Emitter) SetUUID(uuid string) {
	if e != nil {
		e.testUUID = uuid
	}
}

// SetFilePath sets the source file path on the emitter so it propagates to all emitted events.
func (e *Emitter) SetFilePath(path string) {
	if e != nil {
		e.filePath = path
	}
}

// Emit sends an event through the bus.
func (e *Emitter) Emit(event *Event) {
	if e == nil || e.bus == nil {
		return
	}

	// Apply context
	if e.suiteName != "" && event.SuiteName == "" {
		event.SuiteName = e.suiteName
	}
	if e.testName != "" && event.TestName == "" {
		event.TestName = e.testName
	}
	if e.testUUID != "" && event.TestUUID == "" {
		event.TestUUID = e.testUUID
	}
	if e.filePath != "" && event.FilePath == "" {
		event.FilePath = e.filePath
	}

	e.bus.Emit(event)
}

// --- Run lifecycle ---

// RunStarted emits run.started event.
// suiteCounts maps suite name → number of tests in that suite (for IDE suite lifecycle).
func (e *Emitter) RunStarted(tests, suites, workers int, version string, suiteCounts map[string]int) {
	e.Emit(NewEvent(RunStarted).
		WithDetail("tests", tests).
		WithDetail("suites", suites).
		WithDetail("workers", workers).
		WithDetail("version", version).
		WithDetail("suiteCounts", suiteCounts))
}

// RunCompleted emits run.completed event.
func (e *Emitter) RunCompleted(duration time.Duration, passed, failed, skipped int) {
	e.Emit(NewEvent(RunCompleted).
		WithDuration(duration).
		WithDetail("passed", passed).
		WithDetail("failed", failed).
		WithDetail("skipped", skipped))
}

// --- Test lifecycle ---

// TestStarted emits test.started event.
func (e *Emitter) TestStarted() {
	e.Emit(NewTestEvent(TestStarted, e.suiteName, e.testName))
}

// TestCompleted emits test.completed event.
func (e *Emitter) TestCompleted(duration time.Duration) {
	e.Emit(NewTestEvent(TestCompleted, e.suiteName, e.testName).
		WithDuration(duration))
}

// TestFailed emits test.failed event.
func (e *Emitter) TestFailed(duration time.Duration, message string) {
	e.Emit(NewTestEvent(TestFailed, e.suiteName, e.testName).
		WithDuration(duration).
		WithMessage(message))
}

// TestSkipped emits test.skipped event.
func (e *Emitter) TestSkipped(reason string) {
	e.Emit(NewTestEvent(TestSkipped, e.suiteName, e.testName).
		WithMessage(reason))
}

// TestCleanup emits test.cleanup event.
func (e *Emitter) TestCleanup(duration time.Duration) {
	e.Emit(NewTestEvent(TestCleanup, e.suiteName, e.testName).
		WithDuration(duration))
}

// --- Service lifecycle ---

// ServiceStarted emits test.service.started event.
func (e *Emitter) ServiceStarted(serviceName, image string) {
	e.Emit(NewTestEvent(TestServiceStarted, e.suiteName, e.testName).
		WithDetail("service", serviceName).
		WithDetail("image", image))
}

// ServiceReady emits test.service.ready event.
func (e *Emitter) ServiceReady(serviceName string, duration time.Duration) {
	e.Emit(NewTestEvent(TestServiceReady, e.suiteName, e.testName).
		WithDetail("service", serviceName).
		WithDuration(duration))
}

// HealthCheck emits test.healthcheck event.
func (e *Emitter) HealthCheck(serviceName string, attempt int, status string, duration time.Duration) {
	e.Emit(NewTestEvent(TestHealthCheck, e.suiteName, e.testName).
		WithDetail("service", serviceName).
		WithDetail("attempt", attempt).
		WithStatus(status).
		WithDuration(duration))
}

// --- Setup/Execution/Assertion ---

// SetupStarted emits test.setup.started event.
func (e *Emitter) SetupStarted(step int, total int) {
	e.Emit(NewTestEvent(TestSetupStarted, e.suiteName, e.testName).
		WithProgress(step, total))
}

// SetupCompleted emits test.setup.completed event.
func (e *Emitter) SetupCompleted(duration time.Duration) {
	e.Emit(NewTestEvent(TestSetupCompleted, e.suiteName, e.testName).
		WithDuration(duration))
}

// ExecStarted emits test.exec.started event.
func (e *Emitter) ExecStarted(execType, target string) {
	e.Emit(NewTestEvent(TestExecStarted, e.suiteName, e.testName).
		WithDetail("type", execType).
		WithDetail("target", target))
}

// ExecCompleted emits test.exec.completed event.
func (e *Emitter) ExecCompleted(duration time.Duration) {
	e.Emit(NewTestEvent(TestExecCompleted, e.suiteName, e.testName).
		WithDuration(duration))
}

// AssertStarted emits test.assert.started event.
func (e *Emitter) AssertStarted() {
	e.Emit(NewTestEvent(TestAssertStarted, e.suiteName, e.testName))
}

// AssertResult emits test.assert.result event.
func (e *Emitter) AssertResult(assertion string, passed bool, expected, actual any, duration time.Duration) {
	status := "pass"
	if !passed {
		status = "fail"
	}
	e.Emit(NewTestEvent(TestAssertResult, e.suiteName, e.testName).
		WithStatus(status).
		WithDuration(duration).
		WithDetail("assertion", assertion).
		WithDetail("expected", expected).
		WithDetail("actual", actual))
}

// --- Docker operations (for debug mode) ---

// DockerCmd emits docker.command event.
func (e *Emitter) DockerCmd(command string, args []string) {
	e.Emit(NewDockerEvent(DockerCommand, command, args))
}

// DockerCmdDone emits docker.command.done event.
func (e *Emitter) DockerCmdDone(duration time.Duration, err error) {
	event := NewEvent(DockerCommandDone).WithDuration(duration)
	if err != nil {
		event.WithError(err)
	}
	e.Emit(event)
}

// ContainerStart emits container.started event.
func (e *Emitter) ContainerStart(containerID, image string) {
	e.Emit(NewEvent(ContainerStarted).
		WithDetail("container_id", containerID).
		WithDetail("image", image))
}

// ContainerStop emits container.stopped event.
func (e *Emitter) ContainerStop(containerID string) {
	e.Emit(NewEvent(ContainerStopped).
		WithDetail("container_id", containerID))
}

// NetworkCreate emits network.created event.
func (e *Emitter) NetworkCreate(networkName string) {
	e.Emit(NewEvent(NetworkCreated).
		WithDetail("network", networkName))
}

// NetworkRemove emits network.removed event.
func (e *Emitter) NetworkRemove(networkName string) {
	e.Emit(NewEvent(NetworkRemoved).
		WithDetail("network", networkName))
}

// ArtifactSave emits artifact.saved event.
func (e *Emitter) ArtifactSave(name, path string, size int64) {
	e.Emit(NewEvent(ArtifactSaved).
		WithDetail("name", name).
		WithDetail("path", path).
		WithDetail("size", size))
}

// --- Logger-compatible methods (for migration from legacy logger) ---

// Fields is a map of string key-value pairs for log events.
type Fields map[string]string

// Info emits an info log event.
func (e *Emitter) Info(fields Fields) {
	e.emitLog(LogInfo, fields)
}

// Pass emits a pass log event.
func (e *Emitter) Pass(fields Fields) {
	e.emitLog(LogPass, fields)
}

// Fail emits a fail log event.
func (e *Emitter) Fail(fields Fields) {
	e.emitLog(LogFail, fields)
}

// Warn emits a warn log event.
func (e *Emitter) Warn(fields Fields) {
	e.emitLog(LogWarn, fields)
}

// Skip emits a skip log event (same as warn).
func (e *Emitter) Skip(fields Fields) {
	e.emitLog(LogWarn, fields)
}

func (e *Emitter) emitLog(logType Type, fields Fields) {
	if e == nil || e.bus == nil {
		return
	}

	event := NewEvent(logType)
	event.SuiteName = e.suiteName
	event.TestName = e.testName

	// Extract common fields
	if msg, ok := fields["msg"]; ok {
		event.Data.Message = msg
	}
	if action, ok := fields["action"]; ok {
		event.Data.Details["action"] = action
	}

	// Copy all fields to details
	for k, v := range fields {
		if k != "msg" {
			event.Data.Details[k] = v
		}
	}

	e.bus.Emit(event)
}

