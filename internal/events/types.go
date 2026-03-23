// Package events provides an event-driven architecture for logging and reporting.
package events

// Type represents the type of lifecycle event.
type Type string

const (
	// Run lifecycle
	RunStarted   Type = "run.started"
	RunCompleted Type = "run.completed"

	// Test lifecycle
	TestStarted        Type = "test.started"
	TestServiceStarted Type = "test.service.started"
	TestServiceReady   Type = "test.service.ready"
	TestHealthCheck    Type = "test.healthcheck"
	TestSetupStarted      Type = "test.setup.started"
	TestSetupCompleted    Type = "test.setup.completed"
	TestExecStarted       Type = "test.exec.started"
	TestExecCompleted     Type = "test.exec.completed"
	TestAssertStarted     Type = "test.assert.started"
	TestAssertResult      Type = "test.assert.result"
	TestTeardownStarted   Type = "test.teardown.started"
	TestTeardownCompleted Type = "test.teardown.completed"
	TestCompleted         Type = "test.completed"
	TestFailed         Type = "test.failed"
	TestSkipped        Type = "test.skipped"
	TestCleanup        Type = "test.cleanup"
	TestError          Type = "test.error"

	// Docker lifecycle (for debug mode)
	DockerCommand      Type = "docker.command"
	DockerCommandDone  Type = "docker.command.done"
	ContainerStarted   Type = "container.started"
	ContainerStopped   Type = "container.stopped"
	ContainerLogs      Type = "container.logs"
	NetworkCreated     Type = "network.created"
	NetworkRemoved     Type = "network.removed"
	ImageFound         Type = "image.found"
	ImagePull          Type = "image.pull"
	HealthCheckStart   Type = "healthcheck.start"
	HealthCheckRetry   Type = "healthcheck.retry"
	HealthCheckPass    Type = "healthcheck.pass"
	HealthCheckFail    Type = "healthcheck.fail"

	// Artifacts
	ArtifactSaved Type = "artifact.saved"

	// Generic log event (for detailed verbose logging)
	Log     Type = "log"
	LogInfo Type = "log.info"
	LogPass Type = "log.pass"
	LogFail Type = "log.fail"
	LogWarn Type = "log.warn"
)

// Phase represents a high-level phase for benchmark grouping.
type Phase string

const (
	PhaseServiceStartup Phase = "service_startup"
	PhaseHealthCheck     Phase = "healthcheck"
	PhaseSetup           Phase = "setup"
	PhaseExecution       Phase = "execution"
	PhaseAssertion       Phase = "assertion"
	PhaseTeardown        Phase = "teardown"
	PhaseCleanup         Phase = "cleanup"
)

// PhaseForEvent maps event types to their benchmark phase.
func PhaseForEvent(t Type) Phase {
	switch t {
	case TestServiceStarted, TestServiceReady, ContainerStarted:
		return PhaseServiceStartup
	case TestHealthCheck:
		return PhaseHealthCheck
	case TestSetupStarted, TestSetupCompleted:
		return PhaseSetup
	case TestExecStarted, TestExecCompleted:
		return PhaseExecution
	case TestAssertStarted, TestAssertResult:
		return PhaseAssertion
	case TestTeardownStarted, TestTeardownCompleted:
		return PhaseTeardown
	case TestCleanup, ContainerStopped, NetworkRemoved:
		return PhaseCleanup
	default:
		return ""
	}
}
